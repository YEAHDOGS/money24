package memedesk

import (
	"fmt"
	"sync"

	"github.com/YEAHDOGS/money24/engine/meme"
)

const (
	KIND_SHADOW   = "safety-shadow"
	KIND_ORGANIC  = "organic-graduates"
	KIND_FADE     = "first-spike-fade"
	RISK_HIGH     = "high"
	RISK_MEDIUM   = "medium"
	RISK_LOW      = "low"
	DECISION_BUY  = "buy"
	DECISION_PASS = "pass"
	DECISION_SKIP = "refuse"
	DECISION_WAIT = "pending"
	DECISION_FADE = "fade"
)

// Strategy evaluates only facts recorded before or at a potential entry.
// Waiting for a later quote is explicit. Only a trade-enabled kind can buy.
type Strategy interface {
	Evaluate(Observation) Decision
}

// Observation describes the current candidate and the facts available to evaluate it.
type Observation struct {
	Event      meme.Record
	Launch     *meme.Launch
	Safety     *meme.Safety
	Market     *meme.Market
	FirstPrice float64
	Expired    bool
}

// Decision records the strategy's verdict and whether to run a long benchmark.
// The benchmark is independent of paper cash and applies the same modeled costs.
type Decision struct {
	Action    string   `json:"action"`
	Reasons   []string `json:"reasons,omitempty"`
	Benchmark bool     `json:"benchmark,omitempty"`
}

// Kind describes a plug-in and how to build it from a rulebook entry.
type Kind struct {
	ID  string
	Buy bool
	New func(Config) (Strategy, error)
}

var (
	kindsMu sync.RWMutex
	kinds   = map[string]Kind{}
)

// Register adds a strategy kind before use.
func Register(kind Kind) {
	kindsMu.Lock()
	defer kindsMu.Unlock()
	if kind.ID == "" || kind.New == nil || kinds[kind.ID].New != nil {
		panic(fmt.Sprintf("invalid or duplicate meme strategy kind %q", kind.ID))
	}
	kinds[kind.ID] = kind
}

type configured struct {
	config Config
	rule   Strategy
	buy    bool
}

type shadow struct{ entry Entry }
type organic struct{ entry Entry }
type fade struct{ entry Entry }

func init() {
	Register(Kind{ID: KIND_SHADOW, New: func(c Config) (Strategy, error) {
		return shadow{entry: c.Entry}, nil
	}})
	Register(Kind{ID: KIND_ORGANIC, Buy: true, New: func(c Config) (Strategy, error) {
		return organic{entry: c.Entry}, nil
	}})
	Register(Kind{ID: KIND_FADE, New: func(c Config) (Strategy, error) {
		return fade{entry: c.Entry}, nil
	}})
}

func (s shadow) Evaluate(o Observation) Decision {
	reasons := safetyReasons(o, s.entry)
	if len(reasons) > 0 {
		return Decision{Action: DECISION_SKIP, Reasons: reasons, Benchmark: true}
	}
	return Decision{Action: DECISION_PASS, Benchmark: true}
}

func (s organic) Evaluate(o Observation) Decision {
	if o.Event.Kind != meme.KIND_GRADUATION {
		return Decision{Action: DECISION_SKIP, Reasons: []string{REASON_GRADUATION_ONLY}}
	}
	reasons := safetyReasons(o, s.entry)
	bond := o.Event.Graduation
	switch {
	case bond == nil || bond.UniqueBuyers == nil:
		reasons = append(reasons, REASON_BUYERS_UNKNOWN)
	case *bond.UniqueBuyers < s.entry.MinUniqueBuyers:
		reasons = append(reasons, REASON_BUYERS_LOW)
	}
	switch {
	case bond == nil || bond.SameBlock == nil:
		reasons = append(reasons, REASON_BLOCK_UNKNOWN)
	case *bond.SameBlock:
		reasons = append(reasons, REASON_SAME_BLOCK)
	}
	switch {
	case bond == nil || bond.CreatorSold == nil:
		reasons = append(reasons, REASON_CREATOR_UNKNOWN)
	case *bond.CreatorSold:
		reasons = append(reasons, REASON_CREATOR_SOLD)
	}
	switch {
	case bond == nil || bond.DeployerFunded == nil:
		reasons = append(reasons, REASON_FUNDING_UNKNOWN)
	case *bond.DeployerFunded:
		reasons = append(reasons, REASON_DEPLOYER_FUNDED)
	}
	if liquidity(o.Market) < s.entry.MinLiquidityUSD {
		reasons = append(reasons, REASON_LOW_LIQUIDITY)
	}
	if len(reasons) > 0 {
		return Decision{Action: DECISION_SKIP, Reasons: reasons, Benchmark: true}
	}
	return Decision{Action: DECISION_BUY, Benchmark: true}
}

func (s fade) Evaluate(o Observation) Decision {
	if o.Expired {
		return Decision{Action: DECISION_SKIP, Reasons: []string{REASON_NO_SPIKE}}
	}
	if o.FirstPrice <= 0 || price(o.Market) < o.FirstPrice*(1+s.entry.SpikePct) {
		return Decision{Action: DECISION_WAIT}
	}
	return Decision{Action: DECISION_FADE, Benchmark: true}
}
