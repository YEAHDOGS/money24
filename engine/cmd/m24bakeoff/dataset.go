package main

import (
	"cmp"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"math/rand/v2"
	"os"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/YEAHDOGS/money24/engine/calendar"
	"github.com/YEAHDOGS/money24/engine/localai"
	"github.com/YEAHDOGS/money24/engine/market"
	_ "modernc.org/sqlite" // CGO-free SQLite driver, registers "sqlite"
)

// The labeled set is built from Money 24's own news database (a copy, opened read-only) and Yahoo's price
// history. Each item carries only the labels that can be checked without a person's opinion:
//
//   - story: newsd's own grouping of headlines into one story. Labeled "one event", which is only as good
//     as the grouping, so a read-through (the audit file) relabels the stories that are plainly several
//     events and drops the ones that are mixed. That read-through is a judgment, not a measurement.
//   - splice: headlines from two stories in different categories with no tag or title word in common,
//     shuffled together. Labeled "several events" by construction.
//   - bundle: every headline that named one watchlist company or coin on one New York day. Not one event,
//     so it carries no event label; it carries the realized move of that instrument afterwards.
//
// Tickers named: a watchlist ticker is named when one of its own terms appears in a headline as a whole
// word. A term two tickers share ("musk" for Tesla and SpaceX) makes both optional: neither is required
// nor counted wrong.
//
// Direction: the instrument's move from the last price before the first headline to the same time one
// and three trading days later, minus the S&P 500's move over the same span for stocks. Up or down past
// MOVE_THRESHOLD_PCT; flat inside it. This is what happened next, not what the news caused, so even a
// perfect reader would miss some.
const (
	DB_DRIVER          = "sqlite"
	DB_READ_ONLY       = "?mode=ro&immutable=1"
	KIND_STORY         = "story"
	KIND_SPLICE        = "splice"
	KIND_BUNDLE        = "bundle"
	LABEL_FLAT         = "flat"
	MOVE_THRESHOLD_PCT = 1.0
	MIN_STORY_LINES    = 3  // distinct headlines a story needs to be worth reading
	MAX_ITEM_LINES     = 8  // headlines shown per item
	SPLICE_LINES       = 3  // headlines taken from each of the two stories in a splice
	MAX_STORIES        = 40 // story candidates, loudest first
	MAX_SPLICES        = 20
	MAX_BUNDLES        = 60
	MAX_PER_TICKER     = 5 // bundles per ticker, so one loud name does not fill the set
	MIN_BUNDLE_LINES   = 2
	BUNDLE_LOOKBACK    = 30 * 24 * time.Hour
	HISTORY_BEFORE     = 5 * 24 * time.Hour
	HISTORY_AFTER      = 8 * 24 * time.Hour
	SHORT_DAYS         = 1
	LONG_DAYS          = 3
	BENCHMARK          = "SPY"
	CRYPTO_SUFFIX      = "-USD"
	SEED_A, SEED_B     = 24, 11 // the shuffle is fixed so a rebuild gives the same set
	HISTORY_PAUSE      = 400 * time.Millisecond
	MIN_WORD_CHARS     = 5 // title words long enough to count as shared between two stories
	DAY                = 24 * time.Hour
	DIRECT             = "direct"
	UP, DOWN, UNCLEAR  = "up", "down", "unclear"
	SHARED_PREFIX      = "~"
)

// TICKER_TAGS are the watchlist tags that name one company or coin; the rest are themes (AI, OIL, FED).
var (
	TICKER_TAGS = []string{"BTC", "ETH", "SOL", "XRP", "DOGE", "COIN", "MSTR", "TSLA", "SPCX", "AAPL", "NVDA", "MSFT",
		"GOOGL", "AMZN", "META", "MRNA", "CRWD", "AMD", "INTC", "TSM", "AVGO", "NFLX", "DIS", "PLTR", "UBER", "TTWO", "BA"}
	CRYPTO_TAGS = []string{"BTC", "ETH", "SOL", "XRP", "DOGE"}
	WORD        = regexp.MustCompile(`[\pL\pN]+`)
)

// Audit is what a read-through of newsd's stories found: plainly several events, or too mixed to label.
type Audit struct {
	Several []int64 `json:"several"`
	Drop    []int64 `json:"drop"`
}

// Item is one thing a model reads, with the labels it can be scored on. An empty label is not scored.
type Item struct {
	ID         string    `json:"id"`
	Kind       string    `json:"kind"`
	Headlines  []string  `json:"headlines"`
	Instrument string    `json:"instrument"`
	At         time.Time `json:"at"`
	Sources    int       `json:"sources"` // distinct outlets: the importance proxy
	Events     string    `json:"events,omitempty"`
	Tickers    []string  `json:"tickers"`
	Optional   []string  `json:"optional,omitempty"`
	ShortPct   *float64  `json:"short_pct,omitempty"` // market-adjusted move after one trading day
	LongPct    *float64  `json:"long_pct,omitempty"`  // and after three
	Direction  string    `json:"direction,omitempty"` // up, down or flat, from ShortPct
	Baseline   string    `json:"baseline"`            // newsd's bullish and bearish word lists
	StoryIDs   []int64   `json:"story_ids,omitempty"`
	Audited    bool      `json:"audited,omitempty"` // the events label came from the read-through
	Category   string    `json:"category,omitempty"`
}

// Watch is the watchlist as the bake-off uses it: each ticker's terms, and the terms two tickers share.
type Watch struct {
	Terms  map[string][]*regexp.Regexp
	Shared map[string]bool
	Owners map[string][]string // term -> the tickers that list it
	Names  map[string]string   // ticker -> its first term, shown to the model
}

func loadWatch(path string) (Watch, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Watch{}, err
	}
	var doc struct {
		Watch []struct {
			Tag   string   `json:"tag"`
			Terms []string `json:"terms"`
		} `json:"watch"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return Watch{}, err
	}
	w := Watch{Terms: map[string][]*regexp.Regexp{}, Shared: map[string]bool{}, Owners: map[string][]string{}, Names: map[string]string{}}
	for _, e := range doc.Watch {
		if !slices.Contains(TICKER_TAGS, e.Tag) {
			continue
		}
		w.Names[e.Tag] = e.Terms[0]
		for _, term := range e.Terms {
			w.Owners[strings.ToLower(term)] = append(w.Owners[strings.ToLower(term)], e.Tag)
		}
	}
	for _, e := range doc.Watch {
		if !slices.Contains(TICKER_TAGS, e.Tag) {
			continue
		}
		for _, term := range e.Terms {
			term = strings.ToLower(term)
			if len(w.Owners[term]) > 1 {
				w.Shared[term] = true
				continue
			}
			w.Terms[e.Tag] = append(w.Terms[e.Tag], wholeWord(term))
		}
	}
	for term := range w.Shared {
		w.Terms[SHARED_PREFIX+term] = []*regexp.Regexp{wholeWord(term)}
	}
	return w, nil
}

func wholeWord(term string) *regexp.Regexp {
	return regexp.MustCompile(`(?i)(?:^|[^\pL\pN])` + regexp.QuoteMeta(term) + `(?:$|[^\pL\pN])`)
}

// named labels the tickers the headlines name, and the optional ones only a shared term points at.
func (w Watch) named(headlines []string) ([]string, []string) {
	text := strings.Join(headlines, " \n ")
	required, optional := []string{}, []string{}
	for tag, patterns := range w.Terms {
		if strings.HasPrefix(tag, SHARED_PREFIX) {
			continue
		}
		for _, p := range patterns {
			if p.MatchString(text) {
				required = append(required, tag)
				break
			}
		}
	}
	for term := range w.Shared {
		if !w.Terms[SHARED_PREFIX+term][0].MatchString(text) {
			continue
		}
		for _, tag := range w.Owners[term] {
			if !slices.Contains(required, tag) && !slices.Contains(optional, tag) {
				optional = append(optional, tag)
			}
		}
	}
	sort.Strings(required)
	sort.Strings(optional)
	return required, optional
}

// Lexicon is newsd's bullish and bearish word lists, for the rules baseline.
type Lexicon struct{ Bull, Bear []*regexp.Regexp }

func loadLexicon(path string) (Lexicon, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Lexicon{}, err
	}
	var doc struct {
		Bullish []string `json:"bullish"`
		Bearish []string `json:"bearish"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return Lexicon{}, err
	}
	var lx Lexicon
	for _, t := range doc.Bullish {
		lx.Bull = append(lx.Bull, wholeWord(t))
	}
	for _, t := range doc.Bearish {
		lx.Bear = append(lx.Bear, wholeWord(t))
	}
	return lx, nil
}

// lean counts headlines with a bullish word against those with a bearish one.
func (lx Lexicon) lean(headlines []string) string {
	bull, bear := 0, 0
	for _, h := range headlines {
		if slices.ContainsFunc(lx.Bull, func(p *regexp.Regexp) bool { return p.MatchString(h) }) {
			bull++
		}
		if slices.ContainsFunc(lx.Bear, func(p *regexp.Regexp) bool { return p.MatchString(h) }) {
			bear++
		}
	}
	switch {
	case bull > bear:
		return UP
	case bear > bull:
		return DOWN
	}
	return UNCLEAR
}

// distinct keeps the first of each headline, compared without case or punctuation.
func distinct(lines []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, l := range lines {
		l = strings.TrimSpace(l)
		key := strings.ToLower(strings.Join(WORD.FindAllString(l, -1), " "))
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, l)
	}
	return out
}

func titleWords(lines []string) map[string]bool {
	out := map[string]bool{}
	for _, l := range lines {
		for _, w := range WORD.FindAllString(strings.ToLower(l), -1) {
			if len(w) >= MIN_WORD_CHARS {
				out[w] = true
			}
		}
	}
	return out
}

type storyRow struct {
	id        int64
	at        time.Time
	category  string
	outlets   int
	tags      []string
	links     []struct{ Symbol, Relation string }
	headlines []string
}

// loadStories reads the loudest stories with enough distinct headlines.
func loadStories(ctx context.Context, db *sql.DB) ([]storyRow, error) {
	rows, err := db.QueryContext(ctx, `SELECT id, first_seen, category, outlets, detail FROM stories
		WHERE articles >= ? ORDER BY attention DESC, outlets DESC LIMIT 400`, MIN_STORY_LINES)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []storyRow{}
	for rows.Next() {
		var s storyRow
		var first int64
		var detail string
		if err := rows.Scan(&s.id, &first, &s.category, &s.outlets, &detail); err != nil {
			return nil, err
		}
		s.at = time.UnixMilli(first).UTC()
		var d struct {
			Tags  []string                            `json:"tags"`
			Links []struct{ Symbol, Relation string } `json:"links"`
		}
		json.Unmarshal([]byte(detail), &d)
		s.tags, s.links = d.Tags, d.Links
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		lines, err := storyHeadlines(ctx, db, out[i].id)
		if err != nil {
			return nil, err
		}
		out[i].headlines = lines
	}
	return out, nil
}

func storyHeadlines(ctx context.Context, db *sql.DB, id int64) ([]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT e.title FROM story_events se JOIN events e ON e.id = se.event_id
		WHERE se.story_id = ? ORDER BY e.id`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	lines := []string{}
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, err
		}
		lines = append(lines, t)
	}
	return distinct(lines), rows.Err()
}

type eventRow struct {
	at     time.Time
	title  string
	source string
}

// loadEvents reads every headline published since the lookback. Its moment is when it was published:
// newsd backfills feeds when it starts, so when Money 24 first saw an article can be days after it ran,
// and the price move worth measuring starts at publication. A publication time later than the moment
// it was seen is a feed's mistake, and the seen time is used instead.
func loadEvents(ctx context.Context, db *sql.DB, since time.Time) ([]eventRow, error) {
	rows, err := db.QueryContext(ctx, `SELECT seen_at, published_at, title, source FROM events
		WHERE COALESCE(published_at, seen_at) >= ? ORDER BY COALESCE(published_at, seen_at)`, since.UnixMilli())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []eventRow{}
	for rows.Next() {
		var seen int64
		var published sql.NullInt64
		var e eventRow
		if err := rows.Scan(&seen, &published, &e.title, &e.source); err != nil {
			return nil, err
		}
		e.at = time.UnixMilli(seen).UTC()
		if published.Valid && published.Int64 <= seen {
			e.at = time.UnixMilli(published.Int64).UTC()
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func pickInstrument(s storyRow) string {
	for _, l := range s.links {
		if l.Relation == DIRECT {
			return l.Symbol
		}
	}
	if len(s.links) > 0 {
		return s.links[0].Symbol
	}
	return ""
}

// buildSet makes the labeled set. dropped lists the stories a read-through found to be several events.
func buildSet(ctx context.Context, dbPath string, w Watch, lx Lexicon, audit Audit, prices *market.Client) ([]Item, error) {
	db, err := sql.Open(DB_DRIVER, "file:"+dbPath+DB_READ_ONLY)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	stories, err := loadStories(ctx, db)
	if err != nil {
		return nil, fmt.Errorf("stories: %w", err)
	}
	dropped := map[int64]bool{}
	for _, id := range append(slices.Clone(audit.Drop), audit.Several...) {
		dropped[id] = true
	}
	items := []Item{}
	usable := []storyRow{}
	// The first MAX_STORIES candidates are the ones the read-through covered; later ones only feed splices.
	for _, s := range stories {
		if len(s.headlines) < MIN_STORY_LINES {
			continue
		}
		usable = append(usable, s)
		several := slices.Contains(audit.Several, s.id)
		if slices.Contains(audit.Drop, s.id) || len(usable) > MAX_STORIES {
			continue
		}
		lines := s.headlines[:min(len(s.headlines), MAX_ITEM_LINES)]
		req, opt := w.named(lines)
		events := localai.EVENTS_ONE
		if several {
			events = localai.EVENTS_SEVERAL
		}
		items = append(items, Item{ID: fmt.Sprintf("story-%d", s.id), Kind: KIND_STORY, Headlines: lines, Instrument: pickInstrument(s),
			At: s.at, Sources: s.outlets, Events: events, Tickers: req, Optional: opt, Baseline: lx.lean(lines),
			StoryIDs: []int64{s.id}, Category: s.category, Audited: several})
	}
	items = append(items, splices(usable, dropped, w, lx)...)
	events, err := loadEvents(ctx, db, time.Now().Add(-BUNDLE_LOOKBACK))
	if err != nil {
		return nil, fmt.Errorf("events: %w", err)
	}
	items = append(items, bundles(events, w, lx)...)
	labelMoves(ctx, prices, items)
	return items, nil
}

// splices pairs stories from different categories that share no tag and no long title word.
func splices(stories []storyRow, dropped map[int64]bool, w Watch, lx Lexicon) []Item {
	r := rand.New(rand.NewPCG(SEED_A, SEED_B))
	out := []Item{}
	used := map[int64]bool{}
	for i := 0; i < len(stories) && len(out) < MAX_SPLICES; i++ {
		a := stories[i]
		if used[a.id] || dropped[a.id] {
			continue
		}
		for j := len(stories) - 1; j > i; j-- {
			b := stories[j]
			if used[b.id] || dropped[b.id] || a.category == b.category || overlaps(a.tags, b.tags) || sharesWord(a.headlines, b.headlines) {
				continue
			}
			lines := append(slices.Clone(a.headlines[:SPLICE_LINES]), b.headlines[:SPLICE_LINES]...)
			r.Shuffle(len(lines), func(x, y int) { lines[x], lines[y] = lines[y], lines[x] })
			req, opt := w.named(lines)
			out = append(out, Item{ID: fmt.Sprintf("splice-%d-%d", a.id, b.id), Kind: KIND_SPLICE, Headlines: lines,
				Instrument: cmp.Or(pickInstrument(a), pickInstrument(b)), At: a.at, Sources: a.outlets + b.outlets,
				Events: localai.EVENTS_SEVERAL, Tickers: req, Optional: opt, Baseline: lx.lean(lines), StoryIDs: []int64{a.id, b.id}})
			used[a.id], used[b.id] = true, true
			break
		}
	}
	return out
}

func overlaps(a, b []string) bool {
	return slices.ContainsFunc(a, func(t string) bool { return slices.Contains(b, t) })
}

func sharesWord(a, b []string) bool {
	wb := titleWords(b)
	for word := range titleWords(a) {
		if wb[word] {
			return true
		}
	}
	return false
}

// bundles groups the headlines that name exactly one watchlist ticker by ticker and New York day.
func bundles(events []eventRow, w Watch, lx Lexicon) []Item {
	type group struct {
		at      time.Time
		lines   []string
		sources map[string]bool
	}
	groups := map[string]*group{}
	keys := []string{}
	for _, e := range events {
		req, _ := w.named([]string{e.title})
		if len(req) != 1 {
			continue
		}
		key := req[0] + " " + e.at.In(calendar.NEW_YORK).Format(time.DateOnly)
		g := groups[key]
		if g == nil {
			g = &group{at: e.at, sources: map[string]bool{}}
			groups[key] = g
			keys = append(keys, key)
		}
		g.lines = append(g.lines, e.title)
		g.sources[e.source] = true
	}
	// Busiest days first, so the set is the news that most plausibly moved something.
	sort.SliceStable(keys, func(i, j int) bool { return len(groups[keys[i]].sources) > len(groups[keys[j]].sources) })
	out := []Item{}
	perTicker := map[string]int{}
	for _, key := range keys {
		g := groups[key]
		lines := distinct(g.lines)
		ticker := strings.Fields(key)[0]
		// A day whose next trading day has not closed yet cannot be scored on direction; the stories cover it.
		unscorable := addTradingDays(g.at, SHORT_DAYS, slices.Contains(CRYPTO_TAGS, ticker)).After(time.Now())
		if unscorable || len(lines) < MIN_BUNDLE_LINES || perTicker[ticker] >= MAX_PER_TICKER || len(out) >= MAX_BUNDLES {
			continue
		}
		perTicker[ticker]++
		lines = lines[:min(len(lines), MAX_ITEM_LINES)]
		req, opt := w.named(lines)
		out = append(out, Item{ID: "bundle-" + strings.ReplaceAll(key, " ", "-"), Kind: KIND_BUNDLE, Headlines: lines,
			Instrument: ticker, At: g.at, Sources: len(g.sources), Tickers: req, Optional: opt, Baseline: lx.lean(lines)})
	}
	return out
}

// yahooSymbol is the symbol Yahoo keeps history under.
func yahooSymbol(instrument string) string {
	if slices.Contains(CRYPTO_TAGS, instrument) {
		return instrument + CRYPTO_SUFFIX
	}
	return instrument
}

// addTradingDays moves a moment by whole trading days: weekdays for stocks, every day for coins.
func addTradingDays(t time.Time, days int, crypto bool) time.Time {
	for days > 0 {
		t = t.Add(DAY)
		if crypto {
			days--
			continue
		}
		if wd := t.In(calendar.NEW_YORK).Weekday(); wd != time.Saturday && wd != time.Sunday {
			days--
		}
	}
	return t
}

// priceAt is the close of the last candle that opened at or before t.
func priceAt(c market.Chart, t time.Time) (float64, bool) {
	price, ok := 0.0, false
	for _, k := range c.Candles {
		if k.T > t.Unix() {
			break
		}
		if k.C > 0 {
			price, ok = k.C, true
		}
	}
	return price, ok
}

func movePct(c market.Chart, from, to time.Time) (float64, bool) {
	p0, ok0 := priceAt(c, from)
	p1, ok1 := priceAt(c, to)
	if !ok0 || !ok1 || to.After(time.Now()) {
		return 0, false
	}
	return (p1/p0 - 1) * 100, true
}

// labelMoves fills each item's realized move, adjusted for the market for stocks.
func labelMoves(ctx context.Context, prices *market.Client, items []Item) {
	charts := map[string]market.Chart{}
	chart := func(symbol string, at time.Time) (market.Chart, bool) {
		key := symbol + at.Format(time.DateOnly)
		if c, ok := charts[key]; ok {
			return c, len(c.Candles) > 0
		}
		time.Sleep(HISTORY_PAUSE)
		c, err := prices.History(ctx, symbol, at.Add(-HISTORY_BEFORE), at.Add(HISTORY_AFTER))
		if err != nil {
			log.Printf("history %s: %v", symbol, err)
		}
		charts[key] = c
		return c, err == nil && len(c.Candles) > 0
	}
	for i := range items {
		it := &items[i]
		if it.Instrument == "" || strings.ContainsAny(it.Instrument, "=^") {
			continue
		}
		crypto := slices.Contains(CRYPTO_TAGS, it.Instrument)
		c, ok := chart(yahooSymbol(it.Instrument), it.At)
		if !ok {
			continue
		}
		var bench market.Chart
		if !crypto {
			if bench, ok = chart(BENCHMARK, it.At); !ok {
				continue
			}
		}
		for _, h := range []struct {
			days int
			out  **float64
		}{{SHORT_DAYS, &it.ShortPct}, {LONG_DAYS, &it.LongPct}} {
			to := addTradingDays(it.At, h.days, crypto)
			move, ok := movePct(c, it.At, to)
			if !ok {
				continue
			}
			if !crypto {
				benchMove, ok := movePct(bench, it.At, to)
				if !ok {
					continue
				}
				move -= benchMove
			}
			move = math.Round(move*100) / 100
			*h.out = &move
		}
		if it.ShortPct == nil {
			continue
		}
		switch {
		case *it.ShortPct >= MOVE_THRESHOLD_PCT:
			it.Direction = UP
		case *it.ShortPct <= -MOVE_THRESHOLD_PCT:
			it.Direction = DOWN
		default:
			it.Direction = LABEL_FLAT
		}
	}
}
