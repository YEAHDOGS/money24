package memedesk

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/YEAHDOGS/money24/engine/meme"
)

const (
	JOURNAL_FILE           = "meme-desk-journal.jsonl"
	JOURNAL_LEGACY_VERSION = 1
	JOURNAL_VERSION        = 2
	JOURNAL_MODE           = 0o600
	JOURNAL_END            = '\n'
	JOURNAL_NEXT           = "0\n"
)

type journalLine struct {
	Version  int            `json:"version"`
	Rulebook *Rulebook      `json:"rulebook,omitempty"`
	Record   *meme.Record   `json:"record,omitempty"` // legacy or unavailable in the recorder
	Position *meme.Position `json:"position,omitempty"`
	Clock    time.Time      `json:"clock,omitzero"`
}

type journalRecords struct {
	dir    string
	reader *meme.RecordReader
	last   meme.Position
}

func (r *journalRecords) read(pos meme.Position) (meme.Record, error) {
	if pos.File == "" || pos.Offset < 0 {
		return meme.Record{}, errors.New("invalid recorder position")
	}
	next := r.last.End
	if r.reader == nil || r.last.File != pos.File || next > pos.Offset {
		if r.reader != nil {
			r.reader.Close()
		}
		reader, err := meme.OpenRecords(r.dir, pos.File)
		r.reader = reader
		if err != nil {
			return meme.Record{}, err
		}
		next = 0
	}
	for next <= pos.Offset {
		line, found, err := r.reader.Next()
		if err != nil {
			return meme.Record{}, err
		}
		if found.Offset == pos.Offset {
			var row meme.Record
			err := json.Unmarshal(line, &row)
			r.last = found
			return row, err
		}
		next = found.End
	}
	return meme.Record{}, errors.New("recorder position is inside a line")
}

// restoreJournal reads complete journal entries. The caller can truncate the
// returned offset to discard the unfinished tail of an interrupted append.
func restoreJournal(file *os.File, model *engine, recorderDir string, rewrite func(journalLine, []byte) error) (int64, error) {
	reader := bufio.NewReader(file)
	records := journalRecords{dir: recorderDir}
	defer func() {
		if records.reader != nil {
			records.reader.Close()
		}
	}()
	var offset int64
	for {
		line, err := reader.ReadBytes(JOURNAL_END)
		if errors.Is(err, io.EOF) {
			return offset, nil
		}
		if err != nil {
			return offset, err
		}
		if err := restoreLine(model, &records, offset, line, rewrite); err != nil {
			return offset, fmt.Errorf("journal offset %d: %w", offset, err)
		}
		offset += int64(len(line))
	}
}

func restoreLine(model *engine, records *journalRecords, offset int64, raw []byte, rewrite func(journalLine, []byte) error) error {
	var line journalLine
	if string(raw) == JOURNAL_NEXT {
		line.Version = JOURNAL_VERSION
		line.Position = &meme.Position{File: records.last.File, Offset: records.last.End}
	} else if err := json.Unmarshal(raw, &line); err != nil {
		return err
	}
	if offset == 0 && line == (journalLine{Version: JOURNAL_VERSION}) {
		return nil
	}
	if line.Version != JOURNAL_LEGACY_VERSION && line.Version != JOURNAL_VERSION {
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
	case line.Position != nil && model.rulebook.CapitalUSD > 0:
		row, err := records.read(*line.Position)
		if err != nil {
			return fmt.Errorf("observation %s at %d: %w", line.Position.File, line.Position.Offset, err)
		}
		model.observe(row, true)
	case !line.Clock.IsZero() && model.rulebook.CapitalUSD > 0:
		model.advance(line.Clock)
	default:
		return errors.New("journal line is missing data or initial rulebook")
	}
	if rewrite != nil {
		return rewrite(line, raw)
	}
	return nil
}

func appendJournal(file *os.File, line journalLine) error {
	line.Version = JOURNAL_VERSION
	return json.NewEncoder(file).Encode(line)
}

func appendPosition(file *os.File, last *meme.Position, pos meme.Position) error {
	if pos.File == "" || pos.Offset < 0 || pos.End <= pos.Offset {
		return errors.New("invalid recorder position")
	}
	var err error
	if pos.File == last.File && pos.Offset == last.End {
		_, err = file.WriteString(JOURNAL_NEXT)
	} else {
		err = appendJournal(file, journalLine{Position: &pos})
	}
	if err == nil {
		*last = pos
	}
	return err
}

func currentJournal(file *os.File) (bool, error) {
	reader := bufio.NewReader(file)
	first, err := reader.ReadBytes(JOURNAL_END)
	if err != nil && !errors.Is(err, io.EOF) {
		return false, err
	}
	if _, seekErr := file.Seek(0, io.SeekStart); seekErr != nil {
		return false, seekErr
	}
	var header journalLine
	return err == nil && json.Unmarshal(first, &header) == nil && header == (journalLine{Version: JOURNAL_VERSION}), nil
}
