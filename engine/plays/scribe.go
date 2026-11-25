package plays

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const (
	SCRIBE_FILE              = "scribe.json"
	SCRIBE_LIMIT             = 100
	SCRIBE_TIP_MAX           = 4000
	SCRIBE_BODY_MAX          = 32 << 10
	SCRIBE_RESULT_MAX        = 128 << 10
	SCRIBE_ITEMS_MAX         = 20
	SCRIBE_CANDIDATES_MAX    = 6
	SCRIBE_MIN_LEAD          = 3 * time.Hour
	SCRIBE_DEFAULT_LEAD      = 24 * time.Hour
	SCRIBE_MAX_HORIZON       = 366 * 24 * time.Hour
	SCRIBE_COOLDOWN          = 30 * time.Second
	SCRIBE_RESEARCHING       = "researching"
	SCRIBE_READY             = "ready"
	SCRIBE_ERROR             = "error"
	SCRIBE_PRIVATE_PERMS     = 0o600
	SCRIBE_INTERRUPTED_ERROR = "Research was interrupted by a runner restart. Submit the tip again."
)

// ScribeRequest is an unverified tip and the intended planning window.
type ScribeRequest struct {
	Tip             string    `json:"tip"`
	Provider        string    `json:"provider"`
	ExpectedEntryAt time.Time `json:"expected_entry_at"`
	ExpectedExitAt  time.Time `json:"expected_exit_at"`
}

// Scribe is a saved research proposal, never an executable order or a verified claim.
type Scribe struct {
	ID              string            `json:"id"`
	Tip             string            `json:"tip"`
	Status          string            `json:"status"`
	CreatedAt       time.Time         `json:"created_at"`
	UpdatedAt       time.Time         `json:"updated_at"`
	ExpectedEntryAt time.Time         `json:"expected_entry_at"`
	ExpectedExitAt  time.Time         `json:"expected_exit_at"`
	Provider        string            `json:"provider"`
	ProviderName    string            `json:"provider_name"`
	Model           string            `json:"model,omitempty"`
	Web             bool              `json:"web"`
	Summary         string            `json:"summary"`
	Corrections     []string          `json:"corrections"`
	Evidence        []ScribeEvidence  `json:"evidence"`
	Candidates      []ScribeCandidate `json:"candidates"`
	Error           string            `json:"error,omitempty"`
}

// ScribeEvidence is model-reported sourcing; a citation alone never verifies a tip.
type ScribeEvidence struct {
	Label    string `json:"label"`
	Source   string `json:"source"`
	URL      string `json:"url"`
	Verified bool   `json:"verified"`
}

type scribeRun struct {
	paper   sync.Mutex // serializes manual paper starts
	mu      sync.Mutex
	active  bool
	started time.Time
}

func normalizeScribe(in *ScribeRequest, now time.Time) error {
	in.Tip, in.Provider = strings.TrimSpace(in.Tip), strings.TrimSpace(in.Provider)
	if in.Tip == "" || !utf8.ValidString(in.Tip) || utf8.RuneCountInString(in.Tip) > SCRIBE_TIP_MAX {
		return fmt.Errorf("enter a tip of 1–%d characters", SCRIBE_TIP_MAX)
	}
	if in.Provider == ALL_PROVIDERS {
		return errors.New("choose one AI for this research request")
	}
	if in.ExpectedEntryAt.IsZero() {
		in.ExpectedEntryAt = now.Add(SCRIBE_DEFAULT_LEAD)
	}
	if in.ExpectedExitAt.IsZero() {
		in.ExpectedExitAt = in.ExpectedEntryAt.Add(SCRIBE_DEFAULT_LEAD)
	}
	in.ExpectedEntryAt, in.ExpectedExitAt = in.ExpectedEntryAt.UTC(), in.ExpectedExitAt.UTC()
	if in.ExpectedEntryAt.Before(now.Add(SCRIBE_MIN_LEAD)) {
		return errors.New("expected entry must be at least three hours from now")
	}
	if !in.ExpectedExitAt.After(in.ExpectedEntryAt) {
		return errors.New("expected exit must be after expected entry")
	}
	if in.ExpectedExitAt.After(now.Add(SCRIBE_MAX_HORIZON)) {
		return errors.New("plan entry and exit within the next year")
	}
	return nil
}

func (s *Service) scribeHistory(w http.ResponseWriter, r *http.Request) {
	rows, err := s.Store.Scribes()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (s *Service) startScribe(w http.ResponseWriter, r *http.Request) {
	var in ScribeRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, SCRIBE_BODY_MAX))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, errors.New("send a tip and valid entry and exit timestamps"))
		return
	}
	if decoder.Decode(new(any)) != io.EOF {
		writeErr(w, http.StatusBadRequest, errors.New("send one research request"))
		return
	}
	now := time.Now().UTC()
	if err := normalizeScribe(&in, now); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	providers := s.pick(in.Provider)
	if len(providers) == 0 {
		writeErr(w, http.StatusPreconditionFailed, ErrNoProvider)
		return
	}
	s.scribe.mu.Lock()
	if s.scribe.active || now.Sub(s.scribe.started) < SCRIBE_COOLDOWN {
		s.scribe.mu.Unlock()
		writeErr(w, http.StatusTooManyRequests, errors.New("research is busy; wait for it to finish and at least 30 seconds between requests"))
		return
	}
	s.scribe.active, s.scribe.started = true, now
	s.scribe.mu.Unlock()
	prov := providers[0]
	record := Scribe{ID: NewID(now), Tip: in.Tip, Status: SCRIBE_RESEARCHING, CreatedAt: now, UpdatedAt: now,
		ExpectedEntryAt: in.ExpectedEntryAt, ExpectedExitAt: in.ExpectedExitAt, Provider: prov.ID, ProviderName: prov.Name,
		Web: prov.Web, Corrections: []string{}, Evidence: []ScribeEvidence{}, Candidates: []ScribeCandidate{}}
	if err := s.Store.SaveScribe(record); err != nil {
		s.scribe.mu.Lock()
		s.scribe.active = false
		s.scribe.mu.Unlock()
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	go s.researchScribe(record, prov)
	writeJSON(w, http.StatusAccepted, record)
}

func (s *Service) researchScribe(record Scribe, prov Provider) {
	defer func() {
		s.scribe.mu.Lock()
		s.scribe.active = false
		s.scribe.mu.Unlock()
	}()
	ctx, cancel := context.WithTimeout(context.Background(), ADVICE_TIMEOUT+SNAPSHOT_TIMEOUT)
	defer cancel()
	result, err := askScribe(ctx, s.AI, prov, record)
	if err != nil {
		record.Status, record.Error = SCRIBE_ERROR, err.Error()
	} else {
		record.Status, record.Summary, record.Corrections = SCRIBE_READY, result.Summary, result.Corrections
		record.Evidence, record.Candidates, record.Model, record.Web = result.Evidence, result.Candidates, result.Model, result.Web
		s.verifyScribeMarkets(ctx, &record)
	}
	record.UpdatedAt = time.Now().UTC()
	if err := s.Store.SaveScribe(record); err != nil {
		log.Printf("scribe: save research %s: %v", record.ID, err)
	}
}

func (s *Service) interruptScribe() {
	rows, err := s.Store.Scribes()
	if err != nil {
		log.Printf("scribe: load history: %v", err)
		return
	}
	for _, row := range rows {
		if row.Status != SCRIBE_RESEARCHING {
			continue
		}
		row.Status, row.Error, row.UpdatedAt = SCRIBE_ERROR, SCRIBE_INTERRUPTED_ERROR, time.Now().UTC()
		if err := s.Store.SaveScribe(row); err != nil {
			log.Printf("scribe: recover research %s: %v", row.ID, err)
		}
	}
}

// paperScribe starts one exchange-checked candidate on the prediction desk's paper book. It is a
// manual start inside the planned window; the desk prices the fill from the live order book.
func (s *Service) paperScribe(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ID     string `json:"id"`
		Ticker string `json:"ticker"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, SCRIBE_BODY_MAX)).Decode(&in); err != nil || in.ID == "" || !scribeTicker.MatchString(in.Ticker) {
		writeErr(w, http.StatusBadRequest, errors.New("send a research id and a contract ticker"))
		return
	}
	if s.PaperTip == nil {
		writeErr(w, http.StatusPreconditionFailed, errors.New("the prediction desk is not connected"))
		return
	}
	// One start at a time: a second click cannot buy the same candidate twice.
	s.scribe.paper.Lock()
	defer s.scribe.paper.Unlock()
	rows, err := s.Store.Scribes()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	at := slices.IndexFunc(rows, func(row Scribe) bool { return row.ID == in.ID })
	if at < 0 {
		writeErr(w, http.StatusNotFound, ErrNotFound)
		return
	}
	record := rows[at]
	pick := slices.IndexFunc(record.Candidates, func(c ScribeCandidate) bool { return c.Ticker == in.Ticker })
	now := time.Now().UTC()
	if err := scribeStartable(record, pick, now); err != nil {
		writeErr(w, http.StatusConflict, err)
		return
	}
	candidate := &record.Candidates[pick]
	ctx, cancel := context.WithTimeout(r.Context(), SNAPSHOT_TIMEOUT)
	defer cancel()
	line, err := s.PaperTip(ctx, candidate.Ticker, candidate.Side, record.Tip)
	if err != nil {
		writeErr(w, http.StatusConflict, err)
		return
	}
	candidate.Paper = &ScribePaper{At: line.At, Qty: line.Qty, Price: line.Price, CostUSD: line.CostUSD, FeeUSD: line.FeeUSD}
	record.UpdatedAt = now
	if err := s.Store.SaveScribe(record); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, record)
}

// scribeStartable says whether candidate pick of a proposal may be started on paper now.
func scribeStartable(record Scribe, pick int, now time.Time) error {
	switch {
	case record.Status != SCRIBE_READY || pick < 0:
		return errors.New("this research has no such contract")
	case !record.Candidates[pick].Verified || record.Candidates[pick].QuoteAt.IsZero():
		return errors.New("only a contract the exchange confirmed with a quote can be started")
	case record.Candidates[pick].Paper != nil:
		return errors.New("this contract is already a paper position")
	case now.Before(record.ExpectedEntryAt):
		return errors.New("expected entry has not arrived; start it when due")
	case !now.Before(record.ExpectedExitAt):
		return errors.New("expected exit has passed; research the tip again with a new window")
	}
	return nil
}
