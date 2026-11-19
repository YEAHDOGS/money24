package news

import (
	"encoding/json"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/YEAHDOGS/money24/engine/market"
)

const (
	POLYMARKET_PAGE = "https://polymarket.com/market/"
	DEFAULT_STORIES = 60
	MAX_STORIES     = 200
	STORY_MIN_SIZE  = 2    // one article is not a story yet
	BET_MIN_SHARED  = 2    // words a prediction market's question shares with a story
	BET_FIT         = 0.5  // and the share of the question those words make up
	MOVER_VOLUME    = 2.0  // times its usual pace, to count as volume without a story
	MOVER_QUIET     = 20.0 // a story quieter than this does not explain a busy instrument
	MAX_MOVERS      = 12
	MAP_TOP_STORIES = 6
)

// storyBook holds every open group of articles in memory. The database keeps what must outlive a restart.
type storyBook struct {
	mu        sync.RWMutex
	reader    *storyReader
	zone      *time.Location
	clusters  map[*cluster]struct{}
	index     map[string]map[*cluster]struct{} // word -> groups whose headlines carry it
	byID      map[int64]*cluster
	lastEvent int64
	quotes    map[string]market.Tick
	quotesAt  time.Time
	updatedAt time.Time
	symbols   []string        // every instrument in links.json
	direct    map[string]bool // those some tag names directly
}

func newStoryBook(sources []Source) (*storyBook, error) {
	reader, err := loadStoryReader(sources)
	if err != nil {
		return nil, err
	}
	zone, err := time.LoadLocation(MARKET_ZONE)
	if err != nil {
		return nil, err
	}
	b := &storyBook{
		reader: reader, zone: zone, clusters: map[*cluster]struct{}{}, index: map[string]map[*cluster]struct{}{},
		byID: map[int64]*cluster{}, quotes: map[string]market.Tick{}, direct: map[string]bool{},
	}
	seen := map[string]bool{}
	for _, links := range reader.links {
		for _, l := range links {
			b.direct[l.Symbol] = b.direct[l.Symbol] || l.Relation == RELATION_SELF
			if !seen[l.Symbol] {
				seen[l.Symbol] = true
				b.symbols = append(b.symbols, l.Symbol)
			}
		}
	}
	sort.Strings(b.symbols)
	return b, nil
}

// join puts an article into a group and indexes its words.
func (b *storyBook) join(c *cluster, m *member) {
	c.members = append(c.members, m)
	c.dirty = true
	for _, t := range m.tokens {
		c.counts[t]++
		if b.index[t] == nil {
			b.index[t] = map[*cluster]struct{}{}
		}
		b.index[t][c] = struct{}{}
	}
}

// file puts a stored article back under the story the database says it belongs to.
func (b *storyBook) file(e storyEvent) {
	c := b.byID[e.StoryID]
	if c == nil {
		c = &cluster{id: e.StoryID, counts: map[string]int{}}
		b.byID[e.StoryID] = c
		b.clusters[c] = struct{}{}
	}
	b.join(c, b.reader.member(e.StoredEvent))
	c.dirty, c.filed = false, len(c.members)
}

// add files a new article under the open group it fits best, or leaves it to wait alone.
func (b *storyBook) add(e StoredEvent) {
	m := b.reader.member(e)
	if len(m.tokens) == 0 {
		return
	}
	candidates := map[*cluster]int{}
	for _, t := range m.tokens {
		if post := b.index[t]; len(post) <= MAX_POSTING {
			for c := range post {
				candidates[c]++
			}
		}
	}
	var best *cluster
	bestFit := 0.0
	for c, n := range candidates {
		if n < MIN_CANDIDATE {
			continue
		}
		if fit, ok := c.fits(m); ok && fit > bestFit {
			best, bestFit = c, fit
		}
	}
	if best == nil {
		best = &cluster{counts: map[string]int{}}
		b.clusters[best] = struct{}{}
	}
	// The same headline from the same outlet, seen again through another feed, is one article.
	for _, other := range best.members {
		if other.key == m.key && other.Source == m.Source {
			return
		}
	}
	b.join(best, m)
}

// expire closes stories that have gone quiet and drops lone articles that never found company.
func (b *storyBook) expire(now time.Time) {
	for c := range b.clusters {
		age := now.Sub(c.members[len(c.members)-1].SeenAt)
		if (len(c.members) >= STORY_MIN_SIZE && age <= STORY_OPEN) || (len(c.members) < STORY_MIN_SIZE && age <= SEED_TTL) {
			continue
		}
		for t := range c.counts {
			delete(b.index[t], c)
			if len(b.index[t]) == 0 {
				delete(b.index, t)
			}
		}
		delete(b.clusters, c)
		delete(b.byID, c.id)
	}
}

// match gives each story the prediction markets whose question it answers to.
func (b *storyBook) match(markets []liveMarket) {
	for c := range b.clusters {
		if len(c.members) < STORY_MIN_SIZE {
			continue
		}
		profile := c.profile()
		type scored struct {
			bet Bet
			n   int
		}
		found := []scored{}
		for _, m := range markets {
			n := shared(profile, m.tokens)
			if n >= BET_MIN_SHARED && float64(n) >= BET_FIT*float64(len(m.tokens)) {
				found = append(found, scored{m.Bet, n})
			}
		}
		sort.Slice(found, func(i, j int) bool {
			return found[i].n > found[j].n || (found[i].n == found[j].n && found[i].bet.Volume > found[j].bet.Volume)
		})
		c.bets = []Bet{}
		for _, f := range found[:min(len(found), MAX_BETS)] {
			c.bets = append(c.bets, f.bet)
		}
	}
}

// refresh re-reads every story as of now and returns those whose reading changed.
func (b *storyBook) refresh(now time.Time) []*cluster {
	changed := []*cluster{}
	for c := range b.clusters {
		if len(c.members) < STORY_MIN_SIZE {
			continue
		}
		next := b.reader.read(c, now, b.quotes, b.zone)
		before, _ := json.Marshal(c.story)
		after, _ := json.Marshal(next)
		if !c.dirty && string(before) == string(after) {
			continue
		}
		c.story, c.dirty = next, false
		changed = append(changed, c)
	}
	// Oldest first, so story ids follow the order the stories broke in.
	sort.Slice(changed, func(i, j int) bool { return changed[i].members[0].ID < changed[j].members[0].ID })
	return changed
}

// StoryQuery filters the board. Zero values mean no filter.
type StoryQuery struct {
	Limit    int
	Category string
	Tag      string
	Country  string
	Search   string
	Tradable bool // only stories tied to an instrument on the watchlist
}

// Mover is an instrument trading far above its pace with no story on the board to explain it.
type Mover struct {
	Symbol    string  `json:"symbol"`
	MovePct   float64 `json:"move_pct"`
	RelVolume float64 `json:"rel_volume"`
}

// CategoryCount is how many open stories a category holds.
type CategoryCount struct {
	ID      string `json:"id"`
	Stories int    `json:"stories"`
}

// Board is the stories page: the loudest stories first, the categories and the unexplained movers.
type Board struct {
	UpdatedAt  time.Time       `json:"updated_at"`
	QuotesAt   *time.Time      `json:"quotes_at"`
	Categories []CategoryCount `json:"categories"`
	Stories    []Story         `json:"stories"`
	Movers     []Mover         `json:"movers"`
}

func hasPlace(places []Place, id string) bool {
	for _, p := range places {
		if p.ID == id {
			return true
		}
	}
	return false
}

func (q StoryQuery) allows(s Story) bool {
	if q.Category != "" && s.Category != q.Category {
		return false
	}
	if q.Tag != "" && !sharesTag(s.Tags, []string{q.Tag}) {
		return false
	}
	if q.Country != "" && !hasPlace(s.From, q.Country) && !hasPlace(s.About, q.Country) {
		return false
	}
	return q.Search == "" || strings.Contains(strings.ToLower(s.Title), q.Search)
}

// stories returns every open story, loudest first.
func (b *storyBook) stories() []Story {
	out := []Story{}
	for c := range b.clusters {
		if c.id != 0 {
			out = append(out, c.story)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Attention.Score != out[j].Attention.Score {
			return out[i].Attention.Score > out[j].Attention.Score
		}
		return out[i].ID > out[j].ID
	})
	return out
}

// board answers one request for the stories page.
func (b *storyBook) board(q StoryQuery) Board {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if q.Limit <= 0 || q.Limit > MAX_STORIES {
		q.Limit = DEFAULT_STORIES
	}
	q.Search = strings.ToLower(strings.TrimSpace(q.Search))
	board := Board{UpdatedAt: b.updatedAt, Categories: []CategoryCount{}, Stories: []Story{}, Movers: []Mover{}}
	if !b.quotesAt.IsZero() {
		at := b.quotesAt
		board.QuotesAt = &at
	}
	counts := map[string]int{}
	explained := map[string]bool{}
	for _, s := range b.stories() {
		for _, l := range s.Links {
			explained[l.Symbol] = explained[l.Symbol] || (l.Relation == RELATION_SELF && s.Attention.Score >= MOVER_QUIET)
		}
		if q.Tradable && len(s.Links) == 0 {
			continue
		}
		counts[s.Category]++
		if len(board.Stories) < q.Limit && q.allows(s) {
			board.Stories = append(board.Stories, s)
		}
	}
	for id, n := range counts {
		board.Categories = append(board.Categories, CategoryCount{ID: id, Stories: n})
	}
	sort.Slice(board.Categories, func(i, j int) bool {
		a, c := board.Categories[i], board.Categories[j]
		return a.Stories > c.Stories || (a.Stories == c.Stories && a.ID < c.ID)
	})
	now := time.Now()
	for symbol, quote := range b.quotes {
		if rel := relVolume(quote, now, b.zone); b.direct[symbol] && !explained[symbol] && rel >= MOVER_VOLUME {
			board.Movers = append(board.Movers, Mover{Symbol: symbol, MovePct: roundTo(quote.ChangePct, ROUND_RATIO), RelVolume: rel})
		}
	}
	sort.Slice(board.Movers, func(i, j int) bool { return board.Movers[i].RelVolume > board.Movers[j].RelVolume })
	board.Movers = board.Movers[:min(len(board.Movers), MAX_MOVERS)]
	return board
}

// MapStory is a story as the map lists it under a country.
type MapStory struct {
	ID        int64   `json:"id"`
	Title     string  `json:"title"`
	Category  string  `json:"category"`
	Angle     string  `json:"angle"`
	Attention float64 `json:"attention"`
}

// MapPlace is one country's coverage: how many stories and articles, and how loud they are together.
type MapPlace struct {
	ID        string     `json:"id"`
	Stories   int        `json:"stories"`
	Articles  int        `json:"articles"`
	Attention float64    `json:"attention"` // each story's attention, by the share of its articles counted here
	Top       []MapStory `json:"top"`
}

// WorldMap is coverage per country, counted two ways.
type WorldMap struct {
	UpdatedAt time.Time  `json:"updated_at"`
	From      []MapPlace `json:"from"`  // where the outlets covering each story publish from
	About     []MapPlace `json:"about"` // the places the headlines name
}

func mapPlaces(stories []Story, pick func(Story) []Place) []MapPlace {
	byID := map[string]*MapPlace{}
	for _, s := range stories { // loudest first, so each country's top list is already in order
		for _, p := range pick(s) {
			place := byID[p.ID]
			if place == nil {
				place = &MapPlace{ID: p.ID, Top: []MapStory{}}
				byID[p.ID] = place
			}
			place.Stories++
			place.Articles += p.N
			place.Attention += s.Attention.Score * float64(p.N) / float64(s.Articles)
			if len(place.Top) < MAP_TOP_STORIES {
				place.Top = append(place.Top, MapStory{ID: s.ID, Title: s.Title, Category: s.Category, Angle: s.Angle.Label, Attention: s.Attention.Score})
			}
		}
	}
	out := []MapPlace{}
	for _, p := range byID {
		p.Attention = roundTo(p.Attention, ROUND_SCORE)
		out = append(out, *p)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Attention > out[j].Attention || (out[i].Attention == out[j].Attention && out[i].ID < out[j].ID)
	})
	return out
}

// worldMap answers one request for the map page.
func (b *storyBook) worldMap() WorldMap {
	b.mu.RLock()
	defer b.mu.RUnlock()
	stories := b.stories()
	return WorldMap{
		UpdatedAt: b.updatedAt,
		From:      mapPlaces(stories, func(s Story) []Place { return s.From }),
		About:     mapPlaces(stories, func(s Story) []Place { return s.About }),
	}
}

// open returns one open story.
func (b *storyBook) open(id int64) (Story, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	c := b.byID[id]
	if c == nil {
		return Story{}, false
	}
	return c.story, true
}
