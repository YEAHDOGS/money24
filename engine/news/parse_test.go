package news

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

const (
	FIXTURE_DIR     = "testdata"
	FIXTURE_FEEDURL = "https://feeds.example.com/rss"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(FIXTURE_DIR, name))
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func testSource(bundle bool) Source {
	return Source{ID: "test-feed", Label: "Test Feed", Type: TYPE_RSS, Kind: "news", URL: FIXTURE_FEEDURL, Bundle: bundle}
}

func TestParseRSSArchiveFields(t *testing.T) {
	events, err := parseFeed(fixture(t, "rss.xml"), testSource(false))
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("got %d events, want 2", len(events))
	}
	fed := events[0]
	if fed.Author != "Jane Writer, Sam Reporter" {
		t.Errorf("author = %q", fed.Author)
	}
	// content:encoded wins over the description, with markup, scripts, styles and comments gone.
	if want := "The Federal Reserve held rates steady on Friday. Officials said they’d wait & see."; fed.Excerpt != want {
		t.Errorf("excerpt = %q, want %q", fed.Excerpt, want)
	}
	if fed.Summary != "Short teaser." {
		t.Errorf("summary = %q: the live feed keeps the description", fed.Summary)
	}
	if fed.FeedURL != FIXTURE_FEEDURL || fed.Source != "Test Feed" || fed.OutletURL != "https://www.example.com" {
		t.Errorf("feed %q, source %q, outlet %q", fed.FeedURL, fed.Source, fed.OutletURL)
	}
	if fed.PublishedAt == nil || fed.PublishedAt.Format(DAY_LAYOUT) != "2026-11-20" {
		t.Errorf("published = %v", fed.PublishedAt)
	}

	wire := events[1]
	if wire.Source != "Wire Service" || wire.OutletURL != "https://www.wire.example/" {
		t.Errorf("an item naming its outlet: source %q, outlet %q", wire.Source, wire.OutletURL)
	}
	if wire.Excerpt != "Escaped twice & still readable." {
		t.Errorf("double-escaped excerpt = %q", wire.Excerpt)
	}
	if wire.Author != "" {
		t.Errorf("author = %q, want none", wire.Author)
	}
}

func TestParseAtomArchiveFields(t *testing.T) {
	events, err := parseFeed(fixture(t, "atom.xml"), testSource(false))
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("got %d events, want 2", len(events))
	}
	first := events[0]
	if first.Author != "Example Corp, Filing Agent" {
		t.Errorf("author = %q", first.Author)
	}
	if first.Excerpt != "Example Corp reported record revenue." {
		t.Errorf("xhtml content excerpt = %q", first.Excerpt)
	}
	if first.Summary != "Item 2.02 Results of operations" {
		t.Errorf("summary = %q", first.Summary)
	}
	if first.OutletURL != "https://filings.example.gov" || first.FeedURL != FIXTURE_FEEDURL {
		t.Errorf("outlet %q feed %q", first.OutletURL, first.FeedURL)
	}
	if events[1].Excerpt != "Only a summary here." || events[1].Author != "" {
		t.Errorf("summary-only entry: excerpt %q author %q", events[1].Excerpt, events[1].Author)
	}
}

func TestParseGoogleNewsBundle(t *testing.T) {
	events, err := parseFeed(fixture(t, "gnews.xml"), testSource(true))
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("got %d events, want the item and one related article", len(events))
	}
	lead := events[0]
	if lead.Title != "Stocks rally as yields fall" || lead.Source != "Example Times" {
		t.Errorf("title %q source %q", lead.Title, lead.Source)
	}
	// The link stays Google's redirect; the outlet's own site comes from <source url>.
	if !strings.HasPrefix(lead.URL, "https://news.google.com/") || lead.OutletURL != "https://www.exampletimes.com" {
		t.Errorf("url %q outlet %q", lead.URL, lead.OutletURL)
	}
	if lead.Excerpt != "" || lead.Summary != "" {
		t.Errorf("a bundle's description is other outlets' headlines, not an excerpt: %q", lead.Excerpt)
	}
	related := events[1]
	if related.Source != "Other Daily" || related.FeedURL != FIXTURE_FEEDURL || related.OutletURL != "" {
		t.Errorf("related: source %q feed %q outlet %q", related.Source, related.FeedURL, related.OutletURL)
	}
}

func TestParseGDELT(t *testing.T) {
	src := Source{ID: "gdelt-test", Label: "GDELT", Type: TYPE_GDELT, Kind: "news", URL: "https://api.gdeltproject.org/api/v2/doc/doc?query=x"}
	events, err := parseGDELT(fixture(t, "gdelt.json"), src)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("got %d events", len(events))
	}
	e := events[0]
	if e.Source != "example.de" || e.OutletURL != "https://www.example.de" || e.FeedURL != src.URL {
		t.Errorf("source %q outlet %q feed %q", e.Source, e.OutletURL, e.FeedURL)
	}
	if e.Author != "" || e.Excerpt != "" || e.Lang != "German" || e.PublishedAt == nil {
		t.Errorf("GDELT carries no byline or text: author %q excerpt %q lang %q", e.Author, e.Excerpt, e.Lang)
	}
}

func TestExcerptCap(t *testing.T) {
	cases := map[string]string{
		"ascii words":   strings.Repeat("market ", 600),
		"one long word": strings.Repeat("x", 3000),
		"multibyte":     strings.Repeat("相場は上昇した。", 300),
		"html":          "<p>" + strings.Repeat("Rates &amp; yields. ", 200) + "</p>",
	}
	for name, body := range cases {
		got := excerpt(body)
		if n := utf8.RuneCountInString(got); n > EXCERPT_MAX_CHARS {
			t.Errorf("%s: %d characters, want at most %d", name, n, EXCERPT_MAX_CHARS)
		}
		if !strings.HasSuffix(got, ELLIPSIS) || !utf8.ValidString(got) {
			t.Errorf("%s: cut text must end in an ellipsis and stay valid UTF-8", name)
		}
	}
	if short := excerpt("<b>Short</b> text"); short != "Short text" {
		t.Errorf("short excerpt = %q", short)
	}
}
