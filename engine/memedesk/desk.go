package memedesk

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"time"

	"github.com/YEAHDOGS/money24/engine/meme"
)

const (
	ROUTE_STATUS    = "GET /api/memedesk"
	ROUTE_RULEBOOK  = "PUT /api/memedesk/strategies"
	HEADER_TYPE     = "Content-Type"
	CONTENT_JSON    = "application/json"
	RULEBOOK_RELOAD = time.Minute
	DESK_TICK       = 5 * time.Second
)

// Desk owns the live journal, rulebook and paper account. Its methods serialize
// recorder events, rule changes and clock ticks before updating the account.
type Desk struct {
	MayWrite func(*http.Request) bool

	mu          sync.Mutex
	model       *engine
	journal     *os.File
	path        string
	nextReload  time.Time
	rulebookErr error
	journalErr  error
}

func Open(dataDir string) (*Desk, error) {
	d := &Desk{model: newEngine(), path: filepath.Join(dataDir, RULEBOOK_FILE)}
	file, err := os.OpenFile(filepath.Join(dataDir, JOURNAL_FILE), os.O_CREATE|os.O_APPEND|os.O_RDWR, JOURNAL_MODE)
	if err != nil {
		return nil, err
	}
	d.journal = file
	if err := restoreJournal(file, d.model); err != nil {
		file.Close()
		return nil, err
	}
	d.reload(time.Now())
	if d.model.rulebook.CapitalUSD == 0 {
		file.Close()
		return nil, errors.Join(d.rulebookErr, d.journalErr)
	}
	return d, nil
}

func (d *Desk) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.journal.Close()
}

func (d *Desk) reload(now time.Time) {
	if now.Before(d.nextReload) {
		return
	}
	d.nextReload = now.Add(RULEBOOK_RELOAD)
	rb, save, err := readRulebook(d.path)
	if err != nil {
		d.rulebookErr = err
		return
	}
	d.rulebookErr = d.applyRulebook(rb, save)
}

func (d *Desk) applyRulebook(rb Rulebook, save bool) error {
	if d.journalErr != nil {
		return d.journalErr
	}
	if reflect.DeepEqual(rb, d.model.rulebook) && !save {
		return nil
	}
	rules, err := rb.build()
	if err != nil {
		return err
	}
	if save {
		if err := saveRulebook(d.path, rb); err != nil {
			return err
		}
	}
	if reflect.DeepEqual(rb, d.model.rulebook) {
		return nil
	}
	if err := d.append(journalLine{Rulebook: &rb}); err != nil {
		return err
	}
	d.model.configure(rb, rules)
	return nil
}

func (d *Desk) append(line journalLine) error {
	if d.journalErr != nil {
		return d.journalErr
	}
	if err := appendJournal(d.journal, line); err != nil {
		d.journalErr = fmt.Errorf("meme desk journal: %w", err)
	}
	return d.journalErr
}

// Observe consumes a recorder event only after it is written to the desk journal.
func (d *Desk) Observe(row meme.Record) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.reload(time.Now())
	if err := d.append(journalLine{Record: &row}); err == nil {
		d.model.observe(row, true)
	}
}

// Run advances pending decisions even when the recorder has gone quiet.
func (d *Desk) Run(ctx context.Context) {
	ticker := time.NewTicker(DESK_TICK)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case at := <-ticker.C:
			d.tick(at.UTC())
		}
	}
}

func (d *Desk) tick(at time.Time) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.reload(at)
	if d.model.waiting.Len()+d.model.watching.Len() == 0 && d.model.day == at.Format(meme.DAY_LAYOUT) {
		return
	}
	if err := d.append(journalLine{Clock: at}); err == nil {
		d.model.advance(at)
	}
}

func (d *Desk) Status() Status {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := d.model.status(true)
	if d.rulebookErr != nil {
		out.RulebookError = d.rulebookErr.Error()
	}
	if d.journalErr != nil {
		out.JournalError = d.journalErr.Error()
	}
	return out
}

func (d *Desk) Routes() map[string]http.Handler {
	return map[string]http.Handler{
		ROUTE_STATUS: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set(HEADER_TYPE, CONTENT_JSON)
			json.NewEncoder(w).Encode(d.Status())
		}),
		ROUTE_RULEBOOK: http.HandlerFunc(d.put),
	}
}

func (d *Desk) put(w http.ResponseWriter, r *http.Request) {
	if d.MayWrite == nil || !d.MayWrite(r) {
		http.Error(w, "write access required", http.StatusForbidden)
		return
	}
	rb, err := decodeRulebook(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	rb.offerDefaults()
	if _, err := rb.build(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	d.mu.Lock()
	err = d.applyRulebook(rb, true)
	if err == nil {
		d.rulebookErr = nil
	}
	d.mu.Unlock()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set(HEADER_TYPE, CONTENT_JSON)
	json.NewEncoder(w).Encode(rb)
}
