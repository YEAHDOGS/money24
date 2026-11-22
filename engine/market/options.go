package market

import (
	"context"
	"fmt"
	"math"
	"net/url"
	"time"
)

const (
	OPTIONS_PATH   = "/v7/finance/options/%s?date=%d"
	CHAIN_PATH     = "/v7/finance/options/%s?straddle=false"
	RIGHT_CALL     = "C"
	RIGHT_PUT      = "P"
	STRIKE_EPSILON = 0.001
	EXPIRY_LAYOUT  = "2006-01-02"
)

// OptionQuote is one listed contract and the stock under it, from Yahoo's options chain. Yahoo's
// option quotes are delayed like its stock quotes; open interest is as of the last close.
type OptionQuote struct {
	Contract     string  `json:"contract"` // OCC symbol, e.g. AAL261120P00012500
	Bid          float64 `json:"bid"`
	Ask          float64 `json:"ask"`
	Last         float64 `json:"last"`
	Volume       int     `json:"volume"`
	OpenInterest int     `json:"open_interest"`
	IV           float64 `json:"iv"` // implied volatility as a decimal: 0.42 is 42%
	LastTradeAt  int64   `json:"last_trade_at"`

	Spot          float64 `json:"spot"`
	PreviousClose float64 `json:"previous_close"`
	DayVolume     float64 `json:"day_volume"`
	AvgVolume     float64 `json:"avg_volume"` // three-month average daily volume
	NextEarnings  int64   `json:"next_earnings,omitempty"`
	MarketTime    int64   `json:"market_time"`
}

// Option looks up one contract. expiry is YYYY-MM-DD, right is C or P. ErrNotFound means Yahoo
// lists no such strike on that date.
func (c *Client) Option(ctx context.Context, symbol, expiry, right string, strike float64) (OptionQuote, error) {
	day, err := time.Parse(EXPIRY_LAYOUT, expiry)
	if err != nil || !SYMBOL.MatchString(symbol) || (right != RIGHT_CALL && right != RIGHT_PUT) {
		return OptionQuote{}, ErrBadInput
	}
	type contract struct {
		Contract     string  `json:"contractSymbol"`
		Strike       float64 `json:"strike"`
		Bid          float64 `json:"bid"`
		Ask          float64 `json:"ask"`
		Last         float64 `json:"lastPrice"`
		Volume       int     `json:"volume"`
		OpenInterest int     `json:"openInterest"`
		IV           float64 `json:"impliedVolatility"`
		LastTrade    int64   `json:"lastTradeDate"`
	}
	var res struct {
		OptionChain struct {
			Result []struct {
				Quote struct {
					Price         float64 `json:"regularMarketPrice"`
					PreviousClose float64 `json:"regularMarketPreviousClose"`
					Volume        float64 `json:"regularMarketVolume"`
					AvgVolume     float64 `json:"averageDailyVolume3Month"`
					Earnings      int64   `json:"earningsTimestamp"`
					MarketTime    int64   `json:"regularMarketTime"`
				} `json:"quote"`
				Options []struct {
					Calls []contract `json:"calls"`
					Puts  []contract `json:"puts"`
				} `json:"options"`
			} `json:"result"`
		} `json:"optionChain"`
	}
	path := fmt.Sprintf(OPTIONS_PATH, url.PathEscape(symbol), day.Unix())
	if err := c.get(ctx, path, true, &res); err != nil {
		return OptionQuote{}, err
	}
	if len(res.OptionChain.Result) == 0 || len(res.OptionChain.Result[0].Options) == 0 {
		return OptionQuote{}, ErrNotFound
	}
	r := res.OptionChain.Result[0]
	list := r.Options[0].Calls
	if right == RIGHT_PUT {
		list = r.Options[0].Puts
	}
	for _, o := range list {
		if math.Abs(o.Strike-strike) > STRIKE_EPSILON {
			continue
		}
		// Earnings already past are not a coming event.
		earnings := r.Quote.Earnings
		if earnings < time.Now().Unix() {
			earnings = 0
		}
		return OptionQuote{Contract: o.Contract, Bid: o.Bid, Ask: o.Ask, Last: o.Last, Volume: o.Volume, OpenInterest: o.OpenInterest,
			IV: o.IV, LastTradeAt: o.LastTrade, Spot: r.Quote.Price, PreviousClose: r.Quote.PreviousClose, DayVolume: r.Quote.Volume,
			AvgVolume: r.Quote.AvgVolume, NextEarnings: earnings, MarketTime: r.Quote.MarketTime}, nil
	}
	return OptionQuote{}, ErrNotFound
}

// OptionRow is one strike of a chain.
type OptionRow struct {
	Contract     string  `json:"contract"`
	Strike       float64 `json:"strike"`
	Bid          float64 `json:"bid"`
	Ask          float64 `json:"ask"`
	Last         float64 `json:"last"`
	Volume       int     `json:"volume"`
	OpenInterest int     `json:"open_interest"`
	IV           float64 `json:"iv"`         // implied volatility as a decimal
	LastTradeAt  int64   `json:"last_trade"` // Unix seconds of the strike's last trade
}

// Chain is every strike a symbol lists on one expiry, and every expiry it lists at all.
type Chain struct {
	Spot       float64     `json:"spot"`
	MarketTime int64       `json:"market_time"` // Unix seconds the quote is from: Yahoo's chains run about 15 minutes behind
	Expiry     int64       `json:"expiry"`      // Unix seconds, as Yahoo keys its chains
	Expiries   []int64     `json:"expiries"`    // soonest first
	Calls      []OptionRow `json:"calls"`
	Puts       []OptionRow `json:"puts"`
}

// Chain reads one expiry of a symbol's option chain. With expiry 0 it reads the nearest one,
// which is how the list of expiries is first learned.
func (c *Client) Chain(ctx context.Context, symbol string, expiry int64) (Chain, error) {
	if !SYMBOL.MatchString(symbol) {
		return Chain{}, ErrBadInput
	}
	type row struct {
		Contract     string  `json:"contractSymbol"`
		Strike       float64 `json:"strike"`
		Bid          float64 `json:"bid"`
		Ask          float64 `json:"ask"`
		Last         float64 `json:"lastPrice"`
		Volume       int     `json:"volume"`
		OpenInterest int     `json:"openInterest"`
		IV           float64 `json:"impliedVolatility"`
		LastTradeAt  int64   `json:"lastTradeDate"`
	}
	var res struct {
		OptionChain struct {
			Result []struct {
				Expiries []int64 `json:"expirationDates"`
				Quote    struct {
					Price      float64 `json:"regularMarketPrice"`
					MarketTime int64   `json:"regularMarketTime"`
				} `json:"quote"`
				Options []struct {
					Expiry int64 `json:"expirationDate"`
					Calls  []row `json:"calls"`
					Puts   []row `json:"puts"`
				} `json:"options"`
			} `json:"result"`
		} `json:"optionChain"`
	}
	path := fmt.Sprintf(CHAIN_PATH, url.PathEscape(symbol))
	if expiry > 0 {
		path = fmt.Sprintf(OPTIONS_PATH, url.PathEscape(symbol), expiry)
	}
	if err := c.get(ctx, path, true, &res); err != nil {
		return Chain{}, err
	}
	if len(res.OptionChain.Result) == 0 || len(res.OptionChain.Result[0].Options) == 0 {
		return Chain{}, ErrNotFound
	}
	r := res.OptionChain.Result[0]
	out := Chain{Spot: r.Quote.Price, MarketTime: r.Quote.MarketTime, Expiry: r.Options[0].Expiry, Expiries: r.Expiries, Calls: []OptionRow{}, Puts: []OptionRow{}}
	for _, o := range r.Options[0].Calls {
		out.Calls = append(out.Calls, OptionRow(o))
	}
	for _, o := range r.Options[0].Puts {
		out.Puts = append(out.Puts, OptionRow(o))
	}
	return out, nil
}
