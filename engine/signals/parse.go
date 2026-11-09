// Package signals turns callout posts into trade signals and paper-trade plans.
// The parser here is the fast, deterministic first pass. Posts it cannot read fully
// get a low confidence and go to the LLM classifier instead.
package signals

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Kind is what a post asks us to do.
type Kind string

const (
	KIND_ENTRY Kind = "entry"
	KIND_EXIT  Kind = "exit"
	KIND_TRIM  Kind = "trim"
	KIND_ADD   Kind = "add"
	KIND_NOISE Kind = "noise"

	RIGHT_CALL = "C"
	RIGHT_PUT  = "P"

	CONFIDENCE_FULL       = 1.0  // symbol, right, strike, expiry and price all found
	CONFIDENCE_NO_PRICE   = 0.85 // contract found, price missing: we use the live quote
	CONFIDENCE_SYMBOL     = 0.6  // exit/trim/add with a symbol only
	CONFIDENCE_NONE       = 0.0
	EXPIRY_LAYOUT         = "2006-01-02"
	TWO_DIGIT_YEAR_OFFSET = 2000
)

var (
	// Order matters: exit and trim words are checked before entry words ("sold half" is a trim).
	TRIM_WORDS  = regexp.MustCompile(`(?i)\b(trim(med|ming)?|scal(e|ed|ing) out|took (some )?profits?|sold (half|some|\d+%))\b`)
	EXIT_WORDS  = regexp.MustCompile(`(?i)\b(stc|sold|all out|out of|closed|stopped( out)?|exited|cut)\b`)
	ADD_WORDS   = regexp.MustCompile(`(?i)\b(add(ed|ing)?( more)?|averag(ed|ing) (down|up))\b`)
	ENTRY_WORDS = regexp.MustCompile(`(?i)\b(bto|bought|buying|entry|entered|opened|long|in)\b`)

	CASHTAG     = regexp.MustCompile(`\$([A-Za-z]{1,5})\b`)
	BARE_TICKER = regexp.MustCompile(`\b(?i:bto|stc|bought|sold|out of|trimmed|added)\s+([A-Z]{1,5})\b`)
	CONTRACT    = regexp.MustCompile(`(?i)\b(\d+(?:\.\d+)?)\s*(c|p|calls?|puts?)\b`)
	EXPIRY      = regexp.MustCompile(`\b(\d{1,2})/(\d{1,2})(?:/(\d{2}|\d{4}))?\b`)
	PRICE       = regexp.MustCompile(`(?i)(?:@|\bat\b|\bfor\b)\s*\$?(\d*\.?\d+)`)
)

// Signal is one parsed post. Options have Right, Strike and Expiry; stocks and crypto leave them empty.
type Signal struct {
	SourceID   string    `json:"source_id"`
	PostID     string    `json:"post_id"`
	ThreadID   string    `json:"thread_id,omitempty"`
	Kind       Kind      `json:"kind"`
	Symbol     string    `json:"symbol,omitempty"`
	Right      string    `json:"right,omitempty"`
	Strike     float64   `json:"strike,omitempty"`
	Expiry     string    `json:"expiry,omitempty"`
	Price      float64   `json:"price,omitempty"`
	Confidence float64   `json:"confidence"`
	SeenAt     time.Time `json:"seen_at"`
	Raw        string    `json:"raw"`
	PostedAt   time.Time `json:"posted_at,omitzero"` // when the source posted it, when known
	URL        string    `json:"url,omitempty"`
	Manual     bool      `json:"manual,omitempty"` // shared by hand from the app
}

// Parse reads one post. now resolves expiries written without a year.
func Parse(text string, now time.Time) Signal {
	sig := Signal{Kind: KIND_NOISE, Raw: text, SeenAt: now, Confidence: CONFIDENCE_NONE}
	sig.Symbol = findSymbol(text)
	if sig.Symbol == "" {
		return sig
	}

	if m := CONTRACT.FindStringSubmatch(text); m != nil {
		sig.Strike, _ = strconv.ParseFloat(m[1], 64)
		sig.Right = RIGHT_CALL
		if strings.HasPrefix(strings.ToLower(m[2]), "p") {
			sig.Right = RIGHT_PUT
		}
	}
	sig.Expiry = findExpiry(text, now)
	if m := PRICE.FindStringSubmatch(text); m != nil {
		sig.Price, _ = strconv.ParseFloat(m[1], 64)
	}

	contract := sig.Right != "" && sig.Expiry != ""
	switch {
	case TRIM_WORDS.MatchString(text):
		sig.Kind = KIND_TRIM
	case EXIT_WORDS.MatchString(text):
		sig.Kind = KIND_EXIT
	case ADD_WORDS.MatchString(text):
		sig.Kind = KIND_ADD
	case contract || ENTRY_WORDS.MatchString(text) && sig.Price > 0:
		sig.Kind = KIND_ENTRY
	default:
		return sig
	}

	switch {
	case sig.Kind != KIND_ENTRY:
		sig.Confidence = CONFIDENCE_SYMBOL
	case contract && sig.Price > 0:
		sig.Confidence = CONFIDENCE_FULL
	case contract:
		sig.Confidence = CONFIDENCE_NO_PRICE
	default:
		sig.Confidence = CONFIDENCE_SYMBOL
	}
	return sig
}

func findSymbol(text string) string {
	if m := CASHTAG.FindStringSubmatch(text); m != nil {
		return strings.ToUpper(m[1])
	}
	if m := BARE_TICKER.FindStringSubmatch(text); m != nil {
		return m[1]
	}
	return ""
}

// findExpiry turns "11/15" or "11/15/26" into 2026-11-15. A date without a year is the next one on or after today.
func findExpiry(text string, now time.Time) string {
	m := EXPIRY.FindStringSubmatch(text)
	if m == nil {
		return ""
	}
	month, _ := strconv.Atoi(m[1])
	day, _ := strconv.Atoi(m[2])
	if month < 1 || month > 12 || day < 1 || day > 31 {
		return ""
	}
	year := now.Year()
	if m[3] != "" {
		year, _ = strconv.Atoi(m[3])
		if year < 100 {
			year += TWO_DIGIT_YEAR_OFFSET
		}
	}
	date := time.Date(year, time.Month(month), day, 0, 0, 0, 0, now.Location())
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	if m[3] == "" && date.Before(today) {
		date = date.AddDate(1, 0, 0)
	}
	return date.Format(EXPIRY_LAYOUT)
}
