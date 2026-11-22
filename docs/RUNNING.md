# Running Money 24

How to get the whole app running on a fresh machine: Windows, macOS or Linux.
Everything is only. Nothing here can place a real order.

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
and opens Home in your browser:

| Page | Address |
|---|---|
| **Home (start here)** | http://localhost:5173/home.html |
| Plays | http://localhost:5173/plays.html |
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
| `m24d` | Go service | 8025 | Trading, market data and source records |
| `web` | Vite dev server | 5173 | Serves the pages |

All three listen on this machine only. The built programs go in `.run/`, which git ignores.

Your data is kept outside the code folder, so updating or deleting the code does not touch it:

| System | Folder |
|---|---|
| Windows | `%AppData%\Money24\` |
| macOS | `~/Library/Application Support/Money24/` |
| Linux | `~/.config/Money24/` |

`news/` holds the news database. `trading/` holds the journal (`the journal`), every
post received (`posts.jsonl`) and the webhook secret (`webhook-secret`).

## 5. Settings (optional)

It runs with no settings. To change any, copy `.env.example` to `.env` and fill in what you need.
`start` reads `.env` each time it runs. **Never commit `.env`: it holds your keys.**

| Setting | What it is for |
|---|---|
| `USER_AGENT` | Put a contact email in it. Without one, SEC and BLS feeds refuse to answer and those news sources stay empty |
| `WEBULL_APP_KEY`, `WEBULL_APP_SECRET` | Live stock and option quotes from Webull. Without them, plays and trades use Yahoo Finance quotes, delayed up to 15 minutes |
| `ANTHROPIC_API_KEY`, `OPENAI_API_KEY`, `XAI_API_KEY`, `GEMINI_API_KEY` | AI reads of your plays (Claude, OpenAI, Grok, Gemini). Add any you use; each key goes only to its own provider. Plays work without one |
| `M24D_WEBHOOK_SECRET` | Fixes the secret other tools use to send callouts. Left empty, one is generated in the data folder |

Coin prices (Coinbase), token prices (DEX Screener), option chains for plays and the Markets page (Yahoo Finance), and Robinhood contract links need no key.

The desktop app does not read `.env` yet, so AI keys only work when you run with `start`. Moving keys into the OS keychain is planned (docs/PLAYS.md).

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

## 7. Run it around the clock

`start` is for sitting at the machine: one program that stops takes the others down with it. To
leave Money 24 running day and night, use the supervisor instead. Close `start` first: both use
the same two ports.

```sh
npm run forever
```

It builds the two services and the pages, starts the services and watches them. Open
**http://127.0.0.1:8025/** : the runner serves the pages itself, so that one address is the whole app.

| | `npm start` | `npm run forever` |
|---|---|---|
| Pages | Vite dev server on 5173 | Served by `m24d` on 8025, with the news forwarded under `/news/` |
| A service stops | Everything stops | That service alone is restarted: after 1 second, then 2, 4, up to 60 |
| A service hangs | Nothing happens | It is asked every minute; three silences in a row and it is restarted |
| Output | The terminal | `.run/logs/`: one file per service plus `forever.log`, every line timestamped (UTC) |
| Sleep | The machine sleeps as usual | Windows does not idle-sleep while the runner is up |
| A build fails | It does not start | It says so and runs the last good build |
| Seeing it | The terminal window | A Money 24 icon in the Windows system tray for as long as it runs: double-click opens the desk; right-click shows each service's status, opens the logs, restarts or stops everything. `--no-tray` leaves it out |

Each log file holds 5 MB; then it becomes `<name>.log.1` (one older file is kept) and a new one starts.

| Command | What it does |
|---|---|
| `npm run forever` | Build, run and watch. Ctrl+C stops everything |
| `npm run forever -- --no-build` | The same with what was built last time: starts in seconds |
| `npm run forever -- --status` | Whether it is running, each service's uptime, restarts and last health check |
| `npm run forever -- --stop` | Stops the supervisor and both services, and checks nothing is left listening |
| `npm run forever -- --install` | Starts it at logon from now on, with no window, and starts it now |
| `npm run forever -- --uninstall` | No longer starts it at logon. What is running keeps running until `--stop` |

`--install` registers a Windows scheduled task named `Money24` (set to run on battery and with no
time limit); `--install --dry-run` prints it without changing anything. On Linux and macOS it prints
the systemd unit or launchd agent to add by hand.

After `git pull`, stop it and start it again so it rebuilds: `npm run forever -- --stop`, then
`npm run forever` (or log off and on, when it is installed).

### Keeping a laptop awake

| What can stop it | What to do |
|---|---|
| Idle sleep | Nothing: the runner asks Windows to stay awake while it runs. The screen may still turn off |
| Closing the lid | Control Panel → Power Options → **Choose what closing the lid does** → **Do nothing** when plugged in. The supervisor does not change this for you |
| Pressing sleep, or a flat battery | Nothing can run then. Keep it plugged in. When it wakes, a service with dead connections is restarted within three minutes |
| A restart (Windows Update) | The logon task starts it again once **you log in**; until then nothing runs. After a restart, check with `npm run forever -- --status`, or open http://127.0.0.1:8025/ |

On macOS start it with `caffeinate -i npm run forever`; on Linux desktops with
`systemd-inhibit --what=sleep npm run forever`. A server does not sleep.

### With the desktop app

The desktop app starts its own `newsd` and `m24d` on the same two ports. With the supervisor
running those two cannot start: the app's log shows them exiting with "address already in use",
and the window shows the supervisor's services instead, because its pages ask the same local
addresses. So the app becomes a monitor window. Quitting it stops nothing. The other way round,
`npm run forever` refuses to start while the desktop app is open: quit the app first.

### Watching it from your phone

The desk lives at **https://money24.dogs.red/desk/**, the same domain as the public demo. The demo
stays open to everyone; `/desk/` is the runner on this machine, for the owner only.

| Lock | What it does |
|---|---|
| Cloudflare Access on `money24.dogs.red/desk` | A one-time PIN to the owner's email addresses; a week between logins |
| The Worker `money24-desk` (`api/desk/worker.js`) | Checks the Access token's signature, audience, issuer and expiry itself, and refuses anything else |
| The tunnel | Publishes no hostname at all. The Worker reaches `127.0.0.1:8025` through Workers VPC, and nothing else can |
| The runner | Listens on `127.0.0.1` only |

From the phone the desk works fully: the Worker adds the runner's webhook secret to every change
(pasting a link, trading a play, editing rules), and only a request that passed the locks above gets it.

Setting it up on a new machine (the desktop that will run it for good):

1. Install `cloudflared` (`winget install Cloudflare.cloudflared`).
2. Copy `.env` from the old machine: `M24D_TUNNEL_TOKEN` is the tunnel's token. The supervisor runs
   the tunnel as a third service, `tunnel`, health-checked on `http://127.0.0.1:8026/ready`.
3. Copy `%AppData%\Money24\` so the journal and plays come along, or the desk starts empty with a
   new webhook secret. With a new secret, set it on the Worker: `npx wrangler secret put DESK_SECRET`
   from `api/desk/`.
4. `npm run forever -- --install`.

Only one machine may run the tunnel's runner at a time: stop the old one first.

On this machine the desk is at http://127.0.0.1:8025/desk/. News settings are not changed from these
pages; use `npm start` or the desktop app for that.

## 8. Updating

```sh
git pull
npm install
```

Then start it again. The Go services are rebuilt on every start.

## 9. When something goes wrong

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
| `Money 24 is already running around the clock` | The supervisor is up. `npm run forever -- --status` shows it; `-- --stop` stops it |
| A service keeps restarting | Its last lines are in `.run/logs/<name>.log`, and each restart with its exit code is in `.run/logs/forever.log` |
