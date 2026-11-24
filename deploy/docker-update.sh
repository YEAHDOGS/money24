#!/bin/sh
# Keeps a Docker install on the newest SIGNED release, like the PC supervisor does (docs/SERVER.md, "Updates").
# A push to master never deploys; only a tag signed by the pinned release key does. Run it from cron:
#   */10 * * * * /path/to/money24/deploy/docker-update.sh >> ~/money24-update.log 2>&1
# It fetches tags, verifies the newest one, builds it, starts it, and goes back to the old release if the
# desk does not answer. The key is pinned outside the repo on first run (compare its fingerprint with the
# owner's PC: npm run release -- --key), so a commit cannot swap it.
set -eu
cd "$(dirname "$0")/.."

CONFIG="${M24_CONFIG:-$HOME/.config/money24}"
SIGNERS="$CONFIG/release-signers"
PREFIX="release-"
HEALTH_URL="${M24_HEALTH_URL:-http://127.0.0.1:8025/desk/}"
HEALTH_TRIES=30
HEALTH_WAIT=3

mkdir -p "$CONFIG"
if [ ! -f "$SIGNERS" ]; then
  cp deploy/release-signers "$SIGNERS"
  echo "Pinned the release key at $SIGNERS. Its fingerprint, to compare with the owner's PC:"
  awk '{ print $3 " " $4 }' "$SIGNERS" | ssh-keygen -lf -
fi

git fetch --quiet --tags --force origin
current="$(git describe --tags --exact-match 2>/dev/null || true)"

newest=""
for tag in $(git tag --list "$PREFIX*" --sort=-creatordate); do
  [ "$tag" = "$current" ] && { echo "Up to date: $current"; exit 0; }
  # The name inside the signature must be the tag's own name, so an old release cannot come back under a new one.
  signed_name="$(git cat-file tag "$tag" 2>/dev/null | sed -n 's/^tag //p')"
  if [ "$signed_name" = "$tag" ] && git -c gpg.ssh.allowedSignersFile="$SIGNERS" verify-tag --raw "$tag" 2>&1 | grep -q '^Good "git" signature for'; then
    newest="$tag"
    break
  fi
  echo "Skipping $tag: not signed by the pinned release key." >&2
done
[ -n "$newest" ] || { echo "No signed release to install."; exit 0; }

healthy() {
  i=0
  while [ "$i" -lt "$HEALTH_TRIES" ]; do
    curl -fsS -o /dev/null "$HEALTH_URL" && return 0
    i=$((i + 1))
    sleep "$HEALTH_WAIT"
  done
  return 1
}

previous="$(git rev-parse HEAD)"
echo "Installing $newest (was ${current:-$previous})"
git checkout --quiet --detach "$newest"
docker compose up -d --build
if healthy; then
  echo "Running $newest."
  exit 0
fi
echo "$newest did not come up. Going back." >&2
git checkout --quiet --detach "$previous"
docker compose up -d --build
exit 1
