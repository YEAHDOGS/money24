package news

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// The archive: every stored article, read back by New York day for backtests and the calendar's day view.
const (
	ARCHIVE_DEFAULT_LIMIT = 100
	ARCHIVE_MAX_LIMIT     = 500
	ARCHIVE_MAX_DAYS      = 400 // the longest span one /api/archive/days call counts
	ARCHIVE_DEFAULT_DAYS  = 42  // six weeks back from today when no span is asked for
	SEARCH_MAX_CHARS      = 200
	SEARCH_MAX_TERMS      = 8
	FILTER_MAX_CHARS      = 200
	FACET_SOURCES         = 80
	FACET_TAGS            = 40
	FACET_KINDS           = 10
	CURSOR_SEP            = "_"
	SEARCH_PREFIX         = "*"
	SEARCH_QUOTE          = `"`
	SEARCH_AND            = " "
	ARCHIVE_COLUMNS       = `e.id, e.seen_at, e.at, e.published_at, e.feed, COALESCE(f.url, ''), e.source,
		COALESCE(e.outlet_url, ''), e.kind, e.title, e.url, COALESCE(e.author, ''), COALESCE(e.excerpt, e.summary, ''),
		COALESCE(e.lang, ''), COALESCE(e.country, ''), e.tags, COALESCE(e.image, ''),
		COALESCE(s.id, 0), COALESCE(s.title, ''), COALESCE(s.articles, 0)`
	ARCHIVE_FROM = ` FROM events e LEFT JOIN feed_urls f ON f.id = e.feed_url_id
		LEFT JOIN story_events se ON se.event_id = e.id LEFT JOIN stories s ON s.id = se.story_id`
)

// SEARCH_WORD is one word of a search: letters and digits only, so nothing reaches FTS5's query syntax.
var SEARCH_WORD = regexp.MustCompile(`[\p{L}\p{N}]+`)

// ArchivedEvent is one article as the archive serves it: everything its feed said, where it came
// from, and the story it joined. At is the archive's clock (published, never later than seen).
type ArchivedEvent struct {
	StoredEvent
	At      time.Time     `json:"at"`
	Excerpt string        `json:"excerpt,omitempty"`  // the feed's own text; the summary on rows stored before excerpts were kept
	FeedURL string        `json:"feed_url,omitempty"` // empty on rows stored before feed addresses were kept
	Story   *ArchiveStory `json:"story,omitempty"`
}

// ArchiveStory names the story an article belongs to.
type ArchiveStory struct {
	ID       int64  `json:"id"`
	Title    string `json:"title"`
	Articles int    `json:"articles"`
}

// ArchiveQuery is one page of a day's articles. From and To bound the archive clock; zero strings mean no filter.
type ArchiveQuery struct {
	From, To time.Time
	Search   string // an FTS5 expression from searchExpr
	Source   string
	Kind     string
	Tag      string
	Limit    int
	BeforeAt int64 // the cursor: the last article of the previous page
	BeforeID int64
}

// Facet is one value of a filter and how many of the day's articles carry it.
type Facet struct {
	Name string `json:"name"`
	N    int    `json:"n"`
}

// ArchiveFacets list what a day can be filtered by, busiest first.
type ArchiveFacets struct {
	Sources []Facet `json:"sources"`
	Kinds   []Facet `json:"kinds"`
	Tags    []Facet `json:"tags"`
}

// searchExpr turns what a reader typed into an FTS5 query: every word must appear, each as a
// prefix, so "fed rate" finds "Federal Reserve rate cut". Empty when nothing searchable was typed.
func searchExpr(q string) string {
	if runes := []rune(q); len(runes) > SEARCH_MAX_CHARS {
		q = string(runes[:SEARCH_MAX_CHARS])
	}
	words := SEARCH_WORD.FindAllString(q, SEARCH_MAX_TERMS)
	for i, w := range words {
		words[i] = SEARCH_QUOTE + w + SEARCH_QUOTE + SEARCH_PREFIX
	}
	return strings.Join(words, SEARCH_AND)
}

// filters is the WHERE clause and its arguments for a query, without the paging cursor.
func (q ArchiveQuery) filters() (string, []any) {
	where := []string{`e.at >= ?`, `e.at < ?`}
	args := []any{q.From.UnixMilli(), q.To.UnixMilli()}
	if q.Source != "" {
		where, args = append(where, `e.source = ?`), append(args, q.Source)
	}
	if q.Kind != "" {
		where, args = append(where, `e.kind = ?`), append(args, q.Kind)
	}
	if q.Tag != "" {
		where, args = append(where, `EXISTS (SELECT 1 FROM json_each(e.tags) WHERE value = ?)`), append(args, q.Tag)
	}
	if q.Search != "" {
		where, args = append(where, `e.id IN (SELECT rowid FROM events_fts WHERE events_fts MATCH ?)`), append(args, q.Search)
	}
	return ` WHERE ` + strings.Join(where, ` AND `), args
}

// archivePage reads one page newest first, and the cursor for the next one ("" on the last page).
func (s *Store) archivePage(ctx context.Context, q ArchiveQuery) ([]ArchivedEvent, string, error) {
	if q.Limit <= 0 || q.Limit > ARCHIVE_MAX_LIMIT {
		q.Limit = ARCHIVE_DEFAULT_LIMIT
	}
	where, args := q.filters()
	if q.BeforeAt > 0 {
		where += ` AND (e.at < ? OR (e.at = ? AND e.id < ?))`
		args = append(args, q.BeforeAt, q.BeforeAt, q.BeforeID)
	}
	args = append(args, q.Limit+1)
	rows, err := s.db.QueryContext(ctx, `SELECT `+ARCHIVE_COLUMNS+ARCHIVE_FROM+where+` ORDER BY e.at DESC, e.id DESC LIMIT ?`, args...)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	out := []ArchivedEvent{}
	for rows.Next() {
		a, err := scanArchived(rows)
		if err != nil {
			return nil, "", err
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}
	if len(out) <= q.Limit {
		return out, "", nil
	}
	out = out[:q.Limit]
	last := out[len(out)-1]
	return out, fmt.Sprint(last.At.UnixMilli(), CURSOR_SEP, last.ID), nil
}

func scanArchived(rows *sql.Rows) (ArchivedEvent, error) {
	var a ArchivedEvent
	var seen, at int64
	var published sql.NullInt64
	var tags string
	var story ArchiveStory
	err := rows.Scan(&a.ID, &seen, &at, &published, &a.Feed, &a.FeedURL, &a.Source, &a.OutletURL, &a.Kind, &a.Title,
		&a.URL, &a.Author, &a.Excerpt, &a.Lang, &a.Country, &tags, &a.Image, &story.ID, &story.Title, &story.Articles)
	if err != nil {
		return a, err
	}
	a.SeenAt, a.At = time.UnixMilli(seen).UTC(), time.UnixMilli(at).UTC()
	if published.Valid {
		t := time.UnixMilli(published.Int64).UTC()
		a.PublishedAt = &t
	}
	a.Tags = []string{}
	json.Unmarshal([]byte(tags), &a.Tags)
	if story.ID != 0 {
		a.Story = &story
	}
	return a, nil
}

// archiveTotal counts every article the query matches, across all its pages.
func (s *Store) archiveTotal(ctx context.Context, q ArchiveQuery) (int, error) {
	where, args := q.filters()
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM events e`+where, args...).Scan(&n)
	return n, err
}

// archiveFacets lists the outlets, kinds and tags of every article in [from, to).
func (s *Store) archiveFacets(ctx context.Context, from, to time.Time) (ArchiveFacets, error) {
	span := []any{from.UnixMilli(), to.UnixMilli()}
	var f ArchiveFacets
	var err error
	if f.Sources, err = s.facet(ctx, `SELECT source, COUNT(*) n FROM events WHERE at >= ? AND at < ?
		GROUP BY source ORDER BY n DESC, source LIMIT ?`, append(span, FACET_SOURCES)...); err != nil {
		return f, err
	}
	if f.Kinds, err = s.facet(ctx, `SELECT kind, COUNT(*) n FROM events WHERE at >= ? AND at < ?
		GROUP BY kind ORDER BY n DESC, kind LIMIT ?`, append(span, FACET_KINDS)...); err != nil {
		return f, err
	}
	f.Tags, err = s.facet(ctx, `SELECT t.value, COUNT(*) n FROM events, json_each(events.tags) t
		WHERE events.at >= ? AND events.at < ? GROUP BY t.value ORDER BY n DESC, t.value LIMIT ?`, append(span, FACET_TAGS)...)
	return f, err
}

func (s *Store) facet(ctx context.Context, query string, args ...any) ([]Facet, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Facet{}
	for rows.Next() {
		var f Facet
		if err := rows.Scan(&f.Name, &f.N); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// archiveSpan is one day of the archive and where it starts and ends on the archive clock.
type archiveSpan struct {
	Day      string
	From, To time.Time
}

// archiveDays counts the articles in each span. Days with none are left out.
func (s *Store) archiveDays(ctx context.Context, spans []archiveSpan) (map[string]int, error) {
	out := map[string]int{}
	if len(spans) == 0 {
		return out, nil
	}
	values := make([]string, len(spans))
	args := make([]any, 0, len(spans)*3)
	for i, sp := range spans {
		values[i] = `(?, ?, ?)`
		args = append(args, sp.Day, sp.From.UnixMilli(), sp.To.UnixMilli())
	}
	rows, err := s.db.QueryContext(ctx, `WITH d (day, lo, hi) AS (VALUES `+strings.Join(values, `, `)+`)
		SELECT day, (SELECT COUNT(*) FROM events WHERE at >= lo AND at < hi) FROM d`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var day string
		var n int
		if err := rows.Scan(&day, &n); err != nil {
			return nil, err
		}
		if n > 0 {
			out[day] = n
		}
	}
	return out, rows.Err()
}

// archiveBounds is when the oldest archived article was published and when the archive began: the
// first article we saw. Feeds send their history on a first poll, so the first can be far older.
// Both are the zero time on an empty archive.
func (s *Store) archiveBounds(ctx context.Context) (first, since time.Time, err error) {
	var oldest, began sql.NullInt64
	err = s.db.QueryRowContext(ctx, `SELECT (SELECT MIN(at) FROM events), (SELECT MIN(seen_at) FROM events)`).Scan(&oldest, &began)
	if err != nil || !oldest.Valid || !began.Valid {
		return first, since, err
	}
	return time.UnixMilli(oldest.Int64).UTC(), time.UnixMilli(began.Int64).UTC(), nil
}
