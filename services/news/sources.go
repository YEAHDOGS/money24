package main

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"html"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	MAX_BODY_BYTES    = 16 << 20
	SUMMARY_MAX_CHARS = 600
	MIN_INTERVAL      = 15 * time.Second
	GDELT_SPACING     = 6 * time.Second // GDELT allows one request per 5 seconds
	ERROR_SNIPPET     = 160
)

const (
	TYPE_RSS         = "rss"
	TYPE_GDELT       = "gdelt"
	TYPE_POLYMARKET  = "polymarket"
	VENUE_POLYMARKET = "polymarket"
)

var (
	TAG_PATTERN   = regexp.MustCompile(`<[^>]*>`)
	SPACE_PATTERN = regexp.MustCompile(`[\s\p{Zs}]+`)
	TIME_LAYOUTS  = []string{
		time.RFC1123Z, time.RFC1123, time.RFC3339, time.RFC3339Nano,
		"Mon, 2 Jan 2006 15:04:05 -0700", "Mon, 2 Jan 2006 15:04:05 MST",
		"Mon, 02 Jan 2006 15:04 -0700", "2 Jan 2006 15:04:05 -0700",
		"2006-01-02T15:04:05-07:00", "2006-01-02 15:04:05", "20060102T150405Z",
	}
)

// Source is one entry in sources.json.
type Source struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Kind     string `json:"kind"`
	URL      string `json:"url"`
	Every    string `json:"every"`
	interval time.Duration
}

type poller func(ctx context.Context, src Source, f *Fetcher, store *Store, tagger *Tagger) (int, error)

var POLLERS = map[string]poller{
	TYPE_RSS:        pollFeed,
	TYPE_GDELT:      pollGDELT,
	TYPE_POLYMARKET: pollPolymarket,
}

func loadSources(path string) ([]Source, error) {
	raw, err := readConfig(path, "sources.json")
	if err != nil {
		return nil, err
	}
	var doc struct {
		Sources []Source `json:"sources"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}
	for i := range doc.Sources {
		src := &doc.Sources[i]
		if _, ok := POLLERS[src.Type]; !ok {
			return nil, fmt.Errorf("%s: unknown type %q", src.ID, src.Type)
		}
		every, err := time.ParseDuration(src.Every)
		if err != nil {
			return nil, fmt.Errorf("%s: bad every %q", src.ID, src.Every)
		}
		src.interval = max(every, MIN_INTERVAL)
	}
	return doc.Sources, nil
}

// runSource polls one source forever, on its own clock.
func runSource(ctx context.Context, src Source, f *Fetcher, store *Store, tagger *Tagger) {
	poll := POLLERS[src.Type]
	ticker := time.NewTicker(src.interval)
	defer ticker.Stop()
	for {
		added, err := poll(ctx, src, f, store, tagger)
		if err != nil && ctx.Err() == nil {
			log.Printf("[%s] %v", src.ID, err)
		}
		if added > 0 {
			log.Printf("[%s] +%d", src.ID, added)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// Fetcher does polite GETs: one User-Agent, conditional requests, bounded bodies.
type Fetcher struct {
	client     *http.Client
	userAgent  string
	mu         sync.Mutex
	validators map[string][2]string // url -> {ETag, Last-Modified}
	gdeltMu    sync.Mutex
	gdeltLast  time.Time
}

func newFetcher(client *http.Client, userAgent string) *Fetcher {
	return &Fetcher{client: client, userAgent: userAgent, validators: map[string][2]string{}}
}

// get returns the body, or nil when the server says nothing changed (304).
func (f *Fetcher) get(ctx context.Context, rawURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", f.userAgent)
	f.mu.Lock()
	cached := f.validators[rawURL]
	f.mu.Unlock()
	if cached[0] != "" {
		req.Header.Set("If-None-Match", cached[0])
	}
	if cached[1] != "" {
		req.Header.Set("If-Modified-Since", cached[1])
	}

	res, err := f.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusNotModified {
		return nil, nil
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, MAX_BODY_BYTES))
	if err != nil {
		return nil, err
	}
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", res.StatusCode, snippet(body))
	}
	f.mu.Lock()
	f.validators[rawURL] = [2]string{res.Header.Get("ETag"), res.Header.Get("Last-Modified")}
	f.mu.Unlock()
	return body, nil
}

// --- RSS 2.0, RSS 1.0 (RDF) and Atom ---

type feedItem struct {
	Title       string   `xml:"title"`
	Links       []string `xml:"link"`
	Description string   `xml:"description"`
	PubDate     string   `xml:"pubDate"`
	Date        string   `xml:"date"` // dc:date
	GUID        string   `xml:"guid"`
}

type atomLink struct {
	Href string `xml:"href,attr"`
	Rel  string `xml:"rel,attr"`
}

type atomEntry struct {
	Title     string     `xml:"title"`
	Links     []atomLink `xml:"link"`
	Summary   string     `xml:"summary"`
	Content   string     `xml:"content"`
	Updated   string     `xml:"updated"`
	Published string     `xml:"published"`
}

type feedDoc struct {
	Channel struct {
		Items []feedItem `xml:"item"`
	} `xml:"channel"`
	Items   []feedItem  `xml:"item"`
	Entries []atomEntry `xml:"entry"`
}

func pollFeed(ctx context.Context, src Source, f *Fetcher, store *Store, tagger *Tagger) (int, error) {
	body, err := f.get(ctx, src.URL)
	if err != nil || body == nil {
		return 0, err
	}
	events, err := parseFeed(body, src)
	if err != nil {
		return 0, err
	}
	tagAll(events, tagger)
	return store.insertEvents(ctx, events)
}

func parseFeed(body []byte, src Source) ([]Event, error) {
	dec := xml.NewDecoder(bytes.NewReader(body))
	dec.Strict = false
	dec.Entity = xml.HTMLEntity
	dec.CharsetReader = func(_ string, r io.Reader) (io.Reader, error) { return r, nil }
	var doc feedDoc
	if err := dec.Decode(&doc); err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}

	events := []Event{}
	for _, it := range append(doc.Channel.Items, doc.Items...) {
		link := firstNonEmpty(append(it.Links, it.GUID)...)
		e, ok := newEvent(src, it.Title, link, it.Description, firstNonEmpty(it.PubDate, it.Date))
		if ok {
			events = append(events, e)
		}
	}
	for _, en := range doc.Entries {
		e, ok := newEvent(src, en.Title, atomHref(en.Links), firstNonEmpty(en.Summary, en.Content),
			firstNonEmpty(en.Published, en.Updated))
		if ok {
			events = append(events, e)
		}
	}
	return events, nil
}

func newEvent(src Source, title, link, summary, published string) (Event, bool) {
	title = clean(title, 0)
	link = strings.TrimSpace(link)
	if title == "" || !strings.HasPrefix(link, "http") {
		return Event{}, false
	}
	return Event{
		PublishedAt: parseTime(published),
		Feed:        src.ID,
		Source:      src.ID,
		Kind:        src.Kind,
		Title:       title,
		URL:         link,
		Summary:     clean(summary, SUMMARY_MAX_CHARS),
	}, true
}

func atomHref(links []atomLink) string {
	for _, l := range links {
		if l.Rel == "" || l.Rel == "alternate" {
			return l.Href
		}
	}
	if len(links) > 0 {
		return links[0].Href
	}
	return ""
}

// --- GDELT DOC 2.0: global news in 100+ languages ---

type gdeltDoc struct {
	Articles []struct {
		URL           string `json:"url"`
		Title         string `json:"title"`
		SeenDate      string `json:"seendate"`
		Domain        string `json:"domain"`
		Language      string `json:"language"`
		SourceCountry string `json:"sourcecountry"`
	} `json:"articles"`
}

func pollGDELT(ctx context.Context, src Source, f *Fetcher, store *Store, tagger *Tagger) (int, error) {
	// All GDELT sources share one rate limit, so take turns.
	f.gdeltMu.Lock()
	if wait := GDELT_SPACING - time.Since(f.gdeltLast); wait > 0 {
		time.Sleep(wait)
	}
	body, err := f.get(ctx, src.URL)
	f.gdeltLast = time.Now()
	f.gdeltMu.Unlock()
	if err != nil || body == nil {
		return 0, err
	}
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return 0, nil // GDELT answers an empty body when nothing matched
	}
	if trimmed[0] != '{' {
		return 0, fmt.Errorf("gdelt: %s", snippet(trimmed))
	}
	var doc gdeltDoc
	if err := json.Unmarshal(trimmed, &doc); err != nil {
		return 0, fmt.Errorf("gdelt parse: %w", err)
	}

	events := []Event{}
	for _, a := range doc.Articles {
		e, ok := newEvent(src, a.Title, a.URL, "", a.SeenDate)
		if !ok {
			continue
		}
		e.Source = firstNonEmpty(a.Domain, src.ID)
		e.Lang = a.Language
		e.Country = a.SourceCountry
		events = append(events, e)
	}
	tagAll(events, tagger)
	return store.insertEvents(ctx, events)
}

// --- Polymarket: what is tradable right now, and at what odds ---

type polymarketMarket struct {
	ID            json.RawMessage `json:"id"`
	Question      string          `json:"question"`
	Slug          string          `json:"slug"`
	EndDate       string          `json:"endDate"`
	VolumeNum     float64         `json:"volumeNum"`
	Outcomes      json.RawMessage `json:"outcomes"`
	OutcomePrices json.RawMessage `json:"outcomePrices"`
}

func pollPolymarket(ctx context.Context, src Source, f *Fetcher, store *Store, tagger *Tagger) (int, error) {
	body, err := f.get(ctx, src.URL)
	if err != nil || body == nil {
		return 0, err
	}
	var raw []polymarketMarket
	if err := json.Unmarshal(body, &raw); err != nil {
		return 0, fmt.Errorf("polymarket parse: %w", err)
	}

	markets := make([]Market, 0, len(raw))
	for _, m := range raw {
		id := strings.Trim(string(m.ID), `"`)
		if id == "" || m.Question == "" {
			continue
		}
		prices, _ := json.Marshal(map[string][]string{
			"outcomes": stringList(m.Outcomes),
			"prices":   stringList(m.OutcomePrices),
		})
		markets = append(markets, Market{
			ID:       VENUE_POLYMARKET + ":" + id,
			Venue:    VENUE_POLYMARKET,
			Question: m.Question,
			Slug:     m.Slug,
			EndAt:    parseTime(m.EndDate),
			Volume:   m.VolumeNum,
			Prices:   prices,
			Tags:     tagger.match(m.Question),
		})
	}
	return store.upsertMarkets(ctx, markets)
}

// stringList reads a JSON array that may arrive as a real array or as a string holding one.
func stringList(raw json.RawMessage) []string {
	out := []string{}
	if len(raw) == 0 {
		return out
	}
	if raw[0] == '"' {
		var inner string
		if json.Unmarshal(raw, &inner) != nil {
			return out
		}
		raw = json.RawMessage(inner)
	}
	var items []any
	if json.Unmarshal(raw, &items) != nil {
		return out
	}
	for _, it := range items {
		out = append(out, fmt.Sprint(it))
	}
	return out
}

// --- helpers ---

func tagAll(events []Event, tagger *Tagger) {
	for i := range events {
		events[i].Tags = tagger.match(events[i].Title + " " + events[i].Summary)
	}
}

// clean strips HTML, decodes entities, collapses whitespace and truncates to limit runes (0 = no limit).
func clean(s string, limit int) string {
	s = html.UnescapeString(TAG_PATTERN.ReplaceAllString(s, " "))
	s = strings.TrimSpace(SPACE_PATTERN.ReplaceAllString(s, " "))
	runes := []rune(s)
	if limit > 0 && len(runes) > limit {
		return string(runes[:limit]) + "…"
	}
	return s
}

func parseTime(s string) *time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	for _, layout := range TIME_LAYOUTS {
		if t, err := time.Parse(layout, s); err == nil {
			utc := t.UTC()
			return &utc
		}
	}
	return nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v = strings.TrimSpace(v); v != "" {
			return v
		}
	}
	return ""
}

func snippet(b []byte) string {
	s := clean(string(b), ERROR_SNIPPET)
	if s == "" {
		return "(empty body)"
	}
	return s
}
