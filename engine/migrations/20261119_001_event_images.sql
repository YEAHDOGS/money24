-- The picture a feed sends with an article, when it sends one: a link to the outlet's own image.
-- Nothing is downloaded or stored here; the page shows it straight from the outlet.

ALTER TABLE events ADD COLUMN image TEXT;
