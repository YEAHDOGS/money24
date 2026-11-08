// newsd: Money 24's news ingest service. Polls every source in sources.json,
// tags items against watchlist.json, stores them in Postgres, and streams them live.
package main

import (
	"context"
	"embed"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

const (
	DEFAULT_PORT       = "8024"
	DEFAULT_USER_AGENT = "Money24-News/0.1 (DOGS)"
	HTTP_TIMEOUT       = 20 * time.Second
	SHUTDOWN_TIMEOUT   = 5 * time.Second
)

//go:embed sources.json watchlist.json
var defaults embed.FS

func main() {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL is required")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	sources, err := loadSources(os.Getenv("SOURCES_FILE"))
	if err != nil {
		log.Fatalf("sources: %v", err)
	}
	tagger, err := loadTagger(os.Getenv("WATCHLIST_FILE"))
	if err != nil {
		log.Fatalf("watchlist: %v", err)
	}
	store, err := openStore(ctx, dbURL)
	if err != nil {
		log.Fatalf("store: %v", err)
	}

	userAgent := os.Getenv("USER_AGENT")
	if userAgent == "" {
		userAgent = DEFAULT_USER_AGENT
	}
	fetcher := newFetcher(&http.Client{Timeout: HTTP_TIMEOUT}, userAgent)

	for _, src := range sources {
		go runSource(ctx, src, fetcher, store, tagger)
	}

	hub := &Hub{clients: map[chan []byte]struct{}{}}
	go hub.listen(ctx, dbURL)

	port := os.Getenv("PORT")
	if port == "" {
		port = DEFAULT_PORT
	}
	srv := &http.Server{Addr: ":" + port, Handler: routes(store, hub), ReadHeaderTimeout: HTTP_TIMEOUT}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), SHUTDOWN_TIMEOUT)
		defer cancel()
		srv.Shutdown(shutdownCtx)
	}()

	log.Printf("newsd: %d sources, %d watchlist tags, feed on http://localhost:%s", len(sources), len(tagger.rules), port)
	if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

// readConfig returns the override file when a path is set, otherwise the embedded default.
func readConfig(path, name string) ([]byte, error) {
	if path != "" {
		return os.ReadFile(path)
	}
	return defaults.ReadFile(name)
}
