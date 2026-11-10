-- Scheduled events: economic releases, earnings, Treasury auctions, holidays, and anything a
-- calendar source in calendars.json brings in. Times are Unix milliseconds, UTC.

CREATE TABLE IF NOT EXISTS calendar_events (
  id         INTEGER PRIMARY KEY,
  uid        TEXT NOT NULL UNIQUE,          -- source id plus the source's own stable key
  source     TEXT NOT NULL,                 -- calendars.json id
  category   TEXT NOT NULL,                 -- macro | earnings | auction | holiday | other
  starts_at  INTEGER NOT NULL,              -- for all-day events: 00:00 UTC of that date
  all_day    INTEGER NOT NULL DEFAULT 0,
  title      TEXT NOT NULL,
  region     TEXT NOT NULL DEFAULT '',      -- currency or country code: USD, EUR
  symbol     TEXT NOT NULL DEFAULT '',      -- ticker, for earnings
  impact     TEXT NOT NULL DEFAULT '',      -- low | medium | high, or empty when unknown
  forecast   TEXT NOT NULL DEFAULT '',
  previous   TEXT NOT NULL DEFAULT '',
  actual     TEXT NOT NULL DEFAULT '',
  detail     TEXT NOT NULL DEFAULT '',
  url        TEXT NOT NULL DEFAULT '',
  updated_at INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS calendar_events_starts_idx ON calendar_events (starts_at);
CREATE INDEX IF NOT EXISTS calendar_events_source_idx ON calendar_events (source, starts_at);
