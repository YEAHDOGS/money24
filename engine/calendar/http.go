package calendar

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"
)

const (
	ROUTE         = "GET /api/day" // the ServeMux pattern to mount Handler at
	PARAM_DAYS    = "days"
	DEFAULT_DAYS  = 1
	MAX_DAYS      = 31
	CACHE_CONTROL = "public, max-age=60" // a day's label changes at most at the open and at midnight
)

// Handler serves the calendar. Mount it at ROUTE.
//
//	GET /api/day?days=N   today in New York and the days after it: N of them, 1 to 31, 1 when left out
//
// It answers {"now", "stocks_open", "days": [Day...]}. A days value that is not a number, or is
// out of range, is pulled into range rather than refused.
func Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		days, err := strconv.Atoi(r.URL.Query().Get(PARAM_DAYS))
		if err != nil {
			days = DEFAULT_DAYS
		}
		now := time.Now()
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", CACHE_CONTROL)
		json.NewEncoder(w).Encode(struct {
			Now        time.Time `json:"now"`
			StocksOpen bool      `json:"stocks_open"` // the regular session is open right now
			Days       []Day     `json:"days"`
		}{now.UTC(), StocksOpen(now), Ahead(now, min(max(days, DEFAULT_DAYS), MAX_DAYS))})
	})
}
