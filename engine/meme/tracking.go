package meme

import (
	"cmp"
	"slices"
	"time"
)

const (
	MAX_TRACKED         = 600
	MARKET_BATCH        = 30
	FIRST_SAMPLE        = 5 * time.Second
	FIRST_SAFETY        = 10 * time.Second
	EARLY_WINDOW        = 30 * time.Minute
	MIDDLE_WINDOW       = 6 * time.Hour
	RECORD_WINDOW       = 48 * time.Hour
	EARLY_INTERVAL      = time.Minute
	MIDDLE_INTERVAL     = 5 * time.Minute
	LATE_INTERVAL       = time.Hour
	UNLISTED_WINDOW     = 15 * time.Minute
	INACTIVE_WINDOW     = 30 * time.Minute
	SAFETY_RETRY        = 30 * time.Second
	MAX_SAFETY_ATTEMPTS = 3
	MAX_RECENT_EVENTS   = 32768
	MIGRATION_RETRY     = 15 * time.Second
	MIGRATION_INDEXING  = 2 * time.Minute
)

type trackedToken struct {
	mint           string
	start          time.Time
	graduated      time.Time
	lastActivity   time.Time
	nextMarket     time.Time
	nextSafety     time.Time
	safetyAttempts int
	safetyDone     bool
}

type eventKey struct {
	mint string
	kind string
}

// tracking holds bounded sampling state and a recent feed deduplication ring.
type tracking struct {
	tokens     map[string]*trackedToken
	recent     map[eventKey]struct{}
	ring       [MAX_RECENT_EVENTS]eventKey
	ringCursor int
}

func (t *tracking) remember(key eventKey) {
	delete(t.recent, t.ring[t.ringCursor])
	t.ring[t.ringCursor] = key
	t.ringCursor = (t.ringCursor + 1) % len(t.ring)
	t.recent[key] = struct{}{}
}

func (t *tracking) prune(now time.Time) {
	for mint, token := range t.tokens {
		if now.Sub(token.start) >= RECORD_WINDOW {
			delete(t.tokens, mint)
		}
	}
}

func (t *tracking) planEvent(row *Record) {
	if t.tokens[row.Mint] != nil {
		return
	}
	if len(t.tokens) < MAX_TRACKED {
		row.Tracking = TRACK_ADMITTED
		return
	}
	var victim *trackedToken
	for _, token := range t.tokens {
		if !token.graduated.IsZero() || (row.Kind != KIND_GRADUATION && row.At.Sub(token.lastActivity) < INACTIVE_WINDOW) {
			continue
		}
		if victim == nil || token.lastActivity.Before(victim.lastActivity) ||
			(token.lastActivity.Equal(victim.lastActivity) && token.mint < victim.mint) {
			victim = token
		}
	}
	if victim == nil {
		row.Tracking = TRACK_CAPACITY
		return
	}
	row.Tracking, row.Evicted = TRACK_ADMITTED, victim.mint
}

func (t *tracking) applyEvent(row Record) {
	key := eventKey{mint: row.Mint, kind: row.Kind}
	if _, ok := t.recent[key]; !ok {
		t.remember(key)
	}
	if row.Tracking == TRACK_ADMITTED {
		delete(t.tokens, row.Evicted)
		t.tokens[row.Mint] = &trackedToken{
			mint: row.Mint, start: row.At, lastActivity: row.At,
			nextMarket: row.At.Add(FIRST_SAMPLE), nextSafety: row.At.Add(FIRST_SAFETY),
		}
	}
	if token := t.tokens[row.Mint]; token != nil && row.Kind == KIND_GRADUATION {
		token.graduated = row.At
		token.nextMarket = row.At
	}
}

func nextSample(start, request time.Time) time.Time {
	age := request.Sub(start)
	if age < EARLY_WINDOW {
		return minTime(request.Add(EARLY_INTERVAL), start.Add(EARLY_WINDOW))
	}
	if age < MIDDLE_WINDOW {
		return minTime(request.Add(MIDDLE_INTERVAL), start.Add(MIDDLE_WINDOW))
	}
	return request.Add(LATE_INTERVAL)
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

func (t *tracking) marketDue(now time.Time) []*trackedToken {
	due := make([]*trackedToken, 0, len(t.tokens))
	for _, token := range t.tokens {
		if !token.nextMarket.After(now) {
			due = append(due, token)
		}
	}
	slices.SortFunc(due, func(a, b *trackedToken) int {
		if order := a.nextMarket.Compare(b.nextMarket); order != 0 {
			return order
		}
		return cmp.Compare(a.mint, b.mint)
	})
	return due[:min(len(due), MARKET_BATCH)]
}

func (t *tracking) safetyDue(now time.Time) *trackedToken {
	var due *trackedToken
	for _, token := range t.tokens {
		if token.safetyDone || token.safetyAttempts >= MAX_SAFETY_ATTEMPTS ||
			token.nextSafety.After(now) || now.Sub(token.start) >= EARLY_WINDOW {
			continue
		}
		if due == nil || token.nextSafety.Before(due.nextSafety) ||
			(token.nextSafety.Equal(due.nextSafety) && token.mint < due.mint) {
			due = token
		}
	}
	return due
}

func (t *tracking) applySample(row Record, request time.Time) {
	token := t.tokens[row.Mint]
	if token == nil {
		return
	}
	if row.Tracking == TRACK_UNLISTED {
		delete(t.tokens, row.Mint)
		return
	}
	// An in-flight curve quote cannot push out the first post-graduation lookup.
	if !request.Before(token.graduated) {
		token.nextMarket = nextSample(token.start, request)
		if !token.graduated.IsZero() && request.Sub(token.graduated) < MIGRATION_INDEXING &&
			(row.Market == nil || liquidityUSD(*row.Market) <= 0) {
			token.nextMarket = minTime(token.nextMarket, request.Add(MIGRATION_RETRY))
		}
	}
	if row.Market != nil {
		trades := row.Market.Txns[WINDOW_FIVE_MIN]
		if trades.Buys > 0 || trades.Sells > 0 {
			token.lastActivity = row.At
		}
	}
}
