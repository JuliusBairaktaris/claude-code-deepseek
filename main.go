// ccd - run Claude Code with Anthropic and DeepSeek models in one session.
//
//	/model opus    -> claude-opus-5      (your Claude plan)
//	/model fable   -> claude-fable-5     (your Claude plan)
//	/model sonnet  -> deepseek-v4-pro    ($CCD_SONNET)
//	/model haiku   -> deepseek-v4-flash  ($CCD_HAIKU)
//	/model claude-sonnet-5               (any full model id also works)
//
// Claude Code reads ANTHROPIC_BASE_URL once per process, so one session is
// normally one provider. ccd starts a router on localhost, dispatches each
// request on the model id in its body, then launches claude against it.
//
// Anthropic requests are forwarded with Claude Code's own Authorization header
// untouched, so the session stays logged in as itself - your plan, your quota,
// your native features. Only DeepSeek requests get their credential swapped.
// ccd never reads or writes your Claude credentials.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	anthropicHost = "https://api.anthropic.com"
	deepseekHost  = "https://api.deepseek.com/anthropic"
)

var debug = os.Getenv("CCD_DEBUG") == "1"

// Hop-by-hop headers: re-set per connection, never forwarded.
var dropHeaders = []string{"Connection", "Transfer-Encoding", "Keep-Alive", "Upgrade"}

func die(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "ccd: "+format+"\n", a...)
	os.Exit(1)
}

func deepseekKey() string {
	if k := os.Getenv("DEEPSEEK_API_KEY"); k != "" {
		return k
	}
	home, err := os.UserHomeDir()
	if err != nil {
		die("cannot locate home directory: %v", err)
	}
	path := filepath.Join(home, ".claude", ".deepseek-key")
	b, err := os.ReadFile(path)
	if err != nil || len(strings.TrimSpace(string(b))) == 0 {
		die("no DeepSeek key. Set $DEEPSEEK_API_KEY or write it to %s", path)
	}
	return strings.TrimSpace(string(b))
}

// flushWriter pushes every chunk as it arrives so streaming stays live.
type flushWriter struct{ w http.ResponseWriter }

func (f flushWriter) Write(p []byte) (int, error) {
	n, err := f.w.Write(p)
	if fl, ok := f.w.(http.Flusher); ok {
		fl.Flush()
	}
	return n, err
}

type router struct {
	key    string
	client *http.Client
}

func (rt *router) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "ccd: reading request: "+err.Error(), http.StatusBadGateway)
		return
	}

	var payload struct {
		Model string `json:"model"`
	}
	_ = json.Unmarshal(body, &payload)
	deepseek := strings.HasPrefix(payload.Model, "deepseek")

	target := anthropicHost + r.URL.RequestURI()
	if deepseek {
		target = deepseekHost + r.URL.RequestURI()
	}
	if debug {
		host := "api.anthropic.com"
		if deepseek {
			host = "api.deepseek.com"
		}
		model := payload.Model
		if model == "" {
			model = "-"
		}
		fmt.Fprintf(os.Stderr, "[ccd] %s -> %s\n", model, host)
	}

	req, err := http.NewRequestWithContext(r.Context(), r.Method, target, strings.NewReader(string(body)))
	if err != nil {
		http.Error(w, "ccd: building request: "+err.Error(), http.StatusBadGateway)
		return
	}
	req.Header = r.Header.Clone()
	for _, h := range dropHeaders {
		req.Header.Del(h)
	}
	if deepseek {
		req.Header.Set("Authorization", "Bearer "+rt.key)
		req.Header.Del("X-Api-Key")
		// Claude Code's beta list is for api.anthropic.com and means nothing at DeepSeek
		req.Header.Del("Anthropic-Beta")
	} else if req.Header.Get("Authorization") == "" && req.Header.Get("X-Api-Key") == "" {
		http.Error(w, "ccd: claude sent no credential; run `claude` once to log in", http.StatusUnauthorized)
		return
	}

	resp, err := rt.client.Do(req)
	if err != nil {
		http.Error(w, "ccd: upstream: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	for k, vs := range resp.Header {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	for _, h := range dropHeaders {
		w.Header().Del(h)
	}
	w.Header().Del("Content-Length") // length changes with re-chunking
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(flushWriter{w}, resp.Body)
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func main() {
	rt := &router{key: deepseekKey(), client: &http.Client{}} // no timeout: turns can be long

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		die("cannot bind localhost: %v", err)
	}
	go func() { _ = http.Serve(ln, rt) }()

	env := os.Environ()
	// Any of these would put Claude Code in API-token mode and cost you the
	// subscription session (connectors, quota, native auth).
	for _, k := range []string{"ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN", "ANTHROPIC_MODEL",
		"ANTHROPIC_DEFAULT_OPUS_MODEL", "ANTHROPIC_BASE_URL",
		"ANTHROPIC_DEFAULT_SONNET_MODEL", "ANTHROPIC_DEFAULT_HAIKU_MODEL"} {
		env = unset(env, k)
	}
	env = append(env,
		fmt.Sprintf("ANTHROPIC_BASE_URL=http://%s", ln.Addr().String()),
		// [1m] declares DeepSeek's real 1M window to Claude Code; it strips the
		// suffix before the request, so the API still sees a plain model id.
		"ANTHROPIC_DEFAULT_SONNET_MODEL="+envOr("CCD_SONNET", "deepseek-v4-pro[1m]"),
		"ANTHROPIC_DEFAULT_HAIKU_MODEL="+envOr("CCD_HAIKU", "deepseek-v4-flash[1m]"),
	)
	if opus := os.Getenv("CCD_OPUS"); opus != "" {
		env = append(env, "ANTHROPIC_DEFAULT_OPUS_MODEL="+opus)
	}

	cmd := exec.Command("claude", os.Args[1:]...)
	cmd.Env = env
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			os.Exit(ee.ExitCode())
		}
		die("cannot run claude: %v", err)
	}
}

func unset(env []string, key string) []string {
	out := env[:0]
	for _, e := range env {
		if !strings.HasPrefix(e, key+"=") {
			out = append(out, e)
		}
	}
	return out
}
