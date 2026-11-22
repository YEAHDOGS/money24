package signals

import (
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

const (
	CRYPTO_LONG           = "long"
	CRYPTO_SHORT          = "short"
	CRYPTO_USD            = "USD"
	CRYPTO_USDT           = "USDT"
	CRYPTO_USDC           = "USDC"
	CRYPTO_MAX_POST_BYTES = 16 << 10
	CRYPTO_MAX_TARGETS    = 20
	CRYPTO_MIN_PRICE      = 1e-18
	CRYPTO_MAX_PRICE      = 1e12
	CRYPTO_CONF_EXPLICIT  = 1.0
	CRYPTO_CONF_COIN      = 0.9
	CRYPTO_CONF_BARE      = 0.8
	CRYPTO_PRICE          = "\\$?(?:[0-9]{1,3}(?:,[0-9]{3})+|[0-9]+)?\\.[0-9]+|\\$?(?:[0-9]{1,3}(?:,[0-9]{3})+|[0-9]+)"
	CRYPTO_TARGET_LABEL   = `(?:take[ -]?profits?|targets?|tps?)(?:\d{1,2}\b|[ \t]+\d{1,2}[ \t]*[:=]|\b)`
	CRYPTO_STOP_LABEL     = `(?:stop[ -]?loss|stop|s/?l|invalidation)`
	CRYPTO_ENTRY_LABEL    = `(?:entry(?:[ -]+(?:zone|range|price|prices|area))?|entries|(?:buy|sell)[ -]+(?:zone|range|area))`
	CRYPTO_FIELD_PREFIX   = `[ \t]*(?:[:=@]|-(?:[ \t]|$))?[ \t]*`
)

var (
	cryptoDirections  = regexp.MustCompile(`(?i)\b(long|short|buy|sell)\b`)
	cryptoFields      = regexp.MustCompile(`(?i)\b(?:(` + CRYPTO_ENTRY_LABEL + `)\b|(` + CRYPTO_TARGET_LABEL + `)|(` + CRYPTO_STOP_LABEL + `)\b)`)
	cryptoTargets     = regexp.MustCompile(`(?i)\b` + CRYPTO_TARGET_LABEL)
	cryptoPrefix      = regexp.MustCompile(`^` + CRYPTO_FIELD_PREFIX)
	cryptoPair        = regexp.MustCompile(`(?i)\b([a-z0-9]{1,10}?)(?:[ \t]*[/-][ \t]*)?(USDT|USDC|USD)\b`)
	cryptoCrossPair   = regexp.MustCompile(`(?i)\b[a-z0-9]{1,10}[ \t]*/[ \t]*[$#]?[a-z0-9]{1,10}\b`)
	cryptoPriceSuffix = regexp.MustCompile(`^[a-zA-Z0-9.%]`)
	cryptoAtEntry     = regexp.MustCompile(`(?i)(?:@|\bat\b)\s*\$?(?:\d|\.\d)`)
	cryptoCoin        = regexp.MustCompile(`(?:^|[^a-zA-Z0-9])[$#]([a-zA-Z][a-zA-Z0-9]{0,9})\b`)
	cryptoBareAfter   = regexp.MustCompile(`^[ \t:\n]+([A-Z][A-Z0-9]{1,9})\b`)
	cryptoBareBefore  = regexp.MustCompile(`\b([A-Z][A-Z0-9]{1,9})[ \t:\n]+$`)
	cryptoSideZone    = regexp.MustCompile(`(?i)^(?:buy|sell)\b`)
	cryptoPrice       = regexp.MustCompile(`^(?:` + CRYPTO_PRICE + `)`)
	cryptoEntrySep    = regexp.MustCompile(`(?i)^(?:-|~|\bto\b)[ \t]*`)
	cryptoTargetSep   = regexp.MustCompile(`(?i)^(?:->|[-/,;|→]|\n[ \t]*(?:(?:[-•]|[0-9]{1,2}[.)])[ \t]+)?)\s*`)
	cryptoNumbered    = regexp.MustCompile(`^(?:[-•]|[0-9]{1,2}[.)])[ \t]+`)
	cryptoLeverage    = regexp.MustCompile(`(?i)(?:\b(?:leverage|lev)[ \t]*[:=]?[ \t]*(?:x[ \t]*)?([0-9]+(?:\.[0-9]+)?)(?:[ \t]*x)?\b|\b([0-9]+(?:\.[0-9]+)?)[ \t]*x\b|\bx([0-9]+(?:\.[0-9]+)?)\b)`)
	cryptoLevLabel    = regexp.MustCompile(`(?i)\b(?:leverage|lev)\b`)
	stopLabel         = regexp.MustCompile(`(?i)^(?:` + CRYPTO_STOP_LABEL + `)$`)
	cryptoDerivative  = regexp.MustCompile(`(?i)\b(?:perps?|perpetuals?|futures?|margin|isolated|cross[ -]margin)\b`)
	cryptoNonCall     = regexp.MustCompile(`(?i)\b(?:recap|(?:all[ \t]+)?(?:tp\d*|targets?|stop(?:[ -]loss)?|sl)[ \t]+(?:(?:was|is)[ \t]+)?(?:hit|reached|achieved|triggered|filled|secured|done)|(?:hit|reached)\s+(?:all\s+)?(?:tp\d*|targets?|stop|sl)\b|(?:trade|position)\s+closed|stopped\s+out|(?:move|moved|moving|raise|raised|raising|trail|trailed|trailing|adjust|adjusted|update|updated|new)\s+(?:(?:your|the|our)\s+)?(?:stop(?:[ -]loss)?|sl|tp\d*|targets?|entry)|(?:vip\s+(?:access|membership|signals)|join\s+(?:our|my|vip)|subscribe\s+now|referral\s+(?:code|link)|past\s+performance)|(?:potential|possible)\s+(?:long|short|setup)|if\s+.{0,50}\b(?:then|looking)\b)\b|\+\d+(?:\.\d+)?\s*%[ \t]*(?:roi|pnl|profit|gain|booked|banked)\b`)
	cryptoUpdate      = regexp.MustCompile(`(?im)\b(?:update|result|closed|edited)[ \t]*[:|]|\b(?:tp[ \t]*[0-9]+|tps)[ \t]*(?:✅|🎯|hit\b|done\b)`)
	// "Take Profit (TP): 78.37" and "Stop Loss (SL): 80.11" name the field twice; the abbreviation in brackets goes.
	cryptoLabelAlias = regexp.MustCompile(`(?i)[ \t]*\((?:tps?|sl|s/l|stop)\)`)
	cryptoNormalize  = strings.NewReplacer("–", "-", "—", "-", "−", "-", "×", "x", "\r\n", "\n", "\r", "\n", "*", "", "`", "", "_", "")
)

// CryptoCall preserves the quoted levels and original publication time. SeenAt is the replay clock.
type CryptoCall struct {
	PostID     string    `json:"post_id,omitempty"`
	Base       string    `json:"base"`
	Quote      string    `json:"quote"`
	Direction  string    `json:"direction"`
	EntryLow   float64   `json:"entry_low"`
	EntryHigh  float64   `json:"entry_high"`
	Targets    []float64 `json:"targets"`
	Stop       float64   `json:"stop"`
	Leverage   float64   `json:"leverage,omitempty"`
	Derivative bool      `json:"derivative"`
	PostedAt   time.Time `json:"posted_at,omitzero"`
	SeenAt     time.Time `json:"seen_at"`
	Confidence float64   `json:"confidence"`
	// Imported is a call read from a channel's history rather than recorded as it was posted. It is
	// replayed from its publication time, but the channel may have edited or deleted posts since.
	Imported bool `json:"imported,omitempty"`
}

// ParseCryptoCall accepts complete, directional setups. Uncertain or contradictory levels fail closed.
func ParseCryptoCall(post RawPost) (CryptoCall, bool) {
	if len(post.Text) > CRYPTO_MAX_POST_BYTES {
		return CryptoCall{}, false
	}
	body := cryptoLabelAlias.ReplaceAllString(cryptoNormalize.Replace(post.Text), "")
	fields := cryptoFields.FindAllStringIndex(body, -1)
	if len(fields) == 0 || cryptoNonCall.MatchString(body) || cryptoUpdate.MatchString(body) || RECAP_WORDS.MatchString(body) {
		return CryptoCall{}, false
	}
	head := body[:fields[0][0]]
	directions := cryptoDirections.FindAllStringSubmatchIndex(head, -1)
	if len(directions) == 0 && cryptoSideZone.MatchString(body[fields[0][0]:fields[0][1]]) {
		head = body[:fields[0][1]]
		directions = cryptoDirections.FindAllStringSubmatchIndex(head, -1)
		head = head[:directions[0][1]]
	}
	if len(directions) != 1 {
		return CryptoCall{}, false
	}
	loc := directions[0]
	side := CRYPTO_LONG
	switch strings.ToLower(head[loc[2]:loc[3]]) {
	case CRYPTO_SHORT, "sell":
		side = CRYPTO_SHORT
	}
	base, quote, confidence := cryptoMarket(head, loc)
	if base == "" || base == quote {
		return CryptoCall{}, false
	}
	c := CryptoCall{PostID: post.PostID, Base: base, Quote: quote, Direction: side, PostedAt: post.PostedAt,
		SeenAt: post.SeenAt, Confidence: confidence, Derivative: cryptoDerivative.MatchString(body)}
	if lev := cryptoLeverage.FindAllStringSubmatch(body, -1); len(lev) != 0 {
		for _, match := range lev {
			n, err := strconv.ParseFloat(strings.Join(match[1:], ""), 64)
			if err != nil || n < 1 || (c.Leverage != 0 && n != c.Leverage) {
				return CryptoCall{}, false
			}
			c.Leverage = n
		}
	} else if cryptoLevLabel.MatchString(body) {
		return CryptoCall{}, false
	}

	for i, field := range fields {
		end := len(body)
		if i+1 < len(fields) {
			end = fields[i+1][0]
		}
		label := body[field[0]:field[1]]
		value := strings.TrimSpace(cryptoPrefix.ReplaceAllString(body[field[1]:end], ""))
		if cryptoTargets.MatchString(label) {
			if value == "" {
				continue // a "Targets:" heading over TP1, TP2 ... lines
			}
			values := cryptoTargetPrices(value)
			if len(values) == 0 {
				return CryptoCall{}, false
			}
			c.Targets = append(c.Targets, values...)
			continue
		}
		if stopLabel.MatchString(label) {
			if c.Stop != 0 {
				return CryptoCall{}, false
			}
			c.Stop, _ = readCryptoPrice(value)
			if c.Stop <= 0 {
				return CryptoCall{}, false
			}
			continue
		}
		if c.EntryLow != 0 {
			return CryptoCall{}, false
		}
		c.EntryLow, c.EntryHigh = cryptoEntryPrices(value)
		if c.EntryLow <= 0 {
			return CryptoCall{}, false
		}
	}

	if c.EntryLow == 0 {
		if at := cryptoAtEntry.FindStringIndex(head); at != nil {
			value := strings.TrimLeft(head[at[0]:], "@atAT \t\n")
			c.EntryLow, c.EntryHigh = cryptoEntryPrices(value)
		}
	}
	if c.EntryLow == 0 || c.Stop == 0 || len(c.Targets) == 0 || len(c.Targets) > CRYPTO_MAX_TARGETS {
		return CryptoCall{}, false
	}
	slices.Sort(c.Targets)
	if c.Direction == CRYPTO_SHORT {
		slices.Reverse(c.Targets)
	}
	previous := c.EntryHigh
	if c.Direction == CRYPTO_SHORT {
		previous = c.EntryLow
	}
	for _, target := range c.Targets {
		if c.Direction == CRYPTO_LONG && target <= previous || c.Direction == CRYPTO_SHORT && target >= previous {
			return CryptoCall{}, false
		}
		previous = target
	}
	if c.Direction == CRYPTO_LONG && c.Stop >= c.EntryLow || c.Direction == CRYPTO_SHORT && c.Stop <= c.EntryHigh {
		return CryptoCall{}, false
	}
	return c, true
}

func cryptoMarket(head string, direction []int) (string, string, float64) {
	if pairs := cryptoPair.FindAllStringSubmatch(head, -1); len(pairs) != 0 {
		if len(pairs) != 1 {
			return "", "", 0
		}
		return strings.ToUpper(pairs[0][1]), strings.ToUpper(pairs[0][2]), CRYPTO_CONF_EXPLICIT
	}
	if cryptoCrossPair.MatchString(head) {
		return "", "", 0
	}
	if coins := cryptoCoin.FindAllStringSubmatch(head, -1); len(coins) != 0 {
		if len(coins) != 1 {
			return "", "", 0
		}
		return strings.ToUpper(coins[0][1]), CRYPTO_USD, CRYPTO_CONF_COIN
	}
	after := cryptoBareAfter.FindStringSubmatch(head[direction[1]:])
	before := cryptoBareBefore.FindStringSubmatch(head[:direction[0]])
	for _, coin := range [][]string{after, before} {
		if len(coin) != 0 && coin[1] != "SPOT" && coin[1] != "SIGNAL" && coin[1] != "SETUP" && coin[1] != "PERP" {
			return coin[1], CRYPTO_USD, CRYPTO_CONF_BARE
		}
	}
	return "", "", 0
}

func readCryptoPrice(s string) (float64, string) {
	token := cryptoPrice.FindString(s)
	n, err := strconv.ParseFloat(strings.TrimPrefix(strings.ReplaceAll(token, ",", ""), "$"), 64)
	rest := s[len(token):]
	if err != nil || n < CRYPTO_MIN_PRICE || n > CRYPTO_MAX_PRICE || cryptoPriceSuffix.MatchString(rest) || strings.HasPrefix(strings.TrimSpace(rest), "%") {
		return 0, ""
	}
	return n, strings.TrimLeft(rest, " \t")
}

func cryptoEntryPrices(s string) (float64, float64) {
	first, rest := readCryptoPrice(s)
	sep := cryptoEntrySep.FindString(rest)
	if sep == "" {
		return first, first
	}
	second, _ := readCryptoPrice(rest[len(sep):])
	return min(first, second), max(first, second)
}

func cryptoTargetPrices(s string) []float64 {
	s = cryptoNumbered.ReplaceAllString(s, "")
	var prices []float64
	for {
		n, rest := readCryptoPrice(s)
		if n == 0 {
			return nil
		}
		prices = append(prices, n)
		sep := cryptoTargetSep.FindString(rest)
		if sep == "" {
			return prices
		}
		s = strings.TrimSpace(rest[len(sep):])
		if s == "" {
			return prices
		}
		if s[0] != '$' && s[0] != '.' && (s[0] < '0' || s[0] > '9') {
			if strings.HasPrefix(sep, "\n") {
				return prices // A new line of commentary is not part of the target list.
			}
			return nil
		}
	}
}
