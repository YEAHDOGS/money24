package predict

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"
)

const (
	// Scribe tips are hand-picked contracts: one fixed paper stake each, held to settlement.
	TIP_STRATEGY  = "scribe"
	TIP_NAME      = "Scribe tips"
	TIP_STAKE_USD = FAVORITES_STAKE_USD
	TIP_MAX_PRICE = 0.99
	TIP_NOTE      = "Scribe tip, started by hand"
	STATUS_ACTIVE = "active"
)

var (
	ErrTipDeskOff  = errors.New("the prediction desk is off, so nothing would settle this position")
	ErrTipPaused   = errors.New("the prediction desk hit today's loss stop; new positions resume tomorrow, New York time")
	ErrTipHeld     = errors.New("the prediction desk already holds this market")
	ErrTipCash     = errors.New("the prediction desk has too little paper cash for one stake")
	ErrTipNoMarket = errors.New("Kalshi does not list this contract as an open yes/no market")
	ErrTipNoFill   = errors.New("no contracts are offered within the stake right now")
)

// tipBlocked says why the desk may not open a hand-picked position. The caller holds d.mu.
func (d *Desk) tipBlocked(ticker string, now time.Time) error {
	switch {
	case d.off:
		return ErrTipDeskOff
	case d.Book.LostOn(Day(now)) >= d.rulebook.DailyLossUSD:
		return ErrTipPaused
	case d.Book.Holding(ticker) != nil:
		return ErrTipHeld
	case d.Book.Account("").CashUSD < TIP_STAKE_USD:
		return ErrTipCash
	}
	return nil
}

// BuyTip fills one hand-picked contract against the real order book with paper money. It obeys the
// desk's switch, daily loss stop and cash, and the position settles like any other.
func (d *Desk) BuyTip(ctx context.Context, ticker, side, note string) (Line, error) {
	if side != SIDE_YES && side != SIDE_NO {
		return Line{}, ErrTipNoMarket
	}
	d.mu.Lock()
	err := d.tipBlocked(ticker, time.Now())
	d.mu.Unlock()
	if err != nil {
		return Line{}, err
	}
	found, err := d.Kalshi.Lookup(ctx, []string{ticker})
	if err != nil {
		return Line{}, err
	}
	m, ok := found[ticker]
	if !ok || m.Type != MARKET_BINARY || (m.Status != STATUS_ACTIVE && m.Status != STATUS_OPEN_QUERY) || !m.CloseAt.After(time.Now()) {
		return Line{}, ErrTipNoMarket
	}
	asks, err := d.Kalshi.Asks(ctx, ticker, side)
	if err != nil {
		return Line{}, err
	}
	qty, cost, fee, levels, average := fillFrom(asks, TIP_MAX_PRICE, TIP_STAKE_USD)
	if qty <= 0 {
		return Line{}, ErrTipNoFill
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	at := time.Now()
	if err := d.tipBlocked(ticker, at); err != nil {
		return Line{}, err
	}
	pick := candidate{market: m, side: side, spread: m.Ask(side) - m.Bid(side), reasons: []string{TIP_NOTE}}
	if note = strings.TrimSpace(note); note != "" {
		pick.reasons = append(pick.reasons, note)
	}
	line := pick.line(TIP_STRATEGY, KIND_BUY, at)
	line.Price, line.Qty, line.CostUSD, line.FeeUSD = round(average, PRICE_DIGITS), qty, cost, fee
	line.Note = fmt.Sprintf(NOTE_FILL, levels, asks[0].Price) + REASONS_JOIN + line.Note
	line, err = d.Book.Append(line)
	if err != nil {
		return Line{}, err
	}
	log.Printf("%s%s bought %.0f %s %s at %.4f, $%.2f + $%.2f fee", LOG_PREFIX, TIP_STRATEGY, qty, strings.ToUpper(side), ticker, line.Price, cost, fee)
	return line, nil
}
