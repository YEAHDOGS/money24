// m24d is Money 24's trading runner. This first cut is paper only and has no broker code at all:
// it takes callouts on a webhook, plans each entry with its stop and ladder, and appends every
// plan and rejection to a paper journal. Broker execution, positions and X/Telegram adapters come next.
//
//	m24d --data DIR [--addr 127.0.0.1:8025] [--sources FILE]   (M24D_WEBHOOK_SECRET is required)
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/YEAHDOGS/money24/engine/signals"
)

const (
	DEFAULT_ADDR     = "127.0.0.1:8025"
	APP_DIR          = "Money24"
	DATA_SUBDIR      = "trading"
	JOURNAL_FILE     = "paper-journal.jsonl"
	QUEUE_SIZE       = 256
	DIR_PERMS        = 0o755
	FILE_PERMS       = 0o644
	HEADER_TIMEOUT   = 10 * time.Second
	SHUTDOWN_TIMEOUT = 5 * time.Second
	ENV_SECRET       = "M24D_WEBHOOK_SECRET"
)

// entry is one journal line: a plan, a rejection, or an exit/trim/add the runner will act on later.
type entry struct {
	At       time.Time      `json:"at"`
	Action   string         `json:"action"`
	Plan     *signals.Plan  `json:"plan,omitempty"`
	Signal   signals.Signal `json:"signal"`
	Rejected string         `json:"rejected,omitempty"`
}

func main() {
	addr := flag.String("addr", DEFAULT_ADDR, "listen address for the webhook")
	dataDir := flag.String("data", "", "folder for the paper journal (default: user config dir/Money24/trading)")
	sourcesFile := flag.String("sources", "", "override default_sources.json")
	flag.Parse()

	secret := os.Getenv(ENV_SECRET)
	if secret == "" {
		log.Fatalf("set %s: the webhook refuses unauthenticated callers", ENV_SECRET)
	}
	if *dataDir == "" {
		base, err := os.UserConfigDir()
		if err != nil {
			log.Fatal(err)
		}
		*dataDir = filepath.Join(base, APP_DIR, DATA_SUBDIR)
	}
	catalog, err := signals.LoadCatalog(*sourcesFile)
	if err != nil {
		log.Fatal(err)
	}
	if err := os.MkdirAll(*dataDir, DIR_PERMS); err != nil {
		log.Fatal(err)
	}
	journal, err := os.OpenFile(filepath.Join(*dataDir, JOURNAL_FILE), os.O_CREATE|os.O_APPEND|os.O_WRONLY, FILE_PERMS)
	if err != nil {
		log.Fatal(err)
	}
	defer journal.Close()

	queue := make(chan signals.Signal, QUEUE_SIZE)
	done := make(chan struct{})
	go func() {
		defer close(done)
		enc := json.NewEncoder(journal)
		for sig := range queue {
			line := entry{At: time.Now().UTC(), Action: string(sig.Kind), Signal: sig}
			if sig.Kind == signals.KIND_ENTRY {
				plan, err := signals.Build(sig, catalog.SettingsFor(sig.SourceID), 0)
				if err != nil {
					line.Rejected = err.Error()
				} else {
					line.Plan = &plan
				}
			}
			if err := enc.Encode(line); err != nil {
				log.Printf("journal: %v", err)
			}
			log.Printf("%s %s %s rejected=%q", sig.SourceID, sig.Kind, sig.Symbol, line.Rejected)
		}
	}()

	mux := http.NewServeMux()
	mux.Handle("/signals", signals.Webhook(secret, queue))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("ok")) })
	srv := &http.Server{Addr: *addr, Handler: mux, ReadHeaderTimeout: HEADER_TIMEOUT}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		log.Printf("m24d paper runner on %s, %d sources, journal %s", *addr, len(catalog.Sources), journal.Name())
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}()
	<-ctx.Done()

	shutdown, cancel := context.WithTimeout(context.Background(), SHUTDOWN_TIMEOUT)
	defer cancel()
	srv.Shutdown(shutdown)
	close(queue)
	<-done
}
