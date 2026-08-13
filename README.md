# claude-code-deepseek

**Use DeepSeek and Claude models in the same Claude Code session — switch with `/model`, keep your native Claude Code session.**

Pointing `ANTHROPIC_BASE_URL` at DeepSeek replaces Claude entirely; multi-provider gateways
proxy everything through their own registry and client keys, which is why native features
break under them. `ccd` starts a router on localhost, dispatches each request on the model
id, and **forwards Claude requests with Claude Code's own `Authorization` header untouched** —
your session stays logged in as itself, on your plan. Only DeepSeek requests get their
credential swapped.

| `/model` | goes to | billed to |
|---|---|---|
| `opus` | `claude-opus-5` | your Claude plan |
| `fable` | `claude-fable-5` | your Claude plan |
| `sonnet` | `deepseek-v4-pro` | your DeepSeek key |
| `haiku` | `deepseek-v4-flash` | your DeepSeek key |
| any full id — `claude-sonnet-5`, `deepseek-v4-pro` | that model | that provider |

## Install

```sh
curl -fsSL https://raw.githubusercontent.com/JuliusBairaktaris/claude-code-deepseek/main/install.sh | sh
```

Run `ccd` instead of `claude`; every flag passes through. A single static binary — no Python,
no Node, no dependencies — prebuilt for Linux and macOS on amd64/arm64. On Windows use WSL.
Build it yourself with `go build -o ccd .`

You need Claude Code, your existing Claude login, and a
[DeepSeek API key](https://platform.deepseek.com/api_keys). **No Anthropic API key**, and no
Claude credential is ever read or written. The installer stores the DeepSeek key for you, or:

```sh
printf '%s' 'sk-your-key' > ~/.claude/.deepseek-key   # or: export DEEPSEEK_API_KEY=sk-...
```

`$DEEPSEEK_API_KEY` wins over the file. It is sent only to `api.deepseek.com`.

## Opus or Fable in front, DeepSeek doing the work

```sh
CLAUDE_CODE_SUBAGENT_MODEL='deepseek-v4-flash[1m]' ccd --dangerously-skip-permissions --model opus
CLAUDE_CODE_SUBAGENT_MODEL='deepseek-v4-flash[1m]' ccd --dangerously-skip-permissions --model fable
```

Claude plans and reviews, Flash does the legwork, nothing stops to ask — `fable` for the
hardest work if your plan includes it, `opus` otherwise. Note that
`--dangerously-skip-permissions` runs every tool call unprompted — don't point it at a
directory you can't restore from git.

Subagent models come from Claude Code itself, so all three of its levers work — `/config` →
*Subagent model*, the `CLAUDE_CODE_SUBAGENT_MODEL` variable above, or `model:` in an agent's
`.claude/agents/<name>.md` frontmatter. Use the **alias** (`haiku`) rather than a concrete
DeepSeek id anywhere the setting persists, so plain `claude` sessions still resolve it to real
Anthropic Haiku.

## Configuration

```sh
CCD_SONNET='deepseek-v4-flash[1m]' ccd   # remap the sonnet slot
CCD_HAIKU='deepseek-v4-pro[1m]' ccd      # remap the haiku slot
CCD_OPUS='deepseek-v4-pro[1m]' ccd       # opus too, for an all-DeepSeek session
CCD_DEBUG=1 ccd                          # log one "model -> host" line per request
```

Keep the `[1m]` suffix on DeepSeek ids: it declares the real 1M window (both V4 models are
1M in / 384K out) so auto-compact doesn't fire early at an assumed 200K. Claude Code strips
it before the request.

## How it works

`ccd` binds a router to an ephemeral port on `127.0.0.1`, points Claude Code at it, and
launches `claude`. Each request is dispatched on the `model` field in its body: `deepseek*`
goes to `api.deepseek.com/anthropic` with your DeepSeek key, everything else to
`api.anthropic.com` with the credential Claude Code already attached. Responses stream
straight back. The router lives and dies with the session — no daemon, no config file, no
port to pick. One Go file, standard library only.

Verified end to end: every model route above, tool calls on the DeepSeek branch, `--continue`
resume, subagent routing via all three levers, and subscription mode intact (no *"connectors
are disabled"* warning, which appears the moment any `ANTHROPIC_API_KEY` /
`ANTHROPIC_AUTH_TOKEN` is set).

## Good to know

- **Remote Control does not work, and can't be made to.** Claude Code gates it on talking to
  `api.anthropic.com` directly, so *any* tool that sets `ANTHROPIC_BASE_URL` disqualifies the
  session. Start those with plain `claude`.
- **Your prompts go where the model is.** Anything on a `deepseek-*` model goes to DeepSeek
  under their privacy policy — including the earlier conversation, when you switch mid-session.
- **Which provider served this turn?** `/model` shows the slot name, not the model behind it.
  Ask the model (it knows its real id), or run `CCD_DEBUG=1 ccd` for the routing log. A
  session started with plain `claude` routes nowhere — `alias claude=ccd` avoids the mix-up.
- **Model names are whatever DeepSeek's API accepts today.** If they change, set `CCD_SONNET`
  / `CCD_HAIKU`.

## License

MIT
