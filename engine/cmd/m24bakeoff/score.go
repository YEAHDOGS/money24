package main

import (
	"fmt"
	"math"
	"slices"
	"sort"
	"strings"

	"github.com/YEAHDOGS/money24/engine/dogsai"
	"github.com/YEAHDOGS/money24/engine/localai"
)

const (
	MAX_MISTAKES    = 4  // examples shown per job per model
	SHOWN_LINES     = 3  // headlines shown per example
	SHOWN_CHARS     = 90 // characters shown per headline
	SUMMARY_WORDS   = 30 // the prompt's limit on the summary
	BASELINE_RULES  = "newsd rules (no model)"
	ALWAYS_UP       = "always up"
	NO_DATA         = "–"
	FINBERT_NEUTRAL = 0.0
)

// Score is one variant's marks on every job.
type Score struct {
	Variant                      string
	Answers, Valid               int
	EventsRight, EventsN         int
	OneRight, OneN               int
	SeveralRight, SeveralN       int
	TP, FP, FN, TickExact, TickN int
	DirHit, DirCommit, DirN      int
	LongHit, LongCommit, LongN   int
	FlatUnclear, FlatN           int
	ImportanceRho                float64
	ImportanceN                  int
	SummaryOK, SummaryN          int
	Seconds, TokensPerSec        []float64
	OutputTokens                 []float64
	LoadSeconds                  float64
	MemoryGiB, VRAMGiB           float64
	Mistakes                     map[string][]string
	Note                         string
}

func median(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	s := slices.Clone(xs)
	sort.Float64s(s)
	return s[len(s)/2]
}

// ranks gives each value its average rank, for Spearman's correlation.
func ranks(xs []float64) []float64 {
	idx := make([]int, len(xs))
	for i := range idx {
		idx[i] = i
	}
	sort.Slice(idx, func(a, b int) bool { return xs[idx[a]] < xs[idx[b]] })
	out := make([]float64, len(xs))
	for i := 0; i < len(idx); {
		j := i
		for j+1 < len(idx) && xs[idx[j+1]] == xs[idx[i]] {
			j++
		}
		for k := i; k <= j; k++ {
			out[idx[k]] = float64(i+j)/2 + 1
		}
		i = j + 1
	}
	return out
}

// spearman is the rank correlation of two series: 1 when one rises with the other, 0 when unrelated.
func spearman(a, b []float64) float64 {
	if len(a) < 3 {
		return math.NaN()
	}
	ra, rb := ranks(a), ranks(b)
	var ma, mb float64
	for i := range ra {
		ma += ra[i]
		mb += rb[i]
	}
	ma /= float64(len(ra))
	mb /= float64(len(rb))
	var num, da, db float64
	for i := range ra {
		num += (ra[i] - ma) * (rb[i] - mb)
		da += (ra[i] - ma) * (ra[i] - ma)
		db += (rb[i] - mb) * (rb[i] - mb)
	}
	if da == 0 || db == 0 {
		return math.NaN()
	}
	return num / math.Sqrt(da*db)
}

func shown(it Item) string {
	lines := []string{}
	for _, h := range it.Headlines[:min(len(it.Headlines), SHOWN_LINES)] {
		if len(h) > SHOWN_CHARS {
			h = h[:SHOWN_CHARS] + "…"
		}
		lines = append(lines, "“"+h+"”")
	}
	more := ""
	if len(it.Headlines) > SHOWN_LINES {
		more = fmt.Sprintf(" (+%d more)", len(it.Headlines)-SHOWN_LINES)
	}
	return strings.Join(lines, " / ") + more
}

func (s *Score) mistake(job, line string) {
	if len(s.Mistakes[job]) < MAX_MISTAKES {
		s.Mistakes[job] = append(s.Mistakes[job], line)
	}
}

// longLabel is the three-day move's direction, or empty inside the threshold or without data.
func longLabel(it Item) string {
	if it.LongPct == nil {
		return ""
	}
	switch {
	case *it.LongPct >= MOVE_THRESHOLD_PCT:
		return UP
	case *it.LongPct <= -MOVE_THRESHOLD_PCT:
		return DOWN
	}
	return LABEL_FLAT
}

// scoreDirection marks one call against an item's one- and three-day moves.
func (s *Score) scoreDirection(it Item, call string) {
	committed := call == UP || call == DOWN
	if it.Direction == LABEL_FLAT {
		s.FlatN++
		if !committed {
			s.FlatUnclear++
		}
	}
	if it.Direction == UP || it.Direction == DOWN {
		s.DirN++
		if committed {
			s.DirCommit++
			if call == it.Direction {
				s.DirHit++
			} else {
				s.mistake("direction", fmt.Sprintf("%s on %s moved %s %+.1f%% (market-adjusted, next trading day); said %s — %s",
					it.ID, it.Instrument, it.Direction, *it.ShortPct, call, shown(it)))
			}
		}
	}
	if long := longLabel(it); long == UP || long == DOWN {
		s.LongN++
		if committed {
			s.LongCommit++
			if call == long {
				s.LongHit++
			}
		}
	}
}

// scoreReading marks one model reading on every job the item has a label for.
func (s *Score) scoreReading(it Item, r localai.StoryReading) {
	if it.Events != "" {
		s.EventsN++
		right := r.Events == it.Events
		if right {
			s.EventsRight++
		} else {
			s.mistake("events", fmt.Sprintf("%s is %s event(s); said %s — %s", it.ID, it.Events, r.Events, shown(it)))
		}
		if it.Events == localai.EVENTS_ONE {
			s.OneN++
			if right {
				s.OneRight++
			}
		} else {
			s.SeveralN++
			if right {
				s.SeveralRight++
			}
		}
	}
	s.TickN++
	said := slices.DeleteFunc(slices.Clone(r.Tickers), func(t string) bool { return slices.Contains(it.Optional, t) })
	missed, extra := []string{}, []string{}
	for _, t := range it.Tickers {
		if slices.Contains(said, t) {
			s.TP++
		} else {
			s.FN++
			missed = append(missed, t)
		}
	}
	for _, t := range said {
		if !slices.Contains(it.Tickers, t) {
			s.FP++
			extra = append(extra, t)
		}
	}
	if len(missed) == 0 && len(extra) == 0 {
		s.TickExact++
	} else {
		s.mistake("tickers", fmt.Sprintf("%s named %v; said %v (missed %v, extra %v) — %s", it.ID, it.Tickers, r.Tickers, missed, extra, shown(it)))
	}
	s.scoreDirection(it, r.Direction)
	s.SummaryN++
	if words := len(strings.Fields(r.Summary)); words > 0 && words <= SUMMARY_WORDS {
		s.SummaryOK++
	}
}

// finbertCall turns headline sentiments into a call: the mean of positive minus negative, weighted by score.
func finbertCall(sents []dogsai.Sentiment) string {
	total := 0.0
	for _, x := range sents {
		switch x.Label {
		case dogsai.SENTIMENT_POS:
			total += x.Score
		case dogsai.SENTIMENT_NEG:
			total -= x.Score
		}
	}
	switch {
	case total > FINBERT_NEUTRAL:
		return UP
	case total < FINBERT_NEUTRAL:
		return DOWN
	}
	return UNCLEAR
}

func scoreVariant(variant string, items map[string]Item, recs []Record) Score {
	s := Score{Variant: variant, Mistakes: map[string][]string{}}
	imp, src := []float64{}, []float64{}
	for i, r := range recs {
		if r.Item == "" {
			s.Note = r.Error
			continue
		}
		it, ok := items[r.Item]
		if !ok {
			continue
		}
		s.Answers++
		if r.MemoryGiB > 0 {
			s.MemoryGiB, s.VRAMGiB = r.MemoryGiB, r.VRAMGiB
		}
		if i == 0 {
			s.LoadSeconds = r.Result.LoadSeconds
		}
		if r.Error != "" {
			s.mistake("shape", fmt.Sprintf("%s: %s", it.ID, r.Error))
			continue
		}
		s.Valid++
		// The first answer carries the model's load; speed is measured on the rest.
		if i > 0 || r.Result.LoadSeconds == 0 {
			s.Seconds = append(s.Seconds, r.WallSeconds)
		}
		if tps := r.Result.TokensPerSecond(); tps > 0 {
			s.TokensPerSec = append(s.TokensPerSec, tps)
			s.OutputTokens = append(s.OutputTokens, float64(r.Result.OutputTokens))
		}
		if r.Sentiments != nil {
			s.scoreDirection(it, finbertCall(r.Sentiments))
			continue
		}
		if r.Reading == nil {
			continue
		}
		s.scoreReading(it, *r.Reading)
		imp = append(imp, float64(r.Reading.Importance))
		src = append(src, math.Log1p(float64(it.Sources)))
	}
	s.ImportanceRho, s.ImportanceN = spearman(imp, src), len(imp)
	return s
}

// baselines are the marks with no model at all: newsd's own rules, and always saying up.
func baselines(items []Item) []Score {
	rules := Score{Variant: BASELINE_RULES, Mistakes: map[string][]string{}}
	up := Score{Variant: ALWAYS_UP, Mistakes: map[string][]string{}}
	for _, it := range items {
		if it.Events != "" {
			// newsd put every story's headlines in one group, so its answer is always "one".
			rules.EventsN++
			if it.Events == localai.EVENTS_ONE {
				rules.EventsRight++
				rules.OneN++
				rules.OneRight++
			} else {
				rules.SeveralN++
			}
		}
		rules.scoreDirection(it, it.Baseline)
		up.scoreDirection(it, UP)
	}
	return []Score{rules, up}
}

func f1(s Score) string {
	if s.TickN == 0 {
		return NO_DATA
	}
	p := float64(s.TP) / math.Max(1, float64(s.TP+s.FP))
	r := float64(s.TP) / math.Max(1, float64(s.TP+s.FN))
	if p+r == 0 {
		return "0.00"
	}
	return fmt.Sprintf("%.2f", 2*p*r/(p+r))
}

func num(v float64, format string) string {
	if v == 0 || math.IsNaN(v) {
		return NO_DATA
	}
	return fmt.Sprintf(format, v)
}

// renderReport writes the tables and the mistakes as Markdown.
func renderReport(set []Item, records map[string][]Record, order []string) string {
	items := map[string]Item{}
	counts := map[string]int{}
	for _, it := range set {
		items[it.ID] = it
		counts[it.Kind]++
		counts["dir:"+it.Direction]++
	}
	scores := []Score{}
	for _, v := range order {
		scores = append(scores, scoreVariant(v, items, records[v]))
	}
	scores = append(scores, baselines(set)...)
	var b strings.Builder
	fmt.Fprintf(&b, "Set: %d items — %d stories (one event), %d splices (several events), %d day bundles. "+
		"Direction labels (next trading day, market-adjusted, ±%.0f%%): %d up, %d down, %d flat, %d without price data.\n\n",
		len(set), counts[KIND_STORY], counts[KIND_SPLICE], counts[KIND_BUNDLE], MOVE_THRESHOLD_PCT,
		counts["dir:"+UP], counts["dir:"+DOWN], counts["dir:"+LABEL_FLAT], counts["dir:"])
	b.WriteString("| Model | Valid answers | One event or several | (one / several) | Tickers F1 | Tickers exact | " +
		"Direction hit when it calls (1d) | Calls made | Direction hit (3d) | Importance ρ vs outlets | Summary ≤30 words | " +
		"Seconds per item | Tokens/s (writing) | Output tokens | Load s | Memory GiB (GPU) |\n")
	b.WriteString("|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|\n")
	for _, s := range scores {
		mem := NO_DATA
		if s.MemoryGiB > 0 {
			mem = fmt.Sprintf("%.1f (%.1f)", s.MemoryGiB, s.VRAMGiB)
		}
		valid := pct(s.Valid, s.Answers) + fmt.Sprintf(" of %d", s.Answers)
		if s.Answers == 0 {
			valid = NO_DATA
		}
		fmt.Fprintf(&b, "| %s | %s | %s (n=%d) | %s / %s | %s | %s | %s (%d/%d) | %s of %d | %s (%d/%d) | %s | %s | %s | %s | %s | %s | %s |\n",
			s.Variant, valid, pct(s.EventsRight, s.EventsN), s.EventsN, pct(s.OneRight, s.OneN), pct(s.SeveralRight, s.SeveralN),
			f1(s), pct(s.TickExact, s.TickN), pct(s.DirHit, s.DirCommit), s.DirHit, s.DirCommit, pct(s.DirCommit, s.DirN), s.DirN,
			pct(s.LongHit, s.LongCommit), s.LongHit, s.LongCommit, num(s.ImportanceRho, "%.2f"), pct(s.SummaryOK, s.SummaryN),
			num(median(s.Seconds), "%.1f"), num(median(s.TokensPerSec), "%.1f"), num(median(s.OutputTokens), "%.0f"),
			num(s.LoadSeconds, "%.0f"), mem)
	}
	for _, s := range scores {
		if s.Note != "" {
			fmt.Fprintf(&b, "\n**%s** was not run: %s\n", s.Variant, s.Note)
		}
	}
	for _, s := range scores {
		if len(s.Mistakes) == 0 {
			continue
		}
		fmt.Fprintf(&b, "\n### %s: mistakes\n\n", s.Variant)
		for _, job := range []string{"shape", "events", "tickers", "direction"} {
			for _, m := range s.Mistakes[job] {
				fmt.Fprintf(&b, "- **%s** %s\n", job, m)
			}
		}
	}
	return b.String()
}
