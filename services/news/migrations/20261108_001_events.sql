-- Money 24 news store: every headline, filing, release and market we see.
-- seen_at is when WE saw it (the honest backtest clock); published_at is what the source claims.

CREATE TABLE IF NOT EXISTS events (
  id           BIGSERIAL PRIMARY KEY,
  seen_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  published_at TIMESTAMPTZ,
  feed         TEXT NOT NULL,              -- which poller found it (sources.json id)
  source       TEXT NOT NULL,              -- the outlet (feed id for RSS, domain for GDELT)
  kind         TEXT NOT NULL,              -- news | filing | insider | central_bank | macro
  title        TEXT NOT NULL,
  url          TEXT NOT NULL UNIQUE,
  summary      TEXT,
  lang         TEXT,
  country      TEXT,
  tags         TEXT[] NOT NULL DEFAULT '{}' -- watchlist hits: tickers, coins, macro buckets
);

CREATE INDEX IF NOT EXISTS events_seen_at_idx ON events (seen_at DESC);
CREATE INDEX IF NOT EXISTS events_tags_idx ON events USING GIN (tags);

-- Prediction markets: the list of things that are tradable, plus their odds over time.
CREATE TABLE IF NOT EXISTS markets (
  id         TEXT PRIMARY KEY,               -- venue:id
  venue      TEXT NOT NULL,
  question   TEXT NOT NULL,
  slug       TEXT,
  end_at     TIMESTAMPTZ,
  volume     NUMERIC,
  prices     JSONB,                          -- {"outcomes": [...], "prices": [...]}
  tags       TEXT[] NOT NULL DEFAULT '{}',
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS market_prices (
  market_id TEXT NOT NULL REFERENCES markets (id),
  at        TIMESTAMPTZ NOT NULL DEFAULT now(),
  prices    JSONB NOT NULL,
  volume    NUMERIC,
  PRIMARY KEY (market_id, at)
);

-- Live feed: announce every new event. Payload stays far under pg_notify's 8000-byte cap.
CREATE OR REPLACE FUNCTION notify_event() RETURNS trigger AS $$
BEGIN
  PERFORM pg_notify('events', json_build_object(
    'id', NEW.id, 'seen_at', NEW.seen_at, 'published_at', NEW.published_at,
    'source', left(NEW.source, 120), 'kind', NEW.kind, 'title', left(NEW.title, 400),
    'url', left(NEW.url, 1500), 'lang', NEW.lang, 'tags', NEW.tags
  )::text);
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS events_notify ON events;
CREATE TRIGGER events_notify AFTER INSERT ON events FOR EACH ROW EXECUTE FUNCTION notify_event();
