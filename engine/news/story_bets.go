package news

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	// The regular Polymarket source reads the 200 busiest markets. A loud story about something
	// smaller (a game launch, one company's ruling) is looked up by name instead.
	BET_SEARCH_ID        = "polymarket-search"
	BET_SEARCH_LABEL     = "Polymarket search"
	BET_SEARCH_URL       = "https://gamma-api.polymarket.com/public-search?limit_per_type=8&q="
	BET_SEARCH_KIND      = "market"
	BET_SEARCH_EVERY     = 6 * time.Hour // how often one story is looked up
	BET_SEARCH_ATTENTION = 50.0          // only stories this loud are worth a request
	BET_SEARCHES_A_PASS  = 2
	BET_QUERY_WORDS      = 3
)

// betQuery is the few words most of a story's headlines share: what a market about it would be named after.
func betQuery(c *cluster) string {
	words := c.profile()
	sort.SliceStable(words, func(i, j int) bool { return c.counts[words[i]] > c.counts[words[j]] })
	return strings.Join(words[:min(len(words), BET_QUERY_WORDS)], PHRASE_SEPARATOR)
}

// betSearches picks the loudest tradable stories not looked up lately, with the words to look them up by.
func (b *storyBook) betSearches(now time.Time, searched map[int64]time.Time) map[int64]string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	due := []*cluster{}
	for c := range b.clusters {
		if c.id == 0 || len(c.story.Tags) == 0 || c.story.Attention.Score < BET_SEARCH_ATTENTION || now.Sub(searched[c.id]) < BET_SEARCH_EVERY {
			continue
		}
		due = append(due, c)
	}
	sort.Slice(due, func(i, j int) bool { return due[i].story.Attention.Score > due[j].story.Attention.Score })
	out := map[int64]string{}
	for _, c := range due[:min(len(due), BET_SEARCHES_A_PASS)] {
		out[c.id] = betQuery(c)
	}
	return out
}

// searchBets asks Polymarket for open markets matching a few words.
func searchBets(ctx context.Context, f *Fetcher, tagger *Tagger, query string, now time.Time) ([]Market, error) {
	body, err := f.get(ctx, BET_SEARCH_ID, BET_SEARCH_URL+url.QueryEscape(query))
	if err != nil || body == nil {
		return nil, err
	}
	var doc struct {
		Events []struct {
			Markets []struct {
				polymarketMarket
				Closed bool            `json:"closed"`
				Volume json.RawMessage `json:"volume"`
			} `json:"markets"`
		} `json:"events"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("polymarket search parse: %w", err)
	}
	out := []Market{}
	for _, e := range doc.Events {
		for _, m := range e.Markets {
			id := strings.Trim(string(m.ID), `"`)
			end := parseTime(m.EndDate)
			if id == "" || m.Question == "" || m.Closed || (end != nil && end.Before(now)) {
				continue
			}
			volume, _ := strconv.ParseFloat(strings.Trim(string(m.Volume), `"`), 64)
			prices, _ := json.Marshal(map[string][]string{"outcomes": stringList(m.Outcomes), PRICE_KEY: stringList(m.OutcomePrices)})
			out = append(out, Market{ID: VENUE_POLYMARKET + ":" + id, Venue: VENUE_POLYMARKET, Question: m.Question, Slug: m.Slug,
				EndAt: end, Volume: max(volume, m.VolumeNum), Prices: prices, Tags: tagger.match(m.Question)})
		}
	}
	return out, nil
}

// findBets looks up prediction markets for the loudest stories and stores what is open.
// It reports whether anything was stored, so the stories can be matched again at once.
func (s *Service) findBets(ctx context.Context, now time.Time, searched map[int64]time.Time) bool {
	found := false
	for id, query := range s.stories.betSearches(now, searched) {
		searched[id] = now
		if query == "" {
			continue
		}
		markets, err := searchBets(ctx, s.fetcher, s.tagger, query, now)
		if err != nil {
			if ctx.Err() == nil {
				log.Printf("[stories] market search %q: %v", query, err)
			}
			continue
		}
		if err := s.store.upsertMarkets(ctx, markets); err != nil && ctx.Err() == nil {
			log.Printf("[stories] market store: %v", err)
		}
		found = found || len(markets) > 0
	}
	return found
}
