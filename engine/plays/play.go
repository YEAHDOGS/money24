// Package plays is Money 24's core loop: a play is one trade idea with everything it needs to run
// on its own — the contract, the entry, the ladder and runner exits, the stop, the failsafes, the
// evidence that other money is in the same trade, and the market it was planned against.
//
// Every play is checked against the rules (DOGS defaults, all editable), and its report says in
// plain numbers what must happen for it to pay: the price the stock has to reach, by when, and the
// market's own odds of getting there. A play with a failing blocking rule cannot be armed.
//
// Arming hands the play to the runner, which journals its own bracket; the exit watcher closes it.
package plays

import (
	"time"
)

const (
	STATUS_DRAFT  = "draft"  // being built; may fail rules
	STATUS_ARMED  = "armed"  // handed to the runner
	STATUS_CLOSED = "closed" // the runner closed it, or it was dropped
	MARKET_OPTION = "option"
	MARKET_STOCK  = "stock"
	RIGHT_CALL    = "C"
	RIGHT_PUT     = "P"
	EXPIRY_LAYOUT = "2006-01-02"
	OPTION_MULT   = 100
	SHARE_MULT    = 1

	// Evidence kinds. Volume, interest, chain and signal are "other traders are in this" evidence;
	// news is weighed only when it passes the trust filter; technical is MA, RSI and the like.
	EV_VOLUME    = "volume"
	EV_INTEREST  = "open_interest"
	EV_CHAIN     = "onchain"
	EV_NEWS      = "news"
	EV_SIGNAL    = "signal"
	EV_TECHNICAL = "technical"

	STANCE_FOR     = "for"
	STANCE_AGAINST = "against"
	STANCE_NEUTRAL = "neutral"
)

// Contract is what the play buys. Right is empty for shares.
type Contract struct {
	Symbol string  `json:"symbol"`
	Market string  `json:"market"` // option or stock
	Right  string  `json:"right,omitempty"`
	Strike float64 `json:"strike,omitempty"`
	Expiry string  `json:"expiry,omitempty"` // YYYY-MM-DD
}

// Rung is one ladder exit: sell Qty at Price (the contract's price, not the stock's).
type Rung struct {
	Qty   float64 `json:"qty"`
	Price float64 `json:"price"`
}

// Runner is what is left after the ladder: it rides to Target, or out on the stop or the clock.
type Runner struct {
	Qty    float64 `json:"qty"`
	Target float64 `json:"target"`
}

// Failsafes are the plans for when the market does not do what the ladder expects.
type Failsafes struct {
	ExitBy             time.Time `json:"exit_by,omitzero"`      // hard time exit for everything left
	SidewaysMinutes    int       `json:"sideways_minutes"`      // after this long...
	SidewaysMinGainPct float64   `json:"sideways_min_gain_pct"` // ...not up this much: get out
	NoQuoteMinutes     int       `json:"no_quote_minutes"`      // glitch: no quote or a halt this long, cancel and flatten when it returns
	GapPct             float64   `json:"gap_pct"`               // chaos: the stock gaps this far against the play past the stop, flatten at the bid
	Calamity           string    `json:"calamity"`              // the written total-calamity recovery plan
}

// Evidence is one reason to think other money is in this trade, or one fact about it.
type Evidence struct {
	Kind        string    `json:"kind"`
	Label       string    `json:"label"`
	Value       float64   `json:"value,omitempty"`
	Stance      string    `json:"stance"` // for, against or neutral, relative to the play's direction
	Source      string    `json:"source,omitempty"`
	URL         string    `json:"url,omitempty"`
	Verified    bool      `json:"verified"`               // checked by a person, or from a feed with a record
	Breaking    bool      `json:"breaking,omitempty"`     // hard, emergency breaking news
	SourceCount int       `json:"source_count,omitempty"` // independent outlets reporting it
	TrustPct    float64   `json:"trust_pct,omitempty"`    // the source's verified history, 0-100
	Auto        bool      `json:"auto,omitempty"`         // filled in by the engine from market data
	AI          bool      `json:"ai,omitempty"`           // suggested by an AI model: never weighed until verified
	At          time.Time `json:"at,omitzero"`
}

// Market is the snapshot a play was planned against.
type Market struct {
	Spot          float64   `json:"spot"`
	PreviousClose float64   `json:"previous_close"`
	Bid           float64   `json:"bid"`
	Ask           float64   `json:"ask"`
	Last          float64   `json:"last"`
	IV            float64   `json:"iv"` // decimal; for shares this is 20-day historical volatility
	OpenInterest  int       `json:"open_interest"`
	Volume        int       `json:"volume"`     // the contract's (or the stock's) volume today
	DayVolume     float64   `json:"day_volume"` // the stock's volume today
	AvgVolume     float64   `json:"avg_volume"` // the stock's average daily volume
	MA20          float64   `json:"ma20"`
	MA50          float64   `json:"ma50"`
	RSI14         float64   `json:"rsi14"`
	NextEarnings  time.Time `json:"next_earnings,omitzero"`
	Contract      string    `json:"contract,omitempty"` // OCC symbol
	Source        string    `json:"source"`
	At            time.Time `json:"at"`
	Note          string    `json:"note,omitempty"` // what could not be loaded
}

// Play is one trade idea, ready to check, explain and arm.
type Play struct {
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Status    string    `json:"status"`
	Title     string    `json:"title"`
	Thesis    string    `json:"thesis"`
	Theme     string    `json:"theme,omitempty"` // plays on one theme are one bet (FAA outage: AAL, DAL, UAL)
	// Category is what kind of play this is, set by hand ("News play"), so plays can be grouped and compared.
	Category string `json:"category,omitempty"`
	// FoundBy is "finder" when the engine opened the play itself, from the newsd story StoryID.
	FoundBy string `json:"found_by,omitempty"`
	StoryID int64  `json:"story_id,omitempty"`

	Contract  Contract   `json:"contract"`
	Qty       float64    `json:"qty"`
	Entry     float64    `json:"entry"` // limit price per contract or share
	Stop      float64    `json:"stop"` // stop-loss 1: in force until the first exit fills
	// StopsAfter are stop-loss 2 and 3: where the stop moves once exit 1 and then exit 2 have filled.
	// Empty leaves it to the stop ladder rule (the entry, then exit 1's price) or, with the rule off, unmoved.
	StopsAfter []float64 `json:"stops_after,omitempty"`
	Ladder    []Rung     `json:"ladder"`
	Runner    Runner     `json:"runner"`
	Failsafes Failsafes  `json:"failsafes"`
	Evidence  []Evidence `json:"evidence"`
	Market    *Market    `json:"market,omitempty"`
	Links     []string   `json:"links,omitempty"`
	Analyses  []Analysis `json:"analyses,omitempty"`
	Analyzing string     `json:"analyzing,omitempty"` // the provider reading it right now

	JournalID  int64     `json:"journal_id,omitempty"` // the plan's journal id once armed
	ArmedAt    time.Time `json:"armed_at,omitzero"`
	Filled     *Filled   `json:"filled,omitempty"`      // the entry as it actually filled, once armed
	BrokeRules []string  `json:"broke_rules,omitempty"` // blocking rules a trade went through anyway
	Fills      []Fill    `json:"fills,omitempty"`       // what the exit watcher sold, in order
	Result     *Result   `json:"result,omitempty"`      // set when the trade closed
}

// Filled is the entry as it actually happened, beside the entry the play expected.
type Filled struct {
	At        time.Time `json:"at"`
	Price     float64   `json:"price"`
	Qty       float64   `json:"qty"`
	AmountUSD float64   `json:"amount_usd"`
}

// Fill is one exit the runner took: a rung, the stop, the clock.
type Fill struct {
	At     time.Time `json:"at"`
	Qty    float64   `json:"qty"`
	Price  float64   `json:"price"`
	Reason string    `json:"reason"`
	PnLUSD float64   `json:"pnl_usd"`
}

// Result is how a closed play ended.
type Result struct {
	At     time.Time `json:"at"`
	PnLUSD float64   `json:"pnl_usd"`
	PnLPct float64   `json:"pnl_pct"`
	Reason string    `json:"reason"`
}

// Multiplier is 100 for options, 1 for shares.
func (p Play) Multiplier() float64 {
	if p.Contract.Market == MARKET_OPTION {
		return OPTION_MULT
	}
	return SHARE_MULT
}
