// m24bakeoff measures which local models are good at Money 24's reading jobs before any of them is
// trusted. It builds a labeled set from a copy of newsd's database and Yahoo's price history, has each
// model read every item, records what each one said, and scores the answers.
//
//	m24bakeoff --db COPY_OF_news.db --out DIR --build [--audit FILE]
//	m24bakeoff --out DIR --run --models gpt-oss:20b@low,qwen3.5:9b@false,qwen3.5:4b@false [--finbert]
//	m24bakeoff --out DIR --report
//
// A model is name@think, where think is low, medium or high (gpt-oss) or true or false (qwen). The tool
// only reads: it never opens the live database, places no orders and changes nothing in Money 24. It
// is a command run in a terminal window, which is its own window on the desktop; it starts no service.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/YEAHDOGS/money24/engine/dogsai"
	"github.com/YEAHDOGS/money24/engine/localai"
	"github.com/YEAHDOGS/money24/engine/market"
)

const (
	DATASET_FILE   = "dataset.json"
	REPORT_FILE    = "report.md"
	RESULTS_PREFIX = "results-"
	RESULTS_EXT    = ".jsonl"
	FINBERT_ID     = "finbert"
	ITEM_TIMEOUT   = 6 * time.Minute
	BUILD_TIMEOUT  = 20 * time.Minute
	UNLOAD_TIMEOUT = 30 * time.Second
	FILE_PERMS     = 0o644
	DIR_PERMS      = 0o755
	THINK_SEP      = "@"
	DEFAULT_MODELS = "gpt-oss:20b@low,qwen3.5:9b@false,qwen3.5:4b@false"
	WATCHLIST_PATH = "news/watchlist.json"
	RULES_PATH     = "news/story_rules.json"
	GIB            = 1 << 30
)

// Record is what one model said about one item, with what it cost. The whole answer is kept.
type Record struct {
	Variant     string                `json:"variant"`
	Model       string                `json:"model"`
	Think       string                `json:"think"`
	Item        string                `json:"item"`
	At          time.Time             `json:"at"`
	Reading     *localai.StoryReading `json:"reading,omitempty"`
	Sentiments  []dogsai.Sentiment    `json:"sentiments,omitempty"`
	Error       string                `json:"error,omitempty"`
	Result      localai.Result        `json:"result"`
	WallSeconds float64               `json:"wall_seconds"`
	// Memory as Ollama reports it after the variant's first answer: all of the model, and its GPU share.
	MemoryGiB float64 `json:"memory_gib,omitempty"`
	VRAMGiB   float64 `json:"vram_gib,omitempty"`
}

func main() {
	dbPath := flag.String("db", "", "a COPY of newsd's news.db (opened read-only)")
	outDir := flag.String("out", "", "folder for the set, the answers and the report")
	auditPath := flag.String("audit", "", `JSON {"several":[ids],"drop":[ids]}: what a read-through found newsd's stories to be`)
	build := flag.Bool("build", false, "build the labeled set")
	run := flag.Bool("run", false, "have each model read the set")
	report := flag.Bool("report", false, "score the answers and write the report")
	models := flag.String("models", DEFAULT_MODELS, "comma-separated name@think")
	finbert := flag.Bool("finbert", true, "also run FinBERT through DOGS AI when it is up")
	limit := flag.Int("limit", 0, "read only the first N items (0 is all)")
	flag.Parse()
	if *outDir == "" || !*build && !*run && !*report {
		flag.Usage()
		os.Exit(2)
	}
	if err := os.MkdirAll(*outDir, DIR_PERMS); err != nil {
		log.Fatal(err)
	}
	if *build {
		if err := buildCmd(*dbPath, *auditPath, *outDir); err != nil {
			log.Fatal(err)
		}
	}
	if *run {
		if err := runCmd(*outDir, *models, *finbert, *limit); err != nil {
			log.Fatal(err)
		}
	}
	if *report {
		if err := reportCmd(*outDir); err != nil {
			log.Fatal(err)
		}
	}
}

func buildCmd(dbPath, auditPath, outDir string) error {
	if dbPath == "" {
		return errors.New("--build needs --db, a copy of news.db")
	}
	w, err := loadWatch(WATCHLIST_PATH)
	if err != nil {
		return err
	}
	lx, err := loadLexicon(RULES_PATH)
	if err != nil {
		return err
	}
	audit := Audit{}
	if auditPath != "" {
		raw, err := os.ReadFile(auditPath)
		if err != nil {
			return err
		}
		if err := json.Unmarshal(raw, &audit); err != nil {
			return fmt.Errorf("%s: %w", auditPath, err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), BUILD_TIMEOUT)
	defer cancel()
	items, err := buildSet(ctx, dbPath, w, lx, audit, market.NewClient())
	if err != nil {
		return err
	}
	raw, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		return err
	}
	counts := map[string]int{}
	for _, it := range items {
		counts[it.Kind]++
		if it.Direction != "" {
			counts["direction:"+it.Direction]++
		}
	}
	log.Printf("built %d items: %v", len(items), counts)
	return os.WriteFile(filepath.Join(outDir, DATASET_FILE), raw, FILE_PERMS)
}

func loadSet(outDir string) ([]Item, error) {
	raw, err := os.ReadFile(filepath.Join(outDir, DATASET_FILE))
	if err != nil {
		return nil, err
	}
	var items []Item
	return items, json.Unmarshal(raw, &items)
}

func variantFile(outDir, variant string) string {
	safe := strings.NewReplacer(":", "_", "@", "-", "/", "_").Replace(variant)
	return filepath.Join(outDir, RESULTS_PREFIX+safe+RESULTS_EXT)
}

// done reads the items a variant has already answered, so an interrupted run picks up where it stopped.
func done(path string) map[string]bool {
	out := map[string]bool{}
	f, err := os.Open(path)
	if err != nil {
		return out
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 1<<20), 16<<20)
	for sc.Scan() {
		var r Record
		if json.Unmarshal(sc.Bytes(), &r) == nil {
			out[r.Item] = true
		}
	}
	return out
}

func appendRecord(path string, r Record) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, FILE_PERMS)
	if err != nil {
		return err
	}
	defer f.Close()
	raw, err := json.Marshal(r)
	if err != nil {
		return err
	}
	_, err = f.Write(append(raw, '\n'))
	return err
}

func runCmd(outDir, models string, finbert bool, limit int) error {
	items, err := loadSet(outDir)
	if err != nil {
		return err
	}
	if limit > 0 {
		items = items[:min(limit, len(items))]
	}
	w, err := loadWatch(WATCHLIST_PATH)
	if err != nil {
		return err
	}
	o := localai.FromEnv(&http.Client{})
	for _, spec := range strings.Split(models, ",") {
		if err := runVariant(o, w, items, outDir, strings.TrimSpace(spec)); err != nil {
			log.Printf("%s: %v", spec, err)
		}
	}
	if finbert {
		runFinBERT(items, outDir)
	}
	return nil
}

// runVariant has one model read every item it has not read yet, one at a time, then frees its memory.
func runVariant(o localai.Ollama, w Watch, items []Item, outDir, spec string) error {
	model, think, _ := strings.Cut(spec, THINK_SEP)
	if err := o.Up(context.Background()); err != nil {
		return err
	}
	pulled, err := o.Models(context.Background())
	if err != nil {
		return err
	}
	if !slices.Contains(pulled, model) {
		return fmt.Errorf("%w: %s", localai.ErrNoModel, model)
	}
	path := variantFile(outDir, spec)
	seen := done(path)
	first := len(seen) == 0
	for i, it := range items {
		if seen[it.ID] {
			continue
		}
		in := localai.StoryInput{Headlines: it.Headlines, Watchlist: w.Names, Instrument: it.Instrument}
		ctx, cancel := context.WithTimeout(context.Background(), ITEM_TIMEOUT)
		start := time.Now()
		reading, res, err := o.ReadStory(ctx, model, think, in)
		cancel()
		rec := Record{Variant: spec, Model: model, Think: think, Item: it.ID, At: time.Now().UTC(), Result: res,
			WallSeconds: time.Since(start).Seconds()}
		if err != nil {
			rec.Error = err.Error()
		} else {
			rec.Reading = &reading
		}
		if first {
			rec.MemoryGiB, rec.VRAMGiB = memory(o, model)
			first = false
		}
		if err := appendRecord(path, rec); err != nil {
			return err
		}
		log.Printf("%s %d/%d %s %.1fs %s", spec, i+1, len(items), it.ID, rec.WallSeconds, rec.Error)
		if errors.Is(err, localai.ErrDown) {
			return err
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), UNLOAD_TIMEOUT)
	defer cancel()
	return o.Unload(ctx, model)
}

// memory is the loaded model's size and GPU share, as Ollama reports them.
func memory(o localai.Ollama, model string) (float64, float64) {
	loaded, err := o.Running(context.Background())
	if err != nil {
		return 0, 0
	}
	for _, m := range loaded {
		if m.Name == model {
			return float64(m.Size) / GIB, float64(m.SizeVRAM) / GIB
		}
	}
	return 0, 0
}

// runFinBERT reads every headline's sentiment through DOGS AI, when its finance worker is up. When it is
// not, one record says why, so the report shows it was tried.
func runFinBERT(items []Item, outDir string) {
	c := dogsai.Discover(&http.Client{})
	path := variantFile(outDir, FINBERT_ID)
	os.Remove(path)
	if _, err := c.Models(context.Background()); err != nil {
		log.Printf("finbert: %v", err)
		appendRecord(path, Record{Variant: FINBERT_ID, Model: FINBERT_ID, Item: "", At: time.Now().UTC(), Error: err.Error()})
		return
	}
	for _, it := range items {
		ctx, cancel := context.WithTimeout(context.Background(), ITEM_TIMEOUT)
		start := time.Now()
		res, err := c.Sentiment(ctx, it.Headlines)
		cancel()
		rec := Record{Variant: FINBERT_ID, Model: res.Model, Item: it.ID, At: time.Now().UTC(), WallSeconds: time.Since(start).Seconds()}
		if err != nil {
			rec.Error = err.Error()
		} else {
			rec.Sentiments = res.Results
		}
		appendRecord(path, rec)
	}
}

// loadRecords reads every variant's answers in the out folder.
func loadRecords(outDir string) (map[string][]Record, []string, error) {
	paths, err := filepath.Glob(filepath.Join(outDir, RESULTS_PREFIX+"*"+RESULTS_EXT))
	if err != nil {
		return nil, nil, err
	}
	out := map[string][]Record{}
	order := []string{}
	for _, p := range paths {
		f, err := os.Open(p)
		if err != nil {
			return nil, nil, err
		}
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 0, 1<<20), 16<<20)
		for sc.Scan() {
			var r Record
			if json.Unmarshal(sc.Bytes(), &r) != nil {
				continue
			}
			if _, ok := out[r.Variant]; !ok {
				order = append(order, r.Variant)
			}
			out[r.Variant] = append(out[r.Variant], r)
		}
		f.Close()
	}
	return out, order, nil
}

func reportCmd(outDir string) error {
	items, err := loadSet(outDir)
	if err != nil {
		return err
	}
	records, order, err := loadRecords(outDir)
	if err != nil {
		return err
	}
	text := renderReport(items, records, order)
	fmt.Print(text)
	return os.WriteFile(filepath.Join(outDir, REPORT_FILE), []byte(text), FILE_PERMS)
}

// pct formats a share as a whole percentage, or a dash when there was nothing to count.
func pct(num, den int) string {
	if den == 0 {
		return "–"
	}
	return strconv.Itoa(int(float64(num)/float64(den)*100+0.5)) + "%"
}
