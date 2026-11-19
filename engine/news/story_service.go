package news

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/YEAHDOGS/money24/engine/market"
)

const (
	STORY_EVERY    = 30 * time.Second
	SNAPSHOT_EVERY = 10 * time.Minute // how often a story's numbers are written to its record
	QUOTES_EVERY   = 5 * time.Minute  // one batch of quotes for every linked instrument
	BETS_EVERY     = 5 * time.Minute
	PRUNE_EVERY    = 6 * time.Hour
	QUOTES_TIMEOUT = 20 * time.Second
	EVENT_STORIES  = "stories"
	PRICE_KEY      = "prices"
)

// firstPrice reads the first outcome's price (Yes, on a yes-or-no market) from a stored prices document.
func firstPrice(doc string) float64 {
	var prices map[string][]string
	if json.Unmarshal([]byte(doc), &prices) != nil || len(prices[PRICE_KEY]) == 0 {
		return 0
	}
	yes, _ := strconv.ParseFloat(prices[PRICE_KEY][0], 64)
	return yes
}

// loadQuotes fetches a quote for every linked instrument, a batch at a time. A failed batch keeps the old quotes.
func (b *storyBook) loadQuotes(ctx context.Context, client *market.Client) map[string]market.Tick {
	ctx, cancel := context.WithTimeout(ctx, QUOTES_TIMEOUT)
	defer cancel()
	out := map[string]market.Tick{}
	for start := 0; start < len(b.symbols); start += market.MAX_QUOTES {
		ticks, err := client.Ticks(ctx, b.symbols[start:min(start+market.MAX_QUOTES, len(b.symbols))])
		if err != nil {
			log.Printf("[stories] quotes: %v", err)
			continue
		}
		for _, t := range ticks {
			out[t.Symbol] = t
		}
	}
	return out
}

// storyLoop groups new articles into stories, re-reads every open story and records its numbers.
func (s *Service) storyLoop(ctx context.Context) {
	b := s.stories
	quotes := market.NewClient()
	var quotesDue, betsDue, pruneDue time.Time
	searched := map[int64]time.Time{} // story id -> when its prediction markets were last looked up
	first := true
	ticker := time.NewTicker(STORY_EVERY)
	defer ticker.Stop()
	for {
		now := time.Now().UTC()
		cutoff := now.Add(-STORY_OPEN)
		events, err := s.store.eventsForStories(ctx, cutoff, b.lastEvent)
		if err != nil && ctx.Err() == nil {
			log.Printf("[stories] read: %v", err)
		}
		var fresh map[string]market.Tick
		if !now.Before(quotesDue) {
			quotesDue = now.Add(QUOTES_EVERY)
			fresh = b.loadQuotes(ctx, quotes)
		}
		var markets []liveMarket
		if !now.Before(betsDue) {
			betsDue = now.Add(BETS_EVERY)
			if markets, err = s.store.liveMarkets(ctx, now, b.reader.stop); err != nil && ctx.Err() == nil {
				log.Printf("[stories] markets: %v", err)
			}
		}

		b.mu.Lock()
		for _, e := range events {
			b.lastEvent = max(b.lastEvent, e.ID)
			if e.StoryID != 0 {
				b.file(e)
			} else {
				b.add(e.StoredEvent)
			}
		}
		if first {
			first = false
			s.restoreSparks(ctx, cutoff)
		}
		b.expire(now)
		if len(fresh) > 0 {
			b.quotes, b.quotesAt = fresh, now
		}
		if markets != nil {
			b.match(markets)
		}
		changed := b.refresh(now)
		if err := s.store.saveStories(ctx, changed); err != nil && ctx.Err() == nil {
			log.Printf("[stories] save: %v", err)
		}
		points := map[int64]StoryPoint{}
		for _, c := range changed {
			b.byID[c.id] = c
		}
		for c := range b.clusters {
			if c.id == 0 || now.Sub(c.snappedAt) < SNAPSHOT_EVERY {
				continue
			}
			c.snappedAt = now
			st := c.story
			p := StoryPoint{At: now, Articles: st.Articles, Outlets: st.Outlets, LastHour: st.Attention.LastHour,
				Attention: st.Attention.Score, Validity: st.Validity.Score, Symbol: st.Reading.Symbol, Reading: st.Reading.Label}
			for _, l := range st.Links {
				if l.Symbol == st.Reading.Symbol {
					p.RelVolume, p.MovePct = l.RelVolume, l.MovePct
				}
			}
			points[c.id] = p
			c.spark = append(c.spark, st.Attention.Score)
			c.spark = c.spark[max(0, len(c.spark)-SPARK_POINTS):]
			c.story.Spark = c.spark
		}
		if len(changed) > 0 || len(points) > 0 {
			b.updatedAt = now
		}
		open := len(b.byID)
		b.mu.Unlock()

		if err := s.store.addStoryPoints(ctx, points); err != nil && ctx.Err() == nil {
			log.Printf("[stories] record: %v", err)
		}
		if !now.Before(pruneDue) {
			pruneDue = now.Add(PRUNE_EVERY)
			if err := s.store.pruneStoryPoints(ctx, now); err != nil && ctx.Err() == nil {
				log.Printf("[stories] prune: %v", err)
			}
		}
		if len(changed) > 0 {
			data, _ := json.Marshal(map[string]any{"at": now, "open": open, "changed": len(changed)})
			s.hub.broadcast(Message{Event: EVENT_STORIES, Data: data})
		}
		if s.findBets(ctx, now, searched) {
			betsDue = now // match the stories against what was just found on the next pass
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// restoreSparks gives each open story back the attention readings recorded before a restart.
func (s *Service) restoreSparks(ctx context.Context, cutoff time.Time) {
	rows, err := s.store.db.QueryContext(ctx, `SELECT story_id, attention FROM story_metrics WHERE at >= ? ORDER BY at`, cutoff.UnixMilli())
	if err != nil {
		log.Printf("[stories] sparks: %v", err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var attention float64
		if rows.Scan(&id, &attention) != nil {
			return
		}
		if c := s.stories.byID[id]; c != nil {
			c.spark = append(c.spark, attention)
			c.spark = c.spark[max(0, len(c.spark)-SPARK_POINTS):]
		}
	}
}

// StoryDetail is one story with its articles and its recorded numbers.
type StoryDetail struct {
	Story   Story         `json:"story"`
	Events  []StoredEvent `json:"events"`
	History []StoryPoint  `json:"history"`
}

func (a *api) storyBoard(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	writeJSON(w, http.StatusOK, a.svc.stories.board(StoryQuery{
		Limit: int(queryInt(r, "limit", DEFAULT_STORIES)), Category: q.Get("category"), Tag: q.Get("tag"),
		Country: q.Get("country"), Search: q.Get("q"), Tradable: q.Get("tradable") == "1",
	}))
}

func (a *api) storyMap(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, a.svc.stories.worldMap())
}

// storyDetail serves an open story from memory and a closed one as it was last saved.
func (a *api) storyDetail(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "story id must be a number"})
		return
	}
	story, ok := a.svc.stories.open(id)
	if !ok {
		story, ok = a.svc.store.storedStory(r.Context(), id)
	}
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no such story"})
		return
	}
	events, err := a.svc.store.storyArticles(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	history, err := a.svc.store.storyHistory(r.Context(), id, MAX_HISTORY)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, StoryDetail{Story: story, Events: events, History: history})
}
