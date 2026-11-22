package plays

import (
	"fmt"
	"math"
	"time"

	"github.com/YEAHDOGS/money24/engine/market"
)

// unit keeps a strength between 0 and 1.
var unit = func(v float64) float64 { return math.Max(0, math.Min(1, v)) }

const (
	// The scout's setups. Each reads one name's session so far and says whether there is a short-term
	// trade in it, which way, how sure, and how long such a trade usually takes.
	SETUP_ORB       = "opening_range_breakout"
	SETUP_VWAP      = "vwap_reclaim"
	SETUP_FLOW      = "flow_surge"
	SETUP_NEWS      = "news_catalyst"
	SETUP_LAST_HOUR = "last_half_hour"

	ORB_MINUTES      = 30 // the opening range is the session's first half hour
	VWAP_BELOW_MIN   = 15 // minutes on the other side of VWAP before a cross counts as a reclaim
	REL_VOL_WINDOW   = 5  // minutes of volume compared with the session's average minute
	LAST_HOUR_START  = 30 // minutes before the close the last-half-hour window opens...
	LAST_HOUR_WINDOW = 5  // ...and for how long it takes entries
	LAST_HOUR_MOVE   = 0.25
	FLOW_MIN_USD     = 250000 // premium in one minute on one side before it is a surge
	FLOW_SIDE_RATIO  = 2.0    // that side over the other in the same minute
	NEWS_MOVE_PCT    = 0.3    // the stock must already be moving the story's way this much

	HOLD_ORB       = 60 * time.Minute
	HOLD_VWAP      = 45 * time.Minute
	HOLD_FLOW      = 30 * time.Minute
	HOLD_NEWS      = 90 * time.Minute
	HOLD_LAST_HOUR = 20 * time.Minute

	DIR_UP   = 1
	DIR_DOWN = -1
)

// session is one name's day so far, worked out from its minute candles.
type session struct {
	Symbol    string
	Candles   []market.Candle // today's regular session up to now, oldest first
	Open      float64
	Last      float64
	ORBHigh   float64
	ORBLow    float64
	ORBDone   bool
	VWAP      float64
	VWAPSide  int     // the side of VWAP the last VWAP_BELOW_MIN minutes before this one were on, 0 when mixed
	RelVolume float64 // the last few minutes' volume over the session's average minute
	MovePct   float64 // from the open
	FirstMove float64 // the first half hour's move, percent
}

// readSession works out the day from the session's minute candles up to now.
func readSession(symbol string, candles []market.Candle, opens time.Time) (session, bool) {
	s := session{Symbol: symbol}
	for _, c := range candles {
		if c.T >= opens.Unix() && c.C > 0 {
			s.Candles = append(s.Candles, c)
		}
	}
	n := len(s.Candles)
	if n < REL_VOL_WINDOW+1 {
		return s, false
	}
	s.Open, s.Last = s.Candles[0].O, s.Candles[n-1].C
	s.MovePct = (s.Last/s.Open - 1) * PERCENT
	pv, vol, total := 0.0, 0.0, 0.0
	s.ORBHigh, s.ORBLow = s.Candles[0].H, s.Candles[0].L
	for i, c := range s.Candles {
		typical := (c.H + c.L + c.C) / 3
		pv, vol, total = pv+typical*c.V, vol+c.V, total+c.V
		if i < ORB_MINUTES {
			s.ORBHigh, s.ORBLow = math.Max(s.ORBHigh, c.H), math.Min(s.ORBLow, c.L)
		}
		if i == ORB_MINUTES-1 {
			s.FirstMove = (c.C/s.Open - 1) * PERCENT
		}
	}
	s.ORBDone = n > ORB_MINUTES
	if vol > 0 {
		s.VWAP = pv / vol
	}
	recent := 0.0
	for _, c := range s.Candles[n-REL_VOL_WINDOW:] {
		recent += c.V
	}
	if total > 0 {
		s.RelVolume = (recent / REL_VOL_WINDOW) / (total / float64(n))
	}
	if n > VWAP_BELOW_MIN+1 && s.VWAP > 0 {
		above := 0
		for _, c := range s.Candles[n-1-VWAP_BELOW_MIN : n-1] {
			if c.C > s.VWAP {
				above++
			}
		}
		if above == VWAP_BELOW_MIN {
			s.VWAPSide = DIR_UP
		}
		if above == 0 {
			s.VWAPSide = DIR_DOWN
		}
	}
	return s, true
}

// setupHit is a setup that fired: which way, how strongly (0 to 1), how long it usually takes, and why.
type setupHit struct {
	Setup     string
	Direction int
	Strength  float64
	Hold      time.Duration
	Reasons   []string
}

// orbHit fires when the price leaves the opening range on more volume than usual.
func orbHit(s session, f ScoutRules) (setupHit, bool) {
	if !s.ORBDone || s.RelVolume < f.MinRelVolume {
		return setupHit{}, false
	}
	width := s.ORBHigh - s.ORBLow
	if width <= 0 {
		return setupHit{}, false
	}
	dir, past := DIR_UP, s.Last-s.ORBHigh
	if s.Last < s.ORBLow {
		dir, past = DIR_DOWN, s.ORBLow-s.Last
	}
	if past <= 0 {
		return setupHit{}, false
	}
	return setupHit{Setup: SETUP_ORB, Direction: dir, Hold: HOLD_ORB, Strength: unit(0.4 + past/width + (s.RelVolume-1)/4),
		Reasons: []string{fmt.Sprintf("%s broke its first-half-hour range (%.2f to %.2f) at %.2f", s.Symbol, s.ORBLow, s.ORBHigh, s.Last),
			fmt.Sprintf("volume %.1fx the session's average minute", s.RelVolume)}}, true
}

// vwapHit fires when the price crosses VWAP after a quarter hour on the other side, on rising volume.
func vwapHit(s session, f ScoutRules) (setupHit, bool) {
	if s.VWAPSide == 0 || s.RelVolume < f.MinRelVolume {
		return setupHit{}, false
	}
	nowSide := DIR_DOWN
	if s.Last > s.VWAP {
		nowSide = DIR_UP
	}
	if nowSide == s.VWAPSide {
		return setupHit{}, false
	}
	return setupHit{Setup: SETUP_VWAP, Direction: nowSide, Hold: HOLD_VWAP, Strength: unit(0.35 + (s.RelVolume-1)/3),
		Reasons: []string{fmt.Sprintf("%s crossed its VWAP %.2f after %d minutes on the other side", s.Symbol, s.VWAP, VWAP_BELOW_MIN),
			fmt.Sprintf("volume %.1fx the session's average minute", s.RelVolume)}}, true
}

// flowHit fires when one side of the chain took far more premium than the other in the last minute.
func flowHit(symbol string, callUSD, putUSD float64) (setupHit, bool) {
	dir, lead, other := DIR_UP, callUSD, putUSD
	if putUSD > callUSD {
		dir, lead, other = DIR_DOWN, putUSD, callUSD
	}
	if lead < FLOW_MIN_USD || lead < FLOW_SIDE_RATIO*other {
		return setupHit{}, false
	}
	side := map[int]string{DIR_UP: "calls", DIR_DOWN: "puts"}[dir]
	return setupHit{Setup: SETUP_FLOW, Direction: dir, Hold: HOLD_FLOW, Strength: unit(0.3 + math.Log10(lead/FLOW_MIN_USD)/2),
		Reasons: []string{fmt.Sprintf("$%s of %s traded on %s in the last minute, against $%s on the other side", short(lead), side, symbol, short(other))}}, true
}

// newsHit fires when a confirmed story leans one way on the name and the price already agrees.
func newsHit(s session, story foundStory, angle string) (setupHit, bool) {
	dir := DIR_UP
	if angle == ANGLE_BEARISH {
		dir = DIR_DOWN
	}
	if float64(dir)*s.MovePct < NEWS_MOVE_PCT {
		return setupHit{}, false
	}
	return setupHit{Setup: SETUP_NEWS, Direction: dir, Hold: HOLD_NEWS, Strength: unit(0.3 + story.Validity.Score/PERCENT/2 + story.Attention.Score/PERCENT/4),
		Reasons: []string{fmt.Sprintf("story: %s (%d outlets, %s)", story.Title, story.Outlets, angle),
			fmt.Sprintf("%s is %+.2f%% on the day, the story's way", s.Symbol, s.MovePct)}}, true
}

// lastHourHit fires in the last half hour's first minutes, the way the first half hour went.
func lastHourHit(s session, now, closes time.Time) (setupHit, bool) {
	from := closes.Add(-LAST_HOUR_START * time.Minute)
	if now.Before(from) || !now.Before(from.Add(LAST_HOUR_WINDOW*time.Minute)) || math.Abs(s.FirstMove) < LAST_HOUR_MOVE {
		return setupHit{}, false
	}
	dir := DIR_UP
	if s.FirstMove < 0 {
		dir = DIR_DOWN
	}
	return setupHit{Setup: SETUP_LAST_HOUR, Direction: dir, Hold: HOLD_LAST_HOUR, Strength: unit(0.4 + math.Abs(s.FirstMove)/2),
		Reasons: []string{fmt.Sprintf("the first half hour went %+.2f%%: the last half hour has tended to follow it (published research on SPY)", s.FirstMove)}}, true
}
