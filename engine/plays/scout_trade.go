package plays

import (
	"fmt"
	"log"
	"math"
	"sort"
	"time"

	"github.com/YEAHDOGS/money24/engine/market"
)

const (
	// The scout trades its best new play each pass, on the desk's book and through the broker when one is
	// connected, inside its own limits: a few plays open at once, a cap a day, a pause once the day's
	// closed scout plays have lost enough, and only plays whose own odds say they are worth taking.
	FOUND_BY_SCOUT = "scout"
	SCOUT_CATEGORY = "Scout"
	SCOUT_CALAMITY = "Same-day option: on a halt, a gap through the stop or a broken feed, the time exit closes it before the bell."
	SCOUT_NO_QUOTE = 5 // minutes without a quote before the glitch plan applies
	SCOUT_GAP_PCT  = 15

	WHY_SCOUT_OPEN_FULL = "%d scout plays are running, the most the rulebook allows"
	WHY_SCOUT_DAY_FULL  = "%d scout plays today, the most the rulebook allows"
	WHY_SCOUT_PAUSED    = "scout plays closed today lost $%.2f, past the $%.2f pause"
)

// ScoutTrade is the scout's trading part of the rulebook.
type ScoutTrade struct {
	On            bool    `json:"on"`
	MaxOpen       int     `json:"max_open"`
	MaxPerDay     int     `json:"max_per_day"`
	MaxDayLossUSD float64 `json:"max_day_loss_usd"`
	MinOddsPct    float64 `json:"min_odds_pct"` // the market's odds of target 1 within the hold
	MinEdgeUSD    float64 `json:"min_edge_usd"` // rough expected value per contract from those odds
}

// DefaultScoutTrade is how the scout trades until the rulebook says otherwise.
func DefaultScoutTrade() *ScoutTrade {
	return &ScoutTrade{On: true, MaxOpen: 3, MaxPerDay: 10, MaxDayLossUSD: 150, MinOddsPct: 30, MinEdgeUSD: 0}
}

// scoutBook is what the trading limits read from the plays on file.
func (s *Service) scoutBook(now time.Time) (open, today int, dayPnL float64) {
	day := now.In(MARKET_TZ).Format(market.EXPIRY_LAYOUT)
	for _, p := range s.Store.Plays() {
		if p.FoundBy != FOUND_BY_SCOUT {
			continue
		}
		if p.Status == STATUS_ARMED {
			open++
		}
		if p.CreatedAt.In(MARKET_TZ).Format(market.EXPIRY_LAYOUT) == day {
			today++
		}
		if p.Result != nil && p.Result.At.In(MARKET_TZ).Format(market.EXPIRY_LAYOUT) == day {
			dayPnL += p.Result.PnLUSD
		}
	}
	return open, today, dayPnL
}

// playFrom turns a scout candidate into a play with its whole bracket: half at target 1, the rest
// riding to target 2, the stop, and the time exit.
func playFrom(c Candidate, now time.Time) Play {
	half := math.Floor(float64(c.Qty) / 2)
	p := Play{ID: NewID(now), CreatedAt: now.UTC(), UpdatedAt: now.UTC(), Status: STATUS_DRAFT, FoundBy: FOUND_BY_SCOUT,
		Category: SCOUT_CATEGORY, Theme: FLOW_THEME_PREFIX + c.Symbol, Contract: c.Contract, Qty: float64(c.Qty), Entry: c.Entry, Stop: c.Stop,
		Title:     fmt.Sprintf("%s %g%s %s", c.Symbol, c.Contract.Strike, c.Contract.Right, c.Contract.Expiry),
		Thesis:    fmt.Sprintf("Scout, %s: %s", c.Setup, joinReasons(c.Reasons)),
		Failsafes: Failsafes{ExitBy: c.ExitBy, NoQuoteMinutes: SCOUT_NO_QUOTE, GapPct: SCOUT_GAP_PCT, Calamity: SCOUT_CALAMITY},
		Market: &Market{Spot: c.Spot, Bid: c.Liquidity.Bid, Ask: c.Liquidity.Ask, IV: c.IV, OpenInterest: c.Liquidity.OpenInterest,
			Volume: c.Liquidity.Volume, Contract: c.OCC, Source: FLOW_SOURCE, At: now.UTC()}}
	if half >= 1 {
		p.Ladder = []Rung{{Qty: half, Price: c.Target1}}
		p.Runner = Runner{Qty: p.Qty - half, Target: c.Target2}
	} else {
		p.Ladder = []Rung{{Qty: p.Qty, Price: c.Target1}}
	}
	p.Evidence = []Evidence{{Kind: EV_VOLUME, Stance: STANCE_FOR, Source: FLOW_SOURCE, Verified: true, At: now.UTC(),
		Value: float64(c.Liquidity.Volume), Label: joinReasons(c.Reasons)}}
	return p
}

// joinReasons writes a candidate's reasons as one line.
func joinReasons(reasons []string) string {
	out := ""
	for i, r := range reasons {
		if i > 0 {
			out += "; "
		}
		out += r
	}
	return out
}

// tradeBest trades the best of this pass's new plays that clears the limits. It returns why it did
// not, in words, for the board.
func (s *Service) tradeBest(fresh []int, f *ScoutTrade, now time.Time) string {
	if f == nil || !f.On || len(fresh) == 0 {
		return ""
	}
	open, today, dayPnL := s.scoutBook(now)
	switch {
	case f.MaxOpen > 0 && open >= f.MaxOpen:
		return fmt.Sprintf(WHY_SCOUT_OPEN_FULL, open)
	case f.MaxPerDay > 0 && today >= f.MaxPerDay:
		return fmt.Sprintf(WHY_SCOUT_DAY_FULL, today)
	case f.MaxDayLossUSD > 0 && dayPnL <= -f.MaxDayLossUSD:
		return fmt.Sprintf(WHY_SCOUT_PAUSED, -dayPnL, f.MaxDayLossUSD)
	}
	sort.Slice(fresh, func(a, b int) bool { return s.scout.all[fresh[a]].Score > s.scout.all[fresh[b]].Score })
	for _, i := range fresh {
		c := &s.scout.all[i]
		if c.Qty < 1 || c.OddsTarget < f.MinOddsPct || c.EdgeUSD < f.MinEdgeUSD {
			continue
		}
		p := playFrom(*c, now)
		if err := s.Store.Save(p); err != nil {
			return err.Error()
		}
		armed, err := s.paper(p, now)
		if err != nil {
			s.Store.Delete(p.ID)
			c.Traded = "not traded: " + err.Error()
			continue
		}
		c.PlayID, c.Traded = armed.ID, ""
		s.saveScout(*c)
		if !s.ScoutDry {
			s.scout.keepObservation(*c)
		}
		log.Printf(SCOUT_LOG+"traded %s %s: %d at %.2f, stop %.2f, targets %.2f / %.2f, out by %s",
			c.Setup, armed.Title, c.Qty, c.Entry, c.Stop, c.Target1, c.Target2, c.ExitBy.In(MARKET_TZ).Format("15:04"))
		return ""
	}
	return ""
}
