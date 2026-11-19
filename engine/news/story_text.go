package news

import (
	"sort"
	"strings"
	"unicode"
)

const (
	MAX_PHRASE_WORDS  = 3 // the longest phrase a rule file may hold
	MIN_TOKEN_RUNES   = 3 // shorter words say nothing about what a headline is about
	OUTLET_TAIL_WORDS = 4 // "Headline - Outlet Name": a short tail after the last dash is the outlet
	MIN_HEAD_WORDS    = 4 // and the headline before it has to stand on its own
	STEM_MIN_RUNES    = 4 // "rates" and "rate" are one word; "gas" stays "gas"
	PLURAL_SUFFIX     = "s"
	DOUBLE_S          = "ss"
	TITLE_DASH        = " - "
	TITLE_BAR         = " | "
	PHRASE_SEPARATOR  = " "
	MATCH_UNSET       = -1
	MAX_HEADLINE_SIZE = 400 // runes; a feed that sends an essay as a title is cut here
)

// words splits text into lowercase words of letters and digits.
func words(text string) []string {
	return strings.FieldsFunc(strings.ToLower(text), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
}

// headline drops the outlet many feeds append to a title ("... - Reuters", "... | CNBC").
func headline(title string) string {
	if runes := []rune(title); len(runes) > MAX_HEADLINE_SIZE {
		title = string(runes[:MAX_HEADLINE_SIZE])
	}
	cut := max(strings.LastIndex(title, TITLE_DASH), strings.LastIndex(title, TITLE_BAR))
	if cut <= 0 {
		return title
	}
	head, tail := title[:cut], title[cut+len(TITLE_DASH):]
	if len(words(tail)) > OUTLET_TAIL_WORDS || len(words(head)) < MIN_HEAD_WORDS {
		return title
	}
	return head
}

// stem folds a plain plural into its singular, so "cuts" meets "cut".
func stem(word string) string {
	if len([]rune(word)) < STEM_MIN_RUNES || !strings.HasSuffix(word, PLURAL_SUFFIX) || strings.HasSuffix(word, DOUBLE_S) {
		return word
	}
	return strings.TrimSuffix(word, PLURAL_SUFFIX)
}

func isNumber(word string) bool {
	for _, r := range word {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

// tokens returns the words that say what a headline is about: stemmed, sorted, each once.
func tokens(ws []string, stop map[string]bool) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, w := range ws {
		if len([]rune(w)) < MIN_TOKEN_RUNES || stop[w] || isNumber(w) {
			continue
		}
		if w = stem(w); seen[w] {
			continue
		}
		seen[w] = true
		out = append(out, w)
	}
	sort.Strings(out)
	return out
}

// shared counts the words two sorted token lists have in common.
func shared(a, b []string) int {
	n, i, j := 0, 0, 0
	for i < len(a) && j < len(b) {
		switch {
		case a[i] == b[j]:
			n++
			i++
			j++
		case a[i] < b[j]:
			i++
		default:
			j++
		}
	}
	return n
}

// phraseSet maps a phrase of up to MAX_PHRASE_WORDS words to the labels it stands for.
type phraseSet map[string][]string

func (p phraseSet) add(phrase, label string) {
	ws := words(phrase)
	if len(ws) == 0 || len(ws) > MAX_PHRASE_WORDS {
		return
	}
	key := strings.Join(ws, PHRASE_SEPARATOR)
	p[key] = append(p[key], label)
}

// phraseHit is one phrase found in a headline, with the labels it stands for.
type phraseHit struct {
	Phrase string
	Labels []string
}

// find returns every phrase in ws, longest first: a word used by one phrase is not matched again,
// so "new guinea" never also counts as "guinea".
func (p phraseSet) find(ws []string) []phraseHit {
	hits := []phraseHit{}
	for i := 0; i < len(ws); {
		step := 1
		for n := min(MAX_PHRASE_WORDS, len(ws)-i); n >= 1; n-- {
			key := strings.Join(ws[i:i+n], PHRASE_SEPARATOR)
			if labels, ok := p[key]; ok {
				hits = append(hits, phraseHit{Phrase: key, Labels: labels})
				step = n
				break
			}
		}
		i += step
	}
	return hits
}
