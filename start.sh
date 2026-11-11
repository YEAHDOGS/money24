#!/bin/sh
# Starts all of Money 24 on macOS or Linux. Ctrl+C stops everything.
cd "$(dirname "$0")" || exit 1
command -v node >/dev/null || { echo "Node.js is not installed. Get it from https://nodejs.org/ then run this again."; exit 1; }
command -v go >/dev/null || { echo "Go is not installed. Get it from https://go.dev/dl/ then run this again."; exit 1; }
[ -d node_modules ] || npm install || exit 1
exec node scripts/start.js --open
