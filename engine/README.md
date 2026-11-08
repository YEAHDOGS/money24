# Money 24 engine

One Go module, CGO-free, standard library first. The first command is `newsd`, the news and events service.
The trading runner `m24d` from `docs/ARCHITECTURE.md` will live here too.

## newsd

Polls public news, filings, central-bank and macro feeds, tags every item against the watchlist, stores it in
SQLite, and streams new items live over Server-Sent Events. The same binary runs in two places:

| Where | How |
|---|---|
| Windows desktop | The Tauri app (`src/tauri/`) starts it as a sidecar: `npm run desktop` (dev) or `npm run desktop:build` (installer). Data: `%AppData%\red.dogs.money24\news\news.db`. |
| Linux server | `npm run engine:linux`, then install with systemd: see `deploy/newsd.service`. |
| Anywhere, by hand | `go run ./cmd/newsd --data ./data`, then open the feed with `npm run dev` → http://localhost:5173/feed.html |

### How it works

1. Each source in `news/sources.json` runs on its own goroutine. They share one **poll interval** (1s to 15m),
   set from the slider in the UI and saved in SQLite. Some sources have a slower floor (`min_every`).
2. Fetches are polite: one User-Agent, `ETag`/`Last-Modified` (unchanged feeds cost one small 304), gzip.
3. Items are parsed (RSS 2.0, RSS 1.0, Atom, GDELT JSON, Polymarket), tagged with whole-word matches from
   `news/watchlist.json`, and inserted. The URL is the dedupe key.
4. New rows go to the hub, which fans them out to every open `/api/stream`. Each SSE message carries the row id,
   so a client that drops (sleep, network blip) reconnects with `Last-Event-ID` and gets what it missed.
5. The network meter counts compressed bytes per request, raw socket bytes (TLS included) and bytes served to
   clients. It is flushed to the `net_usage` table every 10 s, one row per day per channel.

### API

| Route | Returns |
|---|---|
| `GET /api/events?limit=&before=&kind=&tag=` | newest events (max 500) |
| `GET /api/stream?after=` | SSE: `settings`, `stats` (meter changes), and one message per new event |
| `GET /api/network?since=` | network meter snapshot (live counters plus this month's ledger) |
| `GET /api/settings` · `PUT /api/settings` `{"poll_every_ms": n}` | the global poll interval |
| `GET /healthz` | `ok` |

### Flags (or environment variables)

| Flag | Env | Default |
|---|---|---|
| `--addr` | `NEWSD_ADDR` | `127.0.0.1:8024` |
| `--data` | `NEWSD_DATA` | user config dir `/Money24/news` |
| `--user-agent` | `USER_AGENT` | `Money24-News/0.1 (DOGS)`. **SEC and BLS reject requests until this includes a contact email.** |
| `--origins` | `NEWSD_ORIGINS` | Tauri WebView origins and the Vite dev server |
| `--hosts` | `NEWSD_HOSTS` | localhost only when bound to loopback; set your domain behind a reverse proxy |
| `--admin-token` | `NEWSD_ADMIN_TOKEN` | none. Set it on a server: it guards `PUT /api/settings` |
| `--sources`, `--watchlist` | `SOURCES_FILE`, `WATCHLIST_FILE` | the embedded files |

### Known limits

- Tags are keyword matches ("pound" also matches weight). Headlines stay in their original language.
- GDELT throttles harder than its documented 5 s limit; its four queries are spaced 20 s apart with a 5 m floor.
- Hosting prices in `src/feed/pricing.json` are approximate list prices, not verified for this build.
- With `--admin-token` set, the UI slider cannot change settings yet (the UI does not send the token).
