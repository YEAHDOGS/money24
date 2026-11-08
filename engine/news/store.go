package news

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"time"

	"github.com/YEAHDOGS/money24/engine/migrations"
	_ "modernc.org/sqlite" // CGO-free SQLite driver, registers "sqlite"
)

const (
	DB_FILE          = "news.db"
	DB_DRIVER        = "sqlite"
	DB_PRAGMAS       = "?_pragma=busy_timeout(10000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=foreign_keys(1)&_txlock=immediate"
	MAX_RECENT_LIMIT = 500
	SETTING_POLL     = "poll_every_ms"
)

// StoredEvent is an Event as read back, with its id and our own clock.
type StoredEvent struct {
	ID     int64     `json:"id"`
	SeenAt time.Time `json:"seen_at"`
	Event
}

// Store is the local SQLite database: events, markets, settings and the data-usage ledger.
type Store struct{ db *sql.DB }

// OpenStore opens (or creates) news.db inside dir and applies pending migrations.
func OpenStore(ctx context.Context, dir string) (*Store, error) {
	path := filepath.ToSlash(filepath.Join(dir, DB_FILE))
	db, err := sql.Open(DB_DRIVER, "file:"+path+DB_PRAGMAS)
	if err != nil {
		return nil, err
	}
	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	store := &Store{db: db}
	return store, store.migrate(ctx)
}

// Close closes the database.
func (s *Store) Close() error { return s.db.Close() }

// migrate applies each embedded SQL file once, in name order.
func (s *Store) migrate(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		name TEXT PRIMARY KEY, applied_at INTEGER NOT NULL)`)
	if err != nil {
		return err
	}
	files, err := migrations.FS.ReadDir(".")
	if err != nil {
		return err
	}
	names := []string{}
	for _, f := range files {
		if filepath.Ext(f.Name()) == ".sql" {
			names = append(names, f.Name())
		}
	}
	sort.Strings(names)
	for _, name := range names {
		if err := s.applyMigration(ctx, name); err != nil {
			return fmt.Errorf("migration %s: %w", name, err)
		}
	}
	return nil
}

func (s *Store) applyMigration(ctx context.Context, name string) error {
	var applied bool
	err := s.db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE name = ?)`, name).Scan(&applied)
	if err != nil || applied {
		return err
	}
	body, err := migrations.FS.ReadFile(name)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, string(body)); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations (name, applied_at) VALUES (?, ?)`, name, time.Now().UnixMilli()); err != nil {
		return err
	}
	return tx.Commit()
}

func millis(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.UnixMilli()
}

// insertEvents stores new events, skips URLs already seen, and returns the new rows in id order.
func (s *Store) insertEvents(ctx context.Context, events []Event) ([]StoredEvent, error) {
	if len(events) == 0 {
		return nil, nil
	}
	// Oldest first, so ids (and the live feed) follow publication order within a batch.
	sort.SliceStable(events, func(i, j int) bool {
		a, b := events[i].PublishedAt, events[j].PublishedAt
		if a == nil || b == nil {
			return a == nil && b != nil // undated items first, so dated news lands on top
		}
		return a.Before(*b)
	})
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO events
		(seen_at, published_at, feed, source, kind, title, url, summary, lang, country, tags)
		VALUES (?, ?, ?, ?, ?, ?, ?, NULLIF(?, ''), NULLIF(?, ''), NULLIF(?, ''), ?)
		ON CONFLICT (url) DO NOTHING RETURNING id`)
	if err != nil {
		return nil, err
	}
	defer stmt.Close()

	seen := time.Now().UTC()
	added := []StoredEvent{}
	for _, e := range events {
		tags, _ := json.Marshal(e.Tags)
		var id int64
		err := stmt.QueryRowContext(ctx, seen.UnixMilli(), millis(e.PublishedAt), e.Feed, e.Source, e.Kind,
			e.Title, e.URL, e.Summary, e.Lang, e.Country, string(tags)).Scan(&id)
		if errors.Is(err, sql.ErrNoRows) {
			continue // already stored
		}
		if err != nil {
			return nil, err
		}
		added = append(added, StoredEvent{ID: id, SeenAt: seen, Event: e})
	}
	return added, tx.Commit()
}

// upsertMarkets refreshes each market and records an odds snapshot for history.
func (s *Store) upsertMarkets(ctx context.Context, markets []Market) error {
	if len(markets) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	upsert, err := tx.PrepareContext(ctx, `INSERT INTO markets
		(id, venue, question, slug, end_at, volume, prices, tags, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET question = excluded.question, end_at = excluded.end_at,
			volume = excluded.volume, prices = excluded.prices, tags = excluded.tags, updated_at = excluded.updated_at`)
	if err != nil {
		return err
	}
	defer upsert.Close()
	snapshot, err := tx.PrepareContext(ctx, `INSERT INTO market_prices (market_id, at, prices, volume)
		VALUES (?, ?, ?, ?) ON CONFLICT DO NOTHING`)
	if err != nil {
		return err
	}
	defer snapshot.Close()

	now := time.Now().UnixMilli()
	for _, m := range markets {
		tags, _ := json.Marshal(m.Tags)
		prices := string(m.Prices)
		if _, err := upsert.ExecContext(ctx, m.ID, m.Venue, m.Question, m.Slug, millis(m.EndAt), m.Volume, prices, string(tags), now); err != nil {
			return err
		}
		if _, err := snapshot.ExecContext(ctx, m.ID, now, prices, m.Volume); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// EventQuery filters a read of stored events. Zero values mean no filter.
type EventQuery struct {
	Limit    int
	BeforeID int64
	AfterID  int64
	Kind     string
	Tag      string
}

// events reads stored events newest first (or, with AfterID, the newest ones after it).
func (s *Store) events(ctx context.Context, q EventQuery) ([]StoredEvent, error) {
	if q.Limit <= 0 || q.Limit > MAX_RECENT_LIMIT {
		q.Limit = MAX_RECENT_LIMIT
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, seen_at, published_at, feed, source, kind, title, url,
		COALESCE(summary, ''), COALESCE(lang, ''), COALESCE(country, ''), tags
		FROM events
		WHERE (?1 = 0 OR id < ?1) AND id > ?2 AND (?3 = '' OR kind = ?3)
			AND (?4 = '' OR EXISTS (SELECT 1 FROM json_each(events.tags) WHERE value = ?4))
		ORDER BY id DESC LIMIT ?5`, q.BeforeID, q.AfterID, q.Kind, q.Tag, q.Limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []StoredEvent{}
	for rows.Next() {
		var e StoredEvent
		var seen int64
		var published sql.NullInt64
		var tags string
		if err := rows.Scan(&e.ID, &seen, &published, &e.Feed, &e.Source, &e.Kind, &e.Title, &e.URL,
			&e.Summary, &e.Lang, &e.Country, &tags); err != nil {
			return nil, err
		}
		e.SeenAt = time.UnixMilli(seen).UTC()
		if published.Valid {
			t := time.UnixMilli(published.Int64).UTC()
			e.PublishedAt = &t
		}
		e.Tags = []string{}
		json.Unmarshal([]byte(tags), &e.Tags)
		out = append(out, e)
	}
	return out, rows.Err()
}

// addUsage adds drained meter readings to today's row per channel.
func (s *Store) addUsage(ctx context.Context, day string, tallies map[string]Tally) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for channel, t := range tallies {
		if t == (Tally{}) {
			continue
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO net_usage (day, channel, requests, bytes_in, bytes_out)
			VALUES (?, ?, ?, ?, ?)
			ON CONFLICT (day, channel) DO UPDATE SET requests = requests + excluded.requests,
				bytes_in = bytes_in + excluded.bytes_in, bytes_out = bytes_out + excluded.bytes_out`,
			day, channel, t.Requests, t.In, t.Out)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

// usageSince sums the ledger per channel from day onward (inclusive).
func (s *Store) usageSince(ctx context.Context, day string) (map[string]Tally, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT channel, SUM(requests), SUM(bytes_in), SUM(bytes_out)
		FROM net_usage WHERE day >= ? GROUP BY channel`, day)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]Tally{}
	for rows.Next() {
		var channel string
		var t Tally
		if err := rows.Scan(&channel, &t.Requests, &t.In, &t.Out); err != nil {
			return nil, err
		}
		out[channel] = t
	}
	return out, rows.Err()
}

// pollEvery returns the saved global interval, or fallback when none is saved.
func (s *Store) pollEvery(ctx context.Context, fallback time.Duration) time.Duration {
	var raw string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, SETTING_POLL).Scan(&raw)
	if err != nil {
		return fallback
	}
	ms, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return fallback
	}
	return time.Duration(ms) * time.Millisecond
}

func (s *Store) savePollEvery(ctx context.Context, every time.Duration) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO settings (key, value) VALUES (?, ?)
		ON CONFLICT (key) DO UPDATE SET value = excluded.value`, SETTING_POLL, strconv.FormatInt(every.Milliseconds(), 10))
	return err
}
