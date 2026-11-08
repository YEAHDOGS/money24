package main

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/lib/pq"
)

const (
	DB_CONNECT_ATTEMPTS = 30
	DB_CONNECT_PAUSE    = time.Second
	MAX_RECENT_LIMIT    = 500
)

//go:embed migrations/*.sql
var migrations embed.FS

// Event is one headline, filing or release, ready to store.
type Event struct {
	PublishedAt *time.Time `json:"published_at"`
	Feed        string     `json:"feed"`
	Source      string     `json:"source"`
	Kind        string     `json:"kind"`
	Title       string     `json:"title"`
	URL         string     `json:"url"`
	Summary     string     `json:"summary,omitempty"`
	Lang        string     `json:"lang,omitempty"`
	Country     string     `json:"country,omitempty"`
	Tags        []string   `json:"tags"`
}

// StoredEvent is an Event as read back, with its id and our own clock.
type StoredEvent struct {
	ID     int64     `json:"id"`
	SeenAt time.Time `json:"seen_at"`
	Event
}

// Market is one prediction market and its current odds.
type Market struct {
	ID       string
	Venue    string
	Question string
	Slug     string
	EndAt    *time.Time
	Volume   float64
	Prices   json.RawMessage
	Tags     []string
}

type Store struct{ db *sql.DB }

func openStore(ctx context.Context, url string) (*Store, error) {
	db, err := sql.Open("postgres", url)
	if err != nil {
		return nil, err
	}
	// The database container may still be booting; wait for it instead of crashing.
	for attempt := 1; ; attempt++ {
		err = db.PingContext(ctx)
		if err == nil {
			break
		}
		if attempt == DB_CONNECT_ATTEMPTS {
			return nil, fmt.Errorf("postgres unreachable: %w", err)
		}
		time.Sleep(DB_CONNECT_PAUSE)
	}
	store := &Store{db: db}
	return store, store.migrate(ctx)
}

// migrate applies each embedded SQL file once, in name order.
func (s *Store) migrate(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		name TEXT PRIMARY KEY, applied_at TIMESTAMPTZ NOT NULL DEFAULT now())`)
	if err != nil {
		return err
	}
	files, err := migrations.ReadDir("migrations")
	if err != nil {
		return err
	}
	names := make([]string, 0, len(files))
	for _, f := range files {
		names = append(names, f.Name())
	}
	sort.Strings(names)

	for _, name := range names {
		var applied bool
		err := s.db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE name = $1)`, name).Scan(&applied)
		if err != nil {
			return err
		}
		if applied {
			continue
		}
		body, err := migrations.ReadFile("migrations/" + name)
		if err != nil {
			return err
		}
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, string(body)); err != nil {
			tx.Rollback()
			return fmt.Errorf("migration %s: %w", name, err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations (name) VALUES ($1)`, name); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

// insertEvents stores new events and skips URLs already seen. Returns how many were new.
func (s *Store) insertEvents(ctx context.Context, events []Event) (int, error) {
	if len(events) == 0 {
		return 0, nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO events
		(published_at, feed, source, kind, title, url, summary, lang, country, tags)
		VALUES ($1, $2, $3, $4, $5, $6, NULLIF($7, ''), NULLIF($8, ''), NULLIF($9, ''), $10)
		ON CONFLICT (url) DO NOTHING`)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()

	added := 0
	for _, e := range events {
		res, err := stmt.ExecContext(ctx, e.PublishedAt, e.Feed, e.Source, e.Kind, e.Title, e.URL,
			e.Summary, e.Lang, e.Country, pq.Array(e.Tags))
		if err != nil {
			return 0, err
		}
		n, _ := res.RowsAffected()
		added += int(n)
	}
	return added, tx.Commit()
}

// upsertMarkets refreshes each market and records an odds snapshot for history.
func (s *Store) upsertMarkets(ctx context.Context, markets []Market) (int, error) {
	if len(markets) == 0 {
		return 0, nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	upsert, err := tx.PrepareContext(ctx, `INSERT INTO markets
		(id, venue, question, slug, end_at, volume, prices, tags, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, now())
		ON CONFLICT (id) DO UPDATE SET question = EXCLUDED.question, end_at = EXCLUDED.end_at,
			volume = EXCLUDED.volume, prices = EXCLUDED.prices, tags = EXCLUDED.tags, updated_at = now()`)
	if err != nil {
		return 0, err
	}
	defer upsert.Close()
	snapshot, err := tx.PrepareContext(ctx, `INSERT INTO market_prices (market_id, prices, volume)
		VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`)
	if err != nil {
		return 0, err
	}
	defer snapshot.Close()

	for _, m := range markets {
		prices := []byte(m.Prices)
		if _, err := upsert.ExecContext(ctx, m.ID, m.Venue, m.Question, m.Slug, m.EndAt, m.Volume, prices, pq.Array(m.Tags)); err != nil {
			return 0, err
		}
		if _, err := snapshot.ExecContext(ctx, m.ID, prices, m.Volume); err != nil {
			return 0, err
		}
	}
	return len(markets), tx.Commit()
}

// recentEvents returns the newest events, optionally only those carrying a tag.
func (s *Store) recentEvents(ctx context.Context, limit int, tag string) ([]StoredEvent, error) {
	if limit <= 0 || limit > MAX_RECENT_LIMIT {
		limit = MAX_RECENT_LIMIT
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, seen_at, published_at, feed, source, kind, title, url,
		COALESCE(summary, ''), COALESCE(lang, ''), COALESCE(country, ''), tags
		FROM events WHERE ($2 = '' OR $2 = ANY (tags)) ORDER BY seen_at DESC, id DESC LIMIT $1`, limit, tag)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []StoredEvent{}
	for rows.Next() {
		var e StoredEvent
		var published sql.NullTime
		var tags pq.StringArray
		if err := rows.Scan(&e.ID, &e.SeenAt, &published, &e.Feed, &e.Source, &e.Kind, &e.Title, &e.URL,
			&e.Summary, &e.Lang, &e.Country, &tags); err != nil {
			return nil, err
		}
		if published.Valid {
			e.PublishedAt = &published.Time
		}
		e.Tags = tags
		out = append(out, e)
	}
	return out, rows.Err()
}
