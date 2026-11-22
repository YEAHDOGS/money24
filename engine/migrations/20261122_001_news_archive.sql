-- The news archive: what each article's own feed says about it, kept for backtesting and the
-- calendar's day view. Only what the feeds publish is stored; article pages are never fetched.
-- Rows stored before this migration keep NULL in the new columns; their summary stands in for the excerpt.

-- Every feed address an article was fetched from, stored once. A source whose URL changes in
-- sources.json gets a new row, so each article keeps the exact address it came from.
CREATE TABLE IF NOT EXISTS feed_urls (
  id         INTEGER PRIMARY KEY,
  feed       TEXT NOT NULL,                 -- the sources.json id that used it first
  url        TEXT NOT NULL UNIQUE,
  first_seen INTEGER NOT NULL
);

ALTER TABLE events ADD COLUMN author TEXT;       -- byline: RSS author or dc:creator, Atom author name
ALTER TABLE events ADD COLUMN excerpt TEXT;      -- plain text, at most 1,000 characters, from the feed's own content or description
ALTER TABLE events ADD COLUMN outlet_url TEXT;   -- the outlet's site: as the item names it (Google News <source url>), else the link's origin
ALTER TABLE events ADD COLUMN feed_url_id INTEGER REFERENCES feed_urls (id);
-- The archive's clock: when the source says it was published, never later than we saw it (a wrong
-- future date cannot hide an article), and our own clock when the source gives no date at all.
ALTER TABLE events ADD COLUMN at INTEGER GENERATED ALWAYS AS (MIN(COALESCE(published_at, seen_at), seen_at)) VIRTUAL;

CREATE INDEX IF NOT EXISTS events_at_idx ON events (at);

-- Word search over headline and excerpt. Contentless: the text lives in events only, the index
-- holds just the words. Triggers keep it in step; events are never deleted.
CREATE VIRTUAL TABLE IF NOT EXISTS events_fts USING fts5 (
  title, body, content = '', contentless_delete = 1, tokenize = 'unicode61 remove_diacritics 2'
);

INSERT INTO events_fts (rowid, title, body) SELECT id, title, COALESCE(excerpt, summary, '') FROM events;

CREATE TRIGGER IF NOT EXISTS events_fts_add AFTER INSERT ON events BEGIN
  INSERT INTO events_fts (rowid, title, body) VALUES (new.id, new.title, COALESCE(new.excerpt, new.summary, ''));
END;

CREATE TRIGGER IF NOT EXISTS events_fts_change AFTER UPDATE OF title, summary, excerpt ON events BEGIN
  DELETE FROM events_fts WHERE rowid = old.id;
  INSERT INTO events_fts (rowid, title, body) VALUES (new.id, new.title, COALESCE(new.excerpt, new.summary, ''));
END;
