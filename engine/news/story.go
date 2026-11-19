package news

import (
	"encoding/json"
	"sort"
	"strings"
	"time"
)

const (
	STORY_RULES_FILE = "story_rules.json"
	LINKS_FILE       = "links.json"
	COUNTRIES_FILE   = "countries.json"

	// Grouping. A headline joins the story it shares enough words with; otherwise it waits alone.
	STORY_OPEN        = 36 * time.Hour // a story with no new article for this long is closed
	SEED_TTL          = 24 * time.Hour // a lone article older than this will not start a story
	RECENT_COMPARE    = 8              // a headline is compared with this many of a story's newest articles
	MAX_POSTING       = 2500           // a word carried by more groups than this says nothing; it finds no candidates
	MIN_CANDIDATE     = 2              // shared words before a group is worth comparing
	SIM_PLAIN         = 0.5            // word overlap (shared over all) to join with no watchlist tag in common
	SIM_TAGGED        = 0.32           // and with one
	MIN_SHARED_PLAIN  = 3
	MIN_SHARED_TAGGED = 2
	SIM_SURE          = 0.8  // overlap high enough to join whatever the count
	PROFILE_MIN_SIZE  = 3    // a story this big also has a profile: the words most of its headlines use
	PROFILE_SHARE     = 0.2  // a profile word is in at least this share of the story's headlines
	PROFILE_FIT       = 0.6  // share of a headline's words found in the profile, to join by profile
	PROFILE_FIT_PLAIN = 0.75 // and with no tag in common
	PROFILE_SHARED    = 3

	// What a story carries.
	MAX_TAGS       = 6
	TAG_SHARE      = 0.25 // a tag stays on the story when this share of its headlines carry it
	MAX_LINKS      = 8
	MAX_PLACES     = 12
	MAX_TERMS      = 6
	MAX_BETS       = 3
	SPARK_POINTS   = 36
	ANGLE_BULLISH  = "bullish"
	ANGLE_BEARISH  = "bearish"
	ANGLE_MIXED    = "mixed"
	ANGLE_UNCLEAR  = "unclear"
	ANGLE_LEAN     = 0.3 // (bullish - bearish) / all, past this either way, is a lean
	ANGLE_MIXED_AT = 2   // both sides at least this strong with no lean: mixed
	// Headlines that must take a side before the story has an angle: this many, and this share of them.
	ANGLE_MIN_HEADLINES = 2
	ANGLE_MIN_SHARE     = 0.1
	TREND_RISING        = "rising"
	TREND_STEADY        = "steady"
	TREND_FADING        = "fading"
	RELATION_SELF       = "direct"
)

// StoryRules is story_rules.json: how a story's category and angle are read from its headlines.
type StoryRules struct {
	Categories []struct {
		ID    string   `json:"id"`
		Terms []string `json:"terms"`
	} `json:"categories"`
	Fallback       string            `json:"fallback_category"`
	KindCategories map[string]string `json:"kind_categories"`
	Bullish        []string          `json:"bullish"`
	Bearish        []string          `json:"bearish"`
	Stopwords      []string          `json:"stopwords"`
}

// Angle is which way a story leans, and the headline words that say so.
type Angle struct {
	Label   string      `json:"label"`
	Score   float64     `json:"score"` // -1 all bearish to 1 all bullish
	Bullish int         `json:"bullish"`
	Bearish int         `json:"bearish"`
	Terms   []AngleTerm `json:"terms"`
}

// AngleTerm is one bullish or bearish word and how many headlines used it.
type AngleTerm struct {
	Term string `json:"term"`
	Side string `json:"side"`
	N    int    `json:"n"`
}

// Attention is how loudly a story is being covered right now.
type Attention struct {
	Score    float64 `json:"score"` // 0 to 100
	LastHour int     `json:"last_hour"`
	PrevHour int     `json:"prev_hour"`
	Trend    string  `json:"trend"`
}

// Validity is how far a story can be trusted to be real.
type Validity struct {
	Score       float64 `json:"score"`       // 0 to 100
	Independent int     `json:"independent"` // outlets, counting those that ran the same wire copy once
	Primary     bool    `json:"primary"`     // a filing, release or central bank statement is in it
	Known       int     `json:"known"`       // outlets from our own source list
}

// Place is a country and how many of the story's articles count toward it.
type Place struct {
	ID string `json:"id"`
	N  int    `json:"n"`
}

// Link is one instrument a story can move.
type Link struct {
	Symbol    string  `json:"symbol"`
	Relation  string  `json:"relation"`
	Sign      int     `json:"sign"`
	Why       string  `json:"why"`
	Angle     string  `json:"angle"` // the story's angle carried through the sign
	Quoted    bool    `json:"quoted"`
	Price     float64 `json:"price"`
	MovePct   float64 `json:"move_pct"`
	RelVolume float64 `json:"rel_volume"`
}

// Bet is a prediction market that asks about the story.
type Bet struct {
	ID       string  `json:"id"`
	Question string  `json:"question"`
	URL      string  `json:"url"`
	Yes      float64 `json:"yes"`
	Volume   float64 `json:"volume"`
}

// Reading sets coverage against trading volume for the story's first direct instrument.
type Reading struct {
	Label  string `json:"label"`
	Symbol string `json:"symbol"`
}

// Story is every article about one event, with what it is, what it can move and how loud it is.
type Story struct {
	ID        int64     `json:"id"`
	FirstSeen time.Time `json:"first_seen"`
	LastSeen  time.Time `json:"last_seen"`
	Title     string    `json:"title"`
	Category  string    `json:"category"`
	Angle     Angle     `json:"angle"`
	Tags      []string  `json:"tags"`
	Articles  int       `json:"articles"`
	Outlets   int       `json:"outlets"`
	Attention Attention `json:"attention"`
	Validity  Validity  `json:"validity"`
	From      []Place   `json:"from"`
	About     []Place   `json:"about"`
	Links     []Link    `json:"links"`
	Markets   []Bet     `json:"markets"`
	Reading   Reading   `json:"reading"`
	Spark     []float64 `json:"spark"`
	// Image is the first picture any of the story's articles came with, and ImageBy the outlet it belongs to.
	Image   string `json:"image,omitempty"`
	ImageBy string `json:"image_by,omitempty"`
}

// member is one article inside a group.
type member struct {
	StoredEvent
	from   string   // country id the outlet publishes from, "" when unknown
	about  []string // country ids the headline names
	key    string   // the headline as plain words: equal keys are the same wire copy
	words  []string
	tokens []string
}

// cluster is a group of articles: a lone one waiting for company, or a stored story.
type cluster struct {
	id        int64 // 0 until a second article joins and the story is stored
	members   []*member
	filed     int            // how many of them the database already files under this story
	counts    map[string]int // word -> headlines carrying it
	story     Story
	bets      []Bet
	spark     []float64
	snappedAt time.Time
	dirty     bool // has changes the database has not seen
}

// profile is the words most of the story's headlines use, sorted.
func (c *cluster) profile() []string {
	floor := max(2, int(float64(len(c.members))*PROFILE_SHARE+0.5))
	if len(c.members) < PROFILE_MIN_SIZE {
		floor = 1
	}
	out := []string{}
	for word, n := range c.counts {
		if n >= floor {
			out = append(out, word)
		}
	}
	sort.Strings(out)
	return out
}

func sharesTag(a, b []string) bool {
	for _, x := range a {
		for _, y := range b {
			if x == y {
				return true
			}
		}
	}
	return false
}

// fits says whether an article belongs in this group, and how well.
func (c *cluster) fits(m *member) (float64, bool) {
	best, bestShared, tagged := 0.0, 0, false
	for i := len(c.members) - 1; i >= 0 && i >= len(c.members)-RECENT_COMPARE; i-- {
		other := c.members[i]
		if other.key == m.key {
			return 1, true
		}
		n := shared(m.tokens, other.tokens)
		sim := float64(n) / float64(len(m.tokens)+len(other.tokens)-n)
		if sim > best {
			best, bestShared = sim, n
		}
		tagged = tagged || sharesTag(m.Tags, other.Tags)
	}
	need, needShared := SIM_PLAIN, MIN_SHARED_PLAIN
	if tagged {
		need, needShared = SIM_TAGGED, MIN_SHARED_TAGGED
	}
	if best >= SIM_SURE || (best >= need && bestShared >= needShared) {
		return best, true
	}
	if len(c.members) < PROFILE_MIN_SIZE {
		return best, false
	}
	n := shared(m.tokens, c.profile())
	fit := float64(n) / float64(len(m.tokens))
	needFit := PROFILE_FIT_PLAIN
	if tagged {
		needFit = PROFILE_FIT
	}
	return fit, n >= PROFILE_SHARED && fit >= needFit
}

// storyReader turns headlines into a story's category, angle, places and links. It holds the rule files.
type storyReader struct {
	stop       map[string]bool
	categories phraseSet
	order      map[string]int // category id -> its place in the rule file, for ties
	fallback   string
	kinds      map[string]string
	lexicon    phraseSet
	places     phraseSet
	placeNames map[string]string // lowercase country name -> id, for sources that send a name
	feedPlace  map[string]string // feed id -> country id
	knownFeed  map[string]bool   // feeds that are one named outlet, not a worldwide search
	links      map[string][]Link
}

func loadStoryReader(sources []Source) (*storyReader, error) {
	var rules StoryRules
	var linkDoc struct {
		Links map[string][]Link `json:"links"`
	}
	var placeDoc struct {
		Countries []struct {
			ID    string   `json:"id"`
			Name  string   `json:"name"`
			Terms []string `json:"terms"`
		} `json:"countries"`
	}
	for file, into := range map[string]any{STORY_RULES_FILE: &rules, LINKS_FILE: &linkDoc, COUNTRIES_FILE: &placeDoc} {
		raw, err := defaults.ReadFile(file)
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal(raw, into); err != nil {
			return nil, err
		}
	}
	r := &storyReader{
		stop: map[string]bool{}, categories: phraseSet{}, order: map[string]int{}, fallback: rules.Fallback,
		kinds: rules.KindCategories, lexicon: phraseSet{}, places: phraseSet{}, placeNames: map[string]string{},
		feedPlace: map[string]string{}, knownFeed: map[string]bool{}, links: linkDoc.Links,
	}
	for _, w := range rules.Stopwords {
		r.stop[w] = true
	}
	for i, c := range rules.Categories {
		r.order[c.ID] = i
		for _, t := range c.Terms {
			r.categories.add(t, c.ID)
		}
	}
	for _, t := range rules.Bullish {
		r.lexicon.add(t, ANGLE_BULLISH)
	}
	for _, t := range rules.Bearish {
		r.lexicon.add(t, ANGLE_BEARISH)
	}
	for _, c := range placeDoc.Countries {
		for _, t := range c.Terms {
			r.places.add(t, c.ID)
			r.placeNames[t] = c.ID
		}
	}
	for _, s := range sources {
		r.knownFeed[s.ID] = s.Type == TYPE_RSS && !s.Bundle
		if s.Country != "" {
			r.feedPlace[s.ID] = s.Country
		}
	}
	return r, nil
}

// member reads one stored article: its words, the place it comes from and the places it names.
func (r *storyReader) member(e StoredEvent) *member {
	ws := words(headline(e.Title))
	m := &member{StoredEvent: e, words: ws, tokens: tokens(ws, r.stop), from: r.feedPlace[e.Feed]}
	m.key = strings.Join(ws, PHRASE_SEPARATOR)
	if id, ok := r.placeNames[strings.Join(words(e.Country), PHRASE_SEPARATOR)]; ok {
		m.from = id
	}
	seen := map[string]bool{}
	for _, hit := range r.places.find(ws) {
		for _, id := range hit.Labels {
			if !seen[id] {
				seen[id] = true
				m.about = append(m.about, id)
			}
		}
	}
	return m
}
