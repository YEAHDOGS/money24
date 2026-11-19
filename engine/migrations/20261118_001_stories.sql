-- Stories: every article about the same event, grouped. A story is stored once a second article
-- joins the first; single articles wait in memory. Times are Unix milliseconds, UTC.

CREATE TABLE IF NOT EXISTS stories (
  id         INTEGER PRIMARY KEY,
  first_seen INTEGER NOT NULL,            -- when we saw its first article (the honest backtest clock)
  last_seen  INTEGER NOT NULL,            -- when we saw its newest article
  title      TEXT NOT NULL,               -- the first headline we saw
  category   TEXT NOT NULL,               -- story_rules.json category id
  angle      TEXT NOT NULL,               -- bullish | bearish | mixed | unclear
  articles   INTEGER NOT NULL,
  outlets    INTEGER NOT NULL,
  attention  REAL NOT NULL,               -- 0 to 100
  validity   REAL NOT NULL,               -- 0 to 100
  detail     TEXT NOT NULL DEFAULT '{}'   -- JSON: the whole story as the API serves it
);

CREATE INDEX IF NOT EXISTS stories_last_seen_idx ON stories (last_seen DESC);

-- Which story each article belongs to. An article is in one story at most.
CREATE TABLE IF NOT EXISTS story_events (
  event_id INTEGER PRIMARY KEY REFERENCES events (id),
  story_id INTEGER NOT NULL REFERENCES stories (id)
);

CREATE INDEX IF NOT EXISTS story_events_story_idx ON story_events (story_id);

-- A story's numbers over time: the record that shows whether attention led volume.
-- rel_volume and move_pct belong to the story's first directly linked instrument, when it has a quote.
CREATE TABLE IF NOT EXISTS story_metrics (
  story_id   INTEGER NOT NULL REFERENCES stories (id),
  at         INTEGER NOT NULL,
  articles   INTEGER NOT NULL,
  outlets    INTEGER NOT NULL,
  last_hour  INTEGER NOT NULL,            -- articles seen in the hour before `at`
  attention  REAL NOT NULL,
  validity   REAL NOT NULL,
  symbol     TEXT,
  rel_volume REAL,                        -- volume against the instrument's own average pace
  move_pct   REAL,                        -- the instrument's change on the day
  reading    TEXT,
  PRIMARY KEY (story_id, at)
);

CREATE INDEX IF NOT EXISTS story_metrics_at_idx ON story_metrics (at);
