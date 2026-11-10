// Package dexscreener reads on-chain token prices from DEX Screener's public API: pump.fun,
// PumpSwap, Raydium and the other Solana venues, looked up by token address. It needs no key and
// places no orders. Docs: https://docs.dexscreener.com/api/reference
package dexscreener

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
)

const (
	TOKEN_URL          = "https://api.dexscreener.com/tokens/v1/solana/%s"
	PAIR_URL           = "https://api.dexscreener.com/latest/dex/pairs/solana/%s"
	USER_AGENT         = "money24-m24d/1.0"
	HEADER_USER_AGENT  = "User-Agent"
	MAX_RESPONSE_BYTES = 1 << 20
)

var (
	ErrNotFound   = errors.New("dexscreener: no market for address")
	ErrBadAddress = errors.New("dexscreener: not a Solana address")
	// Addresses come from parsed posts and go into a URL path, so anything but base58 is refused.
	ADDRESS = regexp.MustCompile(`^[1-9A-HJ-NP-Za-km-z]{32,44}$`)
)

// Pair is one pool a token trades in. LiquidityUSD is zero while a pump.fun coin is still on its
// bonding curve: the API reports no pool size until the coin graduates.
type Pair struct {
	Venue string `json:"dexId"` // pumpfun, pumpswap, raydium, ...
	Base  struct {
		Address string `json:"address"`
		Symbol  string `json:"symbol"`
		Name    string `json:"name"`
	} `json:"baseToken"`
	PriceUSD  float64 `json:"priceUsd,string"`
	Liquidity struct {
		USD float64 `json:"usd"`
	} `json:"liquidity"`
	Volume struct {
		Day float64 `json:"h24"` // US dollars traded in the last 24 hours
	} `json:"volume"`
	CreatedAtMs int64 `json:"pairCreatedAt"`
}

func fetch(ctx context.Context, client *http.Client, url string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set(HEADER_USER_AGENT, USER_AGENT)
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("dexscreener: HTTP %d", res.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(res.Body, MAX_RESPONSE_BYTES)).Decode(out)
}

// Fetch returns the deepest pool for a token address. Links to a chart carry the pool's address
// instead of the token's, so an address with no token market is retried as a pool.
func Fetch(ctx context.Context, client *http.Client, address string) (Pair, error) {
	if !ADDRESS.MatchString(address) {
		return Pair{}, ErrBadAddress
	}
	var pairs []Pair
	if err := fetch(ctx, client, fmt.Sprintf(TOKEN_URL, address), &pairs); err != nil {
		return Pair{}, err
	}
	if len(pairs) == 0 {
		var byPool struct {
			Pairs []Pair `json:"pairs"`
		}
		if err := fetch(ctx, client, fmt.Sprintf(PAIR_URL, address), &byPool); err != nil {
			return Pair{}, err
		}
		pairs = byPool.Pairs
	}
	if len(pairs) == 0 {
		return Pair{}, fmt.Errorf("%w: %s", ErrNotFound, address)
	}
	best := pairs[0]
	for _, p := range pairs[1:] {
		if p.Liquidity.USD > best.Liquidity.USD {
			best = p
		}
	}
	return best, nil
}
