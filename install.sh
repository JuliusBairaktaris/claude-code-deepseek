#!/bin/sh
# Installs ccd. curl -fsSL https://raw.githubusercontent.com/JuliusBairaktaris/claude-code-deepseek/main/install.sh | sh
set -eu

REPO=JuliusBairaktaris/claude-code-deepseek
BIN=${CCD_BIN:-$HOME/.local/bin}

os=$(uname -s | tr '[:upper:]' '[:lower:]')
case $(uname -m) in
	x86_64|amd64) arch=amd64 ;;
	aarch64|arm64) arch=arm64 ;;
	*) echo "ccd: unsupported architecture $(uname -m)"; exit 1 ;;
esac
case $os in
	linux|darwin) ;;
	*) echo "ccd: unsupported OS $os (on Windows, run this inside WSL)"; exit 1 ;;
esac

command -v claude >/dev/null 2>&1 || echo "warning: 'claude' is not on PATH - install Claude Code first"

mkdir -p "$BIN"
URL="https://github.com/$REPO/releases/latest/download/ccd-$os-$arch"
echo "downloading $URL"
curl -fsSL "$URL" -o "$BIN/ccd.tmp"
chmod +x "$BIN/ccd.tmp"
mv "$BIN/ccd.tmp" "$BIN/ccd"

KEYFILE=$HOME/.claude/.deepseek-key
if [ ! -s "$KEYFILE" ] && [ -z "${DEEPSEEK_API_KEY:-}" ] && [ -r /dev/tty ]; then
	printf 'DeepSeek API key (https://platform.deepseek.com/api_keys), blank to skip: '
	read -r key < /dev/tty
	if [ -n "$key" ]; then
		mkdir -p "$HOME/.claude"
		(umask 077; printf '%s' "$key" > "$KEYFILE")
	fi
fi

echo "installed $BIN/ccd"
case ":$PATH:" in *":$BIN:"*) ;; *) echo "add $BIN to your PATH";; esac
echo "run: ccd"
