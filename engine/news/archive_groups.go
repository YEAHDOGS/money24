package news

import (
	"context"
	"strconv"
	"time"
)

// A day's articles, grouped by outlet or by story: a busy day holds thousands of articles, so the
// groups come from the database whole, and each one is read page by page when it is opened.
const (
	GROUP_BY_SOURCE = "source"
	GROUP_BY_STORY  = "story"
	STORY_NONE      = "none" // the group of articles that joined no story
	ARCHIVE_GROUPS  = 300    // the most groups one answer lists, biggest first
)

// GROUP_KEYS are the SQL behind each grouping: the key, the group's name, the joins they need, and
// what sorts a group last whatever its size (articles in no story, which a reader looks at least).
var GROUP_KEYS = map[string]struct{ key, name, from, last string }{
	GROUP_BY_SOURCE: {`e.source`, `e.source`, ` FROM events e`, `''`},
	GROUP_BY_STORY: {`COALESCE(CAST(se.story_id AS TEXT), '` + STORY_NONE + `')`, `COALESCE(s.title, '')`,
		` FROM events e LEFT JOIN story_events se ON se.event_id = e.id LEFT JOIN stories s ON s.id = se.story_id`,
		`se.story_id IS NULL`},
}

// ArchiveGroup is one outlet or story of a day: how many articles it has and its newest headline.
// Key is what ?source= or ?story= takes to read the group's articles.
type ArchiveGroup struct {
	Key    string    `json:"key"`
	Name   string    `json:"name"`
	N      int       `json:"n"`
	Latest time.Time `json:"latest"`
	Title  string    `json:"title"`
}

// archiveGroups lists a query's articles by outlet or story, biggest first (articles in no story last),
// and how many groups there are in all (more than it lists on a busy day).
func (s *Store) archiveGroups(ctx context.Context, q ArchiveQuery, by string) ([]ArchiveGroup, int, error) {
	sql := GROUP_KEYS[by]
	where, args := q.filters()
	var total int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(DISTINCT `+sql.key+`)`+sql.from+where, args...).Scan(&total)
	if err != nil {
		return nil, 0, err
	}
	// With a single max(), SQLite takes the bare e.title from the row that holds it: the newest headline.
	rows, err := s.db.QueryContext(ctx, `SELECT `+sql.key+`, `+sql.name+`, COUNT(*) n, MAX(e.at) latest, e.title`+
		sql.from+where+` GROUP BY 1 ORDER BY `+sql.last+`, n DESC, latest DESC LIMIT ?`, append(args, ARCHIVE_GROUPS)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []ArchiveGroup{}
	for rows.Next() {
		var g ArchiveGroup
		var latest int64
		if err := rows.Scan(&g.Key, &g.Name, &g.N, &latest, &g.Title); err != nil {
			return nil, 0, err
		}
		g.Latest = time.UnixMilli(latest).UTC()
		out = append(out, g)
	}
	return out, total, rows.Err()
}

// validStory is true for "", STORY_NONE or a story id.
func validStory(story string) bool {
	if story == "" || story == STORY_NONE {
		return true
	}
	id, err := strconv.ParseInt(story, 10, 64)
	return err == nil && id > 0
}
