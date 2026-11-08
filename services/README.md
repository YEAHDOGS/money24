# Money 24 services

Backend services for the Money 24 money machine. Everything runs locally with one command.

```sh
cd services
cp .env.example .env    # then put a real contact email in USER_AGENT (SEC requires one)
docker compose up --build
```

Open http://localhost:8024 for the live feed. Postgres is on `localhost:5433` (user, password and database: `money24`).

## news (`newsd`, Go)

Polls 25 sources, stores every item in Postgres, and streams new items live.

| Source type | What | Interval |
|---|---|---|
| SEC EDGAR (Atom) | 8-K, 10-Q, 10-K, Form 4 insider trades, as they post | 1–10 min |
| Central banks | Fed, ECB, Bank of England press releases | 2–5 min |
| Macro | BLS releases (CPI, jobs) | 5 min |
| Finance and crypto news | MarketWatch, CNBC, Yahoo Finance, NYT, BBC, FXStreet, CoinDesk, Cointelegraph, Decrypt | 1–2 min |
| GDELT | Global news in 100+ languages, four topic queries | 15 min |
| Polymarket | Top 200 active markets by 24h volume, with odds snapshots | 5 min |

- **Sources** live in `news/sources.json`; the **watchlist** of tags is in `news/watchlist.json`. Override either with `SOURCES_FILE` / `WATCHLIST_FILE`.
- **Tags:** every item is matched (whole words) against the watchlist. News only matters if something tradable can move on it.
- **Clock:** `seen_at` is when we saw it. Backtests must use `seen_at`, never `published_at`.
- **Dedupe:** by URL. Unchanged feeds cost one 304.
- **Migrations:** `news/migrations/*.sql`, applied on start, once each, in name order.

### API

| Route | Returns |
|---|---|
| `GET /api/events?limit=100&tag=BTC` | newest events, optionally by tag |
| `GET /api/stream` | Server-Sent Events, one JSON per new event |
| `GET /healthz` | `ok` |

### Useful queries

```sql
-- What is moving the watchlist right now
SELECT unnest(tags) AS tag, count(*) FROM events WHERE seen_at > now() - interval '1 hour' GROUP BY 1 ORDER BY 2 DESC;

-- Polymarket odds history for one market
SELECT at, prices FROM market_prices WHERE market_id = 'polymarket:512345' ORDER BY at;
```

## Stack note

`services/` is a deliberate exception to the web stack in `AGENTS.md`: long-running pollers and streams fit a Go binary on a VPS better than Workers. Postgres is the local store; the plan is to move the same containers to a VPS.

Known limits: keyword tags are naive ("pound" also matches weight), titles are stored in their original language, and paid wires (Reuters, Bloomberg, Benzinga) are not included.
