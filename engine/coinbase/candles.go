package coinbase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"time"
)

const (
	CANDLES_PATH      = "/products/%s-%s/candles"
	CANDLES_MAX       = 300 // the most candles one request returns
	CANDLE_FIELDS     = 6   // time, low, high, open, close, volume
	MAX_CANDLE_BYTES  = 1 << 18
	PARAM_GRANULARITY = "granularity"
	PARAM_START       = "start"
	PARAM_END         = "end"
	FIELD_TIME        = 0
	FIELD_LOW         = 1
	FIELD_HIGH        = 2
	FIELD_OPEN        = 3
	FIELD_CLOSE       = 4
	FIELD_VOLUME      = 5
	GRANULARITY_HOUR  = time.Hour
	MIN_GRANULARITY   = time.Minute
)

var (
	ErrRateLimited = errors.New("coinbase: rate limited")
	ErrBadRange    = fmt.Errorf("coinbase: a candle request covers at most %d candles", CANDLES_MAX)
)

// Candle is one bar of trading against the US dollar. T is the bar's opening time, Unix seconds.
type Candle struct {
	T int64   `json:"t"`
	O float64 `json:"o"`
	H float64 `json:"h"`
	L float64 `json:"l"`
	C float64 `json:"c"`
	V float64 `json:"v"` // coins traded
}

// Candles returns the bars for symbol (BTC or BTC-USD) between start and end, oldest first. With
// both times zero it returns the latest CANDLES_MAX. The newest bar may still be forming. An hour
// in which nothing traded has no bar.
func Candles(ctx context.Context, client *http.Client, symbol string, granularity time.Duration, start, end time.Time) ([]Candle, error) {
	base, err := Base(symbol)
	if err != nil {
		return nil, err
	}
	if granularity < MIN_GRANULARITY {
		return nil, fmt.Errorf("coinbase: granularity %s is under %s", granularity, MIN_GRANULARITY)
	}
	query := url.Values{PARAM_GRANULARITY: {strconv.Itoa(int(granularity / time.Second))}}
	if !start.IsZero() && !end.IsZero() {
		if end.Sub(start) > granularity*CANDLES_MAX {
			return nil, ErrBadRange
		}
		query.Set(PARAM_START, start.UTC().Format(time.RFC3339))
		query.Set(PARAM_END, end.UTC().Format(time.RFC3339))
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiBase+fmt.Sprintf(CANDLES_PATH, base, QUOTE_CURRENCY)+"?"+query.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set(HEADER_USER_AGENT, USER_AGENT)
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	switch res.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return nil, fmt.Errorf("%w: %s", ErrNotFound, base)
	case http.StatusTooManyRequests:
		return nil, ErrRateLimited
	default:
		return nil, fmt.Errorf("coinbase: HTTP %d for %s candles", res.StatusCode, base)
	}
	var rows [][]float64
	if err := json.NewDecoder(io.LimitReader(res.Body, MAX_CANDLE_BYTES)).Decode(&rows); err != nil {
		return nil, fmt.Errorf("coinbase: %s candles: %w", base, err)
	}
	out := make([]Candle, 0, len(rows))
	for _, row := range rows {
		if len(row) < CANDLE_FIELDS {
			continue
		}
		out = append(out, Candle{T: int64(row[FIELD_TIME]), L: row[FIELD_LOW], H: row[FIELD_HIGH], O: row[FIELD_OPEN], C: row[FIELD_CLOSE], V: row[FIELD_VOLUME]})
	}
	sort.Slice(out, func(a, b int) bool { return out[a].T < out[b].T })
	return out, nil
}
