package news

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Tagger links text to the markets we watch. News only matters if something tradable can move on it.
type Tagger struct {
	rules []tagRule
}

type tagRule struct {
	tag     string
	pattern *regexp.Regexp
}

func LoadTagger(path string) (*Tagger, error) {
	raw, err := readConfig(path, WATCHLIST_FILE)
	if err != nil {
		return nil, err
	}
	var doc struct {
		Watch []struct {
			Tag   string   `json:"tag"`
			Terms []string `json:"terms"`
		} `json:"watch"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}

	t := &Tagger{}
	for _, w := range doc.Watch {
		if w.Tag == "" || len(w.Terms) == 0 {
			return nil, fmt.Errorf("watch entry %q needs a tag and terms", w.Tag)
		}
		quoted := make([]string, len(w.Terms))
		for i, term := range w.Terms {
			quoted[i] = regexp.QuoteMeta(strings.ToLower(term))
		}
		// Whole words only, so "ethereum" never fires on "methereum" and "fed" never fires on "fedex".
		pattern := regexp.MustCompile(`(?i)(?:^|[^\pL\pN])(?:` + strings.Join(quoted, "|") + `)(?:$|[^\pL\pN])`)
		t.rules = append(t.rules, tagRule{tag: w.Tag, pattern: pattern})
	}
	return t, nil
}

func (t *Tagger) match(text string) []string {
	tags := []string{}
	for _, r := range t.rules {
		if r.pattern.MatchString(text) {
			tags = append(tags, r.tag)
		}
	}
	sort.Strings(tags)
	return tags
}

// Size is how many watchlist tags are loaded.
func (t *Tagger) Size() int { return len(t.rules) }
