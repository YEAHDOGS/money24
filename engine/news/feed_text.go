package news

import (
	"html"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"
)

// What the archive keeps of each article, read only from what its feed publishes.
const (
	EXCERPT_MAX_CHARS  = 1000 // the whole excerpt, ellipsis included
	EXCERPT_WORD_SLACK = 80   // a cut backs off this far at most to end on a whole word
	AUTHOR_MAX_CHARS   = 200
	AUTHOR_JOIN        = ", "
	AUTHOR_BY          = "by "
	ELLIPSIS           = "…"
	ATOM_XHTML         = "xhtml"
	EMAIL_AT           = "@"
	TRIM_BEFORE_CUT    = " ,;:-–—"
)

var (
	// Script, style and comment blocks say nothing a reader sees, so they go before any tag is stripped.
	HIDDEN_PATTERN = regexp.MustCompile(`(?is)<script\b.*?</script\s*>|<style\b.*?</style\s*>|<!--.*?-->`)
	TAG_PATTERN    = regexp.MustCompile(`<[^>]*>`)
	// What still looks like a tag once entities are decoded: a description escaped twice.
	ESCAPED_TAG   = regexp.MustCompile(`</?[a-zA-Z][a-zA-Z0-9]*(\s[^<>]*)?/?>`)
	SPACE_PATTERN = regexp.MustCompile(`[\s\p{Zs}]+`)
	// "jane@example.com (Jane Doe)", the RSS 2.0 author format: the name in brackets is the byline.
	EMAIL_NAME = regexp.MustCompile(`^\S+@\S+\s*\((.+)\)$`)
)

// feedSource is an RSS item's <source url="...">Outlet</source>: who really published it.
type feedSource struct {
	URL  string `xml:"url,attr"`
	Name string `xml:",chardata"`
}

// feedPerson is an author: RSS gives the name as text, Atom inside <name>.
type feedPerson struct {
	Name string `xml:"name"`
	Text string `xml:",chardata"`
}

// atomText is an Atom summary or content element. Escaped HTML arrives as text; XHTML arrives as
// elements, which only the inner XML holds.
type atomText struct {
	Type  string `xml:"type,attr"`
	Text  string `xml:",chardata"`
	Inner string `xml:",innerxml"`
}

func (t atomText) value() string {
	if t.Type == ATOM_XHTML {
		return t.Inner
	}
	return t.Text
}

// plainText strips HTML, decodes entities and collapses whitespace: what a reader would see.
func plainText(s string) string {
	s = TAG_PATTERN.ReplaceAllString(HIDDEN_PATTERN.ReplaceAllString(s, " "), " ")
	s = ESCAPED_TAG.ReplaceAllString(html.UnescapeString(s), " ")
	return strings.TrimSpace(SPACE_PATTERN.ReplaceAllString(s, " "))
}

// excerpt is the feed's own text as plain text, at most EXCERPT_MAX_CHARS characters with the
// ellipsis, cut at a word where one ends close enough to the limit.
func excerpt(s string) string {
	s = plainText(s)
	runes := []rune(s)
	if len(runes) <= EXCERPT_MAX_CHARS {
		return s
	}
	cut := runes[:EXCERPT_MAX_CHARS-utf8.RuneCountInString(ELLIPSIS)]
	for i := len(cut) - 1; i >= len(cut)-EXCERPT_WORD_SLACK; i-- {
		if cut[i] == ' ' {
			cut = cut[:i]
			break
		}
	}
	return strings.TrimRight(string(cut), TRIM_BEFORE_CUT) + ELLIPSIS
}

// byline joins an item's authors: RSS <author> and dc:creator, or Atom <author><name>. Email
// addresses alone are left out; a name in brackets after one is kept.
func byline(people []feedPerson, creators []string) string {
	names := []string{}
	seen := map[string]bool{}
	add := func(raw string) {
		name := clean(raw, 0)
		if m := EMAIL_NAME.FindStringSubmatch(name); m != nil {
			name = strings.TrimSpace(m[1])
		}
		if len(name) > len(AUTHOR_BY) && strings.EqualFold(name[:len(AUTHOR_BY)], AUTHOR_BY) {
			name = strings.TrimSpace(name[len(AUTHOR_BY):])
		}
		key := strings.ToLower(name)
		if name == "" || strings.Contains(name, EMAIL_AT) || seen[key] {
			return
		}
		seen[key] = true
		names = append(names, name)
	}
	for _, p := range people {
		add(firstNonEmpty(p.Name, p.Text))
	}
	for _, c := range creators {
		add(c)
	}
	return clean(strings.Join(names, AUTHOR_JOIN), AUTHOR_MAX_CHARS)
}

// webURL is raw when it is an absolute http or https address, otherwise "".
func webURL(raw string) string {
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != SCHEME_HTTP && u.Scheme != SCHEME_HTTPS) {
		return ""
	}
	return raw
}

// origin is the scheme and host of a web address: the outlet's site for a link to one of its articles.
func origin(raw string) string {
	if webURL(raw) == "" {
		return ""
	}
	u, _ := url.Parse(strings.TrimSpace(raw))
	return u.Scheme + "://" + u.Host
}
