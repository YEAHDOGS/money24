package news

import (
	"context"
	"database/sql"
	"time"
)

const (
	CAL_COLUMNS = `uid, source, category, starts_at, all_day, title, region, symbol, impact, forecast, previous, actual, detail, url`
	CAL_UPSERT  = `INSERT INTO calendar_events (` + CAL_COLUMNS + `, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (uid) DO UPDATE SET source = excluded.source, category = excluded.category,
			starts_at = excluded.starts_at, all_day = excluded.all_day, title = excluded.title,
			region = excluded.region, symbol = excluded.symbol, impact = excluded.impact,
			forecast = excluded.forecast, previous = excluded.previous, actual = excluded.actual,
			detail = excluded.detail, url = excluded.url, updated_at = excluded.updated_at`
)

// rowQuerier is the part of *sql.DB and *sql.Tx that reads rows.
type rowQuerier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

func calendarRows(ctx context.Context, q rowQuerier, where string, args ...any) ([]CalEvent, error) {
	rows, err := q.QueryContext(ctx, `SELECT `+CAL_COLUMNS+` FROM calendar_events `+where, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	events := []CalEvent{}
	for rows.Next() {
		var e CalEvent
		var startsAt int64
		if err := rows.Scan(&e.UID, &e.Source, &e.Category, &startsAt, &e.AllDay, &e.Title, &e.Region, &e.Symbol,
			&e.Impact, &e.Forecast, &e.Previous, &e.Actual, &e.Detail, &e.URL); err != nil {
			return nil, err
		}
		e.StartsAt = time.UnixMilli(startsAt).UTC()
		events = append(events, e)
	}
	return events, rows.Err()
}

// calendar returns events that start inside [from, to], soonest first.
func (s *Store) calendar(ctx context.Context, from, to time.Time, limit int) ([]CalEvent, error) {
	return calendarRows(ctx, s.db, `WHERE starts_at BETWEEN ? AND ? ORDER BY starts_at, id LIMIT ?`,
		from.UnixMilli(), to.UnixMilli(), limit)
}

// syncCalendar makes the stored events of one source match a fresh snapshot, over the span of time
// the snapshot covers: new and changed events are written, events that vanished from the snapshot
// (rescheduled or cancelled) are removed. It returns how many rows changed.
func (s *Store) syncCalendar(ctx context.Context, source string, events []CalEvent) (int, error) {
	if len(events) == 0 {
		return 0, nil
	}
	first, last := events[0].StartsAt, events[0].StartsAt
	for _, e := range events {
		if e.StartsAt.Before(first) {
			first = e.StartsAt
		}
		if e.StartsAt.After(last) {
			last = e.StartsAt
		}
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	stored, err := calendarRows(ctx, tx, `WHERE source = ? AND starts_at BETWEEN ? AND ?`, source, first.UnixMilli(), last.UnixMilli())
	if err != nil {
		return 0, err
	}
	old := map[string]CalEvent{}
	for _, e := range stored {
		old[e.UID] = e
	}
	changed := 0
	now := time.Now().UnixMilli()
	fresh := map[string]bool{}
	for _, e := range events {
		if fresh[e.UID] {
			continue
		}
		fresh[e.UID] = true
		if prev, ok := old[e.UID]; ok && sameCalEvent(prev, e) {
			continue
		}
		if _, err := tx.ExecContext(ctx, CAL_UPSERT, e.UID, e.Source, e.Category, e.StartsAt.UnixMilli(), e.AllDay, e.Title,
			e.Region, e.Symbol, e.Impact, e.Forecast, e.Previous, e.Actual, e.Detail, e.URL, now); err != nil {
			return 0, err
		}
		changed++
	}
	for uid := range old {
		if fresh[uid] {
			continue
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM calendar_events WHERE uid = ?`, uid); err != nil {
			return 0, err
		}
		changed++
	}
	return changed, tx.Commit()
}

func sameCalEvent(a, b CalEvent) bool {
	if !a.StartsAt.Equal(b.StartsAt) {
		return false
	}
	a.StartsAt, b.StartsAt = time.Time{}, time.Time{}
	return a == b
}
