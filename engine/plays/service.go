package plays

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/YEAHDOGS/money24/engine/signals"
)

const (
	SOURCE_ID        = "plays" // the journal source every paper-traded play is filed under
	MAX_BODY         = 1 << 20
	HEADLINE_LIMIT   = 15
	NEWS_TIMEOUT     = 4 * time.Second
	SNAPSHOT_TIMEOUT = 20 * time.Second
	REASON_TARGET    = "target"
	ACTION_FILL      = "fill"
	ACTION_CLOSE     = "close"
	ACTION_VOID      = "void"
	REASON_VOID      = "void"
)

// Service is the plays API: CRUD, the link inbox, AI reads, the rulebook and instant paper trades.
type Service struct {
	Store    *Store
	Resolver Resolver
	HTTP     *http.Client
	// Paper hands a bracket to the paper runner and returns its journal id.
	Paper func(signals.Signal, signals.Plan) (int64, error)
	// Lines returns the journal lines that act on a paper plan: fills and the close.
	Lines func(planID int64) []signals.Entry
	// NewsURL is newsd's base URL, for headlines tagged with a play's symbol. Empty skips news.
	NewsURL string
	// MayWrite guards every change: the local desk, or the webhook secret.
	MayWrite func(*http.Request) bool
}

// Item is a play with its report, as the UI reads it.
type Item struct {
	Play   Play   `json:"play"`
	Report Report `json:"report"`
}

func (s *Service) item(p Play, all []Play, rules Rules, now time.Time) Item {
	return Item{Play: p, Report: Build(p, rules, Book{Others: all, Now: now})}
}

// sync copies what the exit watcher did to each armed play: fills, and the close.
func (s *Service) sync() {
	if s.Lines == nil {
		return
	}
	for _, p := range s.Store.Plays() {
		if p.Status != STATUS_ARMED || p.JournalID == 0 {
			continue
		}
		fills, result := []Fill{}, (*Result)(nil)
		for _, e := range s.Lines(p.JournalID) {
			if e.Action == ACTION_VOID {
				result = &Result{At: e.At, Reason: REASON_VOID}
				continue
			}
			if e.Result == nil {
				continue
			}
			fills = append(fills, Fill{At: e.At, Qty: e.Result.Qty, Price: e.Result.Price, Reason: e.Result.Reason, PnLUSD: e.Result.PnLUSD})
			if e.Action == ACTION_CLOSE {
				// A close line carries the whole trade's P&L, the rungs sold before it included.
				result = &Result{At: e.At, PnLUSD: e.Result.PnLUSD, PnLPct: e.Result.PnLPct, Reason: e.Result.Reason}
			}
		}
		if len(fills) == len(p.Fills) && result == nil {
			continue
		}
		s.Store.Update(p.ID, func(q *Play) {
			q.Fills, q.Result = fills, result
			if result != nil {
				q.Status = STATUS_CLOSED
			}
		})
	}
}

// paper hands a play to the paper runner. Blocking rule fails stop it unless the rulebook lets
// paper trades through, in which case the broken rules are written on the play.
func (s *Service) paper(p Play, now time.Time) (Play, error) {
	if p.Status == STATUS_ARMED {
		return p, errors.New("already paper trading")
	}
	if s.Paper == nil {
		return p, errors.New("the paper runner is not connected")
	}
	rules := s.Store.Rules()
	rep := Build(p, rules, Book{Others: s.Store.Plays(), Now: now})
	broke := []string{}
	for _, c := range rep.Checks {
		if c.Status == CHECK_FAIL {
			broke = append(broke, c.ID)
		}
	}
	if len(broke) > 0 && !rules.PaperIgnoresBlocks {
		return p, fmt.Errorf("blocked by %s", strings.Join(broke, ", "))
	}
	if p.Qty <= 0 || p.Entry <= 0 || p.Stop <= 0 || p.Stop >= p.Entry {
		return p, errors.New("a paper trade needs a quantity, an entry and a stop below it")
	}
	sig := signals.Signal{SourceID: SOURCE_ID, PostID: p.ID, Kind: signals.KIND_ENTRY, Symbol: p.Contract.Symbol,
		Right: p.Contract.Right, Strike: p.Contract.Strike, Expiry: p.Contract.Expiry, Price: p.Entry,
		Confidence: 1, SeenAt: now, Raw: p.Title, Manual: true}
	plan := signals.Plan{Signal: sig, Qty: p.Qty, Entry: p.Entry, Stop: p.Stop,
		CostUSD: math.Round(p.Entry*p.Qty*p.Multiplier()*CENT) / CENT, ExitBy: p.Failsafes.ExitBy}
	for _, r := range p.Ladder {
		plan.Targets = append(plan.Targets, signals.Target{Qty: r.Qty, Price: r.Price})
	}
	// The runner rides to its target as the last resting sell; otherwise it waits for the stop or clock.
	if p.Runner.Qty > 0 && p.Runner.Target > 0 {
		plan.Targets = append(plan.Targets, signals.Target{Qty: p.Runner.Qty, Price: p.Runner.Target})
	} else {
		plan.Runner = p.Runner.Qty
	}
	if f := p.Failsafes; f.SidewaysMinutes > 0 {
		plan.TimeStopAt = now.Add(time.Duration(f.SidewaysMinutes) * time.Minute)
		plan.TimeStopAbove = math.Round(p.Entry*(1+f.SidewaysMinGainPct/PERCENT)*CENT) / CENT
	}
	id, err := s.Paper(sig, plan)
	if err != nil {
		return p, err
	}
	return s.Store.Update(p.ID, func(q *Play) {
		q.Status, q.JournalID, q.ArmedAt, q.BrokeRules, q.UpdatedAt = STATUS_ARMED, id, now.UTC(), broke, now.UTC()
	})
}

// headlines reads newsd's items tagged with the symbol. Best effort: no newsd, no headlines.
func (s *Service) headlines(ctx context.Context, symbol string) []Headline {
	if s.NewsURL == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, NEWS_TIMEOUT)
	defer cancel()
	u := fmt.Sprintf("%s/api/events?%s", strings.TrimRight(s.NewsURL, "/"), url.Values{"tag": {symbol}, "limit": {fmt.Sprint(HEADLINE_LIMIT)}}.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil
	}
	res, err := s.HTTP.Do(req)
	if err != nil {
		return nil
	}
	defer res.Body.Close()
	var out []Headline
	if res.StatusCode != http.StatusOK || json.NewDecoder(io.LimitReader(res.Body, MAX_BODY)).Decode(&out) != nil {
		return nil
	}
	return out
}

// analyze runs one AI read in the background and files it on the play, error or not.
func (s *Service) analyze(id, provider, link string) {
	p, err := s.Store.Play(id)
	if err != nil {
		return
	}
	now := time.Now()
	rep := Build(p, s.Store.Rules(), Book{Others: s.Store.Plays(), Now: now})
	ctx := context.Background()
	a, err := Analyze(ctx, s.HTTP, provider, p, rep, s.headlines(ctx, p.Contract.Symbol), link)
	if err != nil {
		a = Analysis{At: time.Now().UTC(), Provider: provider, Error: err.Error()}
		log.Printf("plays: analyze %s: %v", id, err)
	}
	s.Store.Update(id, func(q *Play) {
		q.Analyses = append(q.Analyses, a)
		q.Analyzing = ""
		// The model's evidence joins the play unverified, so it never moves the odds on its own.
		for _, e := range a.Evidence {
			q.Evidence = append(q.Evidence, Evidence{Kind: e.Kind, Label: e.Label, Stance: e.Stance, Source: e.Source, URL: e.URL,
				Breaking: e.Breaking, SourceCount: e.SourceCount, AI: true, At: a.At})
		}
	})
}

func decode(r *http.Request, v any) error {
	return json.NewDecoder(io.LimitReader(r.Body, MAX_BODY)).Decode(v)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

// Routes returns the plays API, to mount behind the runner's origin and host checks.
//
//	GET    /api/plays                 every play with its report (fills synced from the journal)
//	POST   /api/plays                 save a play (new when it has no id)
//	DELETE /api/plays/{id}            remove a play
//	POST   /api/plays/{id}/refresh    reload the market snapshot
//	POST   /api/plays/{id}/paper      paper trade it now
//	POST   /api/plays/{id}/analyze    ask an AI provider to read it   {"provider":"anthropic"}
//	POST   /api/plays/link            paste a link or callout          {"url":"...","provider":"","analyze":true}
//	GET    /api/plays/links           the link history
//	GET    /api/plays/rules           the rulebook
//	PUT    /api/plays/rules           save the rulebook
//	POST   /api/plays/rules/reset     back to the DOGS defaults
//	GET    /api/plays/ai              which AI providers are connected (never the keys)
func (s *Service) Routes() map[string]http.Handler {
	guard := func(h http.HandlerFunc) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if s.MayWrite != nil && !s.MayWrite(r) {
				writeErr(w, http.StatusUnauthorized, errors.New("open the desk on this machine, or send the webhook secret"))
				return
			}
			h(w, r)
		})
	}
	withPlay := func(fn func(http.ResponseWriter, *http.Request, Play)) http.Handler {
		return guard(func(w http.ResponseWriter, r *http.Request) {
			p, err := s.Store.Play(r.PathValue("id"))
			if err != nil {
				writeErr(w, http.StatusNotFound, err)
				return
			}
			fn(w, r, p)
		})
	}
	reply := func(w http.ResponseWriter, p Play) {
		writeJSON(w, http.StatusOK, s.item(p, s.Store.Plays(), s.Store.Rules(), time.Now()))
	}

	return map[string]http.Handler{
		"GET /api/plays": http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			s.sync()
			all, rules, now := s.Store.Plays(), s.Store.Rules(), time.Now()
			items := make([]Item, 0, len(all))
			for _, p := range all {
				items = append(items, s.item(p, all, rules, now))
			}
			writeJSON(w, http.StatusOK, items)
		}),
		"POST /api/plays": guard(func(w http.ResponseWriter, r *http.Request) {
			var in Play
			if err := decode(r, &in); err != nil {
				writeErr(w, http.StatusBadRequest, err)
				return
			}
			now := time.Now().UTC()
			if in.ID == "" {
				in.ID, in.CreatedAt, in.Status = NewID(now), now, STATUS_DRAFT
				if in.Contract.Market == "" {
					in.Contract.Market = map[bool]string{true: MARKET_OPTION, false: MARKET_STOCK}[in.Contract.Right != ""]
				}
			} else if old, err := s.Store.Play(in.ID); err == nil {
				if old.Status != STATUS_DRAFT {
					writeErr(w, http.StatusConflict, errors.New("a play that is paper trading or closed can't be edited; copy it instead"))
					return
				}
				// The engine owns these; an edit from the page can't change them.
				in.CreatedAt, in.Status, in.Analyses, in.Analyzing, in.Links = old.CreatedAt, old.Status, old.Analyses, old.Analyzing, old.Links
			}
			in.UpdatedAt = now
			if err := s.Store.Save(in); err != nil {
				writeErr(w, http.StatusInternalServerError, err)
				return
			}
			reply(w, in)
		}),
		"DELETE /api/plays/{id}": withPlay(func(w http.ResponseWriter, r *http.Request, p Play) {
			if err := s.Store.Delete(p.ID); err != nil {
				writeErr(w, http.StatusInternalServerError, err)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		}),
		"POST /api/plays/{id}/refresh": withPlay(func(w http.ResponseWriter, r *http.Request, p Play) {
			ctx, cancel := context.WithTimeout(r.Context(), SNAPSHOT_TIMEOUT)
			defer cancel()
			m := s.Resolver.Snapshot(ctx, p.Contract, time.Now())
			p, err := s.Store.Update(p.ID, func(q *Play) {
				q.Market = &m
				kept := q.Evidence[:0]
				for _, e := range q.Evidence {
					if !e.Auto {
						kept = append(kept, e)
					}
				}
				q.Evidence = append(autoEvidence(m, q.Contract.Right != RIGHT_PUT), kept...)
			})
			if err != nil {
				writeErr(w, http.StatusInternalServerError, err)
				return
			}
			reply(w, p)
		}),
		// The stake slider: preview any size without saving, then save it on release (drafts only).
		"GET /api/plays/{id}/preview": http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p, err := s.Store.Play(r.PathValue("id"))
			if err != nil {
				writeErr(w, http.StatusNotFound, err)
				return
			}
			units, _ := strconv.ParseFloat(r.URL.Query().Get("units"), 64)
			reply(w, Resize(p, units))
		}),
		"POST /api/plays/{id}/size": withPlay(func(w http.ResponseWriter, r *http.Request, p Play) {
			var in struct {
				Units float64 `json:"units"`
			}
			if err := decode(r, &in); err != nil || in.Units <= 0 {
				writeErr(w, http.StatusBadRequest, errors.New("units must be a positive number"))
				return
			}
			if p.Status != STATUS_DRAFT {
				writeErr(w, http.StatusConflict, errors.New("only a draft can be resized; this one is already paper trading"))
				return
			}
			sized := Resize(p, in.Units)
			p, err := s.Store.Update(p.ID, func(q *Play) {
				q.Qty, q.Ladder, q.Runner, q.UpdatedAt = sized.Qty, sized.Ladder, sized.Runner, time.Now().UTC()
			})
			if err != nil {
				writeErr(w, http.StatusInternalServerError, err)
				return
			}
			reply(w, p)
		}),
		"POST /api/plays/{id}/paper": withPlay(func(w http.ResponseWriter, r *http.Request, p Play) {
			p, err := s.paper(p, time.Now())
			if err != nil {
				writeErr(w, http.StatusConflict, err)
				return
			}
			reply(w, p)
		}),
		"POST /api/plays/{id}/analyze": withPlay(func(w http.ResponseWriter, r *http.Request, p Play) {
			var in struct {
				Provider string `json:"provider"`
			}
			decode(r, &in)
			if in.Provider == "" {
				for _, x := range Providers() {
					if x.Connected {
						in.Provider = x.ID
						break
					}
				}
			}
			if in.Provider == "" {
				writeErr(w, http.StatusPreconditionFailed, ErrNoProvider)
				return
			}
			link := ""
			if len(p.Links) > 0 {
				link = p.Links[0]
			}
			p, _ = s.Store.Update(p.ID, func(q *Play) { q.Analyzing = in.Provider })
			go s.analyze(p.ID, in.Provider, link)
			reply(w, p)
		}),
		"POST /api/plays/link": guard(s.link),
		"GET /api/plays/links": http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, http.StatusOK, s.Store.Links())
		}),
		"GET /api/plays/rules": http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, http.StatusOK, s.Store.Rules())
		}),
		"PUT /api/plays/rules": guard(func(w http.ResponseWriter, r *http.Request) {
			var in Rules
			if err := decode(r, &in); err != nil {
				writeErr(w, http.StatusBadRequest, err)
				return
			}
			out, err := s.Store.SetRules(in)
			if err != nil {
				writeErr(w, http.StatusInternalServerError, err)
				return
			}
			writeJSON(w, http.StatusOK, out)
		}),
		"POST /api/plays/rules/reset": guard(func(w http.ResponseWriter, r *http.Request) {
			out, err := s.Store.SetRules(DOGSRules())
			if err != nil {
				writeErr(w, http.StatusInternalServerError, err)
				return
			}
			writeJSON(w, http.StatusOK, out)
		}),
		"GET /api/plays/ai": http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, http.StatusOK, Providers())
		}),
	}
}

// link is the inbox: a pasted Robinhood or Webull link (or an option symbol, or callout text)
// becomes a complete play with the market loaded, is paper traded at once when the rulebook says
// so, and is read by AI in the background when a provider is connected.
func (s *Service) link(w http.ResponseWriter, r *http.Request) {
	var in struct {
		URL      string `json:"url"`
		Provider string `json:"provider"`
		Analyze  bool   `json:"analyze"`
		Theme    string `json:"theme"`
	}
	if err := decode(r, &in); err != nil || strings.TrimSpace(in.URL) == "" {
		writeErr(w, http.StatusBadRequest, errors.New("paste a link or a contract"))
		return
	}
	now := time.Now()
	ctx, cancel := context.WithTimeout(r.Context(), SNAPSHOT_TIMEOUT)
	defer cancel()
	entry := Link{ID: NewID(now), At: now.UTC(), URL: strings.TrimSpace(in.URL)}
	c, platform, err := s.Resolver.Resolve(ctx, entry.URL)
	entry.Platform, entry.Contract = platform, c
	if err != nil {
		entry.Error = err.Error()
		s.Store.AddLink(entry)
		writeErr(w, http.StatusUnprocessableEntity, err)
		return
	}
	rules := s.Store.Rules()
	p := Draft(c, s.Resolver.Snapshot(ctx, c, now), rules.Template, now)
	p.ID, p.Links, p.Theme = NewID(now), []string{entry.URL}, strings.TrimSpace(in.Theme)
	if err := s.Store.Save(p); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	entry.PlayID = p.ID
	s.Store.AddLink(entry)

	if rules.AutoPaper {
		if armed, err := s.paper(p, now); err == nil {
			p = armed
		} else {
			log.Printf("plays: auto paper %s: %v", p.ID, err)
		}
	}
	if in.Analyze {
		provider := in.Provider
		for _, x := range Providers() {
			if provider == "" && x.Connected {
				provider = x.ID
			}
		}
		if provider != "" {
			p, _ = s.Store.Update(p.ID, func(q *Play) { q.Analyzing = provider })
			go s.analyze(p.ID, provider, entry.URL)
		}
	}
	writeJSON(w, http.StatusOK, s.item(p, s.Store.Plays(), rules, now))
}
