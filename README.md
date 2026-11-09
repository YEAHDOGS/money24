# Money 24 by DOGS

An AI that trades crypto, options, stocks, futures, prediction markets and
forex, around the clock.

**Status: in development. Paper trading phase.**

## What's in here

| Part | What it does | Where | Status |
|---|---|---|---|
| Landing / client (`src/`) | Svelte 5 + Vite + Tailwind v4 landing page, plus the live feed at `feed.html` | Cloudflare Pages · Tauri desktop | Live |
| `newsd` (`engine/cmd/newsd`) | Polls news, filings, central-bank, macro and Polymarket feeds; tags against the watchlist; stores in SQLite; streams over SSE | Desktop sidecar · Linux server | Running |
| `m24d` + signals (`engine/`) | The trading runner. The signals package reads callout sources (X, Telegram, wallets, webhooks), parses them and paper-trades with our own exits | Same as `newsd` | Paper webhook runner; adapters next: [notes/trading-service.md](notes/trading-service.md) |

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

### Docker: Windows, Linux or VPS (same command everywhere)

```sh
cp .env.example .env          # put a real contact email in USER_AGENT
docker compose up -d --build  # http://localhost:8024
docker compose logs -f news
docker compose down           # the news-data volume keeps the database
```

On Windows this needs Docker Desktop. This starts `news` (8024) and `trading` (8025).

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

### m24d: paper trading runner

Takes callouts on a webhook, plans each entry with its stop, ladder and time
exits, and writes every plan and rejection to `paper-journal.jsonl`. It has
no broker code yet, so it can't place a real order. The UI is optional: the
runner works the same headless.

| Where | How |
|---|---|
| Windows desktop | `npm run desktop` starts `newsd` and `m24d` as sidecars. The window opens on News, and **Trading →** switches to the desk |
| Any OS, by hand | `cd engine && go run ./cmd/m24d --data ./data/trading`, then `npm run dev` → http://localhost:5173/trading.html |
| Linux server | `npm run engine:linux`, then `engine/deploy/m24d.service` under systemd (headless) |
| Docker | the `trading` service in `docker-compose.yml` (port 8025) |

The webhook secret comes from `M24D_WEBHOOK_SECRET`. Without it, a random
secret is generated once into `<data>/webhook-secret`.

```sh
curl -H "Authorization: Bearer $(cat engine/data/trading/webhook-secret)" \
  -d '{"source_id":"x:MATTSMATTED","text":"BTO $SPY 600C 11/21 @ 1.20"}' \
  http://127.0.0.1:8025/signals
```

| Route | Returns |
|---|---|
| `POST /signals` | parses a callout (Bearer secret required) and answers with how it was read |
| `GET /api/journal?limit=&after=` | recent journal entries, oldest first |
| `GET /api/stream?after=` | SSE: one message per new entry, with backfill by `Last-Event-ID` |
| `GET /api/sources` | the source catalog and default settings |
| `GET /healthz` | `ok` |

Tests: `cd engine && go test ./signals/`.

### Guardrails

- **Paper only.** The runner refuses to start if any key or endpoint looks
  live.
- Scraper accounts (X, Telegram) are burners, never personal accounts.
- Secrets go in environment variables or the host's secret store, never in git.

## Notes

Research and design live in [notes/](notes/README.md) and [docs/](docs/).
