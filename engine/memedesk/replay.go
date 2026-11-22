package memedesk

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"time"

	"github.com/YEAHDOGS/money24/engine/meme"
)

const REPLAY_CONTEXT_DAYS = 2

type ReplayResult struct {
	Day           string `json:"replay_day"`
	Files         int    `json:"files_read"`
	DayRecords    int    `json:"day_records"`
	MalformedRows int    `json:"malformed_rows"`
	Status
}

// Replay applies the current rulebook to a UTC-day cohort. Earlier files
// establish safety and launch context. Later files can only update that cohort.
func Replay(ctx context.Context, dataDir, recorderDir, day string) (ReplayResult, error) {
	start, err := time.Parse(meme.DAY_LAYOUT, day)
	if err != nil {
		return ReplayResult{}, fmt.Errorf("invalid replay day: %w", err)
	}
	rb, _, err := readRulebook(filepath.Join(dataDir, RULEBOOK_FILE))
	if err != nil {
		return ReplayResult{}, err
	}
	rules, err := rb.build()
	if err != nil {
		return ReplayResult{}, err
	}
	model := newEngine()
	model.configure(rb, rules)
	result := ReplayResult{Day: day}
	for offset := -REPLAY_CONTEXT_DAYS; offset <= REPLAY_CONTEXT_DAYS; offset++ {
		paths, err := meme.RecordFiles(recorderDir, start.AddDate(0, 0, offset).Format(meme.DAY_LAYOUT))
		if err != nil {
			return result, err
		}
		for _, path := range paths {
			result.Files++
			if err := replayFile(ctx, model, &result, recorderDir, path); err != nil {
				return result, err
			}
		}
	}
	result.Status = model.status(true)
	return result, nil
}

func replayFile(ctx context.Context, model *engine, result *ReplayResult, dir, path string) error {
	reader, err := meme.OpenRecords(dir, path)
	if err != nil {
		return err
	}
	defer reader.Close()
	for ctx.Err() == nil {
		line, _, err := reader.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if errors.Is(err, meme.ErrIncompleteRecord) {
			result.MalformedRows++
			return nil
		}
		if err != nil {
			return fmt.Errorf("replay %s: %w", path, err)
		}
		var row meme.Record
		if err := json.Unmarshal(line, &row); err != nil || row.At.IsZero() || row.Kind == "" {
			result.MalformedRows++
			continue
		}
		isDay := row.At.UTC().Format(meme.DAY_LAYOUT) == result.Day
		if isDay {
			result.DayRecords++
		}
		model.observe(row, isDay)
	}
	return ctx.Err()
}
