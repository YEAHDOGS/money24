package webull

import (
	"bytes"
	"context"
	"net/url"
	"strconv"
)

const (
	PARAM_SYMBOLS       = "symbols"
	PARAM_CATEGORY      = "category"
	PARAM_EXTENDED      = "extend_hour_required"
	EXTENDED_ON         = "true"
	JSON_NULL           = "null"
	JSON_QUOTE     byte = '"'
)

// Number reads Webull's numeric fields, which arrive as strings ("185.50"), bare numbers, empty
// strings when there was no trade, or null. Anything unreadable is zero.
type Number float64

func (n *Number) UnmarshalJSON(raw []byte) error {
	raw = bytes.Trim(raw, string(JSON_QUOTE))
	if len(raw) == 0 || string(raw) == JSON_NULL {
		*n = 0
		return nil
	}
	value, err := strconv.ParseFloat(string(raw), 64)
	if err != nil {
		value = 0
	}
	*n = Number(value)
	return nil
}

// Snapshot is the top of the book for one stock, ETF or option contract.
type Snapshot struct {
	Symbol       string `json:"symbol"`
	Price        Number `json:"price"` // last trade
	Bid          Number `json:"bid"`
	Ask          Number `json:"ask"`
	BidSize      Number `json:"bid_size"`
	AskSize      Number `json:"ask_size"`
	Volume       Number `json:"volume"`        // shares, or contracts for options
	OpenInterest Number `json:"open_interest"` // options only
	ImpliedVol   Number `json:"imp_vol"`       // options only, as a decimal: 0.609 is 60.9%
	Delta        Number `json:"delta"`         // options only
	LastTradeAt  int64  `json:"last_trade_time"`
}

func (c *Client) snapshot(ctx context.Context, path, symbol, category string, extra url.Values) (Snapshot, error) {
	query := url.Values{PARAM_SYMBOLS: {symbol}, PARAM_CATEGORY: {category}}
	for name, values := range extra {
		query[name] = values
	}
	var rows []Snapshot
	if err := c.get(ctx, path, query, &rows); err != nil {
		return Snapshot{}, err
	}
	if len(rows) == 0 {
		return Snapshot{}, ErrNotFound
	}
	return rows[0], nil
}

// StockSnapshot returns the current quote for a US stock or ETF, pre-market and after-hours
// included. Webull files the two under different categories, so an empty stock answer is retried
// as an ETF.
func (c *Client) StockSnapshot(ctx context.Context, symbol string) (Snapshot, error) {
	extended := url.Values{PARAM_EXTENDED: {EXTENDED_ON}}
	snap, err := c.snapshot(ctx, PATH_STOCK_SNAPSHOT, symbol, CATEGORY_STOCK, extended)
	if err == nil {
		return snap, nil
	}
	if etf, etfErr := c.snapshot(ctx, PATH_STOCK_SNAPSHOT, symbol, CATEGORY_ETF, extended); etfErr == nil {
		return etf, nil
	}
	return Snapshot{}, err
}

// OptionSnapshot returns the current quote for one option contract, named by OCCSymbol.
func (c *Client) OptionSnapshot(ctx context.Context, occSymbol string) (Snapshot, error) {
	return c.snapshot(ctx, PATH_OPTION_SNAP, occSymbol, CATEGORY_OPTION, nil)
}
