// Package webull is a small client for the market-data half of Webull's OpenAPI (US region):
// signed HTTP requests, and top-of-book snapshots for stocks, ETFs and option contracts.
// It places no orders. Docs: https://developer.webull.com/apis/docs/
package webull

import (
	"context"
	"crypto/hmac"
	"crypto/md5"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

const (
	HOST_PRODUCTION     = "api.webull.com"
	HOST_SANDBOX        = "api.sandbox.webull.com"
	ENV_PRODUCTION      = "production"
	ENV_SANDBOX         = "sandbox"
	SCHEME              = "https://"
	PATH_STOCK_SNAPSHOT = "/market-data/stocks/snapshots/list"
	PATH_OPTION_SNAP    = "/market-data/options/snapshots/list"
	CATEGORY_STOCK      = "US_STOCK"
	CATEGORY_ETF        = "US_ETF"
	CATEGORY_OPTION     = "US_OPTION"
	HEADER_APP_KEY      = "x-app-key"
	HEADER_TIMESTAMP    = "x-timestamp"
	HEADER_SIGNATURE    = "x-signature"
	HEADER_ALGORITHM    = "x-signature-algorithm"
	HEADER_SIG_VERSION  = "x-signature-version"
	HEADER_NONCE        = "x-signature-nonce"
	HEADER_VERSION      = "x-version"
	HEADER_TOKEN        = "x-access-token"
	HEADER_HOST         = "host"
	ALGORITHM           = "HMAC-SHA1"
	SIGNATURE_VERSION   = "1.0"
	API_VERSION         = "v3"
	TIMESTAMP_LAYOUT    = "2006-01-02T15:04:05Z"
	NONCE_BYTES         = 16
	SECRET_SUFFIX       = "&" // the signing key is the app secret followed by this
	MAX_RESPONSE_BYTES  = 1 << 20
	ERROR_SNIPPET       = 200
	REQUEST_TIMEOUT     = 5 * time.Second
	OCC_EXPIRY_LAYOUT   = "060102"
	OCC_STRIKE_SCALE    = 1000 // OCC strikes are thousandths of a dollar, eight digits
)

var (
	ErrNotFound = errors.New("webull: no data for symbol")
	// Python's quote(s, safe="") writes a space as %20 where Go's QueryEscape writes "+".
	SPACE_FIXER = strings.NewReplacer("+", "%20")
)

// Config is what a Client needs. Token is only for apps with 2FA turned on (production default).
type Config struct {
	AppKey    string
	AppSecret string
	Token     string
	Env       string // ENV_SANDBOX (default) or ENV_PRODUCTION
}

// Client makes signed market-data requests.
type Client struct {
	cfg  Config
	host string
	http *http.Client
}

// New returns a client for the sandbox unless cfg.Env says production.
func New(cfg Config) (*Client, error) {
	if cfg.AppKey == "" || cfg.AppSecret == "" {
		return nil, errors.New("webull: app key and app secret are required")
	}
	host := HOST_SANDBOX
	switch cfg.Env {
	case "", ENV_SANDBOX:
	case ENV_PRODUCTION:
		host = HOST_PRODUCTION
	default:
		return nil, fmt.Errorf("webull: unknown environment %q (use %s or %s)", cfg.Env, ENV_SANDBOX, ENV_PRODUCTION)
	}
	return &Client{cfg: cfg, host: host, http: &http.Client{Timeout: REQUEST_TIMEOUT}}, nil
}

// Sign builds Webull's request signature: the query parameters and the six signed headers, sorted
// by name and joined, prefixed with the path, suffixed with the body's MD5 when there is a body,
// URL-encoded, then HMAC-SHA1 under the app secret plus "&", in base64.
func Sign(path string, query url.Values, signed map[string]string, body []byte, secret string) string {
	params := map[string]string{}
	for name, values := range query {
		sorted := append([]string(nil), values...)
		sort.Strings(sorted)
		params[name] = strings.Join(sorted, "&")
	}
	for name, value := range signed {
		params[name] = value
	}
	names := make([]string, 0, len(params))
	for name := range params {
		names = append(names, name)
	}
	sort.Strings(names)
	pairs := make([]string, 0, len(names))
	for _, name := range names {
		pairs = append(pairs, name+"="+params[name])
	}
	text := path + "&" + strings.Join(pairs, "&")
	if len(body) > 0 {
		sum := md5.Sum(body)
		text += "&" + strings.ToUpper(hex.EncodeToString(sum[:]))
	}
	mac := hmac.New(sha1.New, []byte(secret+SECRET_SUFFIX))
	mac.Write([]byte(SPACE_FIXER.Replace(url.QueryEscape(text))))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

// get sends one signed GET and decodes the JSON answer into out.
func (c *Client) get(ctx context.Context, path string, query url.Values, out any) error {
	nonce := make([]byte, NONCE_BYTES)
	if _, err := rand.Read(nonce); err != nil {
		return err
	}
	signed := map[string]string{
		HEADER_APP_KEY:     c.cfg.AppKey,
		HEADER_TIMESTAMP:   time.Now().UTC().Format(TIMESTAMP_LAYOUT),
		HEADER_ALGORITHM:   ALGORITHM,
		HEADER_SIG_VERSION: SIGNATURE_VERSION,
		HEADER_NONCE:       hex.EncodeToString(nonce),
		HEADER_HOST:        c.host,
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, SCHEME+c.host+path+"?"+query.Encode(), nil)
	if err != nil {
		return err
	}
	for name, value := range signed {
		if name != HEADER_HOST {
			req.Header.Set(name, value)
		}
	}
	req.Header.Set(HEADER_SIGNATURE, Sign(path, query, signed, nil, c.cfg.AppSecret))
	req.Header.Set(HEADER_VERSION, API_VERSION)
	if c.cfg.Token != "" {
		req.Header.Set(HEADER_TOKEN, c.cfg.Token)
	}

	res, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, MAX_RESPONSE_BYTES))
	if err != nil {
		return err
	}
	if res.StatusCode != http.StatusOK {
		var apiErr struct {
			Code    string `json:"error_code"`
			Message string `json:"message"`
		}
		if json.Unmarshal(raw, &apiErr) == nil && apiErr.Code != "" {
			return fmt.Errorf("webull: HTTP %d %s: %s", res.StatusCode, apiErr.Code, apiErr.Message)
		}
		return fmt.Errorf("webull: HTTP %d: %.*s", res.StatusCode, ERROR_SNIPPET, raw)
	}
	return json.Unmarshal(raw, out)
}

// OCCSymbol names an option contract the way Webull expects: root, YYMMDD, C or P, then the
// strike in thousandths as eight digits. AAPL 300 call expiring 2026-05-22 is AAPL260522C00300000.
func OCCSymbol(root, right string, strike float64, expiry time.Time) string {
	return fmt.Sprintf("%s%s%s%08d", strings.ToUpper(root), expiry.Format(OCC_EXPIRY_LAYOUT), strings.ToUpper(right), int64(strike*OCC_STRIKE_SCALE+0.5))
}
