// m24d is Money 24's trading runner. This first cut is paper only and has no broker code at all:
// it takes callouts on a webhook, plans each entry with its stop, ladder and time exits, and appends
// every plan and rejection to a paper journal. The same binary runs headless on a Linux or Windows
// server, or as the desktop app's sidecar behind the trading UI (trading.html).
//
//	m24d [--data DIR] [--addr 127.0.0.1:8025] [--origins a,b] [--hosts h] [--sources FILE] [--exit-with-stdin]
//	m24d [--data DIR] --replay
//
// --origins and --hosts also read M24D_ORIGINS and M24D_HOSTS.
//
// The webhook secret comes from M24D_WEBHOOK_SECRET, or is generated once into DIR/webhook-secret.
package main

import (
	"cmp"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/YEAHDOGS/money24/engine/signals"
	"github.com/YEAHDOGS/money24/engine/webull"
)

const (
	DEFAULT_ADDR = "127.0.0.1:8025"
	// Tauri WebView origins (Windows, then macOS/Linux) and the Vite dev server: same list as newsd.
	DEFAULT_ORIGINS  = "http://tauri.localhost,https://tauri.localhost,tauri://localhost,http://localhost:5173,http://127.0.0.1:5173"
	APP_DIR          = "Money24"
	DATA_SUBDIR      = "trading"
	JOURNAL_FILE     = "paper-journal.jsonl"
	POSTS_FILE       = "posts.jsonl"
	SECRET_FILE      = "webhook-secret"
	SECRET_BYTES     = 24
	QUEUE_SIZE       = 256
	DIR_PERMS        = 0o755
	SECRET_PERMS     = 0o600
	HEADER_TIMEOUT   = 10 * time.Second
	SHUTDOWN_TIMEOUT = 5 * time.Second
	ENV_SECRET       = "M24D_WEBHOOK_SECRET"
	PLACEHOLDER      = "change-me" // the value old copies of .env.example shipped with
	SECRET_MIN_CHARS = 16
	LOOPBACK_NAME    = "localhost"
	MAX_TICKER_CHARS = 6 // longer than this is a full option symbol, not a ticker
)

// webhookSecret prefers the environment; otherwise it reuses or creates a random secret in the data dir.
func webhookSecret(dataDir string) (string, error) {
	if s := os.Getenv(ENV_SECRET); s != "" {
		if s == PLACEHOLDER || len(s) < SECRET_MIN_CHARS {
			return "", fmt.Errorf("%s is a placeholder or shorter than %d characters; unset it to generate one", ENV_SECRET, SECRET_MIN_CHARS)
		}
		return s, nil
	}
	path := filepath.Join(dataDir, SECRET_FILE)
	if b, err := os.ReadFile(path); err == nil && len(strings.TrimSpace(string(b))) > 0 {
		return strings.TrimSpace(string(b)), nil
	}
	buf := make([]byte, SECRET_BYTES)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	secret := hex.EncodeToString(buf)
	if err := os.WriteFile(path, []byte(secret+"\n"), SECRET_PERMS); err != nil {
		return "", err
	}
	log.Printf("webhook secret created in %s", path)
	return secret, nil
}

// loopbackHosts limits Host headers when bound to loopback, so a web page can't reach the API by DNS rebinding.
func loopbackHosts(addr string) []string {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil
	}
	ip := net.ParseIP(host)
	if host != LOOPBACK_NAME && (ip == nil || !ip.IsLoopback()) {
		return nil
	}
	return []string{net.JoinHostPort(host, port), net.JoinHostPort(LOOPBACK_NAME, port), net.JoinHostPort("127.0.0.1", port)}
}

// printReplay shows what today's parser makes of every recorded post: a count per kind, then the
// posts that name a ticker but were read as noise, which are the likeliest parser misses.
func printReplay(path string) error {
	counts := map[signals.Kind]int{}
	missed := []string{}
	err := signals.Replay(path, func(p signals.RawPost, sig signals.Signal) {
		counts[sig.Kind]++
		if sig.Kind == signals.KIND_NOISE && sig.Symbol != "" {
			missed = append(missed, p.SourceID+" "+p.PostID+": "+strings.ReplaceAll(p.Text, "\n", " / "))
		}
	})
	if err != nil {
		return err
	}
	for _, k := range []signals.Kind{signals.KIND_ENTRY, signals.KIND_ADD, signals.KIND_TRIM, signals.KIND_EXIT, signals.KIND_NOISE} {
		fmt.Printf("%-6s %d\n", k, counts[k])
	}
	fmt.Printf("\nnoise that names a ticker (%d):\n", len(missed))
	for _, m := range missed {
		fmt.Println("  " + m)
	}
	return nil
}

func main() {
	addr := flag.String("addr", DEFAULT_ADDR, "listen address; use 0.0.0.0:8025 on a server behind a reverse proxy")
	dataDir := flag.String("data", "", "folder for the journal and secret (default: user config dir/Money24/trading)")
	origins := flag.String("origins", cmp.Or(os.Getenv("M24D_ORIGINS"), DEFAULT_ORIGINS), "comma-separated browser origins allowed to call the API")
	hosts := flag.String("hosts", os.Getenv("M24D_HOSTS"), "comma-separated Host headers to accept (default: localhost only when bound to loopback)")
	sourcesFile := flag.String("sources", "", "override default_sources.json")
	replay := flag.Bool("replay", false, "re-parse every recorded post with the current parser, print the result and exit")
	exitWithStdin := flag.Bool("exit-with-stdin", false, "exit when stdin closes (set by the desktop app)")
	quote := flag.String("quote", "", "print the Webull quote for a ticker or option symbol (AAPL, AAPL260522C00300000) and exit")
	flag.Parse()

	quotes, err := webullClient()
	if err != nil {
		log.Fatal(err)
	}
	if *quote != "" {
		if quotes == nil {
			log.Fatalf("set %s and %s first", ENV_WEBULL_KEY, ENV_WEBULL_SECRET)
		}
		if err := printQuote(quotes, *quote); err != nil {
			log.Fatal(err)
		}
		return
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if *exitWithStdin {
		go func() {
			io.Copy(io.Discard, os.Stdin)
			stop()
		}()
	}

	if *dataDir == "" {
		base, err := os.UserConfigDir()
		if err != nil {
			log.Fatal(err)
		}
		*dataDir = filepath.Join(base, APP_DIR, DATA_SUBDIR)
	}
	if err := os.MkdirAll(*dataDir, DIR_PERMS); err != nil {
		log.Fatal(err)
	}
	postsPath := filepath.Join(*dataDir, POSTS_FILE)
	if *replay {
		if err := printReplay(postsPath); err != nil {
			log.Fatal(err)
		}
		return
	}
	secret, err := webhookSecret(*dataDir)
	if err != nil {
		log.Fatalf("webhook secret: %v", err)
	}
	catalog, err := signals.LoadCatalog(*sourcesFile)
	if err != nil {
		log.Fatal(err)
	}
	journal, err := signals.OpenJournal(filepath.Join(*dataDir, JOURNAL_FILE))
	if err != nil {
		log.Fatal(err)
	}
	defer journal.Close()
	recorder, err := signals.OpenRecorder(postsPath)
	if err != nil {
		log.Fatal(err)
	}
	defer recorder.Close()
	record := func(sig signals.Signal) {
		if err := recorder.Record(sig); err != nil {
			log.Printf("recorder: %v", err)
		}
	}

	// Without a Webull key there are no live quotes: the gate plans at the called price and notes
	// what it could not check.
	var quoteFunc signals.QuoteFunc
	if quotes != nil {
		quoteFunc = webullQuotes(quotes)
		log.Printf("live quotes: Webull (%s)", cmp.Or(os.Getenv(ENV_WEBULL_ENV), webull.ENV_SANDBOX))
	}
	runner := signals.NewRunner(catalog, journal, quoteFunc)
	queue := make(chan signals.Signal, QUEUE_SIZE)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for sig := range queue {
			e, fresh, err := runner.Handle(sig, time.Now())
			if err != nil {
				log.Printf("journal: %v", err)
			}
			if !fresh {
				log.Printf("%s %s duplicate post %s", sig.SourceID, sig.Symbol, sig.PostID)
				continue
			}
			log.Printf("%s %s %s rejected=%q note=%q", sig.SourceID, sig.Kind, sig.Symbol, e.Rejected, e.Note)
		}
	}()

	allowedHosts := loopbackHosts(*addr)
	if *hosts != "" {
		allowedHosts = strings.Split(*hosts, ",")
	}
	opts := signals.APIOptions{Origins: strings.Split(*origins, ","), Hosts: allowedHosts}
	resolver := func(ctx context.Context, statusURL string) (signals.Tweet, error) {
		return signals.ResolveTweet(ctx, http.DefaultClient, statusURL)
	}
	handler := signals.API(journal, catalog, signals.Webhook(secret, opts.TrustsLocalDesk, resolver, record, queue), opts)
	// Request contexts end with ctx, so open UI streams close on shutdown instead of holding it up.
	srv := &http.Server{Addr: *addr, Handler: handler, ReadHeaderTimeout: HEADER_TIMEOUT,
		BaseContext: func(net.Listener) context.Context { return ctx }}
	go func() {
		log.Printf("m24d paper runner on %s, %d sources, data %s", *addr, len(catalog.Sources), *dataDir)
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
