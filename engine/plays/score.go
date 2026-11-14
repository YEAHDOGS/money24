package plays

import (
	"fmt"
	"math"
	"strings"
)

const (
	NEUTRAL        = 50.0 // a component with nothing to say
	SCORE_MAX      = 100.0
	LOG2_STEP      = 25.0 // points per doubling of volume or open interest over its bar
	NEWS_STEP      = 15.0 // points per trusted headline for or against
	BREAKING_BOOST = 2.0  // breaking news counts double
	TECH_STEP      = 12.5 // points per indicator agreeing or disagreeing
	SIGNAL_STEP    = 10.0 // points per trader signal, scaled by the source's trust
	RSI_HIGH       = 70.0
	RSI_LOW        = 30.0
	ODDS_FLOOR     = 1.0
	ODDS_CEILING   = 95.0 // nothing is a sure thing
	MULT_BASE      = 0.5  // the evidence moves the market's odds between half and one and a half times

	COMP_VOLUME    = "volume"
	COMP_NEWS      = "news"
	COMP_INTEREST  = "open_interest"
	COMP_TECHNICAL = "technicals"
	COMP_SIGNALS   = "signals"
)

// Odds is the market's chance of reaching the first exit, moved by the evidence score.
type Odds struct {
	MarketPct  float64            `json:"market_pct"` // touch odds of T1 from volatility alone
	Score      float64            `json:"score"`      // 0-100, 50 is neutral
	Components map[string]float64 `json:"components"` // each 0-100; missing means no data
	Weights    map[string]float64 `json:"weights"`    // as used, after the breaking-news override
	DOGSPct    float64            `json:"dogs_pct"`   // MarketPct times the evidence multiplier
	Ignored    []string           `json:"ignored"`    // news left out by the trust filter, and why
	Breaking   bool               `json:"breaking"`   // trusted breaking news is on the play
}

func stanceSign(s string) float64 {
	switch s {
	case STANCE_FOR:
		return 1
	case STANCE_AGAINST:
		return -1
	}
	return 0
}

func clampScore(v float64) float64 { return math.Max(0, math.Min(SCORE_MAX, v)) }

// odds weighs the evidence. Volume leads; trusted news outranks the indicators and, when it is
// breaking, takes their weight entirely; news without enough sources or history is ignored.
func odds(p Play, r Rules, levels []Level) Odds {
	out := Odds{Components: map[string]float64{}, Weights: map[string]float64{}, Ignored: []string{}}
	for _, l := range levels {
		if strings.HasPrefix(l.Name, "T") || l.Name == LEVEL_RUNNER {
			out.MarketPct = l.ChancePct
			break
		}
	}
	var m Market
	if p.Market != nil {
		m = *p.Market
	}
	bullish := p.Contract.Right != RIGHT_PUT

	// Volume: the stock's day against its average, and the contract's day against the bar.
	if m.AvgVolume > 0 && m.DayVolume > 0 {
		v := clampScore(NEUTRAL + LOG2_STEP*math.Log2(m.DayVolume/m.AvgVolume))
		if bar := r.param(RULE_VOLUME, P_MIN_CONTRACT_VOLUME); bar > 0 && m.Volume > 0 {
			v = (v + clampScore(NEUTRAL+LOG2_STEP*math.Log2(float64(m.Volume)/bar))) / 2 // each half capped, so one cannot drown the other
		}
		out.Components[COMP_VOLUME] = clampScore(v)
	}
	// Open interest against the liquidity bar.
	if bar := r.param(RULE_LIQUIDITY, P_MIN_OPEN_INTEREST); p.Contract.Market == MARKET_OPTION && m.OpenInterest > 0 && bar > 0 {
		out.Components[COMP_INTEREST] = clampScore(NEUTRAL + LOG2_STEP*math.Log2(float64(m.OpenInterest)/bar))
	}

	news, newsSeen := NEUTRAL, false
	tech, techSeen := NEUTRAL, false
	sig, sigSeen := NEUTRAL, false
	minSources, minTrust := r.NewsMinSources, r.NewsMinTrustPct
	for _, e := range p.Evidence {
		sign := stanceSign(e.Stance)
		switch e.Kind {
		case EV_NEWS:
			switch {
			case e.AI && !e.Verified:
				out.Ignored = append(out.Ignored, fmt.Sprintf("%s: suggested by AI, not verified", e.Label))
				continue
			case e.SourceCount < minSources:
				out.Ignored = append(out.Ignored, fmt.Sprintf("%s: %d sources, needs %d", e.Label, e.SourceCount, minSources))
				continue
			case e.TrustPct < minTrust:
				out.Ignored = append(out.Ignored, fmt.Sprintf("%s: source trust %.0f, needs %.0f", e.Label, e.TrustPct, minTrust))
				continue
			}
			step := NEWS_STEP
			if e.Breaking {
				step *= BREAKING_BOOST
				out.Breaking = true
			}
			news, newsSeen = news+sign*step, true
		case EV_TECHNICAL:
			tech, techSeen = tech+sign*TECH_STEP, true
		case EV_SIGNAL, EV_CHAIN:
			trust := math.Max(e.TrustPct, NEUTRAL) / SCORE_MAX
			sig, sigSeen = sig+sign*SIGNAL_STEP*trust*2, true
		case EV_VOLUME, EV_INTEREST:
			// Hand-entered flow (a sweep, a block print, unusual activity) counts with the volume.
			base, ok := out.Components[COMP_VOLUME]
			if !ok {
				base = NEUTRAL
			}
			out.Components[COMP_VOLUME] = clampScore(base + sign*NEWS_STEP)
		}
	}
	// The indicators the engine read itself: trend (price vs the averages) and stretch (RSI).
	if m.Spot > 0 && m.MA20 > 0 && m.MA50 > 0 {
		trend := 0.0
		if m.Spot > m.MA20 && m.MA20 > m.MA50 {
			trend = 1
		} else if m.Spot < m.MA20 && m.MA20 < m.MA50 {
			trend = -1
		}
		if !bullish {
			trend = -trend
		}
		tech, techSeen = tech+trend*TECH_STEP, true
	}
	if m.RSI14 > 0 {
		stretch := 0.0
		if m.RSI14 >= RSI_HIGH {
			stretch = -1 // stretched up: bad for a call, good for a put
		} else if m.RSI14 <= RSI_LOW {
			stretch = 1
		}
		if !bullish {
			stretch = -stretch
		}
		tech, techSeen = tech+stretch*TECH_STEP, true
	}
	if newsSeen {
		out.Components[COMP_NEWS] = clampScore(news)
	}
	if techSeen {
		out.Components[COMP_TECHNICAL] = clampScore(tech)
	}
	if sigSeen {
		out.Components[COMP_SIGNALS] = clampScore(sig)
	}

	weights := map[string]float64{COMP_VOLUME: r.Weights.Volume, COMP_NEWS: r.Weights.News, COMP_INTEREST: r.Weights.OpenInterest,
		COMP_TECHNICAL: r.Weights.Technicals, COMP_SIGNALS: r.Weights.Signals}
	if out.Breaking && r.BreakingOverridesTechnicals {
		weights[COMP_NEWS] += weights[COMP_TECHNICAL]
		weights[COMP_TECHNICAL] = 0
	}
	total, sum := 0.0, 0.0
	for k, score := range out.Components {
		w := weights[k]
		out.Weights[k] = w
		total, sum = total+w, sum+w*score
	}
	out.Score = NEUTRAL
	if total > 0 {
		out.Score = math.Round(sum/total*10) / 10
	}
	if out.MarketPct > 0 {
		dogs := out.MarketPct * (MULT_BASE + out.Score/SCORE_MAX)
		out.DOGSPct = math.Round(math.Max(ODDS_FLOOR, math.Min(ODDS_CEILING, dogs))*10) / 10
	}
	return out
}
