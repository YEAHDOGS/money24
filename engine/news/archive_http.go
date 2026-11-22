package news

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	ARCHIVE_PATH      = "/api/archive"
	ARCHIVE_DAYS_PATH = "/api/archive/days"
	// The archive changes only as new articles arrive; a minute's copy saves a reload on every page turn.
	CACHE_ARCHIVE = "private, max-age=60"
)

var (
	ERR_BAD_DAY    = errors.New("day must be YYYY-MM-DD")
	ERR_BAD_SPAN   = fmt.Errorf("from and to must be YYYY-MM-DD, from on or before to, at most %d days apart", ARCHIVE_MAX_DAYS)
	ERR_BAD_CURSOR = errors.New("before must be the next cursor of a previous page")
	ERR_LONG_VALUE = fmt.Errorf("a filter is longer than %d characters", FILTER_MAX_CHARS)
)

// ArchiveDays is how many articles each New York day holds. Days with none are left out.
type ArchiveDays struct {
	Zone  string         `json:"zone"`
	First string         `json:"first,omitempty"` // the oldest day with an article, empty on an empty archive
	Since string         `json:"since,omitempty"` // the day the archive began: every day since is covered
	From  string         `json:"from"`
	To    string         `json:"to"`
	Days  map[string]int `json:"days"`
}

// ArchivePage is one page of a day's articles, newest first. Total and Facets come with the first page only.
type ArchivePage struct {
	Day      string          `json:"day"`
	Zone     string          `json:"zone"`
	Articles []ArchivedEvent `json:"articles"`
	Next     string          `json:"next,omitempty"` // pass as ?before= for the next page
	Total    *int            `json:"total,omitempty"`
	Facets   *ArchiveFacets  `json:"facets,omitempty"`
}

// dayStart is 00:00 New York time on a YYYY-MM-DD day.
func dayStart(day string, zone *time.Location) (time.Time, error) {
	t, err := time.ParseInLocation(DAY_LAYOUT, day, zone)
	if err != nil {
		return time.Time{}, ERR_BAD_DAY
	}
	return t, nil
}

func writeArchive(w http.ResponseWriter, v any) {
	h := w.Header()
	h.Set("Content-Type", "application/json")
	h.Set("Cache-Control", CACHE_ARCHIVE)
	json.NewEncoder(w).Encode(v)
}

// archiveDays answers GET /api/archive/days?from=YYYY-MM-DD&to=YYYY-MM-DD: articles per New York day.
// By default the six weeks up to today.
func (a *api) archiveDays(w http.ResponseWriter, r *http.Request) {
	zone := a.svc.stories.zone // the archive keeps New York days, as the market does
	q := r.URL.Query()
	today := time.Now().In(zone)
	to := today.Format(DAY_LAYOUT)
	if v := q.Get("to"); v != "" {
		to = v
	}
	end, err := dayStart(to, zone)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": ERR_BAD_SPAN.Error()})
		return
	}
	from := end.AddDate(0, 0, 1-ARCHIVE_DEFAULT_DAYS).Format(DAY_LAYOUT)
	if v := q.Get("from"); v != "" {
		from = v
	}
	start, err := dayStart(from, zone)
	if err != nil || start.After(end) || start.AddDate(0, 0, ARCHIVE_MAX_DAYS).Before(end) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": ERR_BAD_SPAN.Error()})
		return
	}
	spans := []archiveSpan{}
	for day := start; !day.After(end); day = day.AddDate(0, 0, 1) {
		spans = append(spans, archiveSpan{Day: day.Format(DAY_LAYOUT), From: day, To: day.AddDate(0, 0, 1)})
	}
	counts, err := a.svc.store.archiveDays(r.Context(), spans)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	out := ArchiveDays{Zone: zone.String(), From: from, To: to, Days: counts}
	if first, since, err := a.svc.store.archiveBounds(r.Context()); err == nil && !first.IsZero() {
		out.First, out.Since = first.In(zone).Format(DAY_LAYOUT), since.In(zone).Format(DAY_LAYOUT)
	}
	writeArchive(w, out)
}

// archive answers GET /api/archive?day=YYYY-MM-DD&q=&source=&kind=&tag=&limit=&before=: one New York
// day's articles, newest first, searched by the words of their title and excerpt.
func (a *api) archive(w http.ResponseWriter, r *http.Request) {
	zone := a.svc.stories.zone // the archive keeps New York days, as the market does
	q := r.URL.Query()
	start, err := dayStart(q.Get("day"), zone)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	for _, key := range []string{"source", "kind", "tag"} {
		if len(q.Get(key)) > FILTER_MAX_CHARS {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": ERR_LONG_VALUE.Error()})
			return
		}
	}
	query := ArchiveQuery{
		From: start, To: start.AddDate(0, 0, 1),
		Search: searchExpr(q.Get("q")),
		Source: strings.TrimSpace(q.Get("source")), Kind: q.Get("kind"), Tag: q.Get("tag"),
		Limit: int(queryInt(r, "limit", ARCHIVE_DEFAULT_LIMIT)),
	}
	if cursor := q.Get("before"); cursor != "" {
		at, id, ok := strings.Cut(cursor, CURSOR_SEP)
		query.BeforeAt, _ = strconv.ParseInt(at, 10, 64)
		query.BeforeID, _ = strconv.ParseInt(id, 10, 64)
		if !ok || query.BeforeAt <= 0 || query.BeforeID <= 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": ERR_BAD_CURSOR.Error()})
			return
		}
	}

	ctx := r.Context()
	articles, next, err := a.svc.store.archivePage(ctx, query)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	page := ArchivePage{Day: start.Format(DAY_LAYOUT), Zone: zone.String(), Articles: articles, Next: next}
	if query.BeforeAt == 0 {
		total, err := a.svc.store.archiveTotal(ctx, query)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		facets, err := a.svc.store.archiveFacets(ctx, query.From, query.To)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		page.Total, page.Facets = &total, &facets
	}
	writeArchive(w, page)
}
