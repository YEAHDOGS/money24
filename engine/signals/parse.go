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
	KIND_RECAP Kind = "recap" // a victory lap about a finished trade: a claim to check, never a trade
	KIND_NOISE Kind = "noise"

	RIGHT_CALL = "C"
	RIGHT_PUT  = "P"

	CONFIDENCE_FULL       = 1.0  // the whole trade and its price found: a full contract, or a share or coin entry
	CONFIDENCE_NO_PRICE   = 0.85 // contract found, price missing: we use the live quote
	CONFIDENCE_SYMBOL     = 0.6  // exit/trim/add with a symbol only
	CONFIDENCE_NONE       = 0.0
	EXPIRY_LAYOUT         = "2006-01-02"
	TWO_DIGIT_YEAR_OFFSET = 2000
	MAX_EXPIRY_YEARS_OUT  = 3 // a trailing number further out than this is not a year

	MONTH_ABBREV_LEN = 3

	SIZE_HEAVY = "heavy"
	SIZE_LIGHT = "light"
	SIZE_LOTTO = "lotto"
	MONTHS     = "jan|feb|mar|apr|may|jun|jul|aug|sep|oct|nov|dec" // monthNumber indexes into this
	// Whole month names only, so "decent 20" or "margin 5" is not a date.
	MONTH_NAMES = `jan(?:uary)?|feb(?:ruary)?|mar(?:ch)?|apr(?:il)?|may|june?|july?|aug(?:ust)?|sep(?:t|tember)?|oct(?:ober)?|nov(?:ember)?|dec(?:ember)?`
)

var (
	// Order matters: exit and trim words are checked before entry words ("sold half" is a trim).
	TRIM_WORDS = regexp.MustCompile(`(?i)\b(trim(med|ming)?|scal(e|ed|ing) out|took (some )?profits?|sold (half|some|\d+%))\b`)
	EXIT_WORDS = regexp.MustCompile(`(?i)\b(stc|sold|all out|out of|closed|stopped( out)?|exited|cut)\b`)
	ADD_WORDS  = regexp.MustCompile(`(?i)\b(add(ed|ing)?( more)?|averag(ed|ing) (down|up))\b`)
	// Verbs that say a buy happened, clear enough to act on without a price ("Bought $PEPE").
	BOUGHT_WORDS = regexp.MustCompile(`(?i)\b(bto|bought|buying|entered|opened)\b`)
	ENTRY_WORDS  = regexp.MustCompile(`(?i)\b(bto|bought|buying|entry|entered|opened|long|in)\b`)

	CASHTAG     = regexp.MustCompile(`\$([A-Za-z]{1,5})\b`)
	BARE_TICKER = regexp.MustCompile(`\b(?i:bto|stc|bought|sold|out of|trimmed|added)\s+([A-Z]{1,5})\b`)
	CONTRACT    = regexp.MustCompile(`(?i)\b(\d+(?:\.\d+)?)\s*(c|p|calls?|puts?)\b`)
	EXPIRY      = regexp.MustCompile(`\b(\d{1,2})/(\d{1,2})(?:/(\d{2}|\d{4}))?\b`)
	PRICE       = regexp.MustCompile(`(?i)(?:@|\bat\b|\bfor\b)\s*\$?(\d*\.?\d+)`)
	// "$2.05 ➡️ $3.24" or "58% GAINER": the caller is reporting a result, not calling a trade.
	RECAP_MOVE  = regexp.MustCompile(`\$?(\d*\.?\d+)\s*(?:\x{27A1}\x{FE0F}?|\x{2192}|-+>|=+>)\s*\$?(\d*\.?\d+)`)
	RECAP_WORDS = regexp.MustCompile(`(?i)\d\s*%\s*(gainer|gains?|winner|win|profit|banked)\b`)
	// A price alone on its line ("$0.75"), the way multi-line callouts end. Needs the $ so a lone number is not a price.
	PRICE_LINE = regexp.MustCompile(`(?m)^\s*\$(\d*\.?\d+)\s*$`)
	// "20 NOV 26", "20NOV", "20 November 2026": day first. Checked before month-first, or "NOV 26" would win.
	EXPIRY_DAY_MONTH = regexp.MustCompile(`(?i)\b(\d{1,2})\s*(` + MONTH_NAMES + `)\.?(?:\s*(\d{4}|\d{2}))?\b`)
	// "NOV 20", "Nov 20th, 2026": month first.
	EXPIRY_MONTH_DAY = regexp.MustCompile(`(?i)\b(` + MONTH_NAMES + `)\.?\s*(\d{1,2})(?:st|nd|rd|th)?(?:,?\s+(\d{4}))?\b`)
	STRIKE_SUFFIX    = regexp.MustCompile(`(?i)^\s*(c|p|calls?|puts?)\b`)

	// Size words describe the caller's size, never ours. They are kept as a tag for backtests.
	SIZE_WORDS = []struct {
		size  string
		words *regexp.Regexp
	}{
		{SIZE_LOTTO, regexp.MustCompile(`(?i)\b(lotto|lottos|lottery|yolo)\b`)},
		{SIZE_HEAVY, regexp.MustCompile(`(?i)\b(heavy|full size|sizing up|sized up|big size)\b`)},
		{SIZE_LIGHT, regexp.MustCompile(`(?i)\b(light|small|starter|half size|tiny)\b`)},
	}
)

// Signal is one parsed post. Options have Right, Strike and Expiry; stocks and crypto leave them empty.
type Signal struct {
	SourceID   string    `json:"source_id"`
	PostID     string    `json:"post_id"`
	ThreadID   string    `json:"thread_id,omitempty"`
	Kind       Kind      `json:"kind"`
	Symbol     string    `json:"symbol,omitempty"`
	Market     string    `json:"market,omitempty"` // "crypto" when the runner prices it as a coin; empty for stocks and options
	Right      string    `json:"right,omitempty"`
	Strike     float64   `json:"strike,omitempty"`
	Expiry     string    `json:"expiry,omitempty"`
	Price      float64   `json:"price,omitempty"`
	ExitPrice  float64   `json:"exit_price,omitempty"` // recap posts: the exit the caller claims
	Size       string    `json:"size,omitempty"`       // caller's size word: heavy, light or lotto
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
	} else if m := PRICE_LINE.FindStringSubmatch(text); m != nil && sig.Right != "" {
		sig.Price, _ = strconv.ParseFloat(m[1], 64)
	}
	for _, s := range SIZE_WORDS {
		if s.words.MatchString(text) {
			sig.Size = s.size
			break
		}
	}

	contract := sig.Right != "" && sig.Expiry != ""
	// A share or a coin: with no strike or expiry in the post, symbol and price are the whole trade.
	plain := sig.Right == "" && sig.Expiry == ""
	move := RECAP_MOVE.FindStringSubmatch(text)
	switch {
	case TRIM_WORDS.MatchString(text):
		sig.Kind = KIND_TRIM
	case EXIT_WORDS.MatchString(text):
		sig.Kind = KIND_EXIT
	case move != nil || RECAP_WORDS.MatchString(text):
		sig.Kind = KIND_RECAP
		if move != nil {
			sig.Price, _ = strconv.ParseFloat(move[1], 64)
			sig.ExitPrice, _ = strconv.ParseFloat(move[2], 64)
		}
	case ADD_WORDS.MatchString(text):
		sig.Kind = KIND_ADD
	case contract || ENTRY_WORDS.MatchString(text) && sig.Price > 0 || plain && BOUGHT_WORDS.MatchString(text):
		sig.Kind = KIND_ENTRY
	default:
		return sig
	}

	// How much of the trade we could read. A full contract is fully read whatever the action,
	// so an "adding" post can stand in for an entry we never saw.
	read := contract || plain && sig.Kind == KIND_ENTRY
	switch {
	case read && sig.Price > 0:
		sig.Confidence = CONFIDENCE_FULL
	case read:
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

// findExpiry reads "11/15", "11/15/26", "20 NOV 26" or "Nov 20" as 2026-11-20.
// A date without a year is the next one on or after today.
func findExpiry(text string, now time.Time) string {
	if m := EXPIRY.FindStringSubmatch(text); m != nil {
		month, _ := strconv.Atoi(m[1])
		day, _ := strconv.Atoi(m[2])
		return expiryDate(month, day, m[3], now)
	}
	if loc := EXPIRY_DAY_MONTH.FindStringSubmatchIndex(text); loc != nil {
		day, _ := strconv.Atoi(text[loc[2]:loc[3]])
		year := ""
		// "20 NOV 26C" is Nov 20 with a 26 strike; "20 NOV 26 $30 CALL" has 26 as the year.
		if loc[6] >= 0 && !STRIKE_SUFFIX.MatchString(text[loc[7]:]) {
			year = text[loc[6]:loc[7]]
		}
		return expiryDate(monthNumber(text[loc[4]:loc[5]]), day, year, now)
	}
	if m := EXPIRY_MONTH_DAY.FindStringSubmatch(text); m != nil {
		day, _ := strconv.Atoi(m[2])
		return expiryDate(monthNumber(m[1]), day, m[3], now)
	}
	return ""
}

// monthNumber turns "nov" (any case) into 11.
func monthNumber(name string) int {
	return strings.Index(MONTHS, strings.ToLower(name[:MONTH_ABBREV_LEN]))/len("jan|") + 1
}

// expiryDate validates the parts and formats them. year is "", two digits or four.
func expiryDate(month, day int, year string, now time.Time) string {
	if month < 1 || month > 12 || day < 1 || day > 31 {
		return ""
	}
	y := now.Year()
	if year != "" {
		y, _ = strconv.Atoi(year)
		if y < 100 {
			y += TWO_DIGIT_YEAR_OFFSET
		}
		if y < now.Year() || y > now.Year()+MAX_EXPIRY_YEARS_OUT {
			return ""
		}
	}
	date := time.Date(y, time.Month(month), day, 0, 0, 0, 0, now.Location())
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	if year == "" && date.Before(today) {
		date = date.AddDate(1, 0, 0)
	}
	return date.Format(EXPIRY_LAYOUT)
}
