#!/bin/sh
# Money 24 server install for Linux (Ubuntu, Debian, Amazon Linux; x86-64 or ARM): the one command to
# run on the machine that will run Money 24.
#
#   git clone https://github.com/YEAHDOGS/money24 && cd money24
#   sh deploy/install.sh [--from /path/money24-move.tar.gz] [--no-start]
#
# Installs Node.js LTS and Go when they are missing or too old, from nodejs.org and go.dev over HTTPS,
# each archive checked against its published SHA-256, into ~/.local/share/money24-tools (no root
# needed). Then hands over to scripts/server.js install: it pins the release key, builds the newest
# signed release, writes a systemd user unit, turns on lingering and starts it. From then on it keeps
# itself on the newest signed release.
set -eu

ROOT=$(cd "$(dirname "$0")/.." && pwd)
TOOLS="$HOME/.local/share/money24-tools"
NODE_MIN_MAJOR=20
NODE_MIN_MINOR=19
GO_MIN_MINOR=26
NODE_INDEX=https://nodejs.org/dist/index.json
NODE_DIST=https://nodejs.org/dist
GO_INDEX='https://go.dev/dl/?mode=json'
GO_DIST=https://go.dev/dl
PROFILE="$HOME/.profile"
PATH_LINE="export PATH=\"$TOOLS/node/bin:$TOOLS/go/bin:\$PATH\""

say() { printf '%s\n' "$*"; }
die() { printf '\n%s\n\n' "$*" >&2; exit 1; }

command -v git >/dev/null || die "git is missing: sudo apt install git (Ubuntu, Debian) or sudo dnf install git (Amazon Linux)."
command -v curl >/dev/null || die "curl is missing: sudo apt install curl, or sudo dnf install curl."
command -v tar >/dev/null || die "tar is missing."
command -v ssh-keygen >/dev/null || die "ssh-keygen is missing (it checks release signatures): sudo apt install openssh-client, or sudo dnf install openssh-clients."
[ -d "$ROOT/.git" ] || die "$ROOT is not a git clone, and updates need one: git clone https://github.com/YEAHDOGS/money24"

case "$(uname -m)" in
  x86_64 | amd64) NODE_ARCH=x64; GO_ARCH=amd64 ;;
  aarch64 | arm64) NODE_ARCH=arm64; GO_ARCH=arm64 ;;
  *) die "Unsupported processor: $(uname -m)." ;;
esac

export PATH="$TOOLS/node/bin:$TOOLS/go/bin:$PATH"
mkdir -p "$TOOLS"

# fetch URL FILE: downloads over HTTPS only, failing on any error.
fetch() { curl --proto '=https' --tlsv1.2 -fsSL "$1" -o "$2"; }

# check FILE SHA256: the archive must match its published hash, or nothing is installed.
check() {
  [ "$(sha256sum "$1" | cut -d' ' -f1)" = "$2" ] || die "$1 does not match its published SHA-256. Nothing was installed."
}

node_ok() {
  command -v node >/dev/null || return 1
  v=$(node -p 'process.versions.node')
  major=${v%%.*}; rest=${v#*.}; minor=${rest%%.*}
  [ "$major" -gt "$NODE_MIN_MAJOR" ] || { [ "$major" -eq "$NODE_MIN_MAJOR" ] && [ "$minor" -ge "$NODE_MIN_MINOR" ]; }
}

go_ok() {
  command -v go >/dev/null || return 1
  minor=$(go env GOVERSION | sed 's/^go1\.\([0-9]*\).*/\1/')
  [ "$minor" -ge "$GO_MIN_MINOR" ]
}

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

if ! node_ok; then
  # index.json lists every release, newest first, one per line; the first with an "lts" name is the newest LTS.
  fetch "$NODE_INDEX" "$tmp/index.json"
  version=$(grep '"lts":"' "$tmp/index.json" | head -n 1 | sed 's/.*"version":"\(v[0-9.]*\)".*/\1/')
  [ -n "$version" ] || die "Could not read the newest Node.js LTS from $NODE_INDEX."
  file="node-$version-linux-$NODE_ARCH.tar.gz"
  say "Installing Node.js $version..."
  fetch "$NODE_DIST/$version/$file" "$tmp/$file"
  fetch "$NODE_DIST/$version/SHASUMS256.txt" "$tmp/SHASUMS256.txt"
  check "$tmp/$file" "$(grep " $file\$" "$tmp/SHASUMS256.txt" | cut -d' ' -f1)"
  rm -rf "$TOOLS/node" && mkdir -p "$TOOLS/node"
  tar -xzf "$tmp/$file" -C "$TOOLS/node" --strip-components=1
fi

if ! go_ok; then
  # go.dev lists the current stable releases with each archive's SHA-256; Node (now present) reads it.
  fetch "$GO_INDEX" "$tmp/go.json"
  line=$(node -e '
    const [os, arch] = ["linux", process.argv[1]];
    const rel = JSON.parse(require("fs").readFileSync(process.argv[2], "utf8")).find((r) => r.stable);
    const f = rel && rel.files.find((x) => x.os === os && x.arch === arch && x.kind === "archive");
    if (f) console.log(f.filename + " " + f.sha256);' "$GO_ARCH" "$tmp/go.json")
  [ -n "$line" ] || die "Could not read the newest Go release from go.dev."
  file=${line% *}; sum=${line#* }
  say "Installing ${file%.linux*}..."
  fetch "$GO_DIST/$file" "$tmp/$file"
  check "$tmp/$file" "$sum"
  rm -rf "$TOOLS/go"
  tar -xzf "$tmp/$file" -C "$TOOLS"
fi

# Later shells (and SSH sessions) find the same tools.
if ! grep -qsF "$TOOLS" "$PROFILE"; then
  printf '\n# Money 24 tools (deploy/install.sh)\n%s\n' "$PATH_LINE" >> "$PROFILE"
fi
command -v cloudflared >/dev/null || say "cloudflared is not installed: there is no tunnel to your phone until it is (see docs/SERVER.md)."

exec node "$ROOT/scripts/server.js" install "$@"
