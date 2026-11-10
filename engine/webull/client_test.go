package webull

import (
	"encoding/json"
	"net/url"
	"testing"
	"time"
)

// The worked example on Webull's signature page: a wrong signature means every request is refused,
// and there is no way to see that without live credentials, so this vector is pinned.
func TestSignMatchesWebullExample(t *testing.T) {
	query := url.Values{"a1": {"webull"}, "a2": {"123"}, "a3": {"xxx"}, "q1": {"yyy"}}
	signed := map[string]string{
		HEADER_APP_KEY:     "776da210ab4a452795d74e726ebd74b6",
		HEADER_TIMESTAMP:   "2022-01-04T03:55:31Z",
		HEADER_SIG_VERSION: "1.0",
		HEADER_ALGORITHM:   "HMAC-SHA1",
		HEADER_NONCE:       "48ef5afed43d4d91ae514aaeafbc29ba",
		HEADER_HOST:        "api.webull.com",
	}
	body := []byte(`{"k1":123,"k2":"this is the api request body","k3":true,"k4":{"foo":[1,2]}}`)
	got := Sign("/trade/place_order", query, signed, body, "0f50a2e853334a9aae1a783bee120c1f")
	if want := "kvlS6opdZDhEBo5jq40nHYXaLvM="; got != want {
		t.Fatalf("signature = %q, want %q", got, want)
	}
}

func TestOCCSymbol(t *testing.T) {
	expiry := time.Date(2026, 5, 22, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		root, right string
		strike      float64
		want        string
	}{
		{"AAPL", "C", 300, "AAPL260522C00300000"},
		{"spy", "p", 587.5, "SPY260522P00587500"},
		{"F", "C", 13.5, "F260522C00013500"},
	}
	for _, c := range cases {
		if got := OCCSymbol(c.root, c.right, c.strike, expiry); got != c.want {
			t.Errorf("OCCSymbol(%s %s %v) = %q, want %q", c.root, c.right, c.strike, got, c.want)
		}
	}
}

func TestSnapshotReadsStringsNumbersAndBlanks(t *testing.T) {
	raw := `[{"symbol":"AAPL260522C00300000","price":"47.35","bid":"47.345","ask":47.355,"bid_size":"1","ask_size":"2","volume":"","open_interest":null,"imp_vol":"0.609","last_trade_time":1710849600000}]`
	var rows []Snapshot
	if err := json.Unmarshal([]byte(raw), &rows); err != nil {
		t.Fatal(err)
	}
	s := rows[0]
	if s.Bid != 47.345 || s.Ask != 47.355 || s.Volume != 0 || s.OpenInterest != 0 || s.ImpliedVol != 0.609 || s.LastTradeAt != 1710849600000 {
		t.Fatalf("unexpected snapshot: %+v", s)
	}
}
