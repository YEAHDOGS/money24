# Money 24 by DOGS

An AI that trades crypto, options, stocks, futures, prediction markets and
forex, around the clock.

**Status: in development. Paper trading phase.**

## What's in here

| Part | What it does | Where | Status |
|---|---|---|---|
| Landing / client (`src/`) | Svelte 5 + Vite + Tailwind v4 landing page, plus the live feed at `feed.html` | Cloudflare Pages · Tauri desktop | Live |
| `newsd` (`engine/cmd/newsd`) | Polls news, filings, central-bank, macro and Polymarket feeds; tags against the watchlist; stores in SQLite; streams over SSE | Desktop sidecar · Linux server | Running |
| `m24d` + signals (`engine/`) | The trading runner. The signals package reads callout sources (X, Telegram, wallets, webhooks), parses them and paper-trades with our own exits | Same as `newsd` | Planned: [notes/trading-service.md](notes/trading-service.md) |

Everything server-side is one Go module in `engine/` (see
[docs/ARCHITECTURE.md](docs/ARCHITECTURE.md)). Each process keeps its own
SQLite database. Cloudflare is only the control plane, later on.

## Run it

### Client

```sh
npm install
npm run dev          # http://localhost:5173  (feed: /feed.html)
npm run build        # production build into dist/
```

Pushes to `master` deploy https://money24.dogs.red.

### newsd: by hand (any OS)

```sh
cd engine
USER_AGENT="Money24-News/0.1 (DOGS; you@example.com)" go run ./cmd/newsd --data ./data
```

Then `npm run dev` and open http://localhost:5173/feed.html. The API is on
`127.0.0.1:8024`. **SEC and BLS reject requests until `USER_AGENT` includes
a contact email.**

### newsd: Windows desktop app

```sh
npm run desktop          # dev: Tauri starts newsd as a sidecar
npm run desktop:build    # installer
```

Data lives in `%AppData%\red.dogs.money24\news\news.db`.

### newsd: Linux VPS

```sh
npm run engine:linux                     # cross-compiles the binary
# copy it to the server, then install engine/deploy/newsd.service with systemd
sudo systemctl enable --now newsd
journalctl -u newsd -f                   # logs
```

On a server, set `NEWSD_ADMIN_TOKEN` and `NEWSD_HOSTS` (your domain behind
the reverse proxy). All flags are listed in [engine/README.md](engine/README.md).

### m24d / signals (planned)

It will run the same three ways as `newsd`: `go run ./cmd/m24d`, as a desktop
sidecar, or under systemd. It reads `newsd`'s `/api/stream` for news
context, so start `newsd` first.

### Guardrails

- **Paper only.** The runner refuses to start if any key or endpoint looks
  live.
- Scraper accounts (X, Telegram) are burners, never personal accounts.
- Secrets go in environment variables or the host's secret store, never in git.

## Notes

Research and design live in [notes/](notes/README.md) and [docs/](docs/).
