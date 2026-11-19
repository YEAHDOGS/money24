package news

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"
)

const (
	STORY_KINDS_SQL  = `('news', 'central_bank', 'macro')` // filings arrive by the thousand and tell no shared story
	MAX_STORY_EVENTS = 200
	MAX_HISTORY      = 500
	METRICS_KEEP     = 30 * 24 * time.Hour
	MARKETS_FRESH    = 24 * time.Hour // a prediction market not refreshed for this long has left the top of the board
)

// StoryPoint is one reading of a story's numbers, kept so coverage can later be set against volume.
type StoryPoint struct {
	At        time.Time `json:"at"`
	Articles  int       `json:"articles"`
	Outlets   int       `json:"outlets"`
	LastHour  int       `json:"last_hour"`
	Attention float64   `json:"attention"`
	Validity  float64   `json:"validity"`
	Symbol    string    `json:"symbol,omitempty"`
	RelVolume float64   `json:"rel_volume,omitempty"`
	MovePct   float64   `json:"move_pct,omitempty"`
	Reading   string    `json:"reading,omitempty"`
}

// storyEvent is an article with the story it was filed under (0 for none yet).
type storyEvent struct {
	StoredEvent
	StoryID int64
}

// eventsForStories reads the articles stories are made of, oldest first: those seen since cutoff, and
// every article of a story still open then, however old. With afterID it reads only newer rows.
func (s *Store) eventsForStories(ctx context.Context, cutoff time.Time, afterID int64) ([]storyEvent, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT e.id, e.seen_at, e.published_at, e.feed, e.source, e.kind, e.title, e.url,
		COALESCE(e.country, ''), e.tags, COALESCE(se.story_id, 0)
		FROM events e LEFT JOIN story_events se ON se.event_id = e.id
		WHERE e.id > ?1 AND e.kind IN `+STORY_KINDS_SQL+`
			AND (e.seen_at >= ?2 OR se.story_id IN (SELECT id FROM stories WHERE last_seen >= ?2))
		ORDER BY e.id`, afterID, cutoff.UnixMilli())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []storyEvent{}
	for rows.Next() {
		var e storyEvent
		var seen int64
		var published sql.NullInt64
		var tags string
		if err := rows.Scan(&e.ID, &seen, &published, &e.Feed, &e.Source, &e.Kind, &e.Title, &e.URL, &e.Country, &tags, &e.StoryID); err != nil {
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

// saveStories writes every changed story and files its new articles under it, in one transaction.
// A story with no id yet gets one.
func (s *Store) saveStories(ctx context.Context, changed []*cluster) error {
	if len(changed) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, c := range changed {
		st := c.story
		if c.id == 0 {
			res, err := tx.ExecContext(ctx, `INSERT INTO stories (first_seen, last_seen, title, category, angle, articles, outlets, attention, validity)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, st.FirstSeen.UnixMilli(), st.LastSeen.UnixMilli(), st.Title, st.Category, st.Angle.Label,
				st.Articles, st.Outlets, st.Attention.Score, st.Validity.Score)
			if err != nil {
				return err
			}
			if c.id, err = res.LastInsertId(); err != nil {
				return err
			}
			c.story.ID = c.id
		}
		detail, _ := json.Marshal(c.story)
		if _, err := tx.ExecContext(ctx, `UPDATE stories SET last_seen = ?, title = ?, category = ?, angle = ?, articles = ?, outlets = ?,
			attention = ?, validity = ?, detail = ? WHERE id = ?`, st.LastSeen.UnixMilli(), st.Title, st.Category, st.Angle.Label,
			st.Articles, st.Outlets, st.Attention.Score, st.Validity.Score, string(detail), c.id); err != nil {
			return err
		}
		for _, m := range c.members[c.filed:] {
			if _, err := tx.ExecContext(ctx, `INSERT INTO story_events (event_id, story_id) VALUES (?, ?) ON CONFLICT (event_id) DO NOTHING`, m.ID, c.id); err != nil {
				return err
			}
		}
		c.filed = len(c.members)
	}
	return tx.Commit()
}

// addStoryPoints records one reading per story.
func (s *Store) addStoryPoints(ctx context.Context, points map[int64]StoryPoint) error {
	if len(points) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for id, p := range points {
		if _, err := tx.ExecContext(ctx, `INSERT INTO story_metrics
			(story_id, at, articles, outlets, last_hour, attention, validity, symbol, rel_volume, move_pct, reading)
			VALUES (?, ?, ?, ?, ?, ?, ?, NULLIF(?, ''), NULLIF(?, 0), NULLIF(?, 0), NULLIF(?, ''))
			ON CONFLICT DO NOTHING`, id, p.At.UnixMilli(), p.Articles, p.Outlets, p.LastHour, p.Attention, p.Validity,
			p.Symbol, p.RelVolume, p.MovePct, p.Reading); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// storyHistory returns a story's readings, oldest first.
func (s *Store) storyHistory(ctx context.Context, id int64, limit int) ([]StoryPoint, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT at, articles, outlets, last_hour, attention, validity, COALESCE(symbol, ''),
		COALESCE(rel_volume, 0), COALESCE(move_pct, 0), COALESCE(reading, '')
		FROM (SELECT * FROM story_metrics WHERE story_id = ? ORDER BY at DESC LIMIT ?) ORDER BY at`, id, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []StoryPoint{}
	for rows.Next() {
		var p StoryPoint
		var at int64
		if err := rows.Scan(&at, &p.Articles, &p.Outlets, &p.LastHour, &p.Attention, &p.Validity, &p.Symbol, &p.RelVolume, &p.MovePct, &p.Reading); err != nil {
			return nil, err
		}
		p.At = time.UnixMilli(at).UTC()
		out = append(out, p)
	}
	return out, rows.Err()
}

// pruneStoryPoints drops readings older than METRICS_KEEP.
func (s *Store) pruneStoryPoints(ctx context.Context, now time.Time) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM story_metrics WHERE at < ?`, now.Add(-METRICS_KEEP).UnixMilli())
	return err
}

// storedStory reads a closed story back as it was last saved.
func (s *Store) storedStory(ctx context.Context, id int64) (Story, bool) {
	var detail string
	var st Story
	err := s.db.QueryRowContext(ctx, `SELECT detail FROM stories WHERE id = ?`, id).Scan(&detail)
	if err != nil || json.Unmarshal([]byte(detail), &st) != nil {
		return st, false
	}
	return st, true
}

// storyArticles returns a story's articles, newest first.
func (s *Store) storyArticles(ctx context.Context, id int64) ([]StoredEvent, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT e.id, e.seen_at, e.published_at, e.feed, e.source, e.kind, e.title, e.url,
		COALESCE(e.lang, ''), COALESCE(e.country, ''), e.tags
		FROM story_events se JOIN events e ON e.id = se.event_id WHERE se.story_id = ? ORDER BY e.id DESC LIMIT ?`, id, MAX_STORY_EVENTS)
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
		if err := rows.Scan(&e.ID, &seen, &published, &e.Feed, &e.Source, &e.Kind, &e.Title, &e.URL, &e.Lang, &e.Country, &tags); err != nil {
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

// liveMarket is a prediction market still near the top of its venue's board.
type liveMarket struct {
	Bet
	tokens []string
}

// liveMarkets reads the prediction markets refreshed lately that have not ended.
func (s *Store) liveMarkets(ctx context.Context, now time.Time, stop map[string]bool) ([]liveMarket, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, question, COALESCE(slug, ''), COALESCE(volume, 0), COALESCE(prices, '{}')
		FROM markets WHERE updated_at >= ? AND (end_at IS NULL OR end_at > ?)`, now.Add(-MARKETS_FRESH).UnixMilli(), now.UnixMilli())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []liveMarket{}
	for rows.Next() {
		var m liveMarket
		var slug, prices string
		if err := rows.Scan(&m.ID, &m.Question, &slug, &m.Volume, &prices); err != nil {
			return nil, err
		}
		m.URL, m.Yes = POLYMARKET_PAGE+slug, firstPrice(prices)
		m.tokens = tokens(words(m.Question), stop)
		out = append(out, m)
	}
	return out, rows.Err()
}
