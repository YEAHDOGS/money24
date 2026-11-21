package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/YEAHDOGS/money24/engine/coinbase"
	"github.com/YEAHDOGS/money24/engine/dexscreener"
	"github.com/YEAHDOGS/money24/engine/market"
	"github.com/YEAHDOGS/money24/engine/signals"
	"github.com/YEAHDOGS/money24/engine/webull"
)

const (
	ENV_WEBULL_KEY    = "WEBULL_APP_KEY"
	ENV_WEBULL_SECRET = "WEBULL_APP_SECRET"
	ENV_WEBULL_TOKEN  = "WEBULL_ACCESS_TOKEN" // only for apps with 2FA on
	ENV_WEBULL_ENV    = "WEBULL_ENV"          // sandbox (default) or production
	QUOTE_TIMEOUT     = 6 * time.Second
	// Assumed venue fee on each side of an on-chain trade. A round number on the high side of what
	// pump.fun and the Solana pools charge, so results do not flatter.
	POOL_FEE_PCT     = 1.0
	COIN_PAIR_SUFFIX = "-USD"
)

// webullClient builds the quote client from the environment, or returns nil when no key is set.
func webullClient() (*webull.Client, error) {
	key, secret := os.Getenv(ENV_WEBULL_KEY), os.Getenv(ENV_WEBULL_SECRET)
	if key == "" && secret == "" {
		return nil, nil
	}
	return webull.New(webull.Config{AppKey: key, AppSecret: secret, Token: os.Getenv(ENV_WEBULL_TOKEN), Env: os.Getenv(ENV_WEBULL_ENV)})
}

// snapshotFor asks for the option contract when the signal names one, otherwise the stock.
func snapshotFor(ctx context.Context, client *webull.Client, sig signals.Signal) (webull.Snapshot, error) {
	if sig.Right == "" || sig.Expiry == "" {
		return client.StockSnapshot(ctx, sig.Symbol)
	}
	expiry, err := time.Parse(signals.EXPIRY_LAYOUT, sig.Expiry)
	if err != nil {
		return webull.Snapshot{}, fmt.Errorf("expiry %q: %w", sig.Expiry, err)
	}
	return client.OptionSnapshot(ctx, webull.OCCSymbol(sig.Symbol, sig.Right, sig.Strike, expiry))
}

// liveQuotes prices on-chain tokens from DEX Screener and listed coins from Coinbase, neither of
// which needs a key, and everything else from Webull when a key is set, or else from Yahoo's
// delayed option chain and quotes, so a play is priced with no keys at all. A failed lookup
// is logged and reported as "no quote", so the runner plans at the called price and notes what it
// could not check.
func liveQuotes(client *webull.Client, prices *market.Client) signals.QuoteFunc {
	return func(sig signals.Signal) (signals.Quote, bool) {
		ctx, cancel := context.WithTimeout(context.Background(), QUOTE_TIMEOUT)
		defer cancel()
		if sig.Address != "" {
			pair, err := dexscreener.Fetch(ctx, http.DefaultClient, sig.Address)
			if err != nil {
				log.Printf("quote %s: %v", sig.Symbol, err)
				return signals.Quote{}, false
			}
			return signals.Quote{Bid: pair.PriceUSD, Ask: pair.PriceUSD, Volume: int(pair.Volume.Day), LiquidityUSD: pair.Liquidity.USD, FeePct: POOL_FEE_PCT}, true
		}
		if sig.Market == signals.MARKET_CRYPTO {
			coin, err := coinbase.Fetch(ctx, http.DefaultClient, sig.Symbol)
			if err != nil {
				log.Printf("quote %s: %v", sig.Symbol, err)
				return signals.Quote{}, false
			}
			return signals.Quote{Bid: coin.Bid, Ask: coin.Ask, Volume: int(coin.Volume)}, true
		}
		if client == nil {
			return yahooQuote(ctx, prices, sig)
		}
		snap, err := snapshotFor(ctx, client, sig)
		if err != nil {
			log.Printf("quote %s: %v", sig.Symbol, err)
			return signals.Quote{}, false
		}
		return signals.Quote{Bid: float64(snap.Bid), Ask: float64(snap.Ask), OpenInterest: int(snap.OpenInterest), Volume: int(snap.Volume)}, true
	}
}

// printQuote looks up one symbol (a ticker, or a full option symbol such as AAPL260522C00300000)
// and prints what Webull returns: the quickest way to check a new key.
func printQuote(client *webull.Client, symbol string) error {
	ctx, cancel := context.WithTimeout(context.Background(), QUOTE_TIMEOUT)
	defer cancel()
	snap, err := client.OptionSnapshot(ctx, symbol)
	if len(symbol) <= MAX_TICKER_CHARS {
		snap, err = client.StockSnapshot(ctx, symbol)
	}
	if err != nil {
		return err
	}
	fmt.Printf("%s  bid %.3f x %.0f  ask %.3f x %.0f  last %.3f  volume %.0f  open interest %.0f\n",
		snap.Symbol, snap.Bid, snap.BidSize, snap.Ask, snap.AskSize, snap.Price, snap.Volume, snap.OpenInterest)
	return nil
}

// priceHistory reads past candles for scoring a source's old calls. Coins are looked up as their
// US dollar pair. An option call is followed on its underlying stock: old option prices have no
// free source, and the stock's direction is what the call was betting on.
func priceHistory(client *market.Client) signals.HistoryFunc {
	return func(ctx context.Context, sig signals.Signal, from, to time.Time) ([]signals.PriceBar, error) {
		symbol := sig.Symbol
		if sig.Market == signals.MARKET_CRYPTO {
			symbol += COIN_PAIR_SUFFIX
		}
		chart, err := client.History(ctx, symbol, from, to)
		if err != nil {
			return nil, err
		}
		bars := make([]signals.PriceBar, 0, len(chart.Candles))
		for _, c := range chart.Candles {
			bars = append(bars, signals.PriceBar{At: time.Unix(c.T, 0), Open: c.O, High: c.H, Low: c.L, Close: c.C})
		}
		return bars, nil
	}
}

// yahooQuote is the keyless fallback: the option's bid and ask from Yahoo's chain, or a stock's
// last price a cent either side. Delayed, so exits trail the live market by minutes.
func yahooQuote(ctx context.Context, prices *market.Client, sig signals.Signal) (signals.Quote, bool) {
	if sig.Right != "" && sig.Expiry != "" {
		q, err := prices.Option(ctx, sig.Symbol, sig.Expiry, sig.Right, sig.Strike)
		if err != nil {
			log.Printf("quote %s (yahoo): %v", sig.Symbol, err)
			return signals.Quote{}, false
		}
		return signals.Quote{Bid: q.Bid, Ask: q.Ask, OpenInterest: q.OpenInterest, Volume: q.Volume}, true
	}
	ticks, err := prices.Ticks(ctx, []string{sig.Symbol})
	if err != nil || len(ticks) == 0 || ticks[0].Price <= 0 {
		return signals.Quote{}, false
	}
	return signals.Quote{Bid: ticks[0].Price - signals.PRICE_TICK, Ask: ticks[0].Price + signals.PRICE_TICK}, true
}
