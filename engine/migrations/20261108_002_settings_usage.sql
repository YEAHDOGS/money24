-- Engine settings (the global poll interval) and the data-usage ledger behind the network monitor.

CREATE TABLE IF NOT EXISTS settings (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL
);

-- One row per UTC day per channel. A channel is a source id, '_wire' (raw socket bytes, TLS included)
-- or '_served' (bytes sent to UI and API clients, the real egress on a hosted server).
CREATE TABLE IF NOT EXISTS net_usage (
  day       TEXT NOT NULL,                -- YYYY-MM-DD, UTC
  channel   TEXT NOT NULL,
  requests  INTEGER NOT NULL DEFAULT 0,
  bytes_in  INTEGER NOT NULL DEFAULT 0,
  bytes_out INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (day, channel)
);
