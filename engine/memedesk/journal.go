package memedesk

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/YEAHDOGS/money24/engine/meme"
)

const (
	JOURNAL_FILE    = "meme-desk-journal.jsonl"
	JOURNAL_VERSION = 1
	JOURNAL_MODE    = 0o600
)

type journalLine struct {
	Version  int          `json:"version"`
	Rulebook *Rulebook    `json:"rulebook,omitempty"`
	Record   *meme.Record `json:"record,omitempty"`
	Clock    time.Time    `json:"clock,omitzero"`
}

// restoreJournal rebuilds the desk from the same stream of observations and
// rulebook revisions that produced its original decisions. Only the unfinished
// last line from an interrupted write may be discarded.
func restoreJournal(file *os.File, model *engine) error {
	reader := bufio.NewReader(file)
	var offset int64
	for {
		line, err := reader.ReadBytes('\n')
		if errors.Is(err, io.EOF) {
			if len(line) != 0 {
				return file.Truncate(offset)
			}
			return nil
		}
		if err != nil {
			return err
		}
		var entry journalLine
		if err := json.Unmarshal(line, &entry); err != nil {
			return fmt.Errorf("journal offset %d: %w", offset, err)
		}
		if err := entry.apply(model); err != nil {
			return fmt.Errorf("journal offset %d: %w", offset, err)
		}
		offset += int64(len(line))
	}
}

func (line journalLine) apply(model *engine) error {
	if line.Version != JOURNAL_VERSION {
		return fmt.Errorf("unsupported journal version %d", line.Version)
	}
	switch {
	case line.Rulebook != nil:
		rules, err := line.Rulebook.build()
		if err != nil {
			return err
		}
		model.configure(*line.Rulebook, rules)
	case line.Record != nil && model.rulebook.CapitalUSD > 0:
		model.observe(*line.Record, true)
	case !line.Clock.IsZero() && model.rulebook.CapitalUSD > 0:
		model.advance(line.Clock)
	default:
		return errors.New("journal line is missing data or initial rulebook")
	}
	return nil
}

func appendJournal(file *os.File, line journalLine) error {
	line.Version = JOURNAL_VERSION
	data, err := json.Marshal(line)
	if err != nil {
		return err
	}
	_, err = io.Copy(file, bytes.NewReader(append(data, '\n')))
	return err
}
