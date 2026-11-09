# Money 24 by DOGS

An AI that trades crypto, options, stocks, futures, prediction markets and
forex, around the clock.

**Status: in development. Paper trading phase.**

## What's in here

| Part | What it does | Where it runs | Status |
|---|---|---|---|
| Landing / client app | Svelte 5 + Vite + Tailwind v4. Later: source manager, charts, scoreboards | Cloudflare Pages | Landing page live |
| News service | Polls news feeds 24/7, stores and tags headlines, serves them to the other services | Docker → VPS | Planned (Go) |
| Trading service | Copy-trading signals: reads sources, parses callouts, paper-trades with our own exits | Docker → VPS | Planned, see [notes/trading-service.md](notes/trading-service.md) |
| Database | Shared Postgres. `LISTEN/NOTIFY` is the queue between services, so there's no extra broker | Docker → VPS | Planned |

Why two homes: the landing page and API are serverless on Cloudflare. The
services run headless browsers and long-lived sockets 24/7, which Workers
can't do, so they run in Docker: on a laptop first, then on a VPS.

## Run it

### Client app (works today)

```sh
npm install
npm run dev        # http://localhost:5173
npm run build      # production build into dist/
```

Pushes to `master` deploy https://money24.dogs.red.

### Services (planned: these commands describe the target layout)

```
services/
  news/        # Go
  trading/     # Node: adapters, classifier, gate, executor
docker-compose.yml
.env.example   # copy to .env, never commit .env
```

```sh
cp .env.example .env              # broker paper keys, LLM key, burner X/Telegram logins
docker compose up -d              # postgres + news + trading
docker compose logs -f trading    # watch signals arrive and trades go in
docker compose down               # stop everything
```

Run one service on its own:

```sh
docker compose up -d postgres news       # news only
docker compose up -d postgres trading    # trading only (still reads news from the DB)
```

Moving to a VPS uses the same compose file:

```sh
git pull && docker compose up -d --build
```

### Guardrails

- **Paper only.** The trading service refuses to start if any key or
  endpoint looks live.
- Scraper accounts (X, Telegram) are burner accounts, never personal ones.
- Secrets live in `.env` locally and in the host's secret store on the VPS.
  They never go in git.

## Notes

Research and design live in [notes/](notes/README.md).
