# Running Money 24

How to get the whole app running on a fresh machine: Windows, macOS or Linux.
Everything is paper only. Nothing here can place a real order.

## 1. Install two things

| Tool | Version | Where |
|---|---|---|
| Node.js | 22 or newer | https://nodejs.org/ |
| Go | 1.26 or newer | https://go.dev/dl/ |

Git is needed to download the code. Rust is only needed for the desktop app (step 6).

Check them in a terminal:

```sh
node -v
go version
```

## 2. Download the code

```sh
git clone https://github.com/YEAHDOGS/money24.git
cd money24
```

## 3. Start it

**Windows:** double-click `start.bat` (or run it from a terminal).

**macOS and Linux:**

```sh
./start.sh
```

The first run installs the packages and builds the two Go services, which takes a minute or two
and needs the internet. After that it starts in seconds. When it is ready it prints the addresses
and opens the trading desk in your browser:

| Page | Address |
|---|---|
| Trading desk | http://localhost:5173/trading.html |
| Markets (quotes and charts) | http://localhost:5173/markets.html |
| News and calendar | http://localhost:5173/feed.html |

Leave the terminal window open. **Ctrl+C in it stops everything.**

The same thing without the browser opening:

```sh
npm install     # first time only
npm start
```

## 4. What is running

`start` runs three programs and labels each one's output:

| Label | Program | Port | What it does |
|---|---|---|---|
| `newsd` | Go service | 8024 | Collects news and the events calendar |
| `m24d` | Go service | 8025 | Paper trading, market data and source records |
| `web` | Vite dev server | 5173 | Serves the pages |

All three listen on this machine only. The built programs go in `.run/`, which git ignores.

Your data is kept outside the code folder, so updating or deleting the code does not touch it:

| System | Folder |
|---|---|
| Windows | `%AppData%\Money24\` |
| macOS | `~/Library/Application Support/Money24/` |
| Linux | `~/.config/Money24/` |

`news/` holds the news database. `trading/` holds the paper journal (`paper-journal.jsonl`), every
post received (`posts.jsonl`) and the webhook secret (`webhook-secret`).

## 5. Settings (optional)

It runs with no settings. To change any, copy `.env.example` to `.env` and fill in what you need.
`start` reads `.env` each time it runs. **Never commit `.env`: it holds your keys.**

| Setting | What it is for |
|---|---|
| `USER_AGENT` | Put a contact email in it. Without one, SEC and BLS feeds refuse to answer and those news sources stay empty |
| `WEBULL_APP_KEY`, `WEBULL_APP_SECRET` | Live stock and option quotes for paper trades. Without them, those trades are planned at the called price |
| `M24D_WEBHOOK_SECRET` | Fixes the secret other tools use to send callouts. Left empty, one is generated in the data folder |

Coin prices (Coinbase), token prices (DEX Screener) and the Markets page (Yahoo Finance) need no key.

## 6. Other ways to run it

**Each program by hand**, in three terminals, when you want to restart one alone:

```sh
cd engine && go run -buildvcs=false ./cmd/newsd
cd engine && go run -buildvcs=false ./cmd/m24d
npm run dev
```

**Desktop app** (needs Rust and the Tauri prerequisites: https://tauri.app/start/prerequisites/):

```sh
npm run desktop
```

**On a server with Docker**, for the two services only:

```sh
cp .env.example .env
docker compose up -d --build
```

## 7. Updating

```sh
git pull
npm install
```

Then start it again. The Go services are rebuilt on every start.

## 8. When something goes wrong

| What you see | What to do |
|---|---|
| `Port ... is already in use` | Money 24 is already running in another window, or another program has the port. Close it and start again |
| `Go is not installed or not on PATH` | Install Go, then open a **new** terminal so it is found |
| `Packages are not installed yet` | Run `npm install` |
| A service `did not build` | The Go error is printed just above. Check `go version` is 1.26 or newer |
| Pages load but show "Runner offline" | `m24d` stopped. Look for its last lines in the terminal, then start again |
| News page is empty | Give it a minute after the first start. For SEC and BLS sources, set `USER_AGENT` in `.env` |
| Markets page shows no data | Yahoo Finance is not answering. It is an unofficial source and sometimes throttles; try again later |
| `start.sh: Permission denied` | Run `chmod +x start.sh` once |
