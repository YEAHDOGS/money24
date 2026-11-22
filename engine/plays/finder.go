package plays

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/YEAHDOGS/money24/engine/calendar"
	"github.com/YEAHDOGS/money24/engine/market"
)

const (
	// The finder is the part of Money 24 that works with nobody at the keyboard: it reads the stories
	// newsd grouped, and where a loud, well-sourced story leans one way on a stock, it drafts the
	// option play and trades it. It never trades through a blocking rule.
	FINDER_EVERY    = 5 * time.Minute
	FINDER_TIMEOUT  = 45 * time.Second
	FINDER_STORIES  = "/api/stories?tradable=1&limit=40"
	FINDER_MAX_BODY = 4 << 20
	FOUND_BY_FINDER = "finder"
	FINDER_CATEGORY = "Found from news"
	FINDER_SOURCE   = "Money 24 stories"
	// What a pass was doing, and what it decided about a story.
	FINDER_LOOKING  = "looking"
	FINDER_OFF      = "off"
	FINDER_CLOSED   = "closed"
	FINDER_FULL     = "full"
	FINDER_PAUSED   = "paused"
	FINDER_ERROR    = "error"
	FINDER_OPENED   = "opened"
	FINDER_DRAFTED  = "drafted"
	FINDER_SKIPPED  = "skipped"
	ANGLE_BULLISH   = "bullish"
	ANGLE_BEARISH   = "bearish"
	RELATION_DIRECT = "direct"
	DAY_SECONDS     = 24 * 60 * 60
	MOVE_NOISE_PCT  = 0.5   // a move smaller than this says nothing about direction
	NOT_A_STOCK     = "=-^" // futures, coins and indexes carry one of these in their symbol: no listed options here
)

// FinderRules is the finder's part of the rulebook.
type FinderRules struct {
	On           bool     `json:"on"`
	Readings     []string `json:"readings"`      // the coverage-against-volume readings that may open a play
	MinAttention float64  `json:"min_attention"` // 0 to 100
	MinValidity  float64  `json:"min_validity"`  // 0 to 100
	MaxPerDay    int      `json:"max_per_day"`
	MinDays      int      `json:"min_days"`      // the nearest expiry at least this many days out
	OTMPct       float64  `json:"otm_pct"`       // how far past the stock price the strike is picked
	OpenWaitMin  int      `json:"open_wait_min"` // minutes after the open before it looks
	CloseWaitMin int      `json:"close_wait_min"`
	// A story's angle is a count of loaded headline words, and words get it wrong. So the finder asks for
	// more than a label: enough headlines on that side, and a price that is not moving the other way.
	MinSided      int  `json:"min_sided"`       // headlines that must take the play's side
	MoveMustAgree bool `json:"move_must_agree"` // skip when the instrument is moving against the angle
	// MaxDayLossUSD pauses the finder for the day once the plays closed today have lost this much. 0 never pauses.
	MaxDayLossUSD float64 `json:"max_day_loss_usd"`
}

// DefaultFinder is how the finder runs until the rulebook says otherwise.
func DefaultFinder() *FinderRules {
	return &FinderRules{On: true, Readings: []string{"early", "confirmed"}, MinAttention: 50, MinValidity: 60,
		MaxPerDay: 6, MinDays: 7, OTMPct: 2, OpenWaitMin: 15, CloseWaitMin: 15, MinSided: 3, MoveMustAgree: true, MaxDayLossUSD: 150}
}

// foundStory is the part of a newsd story the finder reads.
type foundStory struct {
	ID      int64    `json:"id"`
	Title   string   `json:"title"`
	Tags    []string `json:"tags"`
	Outlets int      `json:"outlets"`
	Angle   struct {
		Bullish int `json:"bullish"`
		Bearish int `json:"bearish"`
	} `json:"angle"`
	Attention struct {
		Score float64 `json:"score"`
	} `json:"attention"`
	Validity struct {
		Score       float64 `json:"score"`
		Independent int     `json:"independent"`
	} `json:"validity"`
	Links []struct {
		Symbol    string  `json:"symbol"`
		Relation  string  `json:"relation"`
		Angle     string  `json:"angle"`
		RelVolume float64 `json:"rel_volume"`
		MovePct   float64 `json:"move_pct"`
	} `json:"links"`
	Reading struct {
		Label  string `json:"label"`
		Symbol string `json:"symbol"`
	} `json:"reading"`
}

// sessionOpen says whether the US stock session is open with the finder's waits on both ends. The
// trading calendar knows the holidays and the 1 PM half days, so the finder never looks on either.
func (f FinderRules) sessionOpen(now time.Time) bool {
	day := calendar.On(now)
	if day.Session == calendar.SESSION_CLOSED {
		return false
	}
	from := day.Opens.Add(time.Duration(f.OpenWaitMin) * time.Minute)
	until := day.Closes.Add(-time.Duration(f.CloseWaitMin) * time.Minute)
	return !now.Before(from) && now.Before(until)
}

// wants says whether a story is worth a play, and on which instrument and side. When it is not,
// why says so in words, for the finder's log.
func (f FinderRules) wants(s foundStory) (symbol, right, why string) {
	reading := false
	for _, r := range f.Readings {
		reading = reading || r == s.Reading.Label
	}
	switch {
	case s.Validity.Score < f.MinValidity:
		return "", "", fmt.Sprintf("validity %.0f is under %.0f", s.Validity.Score, f.MinValidity)
	case !reading:
		return "", "", fmt.Sprintf("coverage against volume reads %s", s.Reading.Label)
	case strings.ContainsAny(s.Reading.Symbol, NOT_A_STOCK):
		return "", "", s.Reading.Symbol + " has no listed options here"
	}
	for _, l := range s.Links {
		if l.Symbol != s.Reading.Symbol || l.Relation != RELATION_DIRECT {
			continue
		}
		sided, side := s.Angle.Bullish, RIGHT_CALL
		if l.Angle == ANGLE_BEARISH {
			sided, side = s.Angle.Bearish, RIGHT_PUT
		}
		against := (side == RIGHT_CALL && l.MovePct < 0) || (side == RIGHT_PUT && l.MovePct > 0)
		switch {
		case l.Angle != ANGLE_BULLISH && l.Angle != ANGLE_BEARISH:
			return "", "", "its angle is " + l.Angle
		case sided < f.MinSided:
			return "", "", fmt.Sprintf("%d headlines take its side, needs %d", sided, f.MinSided)
		case f.MoveMustAgree && against && math.Abs(l.MovePct) >= MOVE_NOISE_PCT:
			return "", "", fmt.Sprintf("%s is moving %+.1f%%, against a %s story", l.Symbol, l.MovePct, l.Angle)
		}
		return l.Symbol, side, ""
	}
	return "", "", "no directly linked instrument has a quote"
}

// FinderNote is what the finder decided about one story on its last pass.
type FinderNote struct {
	StoryID int64  `json:"story_id"`
	Title   string `json:"title"`
	Symbol  string `json:"symbol,omitempty"`
	Outcome string `json:"outcome"` // opened, drafted or skipped
	Why     string `json:"why,omitempty"`
	PlayID  string `json:"play_id,omitempty"`
}

// FinderStatus is the finder's last pass, for the Rules tab: nothing it does is hidden.
type FinderStatus struct {
	BytesOnDisk int64        `json:"bytes_on_disk"`
	At          time.Time    `json:"at"`
	State       string       `json:"state"` // looking, off, closed (the session), full (the day's count), paused (the day's loss) or error
	Why         string       `json:"why,omitempty"`
	Today       int          `json:"today"`   // plays it opened today
	DayPnL      float64      `json:"day_pnl"` // every play closed today, in dollars
	Notes       []FinderNote `json:"notes"`   // the stories loud enough to be looked at, loudest first
	NextAt      time.Time    `json:"next_at"`
	EveryMs     int64        `json:"every_ms"`
}

// finderLog holds the last pass for the API.
type finderLog struct {
	mu   sync.Mutex
	last FinderStatus
}

func (l *finderLog) set(st FinderStatus) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.last = st
}

// FinderStatus returns the finder's last pass.
func (s *Service) FinderStatus() FinderStatus {
	s.finder.mu.Lock()
	status := s.finder.last
	s.finder.mu.Unlock()
	status.BytesOnDisk = s.Store.bytesOnDisk()
	return status
}

// stories reads the tradable stories from newsd, loudest first.
func (s *Service) stories(ctx context.Context) ([]foundStory, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(s.NewsURL, "/")+FINDER_STORIES, nil)
	if err != nil {
		return nil, err
	}
	res, err := s.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("newsd stories: HTTP %d", res.StatusCode)
	}
	var board struct {
		Stories []foundStory `json:"stories"`
	}
	err = json.NewDecoder(http.MaxBytesReader(nil, res.Body, FINDER_MAX_BODY)).Decode(&board)
	return board.Stories, err
}

// pickContract chooses the option for a story: the first expiry at least MinDays out, and the strike
// with a two-sided quote nearest to OTMPct past the stock price.
func (s *Service) pickContract(ctx context.Context, symbol, right string, f FinderRules, now time.Time) (Contract, error) {
	near, err := s.Resolver.Market.Chain(ctx, symbol, 0)
	if err != nil {
		return Contract{}, err
	}
	var expiry int64
	for _, e := range near.Expiries {
		if e >= now.Unix()+int64(f.MinDays)*DAY_SECONDS {
			expiry = e
			break
		}
	}
	if expiry == 0 {
		return Contract{}, errors.New("no expiry far enough out")
	}
	chain, err := s.Resolver.Market.Chain(ctx, symbol, expiry)
	if err != nil {
		return Contract{}, err
	}
	rows, target := chain.Calls, chain.Spot*(1+f.OTMPct/PERCENT)
	if right == RIGHT_PUT {
		rows, target = chain.Puts, chain.Spot*(1-f.OTMPct/PERCENT)
	}
	var best *market.OptionRow
	for i, row := range rows {
		if row.Bid <= 0 || row.Ask <= 0 {
			continue
		}
		if best == nil || math.Abs(row.Strike-target) < math.Abs(best.Strike-target) {
			best = &rows[i]
		}
	}
	if best == nil || chain.Spot <= 0 {
		return Contract{}, errors.New("no strike with a two-sided quote")
	}
	return Contract{Symbol: symbol, Market: MARKET_OPTION, Right: right, Strike: best.Strike,
		Expiry: time.Unix(chain.Expiry, 0).UTC().Format(market.EXPIRY_LAYOUT)}, nil
}

// open drafts the play for one story and trades it when no blocking rule fails. Unlike a pasted
// link, a play nobody looked at never trades through a block: it waits as a draft.
func (s *Service) open(ctx context.Context, story foundStory, symbol, right string, rules Rules, now time.Time) (Play, string, error) {
	c, err := s.pickContract(ctx, symbol, right, *rules.Finder, now)
	if err != nil {
		return Play{}, "", err
	}
	p := Draft(c, s.Resolver.Snapshot(ctx, c, now), rules.Template, now)
	p.ID, p.StoryID, p.FoundBy, p.Category = NewID(now), story.ID, FOUND_BY_FINDER, FINDER_CATEGORY
	if len(story.Tags) > 0 {
		p.Theme = story.Tags[0]
	}
	p.Thesis = fmt.Sprintf("%s (%d outlets, attention %.0f, validity %.0f, coverage against volume: %s)",
		story.Title, story.Outlets, story.Attention.Score, story.Validity.Score, story.Reading.Label)
	p.Evidence = append(p.Evidence, Evidence{Kind: EV_NEWS, Label: story.Title, Stance: STANCE_FOR, Source: FINDER_SOURCE,
		Verified: true, Auto: true, SourceCount: story.Validity.Independent, TrustPct: story.Validity.Score, At: now.UTC()})
	if err := s.Store.Save(p); err != nil {
		return Play{}, "", err
	}
	blocked := []string{}
	for _, check := range Build(p, rules, Book{Others: s.Store.Plays(), Now: now}).Checks {
		if check.Status == CHECK_FAIL {
			blocked = append(blocked, check.ID)
		}
	}
	if len(blocked) > 0 {
		return p, "blocked by " + strings.Join(blocked, ", "), nil
	}
	armed, err := s.paper(p, now)
	if err != nil {
		return p, err.Error(), nil
	}
	return armed, "", nil
}

// find runs one pass and returns what it decided. The loudest story that earns a play and has none
// yet gets one: one play a pass, so a busy hour cannot spend the whole day's count at once.
func (s *Service) find(ctx context.Context, now time.Time) FinderStatus {
	st := FinderStatus{At: now.UTC(), State: FINDER_LOOKING, Notes: []FinderNote{}, NextAt: now.Add(FINDER_EVERY).UTC(), EveryMs: FINDER_EVERY.Milliseconds()}
	rules := s.Store.Rules()
	f := rules.Finder
	seen, busy := map[int64]bool{}, map[string]bool{}
	day := now.In(MARKET_TZ).Format(market.EXPIRY_LAYOUT)
	for _, p := range s.Store.Plays() {
		seen[p.StoryID] = seen[p.StoryID] || p.StoryID != 0
		busy[p.Contract.Symbol] = busy[p.Contract.Symbol] || p.Status != STATUS_CLOSED
		if p.FoundBy == FOUND_BY_FINDER && p.CreatedAt.In(MARKET_TZ).Format(market.EXPIRY_LAYOUT) == day {
			st.Today++
		}
		if p.Result != nil && p.Result.At.In(MARKET_TZ).Format(market.EXPIRY_LAYOUT) == day {
			st.DayPnL += p.Result.PnLUSD
		}
	}
	switch {
	case f == nil || !f.On || s.NewsURL == "":
		st.State = FINDER_OFF
	case !f.sessionOpen(now):
		st.State, st.Why = FINDER_CLOSED, "the stock session is closed"
	case st.Today >= f.MaxPerDay:
		st.State, st.Why = FINDER_FULL, fmt.Sprintf("%d plays opened today, the most the rulebook allows", st.Today)
	case f.MaxDayLossUSD > 0 && st.DayPnL <= -f.MaxDayLossUSD:
		st.State, st.Why = FINDER_PAUSED, fmt.Sprintf("plays closed today lost $%.2f, past the $%.2f pause", -st.DayPnL, f.MaxDayLossUSD)
	}
	if st.State != FINDER_LOOKING {
		return st
	}
	stories, err := s.stories(ctx)
	if err != nil {
		st.State, st.Why = FINDER_ERROR, err.Error()
		return st
	}
	opened := false
	for _, story := range stories {
		if story.Attention.Score < f.MinAttention {
			continue // too quiet to be worth a line in the log
		}
		note := FinderNote{StoryID: story.ID, Title: story.Title, Outcome: FINDER_SKIPPED}
		symbol, right, why := f.wants(story)
		note.Symbol, note.Why = symbol, why
		switch {
		case why != "":
		case seen[story.ID]:
			note.Why = "this story already has a play"
		case busy[symbol]:
			note.Why = "a play on " + symbol + " is already open"
		case opened:
			note.Why = "one play a pass: it is next in line"
		default:
			p, held, err := s.open(ctx, story, symbol, right, rules, now)
			switch {
			case err != nil:
				note.Why = err.Error()
			case held != "":
				note.Outcome, note.Why, note.PlayID = FINDER_DRAFTED, held, p.ID
			default:
				note.Outcome, note.PlayID = FINDER_OPENED, p.ID
			}
			opened = opened || err == nil
			log.Printf("finder: story %d %s: %s %s", story.ID, symbol, note.Outcome, note.Why)
		}
		st.Notes = append(st.Notes, note)
	}
	return st
}

// RunFinder looks for plays every FINDER_EVERY until ctx ends.
func (s *Service) RunFinder(ctx context.Context) {
	ticker := time.NewTicker(FINDER_EVERY)
	defer ticker.Stop()
	for {
		pass, cancel := context.WithTimeout(ctx, FINDER_TIMEOUT)
		st := s.find(pass, time.Now())
		cancel()
		if st.State == FINDER_ERROR && ctx.Err() == nil {
			log.Printf("finder: %s", st.Why)
		}
		s.finder.set(st)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
