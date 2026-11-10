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
	// pump.fun and the Solana pools charge, so paper results do not flatter.
	POOL_FEE_PCT = 1.0
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
// which needs a key, and everything else from Webull when a key is set. A failed lookup is logged and reported as "no quote", so the runner plans at the
// called price and notes what it could not check.
func liveQuotes(client *webull.Client) signals.QuoteFunc {
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
			return signals.Quote{}, false
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
