package news

import (
	"math"
	"sort"
	"strings"
	"time"

	"github.com/YEAHDOGS/money24/engine/market"
)

const (
	HOUR              = time.Hour
	ATTENTION_WINDOW  = 6 * time.Hour // outlets that covered the story inside this window count toward attention
	ATTENTION_REF     = 80.0          // the raw count that scores 100: about 30 outlets and 20 articles in the hour
	OUTLET_WEIGHT     = 2.0           // one more outlet is worth two more articles from the same ones
	SCORE_MAX         = 100.0
	RISING_FACTOR     = 1.5 // this hour against the one before, to call coverage rising
	FADING_FACTOR     = 0.5
	RISING_MIN        = 3 // and at least this many articles this hour
	VALID_INDEPENDENT = 60.0
	VALID_PRIMARY     = 25.0
	VALID_KNOWN       = 15.0
	INDEPENDENT_FULL  = 5 // independent sources that earn the whole independent share

	// Coverage against volume. Volume is measured against the instrument's own three-month average pace.
	READING_EARLY     = "early"     // coverage rising, volume not yet
	READING_CONFIRMED = "confirmed" // both up: real, and usually late
	READING_EXHAUSTED = "exhausted" // the price moved and coverage is dying
	READING_QUIET     = "quiet"
	READING_NONE      = "none" // no linked instrument has a quote
	LOUD_ATTENTION    = 35.0
	VOLUME_UP         = 1.5
	VOLUME_FLAT       = 1.2
	MOVED_PCT         = 2.0
	MARKET_ZONE       = "America/New_York"
	SESSION_OPEN_MIN  = 9*60 + 30
	SESSION_MINUTES   = 390.0
	SESSION_MIN_SHARE = 0.1 // the first minutes of a session would otherwise divide by almost nothing
	QUOTE_EQUITY      = "EQUITY"
	QUOTE_ETF         = "ETF"
	KIND_FILING       = "filing"
	KIND_CENTRAL_BANK = "central_bank"
	KIND_MACRO        = "macro"
	ROUND_SCORE       = 10.0  // scores keep one decimal
	ROUND_RATIO       = 100.0 // ratios keep two
)

var PRIMARY_KINDS = map[string]bool{KIND_FILING: true, KIND_CENTRAL_BANK: true, KIND_MACRO: true}

func roundTo(v, unit float64) float64 { return math.Round(v*unit) / unit }

// topPlaces turns a count per country into a list, most articles first.
func topPlaces(counts map[string]int) []Place {
	out := []Place{}
	for id, n := range counts {
		out = append(out, Place{ID: id, N: n})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].N > out[j].N || (out[i].N == out[j].N && out[i].ID < out[j].ID) })
	return out[:min(len(out), MAX_PLACES)]
}

// category is the one most headlines vote for; the rule file's order settles ties.
func (r *storyReader) category(c *cluster) string {
	votes := map[string]int{}
	for _, m := range c.members {
		if id, ok := r.kinds[m.Kind]; ok {
			votes[id]++
			continue
		}
		voted := map[string]bool{}
		for _, hit := range r.categories.find(m.words) {
			for _, id := range hit.Labels {
				if !voted[id] {
					voted[id] = true
					votes[id]++
				}
			}
		}
	}
	best := r.fallback
	for id, n := range votes {
		if n > votes[best] || (n == votes[best] && best != r.fallback && r.order[id] < r.order[best]) {
			best = id
		}
	}
	return best
}

// angle counts the headlines that use a bullish word and those that use a bearish one.
func (r *storyReader) angle(c *cluster) Angle {
	a := Angle{Label: ANGLE_UNCLEAR, Terms: []AngleTerm{}}
	terms := map[string]*AngleTerm{}
	for _, m := range c.members {
		sides := map[string]bool{}
		for _, hit := range r.lexicon.find(m.words) {
			side := hit.Labels[0]
			sides[side] = true
			if terms[hit.Phrase] == nil {
				terms[hit.Phrase] = &AngleTerm{Term: hit.Phrase, Side: side}
			}
			terms[hit.Phrase].N++
		}
		if sides[ANGLE_BULLISH] {
			a.Bullish++
		}
		if sides[ANGLE_BEARISH] {
			a.Bearish++
		}
	}
	for _, t := range terms {
		a.Terms = append(a.Terms, *t)
	}
	sort.Slice(a.Terms, func(i, j int) bool {
		return a.Terms[i].N > a.Terms[j].N || (a.Terms[i].N == a.Terms[j].N && a.Terms[i].Term < a.Terms[j].Term)
	})
	a.Terms = a.Terms[:min(len(a.Terms), MAX_TERMS)]
	// One loaded word among thirty headlines is not a lean: enough of them have to take a side.
	if a.Bullish+a.Bearish < max(ANGLE_MIN_HEADLINES, int(math.Ceil(ANGLE_MIN_SHARE*float64(len(c.members))))) {
		return a
	}
	a.Score = roundTo(float64(a.Bullish-a.Bearish)/float64(a.Bullish+a.Bearish), ROUND_RATIO)
	switch {
	case a.Score >= ANGLE_LEAN:
		a.Label = ANGLE_BULLISH
	case a.Score <= -ANGLE_LEAN:
		a.Label = ANGLE_BEARISH
	case a.Bullish >= ANGLE_MIXED_AT && a.Bearish >= ANGLE_MIXED_AT:
		a.Label = ANGLE_MIXED
	}
	return a
}

// tags keeps the watchlist tags enough of the story's headlines carry, most carried first.
func tags(c *cluster) []string {
	counts := map[string]int{}
	for _, m := range c.members {
		for _, t := range m.Tags {
			counts[t]++
		}
	}
	floor := int(math.Ceil(float64(len(c.members)) * TAG_SHARE))
	out := []string{}
	for t, n := range counts {
		if n >= floor {
			out = append(out, t)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return counts[out[i]] > counts[out[j]] || (counts[out[i]] == counts[out[j]] && out[i] < out[j])
	})
	return out[:min(len(out), MAX_TAGS)]
}

// flip carries a story's angle through a link's sign.
func flip(angle string, sign int) string {
	if sign >= 0 || (angle != ANGLE_BULLISH && angle != ANGLE_BEARISH) {
		return angle
	}
	if angle == ANGLE_BULLISH {
		return ANGLE_BEARISH
	}
	return ANGLE_BULLISH
}

// relVolume is today's volume against the instrument's usual pace: 1 is ordinary, 2 is twice as busy.
// During the stock session the average is scaled to the share of the day gone, in a straight line.
// Trading is heavier at the open and close, so this reads high early; it is a pace, not a forecast.
func relVolume(q market.Tick, now time.Time, zone *time.Location) float64 {
	if q.Volume <= 0 || q.AvgVolume <= 0 {
		return 0
	}
	share := 1.0
	if q.State == market.STATE_REGULAR && (q.Type == QUOTE_EQUITY || q.Type == QUOTE_ETF) {
		local := now.In(zone)
		gone := float64(local.Hour()*60+local.Minute()-SESSION_OPEN_MIN) / SESSION_MINUTES
		share = math.Min(1, math.Max(SESSION_MIN_SHARE, gone))
	}
	return roundTo(q.Volume/(q.AvgVolume*share), ROUND_RATIO)
}

// links lists what the story's tags can move, the first tag's instruments first, with their quotes.
func (r *storyReader) linksFor(storyTags []string, angle string, quotes map[string]market.Tick, now time.Time, zone *time.Location) []Link {
	out := []Link{}
	seen := map[string]bool{}
	for _, tag := range storyTags {
		for _, l := range r.links[tag] {
			if seen[l.Symbol] || len(out) == MAX_LINKS {
				continue
			}
			seen[l.Symbol] = true
			l.Angle = flip(angle, l.Sign)
			if q, ok := quotes[l.Symbol]; ok && q.Price > 0 {
				l.Quoted, l.Price, l.MovePct, l.RelVolume = true, q.Price, roundTo(q.ChangePct, ROUND_RATIO), relVolume(q, now, zone)
			}
			out = append(out, l)
		}
	}
	return out
}

// reading sets coverage against the first direct instrument that has a quote and a volume.
func reading(a Attention, links []Link) Reading {
	for _, l := range links {
		if l.Relation != RELATION_SELF || !l.Quoted || l.RelVolume == 0 {
			continue
		}
		loud := a.Score >= LOUD_ATTENTION
		label := READING_QUIET
		switch {
		case loud && l.RelVolume >= VOLUME_UP:
			label = READING_CONFIRMED
		case loud && a.Trend == TREND_RISING && l.RelVolume < VOLUME_FLAT:
			label = READING_EARLY
		case loud && a.Trend == TREND_FADING && math.Abs(l.MovePct) >= MOVED_PCT:
			label = READING_EXHAUSTED
		}
		return Reading{Label: label, Symbol: l.Symbol}
	}
	return Reading{Label: READING_NONE}
}

// read builds the story a group of articles tells, as of now.
func (r *storyReader) read(c *cluster, now time.Time, quotes map[string]market.Tick, zone *time.Location) Story {
	first, last := c.members[0], c.members[len(c.members)-1]
	s := Story{
		ID: c.id, FirstSeen: first.SeenAt, LastSeen: last.SeenAt, Title: strings.TrimSpace(headline(first.Title)),
		Articles: len(c.members), Category: r.category(c), Angle: r.angle(c), Tags: tags(c), Markets: c.bets, Spark: c.spark,
	}
	outlets, recent, known, keys := map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}
	from, about := map[string]int{}, map[string]int{}
	for _, m := range c.members {
		outlets[m.Source], keys[m.key] = true, true
		if r.knownFeed[m.Feed] {
			known[m.Source] = true
		}
		s.Validity.Primary = s.Validity.Primary || PRIMARY_KINDS[m.Kind]
		age := now.Sub(m.SeenAt)
		if age <= ATTENTION_WINDOW {
			recent[m.Source] = true
		}
		if age <= HOUR {
			s.Attention.LastHour++
		} else if age <= 2*HOUR {
			s.Attention.PrevHour++
		}
		if m.from != "" {
			from[m.from]++
		}
		for _, id := range m.about {
			about[id]++
		}
		if s.Image == "" && m.Image != "" {
			s.Image, s.ImageBy = m.Image, m.Source
		}
	}
	s.Outlets, s.From, s.About = len(outlets), topPlaces(from), topPlaces(about)

	raw := OUTLET_WEIGHT*float64(len(recent)) + float64(s.Attention.LastHour)
	s.Attention.Score = roundTo(math.Min(SCORE_MAX, SCORE_MAX*math.Log1p(raw)/math.Log1p(ATTENTION_REF)), ROUND_SCORE)
	s.Attention.Trend = TREND_STEADY
	switch {
	case s.Attention.LastHour >= RISING_MIN && float64(s.Attention.LastHour) >= RISING_FACTOR*float64(s.Attention.PrevHour):
		s.Attention.Trend = TREND_RISING
	case float64(s.Attention.LastHour) <= FADING_FACTOR*float64(s.Attention.PrevHour):
		s.Attention.Trend = TREND_FADING
	}

	s.Validity.Independent, s.Validity.Known = min(len(keys), len(outlets)), len(known)
	s.Validity.Score = VALID_INDEPENDENT * float64(min(s.Validity.Independent, INDEPENDENT_FULL)) / INDEPENDENT_FULL
	if s.Validity.Primary {
		s.Validity.Score += VALID_PRIMARY
	}
	if s.Validity.Known > 0 {
		s.Validity.Score += VALID_KNOWN
	}

	s.Links = r.linksFor(s.Tags, s.Angle.Label, quotes, now, zone)
	s.Reading = reading(s.Attention, s.Links)
	if s.Markets == nil {
		s.Markets = []Bet{}
	}
	if s.Spark == nil {
		s.Spark = []float64{}
	}
	return s
}
