package meme

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/YEAHDOGS/money24/engine/dexscreener"
)

const (
	DEX_SOURCE        = "dexscreener"
	RUG_SOURCE        = "rugcheck"
	RUG_URL           = "https://api.rugcheck.xyz/v1/tokens/%s/report"
	WINDOW_FIVE_MIN   = "m5"
	SOLANA_CHAIN      = "solana"
	HEADER_RETRY      = "Retry-After"
	MAX_LOOKUP_BYTES  = 4 << 20
	LOOKUP_TIMEOUT    = 8 * time.Second
	HTTP_ERROR_DELAY  = 15 * time.Second
	RATE_LIMIT_DELAY  = time.Minute
	RETRY_SECONDS_MAX = int64(^uint64(0)>>1) / int64(time.Second)
)

// Market holds the selected pool at observation time. Transaction counts and
// USD volume are rolling provider windows (m5, h1, h6 and h24).
type Market struct {
	Venue       string                `json:"dexId"`
	Chain       string                `json:"chainId"`
	Pool        string                `json:"pairAddress"`
	Base        Token                 `json:"baseToken"`
	Quote       Token                 `json:"quoteToken"`
	PriceUSD    *float64              `json:"priceUsd,omitempty,string"`
	PriceNative *float64              `json:"priceNative,omitempty,string"`
	MarketCap   *float64              `json:"marketCap,omitempty"`
	FDV         *float64              `json:"fdv,omitempty"`
	Liquidity   *Liquidity            `json:"liquidity,omitempty"`
	Volume      map[string]float64    `json:"volume,omitempty"`
	Txns        map[string]TradeCount `json:"txns,omitempty"`
	CreatedAtMs *int64                `json:"pairCreatedAt,omitempty"`
}

type Token struct {
	Address string `json:"address"`
	Name    string `json:"name,omitempty"`
	Symbol  string `json:"symbol,omitempty"`
}

type Liquidity struct {
	USD   *float64 `json:"usd,omitempty"`
	Base  *float64 `json:"base,omitempty"`
	Quote *float64 `json:"quote,omitempty"`
}

type TradeCount struct {
	Buys  int `json:"buys"`
	Sells int `json:"sells"`
}

// Safety records the first available RugCheck report in the early lookup window.
// Authorities are null when RugCheck explicitly reports they have been revoked.
type Safety struct {
	Creator         string          `json:"creator,omitempty"`
	Metadata        Token           `json:"tokenMeta"`
	MintAuthority   json.RawMessage `json:"mintAuthority,omitempty"`
	FreezeAuthority json.RawMessage `json:"freezeAuthority,omitempty"`
	TotalHolders    *int            `json:"totalHolders,omitempty"`
	PriceUSD        *float64        `json:"price,omitempty"`
	LiquidityUSD    *float64        `json:"totalMarketLiquidity,omitempty"`
	TopHolders      []HolderShare   `json:"topHolders,omitempty"`
	Markets         []SafetyMarket  `json:"markets,omitempty"`
	Risks           []Risk          `json:"risks,omitempty"`
	Rugged          *bool           `json:"rugged,omitempty"`
	Score           *int            `json:"score,omitempty"`
	ScoreNormalised *int            `json:"score_normalised,omitempty"`
	CreatorTokens   json.RawMessage `json:"creatorTokens,omitempty"`
	DetectedAt      string          `json:"detectedAt,omitempty"`
}

type HolderShare struct {
	Percent *float64 `json:"pct,omitempty"`
	Insider *bool    `json:"insider,omitempty"`
}

type SafetyMarket struct {
	Pool  string  `json:"pubkey"`
	Venue string  `json:"marketType"`
	LP    *LPLock `json:"lp,omitempty"`
}

type LPLock struct {
	LockedPct *float64 `json:"lpLockedPct,omitempty"`
	LockedUSD *float64 `json:"lpLockedUSD,omitempty"`
}

type Risk struct {
	Name  string `json:"name"`
	Level string `json:"level"`
	Value string `json:"value,omitempty"`
}

type httpFailure struct {
	code  int
	retry string
}

func (e *httpFailure) Error() string {
	return fmt.Sprintf("HTTP %d from data provider", e.code)
}

func retryDelay(err error, now time.Time) time.Duration {
	var response *httpFailure
	if !errors.As(err, &response) ||
		(response.code != http.StatusTooManyRequests && response.code != http.StatusServiceUnavailable) {
		return HTTP_ERROR_DELAY
	}
	if n, err := strconv.ParseInt(response.retry, 10, 64); err == nil && n >= 0 {
		return max(time.Duration(min(n, RETRY_SECONDS_MAX))*time.Second, HTTP_ERROR_DELAY)
	}
	if at, err := http.ParseTime(response.retry); err == nil {
		return max(at.Sub(now), HTTP_ERROR_DELAY)
	}
	return RATE_LIMIT_DELAY
}

func readJSON(ctx context.Context, client *http.Client, url string, target any) error {
	ctx, cancel := context.WithTimeout(ctx, LOOKUP_TIMEOUT)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set(dexscreener.HEADER_USER_AGENT, dexscreener.USER_AGENT)
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return &httpFailure{code: res.StatusCode, retry: res.Header.Get(HEADER_RETRY)}
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, MAX_LOOKUP_BYTES+1))
	if err != nil {
		return err
	}
	if len(data) > MAX_LOOKUP_BYTES {
		return fmt.Errorf("data provider response too large")
	}
	return json.Unmarshal(data, target)
}

func liquidityUSD(pair Market) float64 {
	if pair.Liquidity == nil || pair.Liquidity.USD == nil {
		return 0
	}
	return *pair.Liquidity.USD
}

func betterMarket(pair, current Market) bool {
	if liquidityUSD(pair) != liquidityUSD(current) {
		return liquidityUSD(pair) > liquidityUSD(current)
	}
	if (pair.PriceUSD != nil) != (current.PriceUSD != nil) {
		return pair.PriceUSD != nil
	}
	return pair.Pool < current.Pool
}

func fetchMarket(ctx context.Context, client *http.Client, url string, mints []string) (map[string]Market, error) {
	var pairs []Market
	if err := readJSON(ctx, client, fmt.Sprintf(url, strings.Join(mints, ",")), &pairs); err != nil {
		return nil, err
	}
	selected := make(map[string]Market, len(mints))
	requested := make(map[string]bool, len(mints))
	for _, mint := range mints {
		requested[mint] = true
	}
	for _, pair := range pairs {
		if pair.Chain != SOLANA_CHAIN || pair.Pool == "" || !requested[pair.Base.Address] {
			continue
		}
		mint := pair.Base.Address
		if current, ok := selected[mint]; !ok || betterMarket(pair, current) {
			selected[mint] = pair
		}
	}
	return selected, nil
}

func fetchSafety(ctx context.Context, client *http.Client, url, mint string) (*Safety, error) {
	var response struct {
		Mint string `json:"mint"`
		Safety
	}
	if err := readJSON(ctx, client, fmt.Sprintf(url, mint), &response); err != nil {
		return nil, err
	}
	if response.Mint != mint {
		return nil, fmt.Errorf("RugCheck report mint mismatch")
	}
	return &response.Safety, nil
}
