-- Money 24 news store: every headline, filing and release we see, plus prediction markets.
-- Times are Unix milliseconds, UTC. seen_at is when WE saw it (the honest backtest clock);
-- published_at is what the source claims and may be missing or wrong.

CREATE TABLE IF NOT EXISTS events (
  id           INTEGER PRIMARY KEY,         -- rises with arrival order; the live stream resumes from it
  seen_at      INTEGER NOT NULL,
  published_at INTEGER,
  feed         TEXT NOT NULL,               -- which poller found it (sources.json id)
  source       TEXT NOT NULL,               -- display label: outlet name, or domain for GDELT
  kind         TEXT NOT NULL,               -- news | filing | insider | central_bank | macro
  title        TEXT NOT NULL,
  url          TEXT NOT NULL UNIQUE,        -- dedupe key
  summary      TEXT,
  lang         TEXT,
  country      TEXT,
  tags         TEXT NOT NULL DEFAULT '[]'   -- JSON array of watchlist hits: tickers, coins, macro buckets
);

CREATE INDEX IF NOT EXISTS events_kind_id_idx ON events (kind, id DESC);
CREATE INDEX IF NOT EXISTS events_seen_at_idx ON events (seen_at DESC);

-- Prediction markets: what is tradable, plus odds over time.
CREATE TABLE IF NOT EXISTS markets (
  id         TEXT PRIMARY KEY,              -- venue:id
  venue      TEXT NOT NULL,
  question   TEXT NOT NULL,
  slug       TEXT,
  end_at     INTEGER,
  volume     REAL,
  prices     TEXT,                          -- JSON {"outcomes": [...], "prices": [...]}
  tags       TEXT NOT NULL DEFAULT '[]',
  updated_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS market_prices (
  market_id TEXT NOT NULL REFERENCES markets (id),
  at        INTEGER NOT NULL,
  prices    TEXT NOT NULL,
  volume    REAL,
  PRIMARY KEY (market_id, at)
);
