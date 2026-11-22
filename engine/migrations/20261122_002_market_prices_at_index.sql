-- The nightly backup exports market_prices one New York day at a time, which needs a way to find
-- a day's rows without reading the whole table. Times are Unix milliseconds, UTC.

CREATE INDEX IF NOT EXISTS market_prices_at_idx ON market_prices (at);
