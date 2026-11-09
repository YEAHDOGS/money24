// newsd is Money 24's news and events service. One binary, two homes: the desktop app starts it as
// a sidecar, and a Linux server runs it headless under systemd.
//
//	newsd --data DIR [--addr 127.0.0.1:8024] [--origins a,b] [--user-agent "App contact@example.com"]
package main

import (
	"context"
	"errors"
	"flag"
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

	"github.com/YEAHDOGS/money24/engine/news"
)

const (
	DEFAULT_ADDR       = "127.0.0.1:8024"
	DEFAULT_USER_AGENT = "Money24-News/0.1 (DOGS)"
	// Tauri WebView origins (Windows, then macOS/Linux) and the Vite dev server.
	DEFAULT_ORIGINS  = "http://tauri.localhost,https://tauri.localhost,tauri://localhost,http://localhost:5173,http://127.0.0.1:5173"
	APP_DIR          = "Money24"
	DATA_SUBDIR      = "news"
	FETCH_TIMEOUT    = 20 * time.Second
	HEADER_TIMEOUT   = 10 * time.Second
	SHUTDOWN_TIMEOUT = 5 * time.Second
	DIR_PERMS        = 0o755
	LOOPBACK_NAME    = "localhost"
)

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func defaultDataDir() string {
	base, err := os.UserConfigDir()
	if err != nil {
		return DATA_SUBDIR
	}
	return filepath.Join(base, APP_DIR, DATA_SUBDIR)
}

// loopbackHosts returns the Host headers to accept when bound to loopback, so a web page cannot
// reach this API through DNS rebinding. Any other bind address accepts every host.
func loopbackHosts(addr string) []string {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil
	}
	ip := net.ParseIP(host)
	if host != LOOPBACK_NAME && (ip == nil || !ip.IsLoopback()) {
		return nil
	}
	return []string{net.JoinHostPort(LOOPBACK_NAME, port), net.JoinHostPort(host, port)}
}

// allowedHosts uses the --hosts list when given, otherwise the loopback defaults.
func allowedHosts(list, addr string) []string {
	if list != "" {
		return strings.Split(list, ",")
	}
	return loopbackHosts(addr)
}

func main() {
	addr := flag.String("addr", env("NEWSD_ADDR", DEFAULT_ADDR), "listen address; use 0.0.0.0:8024 on a server")
	dataDir := flag.String("data", env("NEWSD_DATA", defaultDataDir()), "folder for news.db")
	userAgent := flag.String("user-agent", env("USER_AGENT", DEFAULT_USER_AGENT), `sent to every source; SEC wants "App contact@example.com"`)
	origins := flag.String("origins", env("NEWSD_ORIGINS", DEFAULT_ORIGINS), "comma-separated browser origins allowed to call the API")
	adminToken := flag.String("admin-token", os.Getenv("NEWSD_ADMIN_TOKEN"), "bearer token required to change settings (set this on a server)")
	sourcesFile := flag.String("sources", os.Getenv("SOURCES_FILE"), "override sources.json")
	watchlistFile := flag.String("watchlist", os.Getenv("WATCHLIST_FILE"), "override watchlist.json")
	hosts := flag.String("hosts", os.Getenv("NEWSD_HOSTS"), "comma-separated Host headers to accept, e.g. news.example.com behind a reverse proxy (default: localhost only when bound to loopback)")
	exitWithStdin := flag.Bool("exit-with-stdin", false, "exit when stdin closes (set by the desktop app, so newsd dies with it)")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if *exitWithStdin {
		go func() {
			io.Copy(io.Discard, os.Stdin)
			stop()
		}()
	}

	if err := os.MkdirAll(*dataDir, DIR_PERMS); err != nil {
		log.Fatalf("data dir: %v", err)
	}
	svc, err := news.NewService(ctx, news.Config{
		DataDir:       *dataDir,
		UserAgent:     *userAgent,
		SourcesFile:   *sourcesFile,
		WatchlistFile: *watchlistFile,
		HTTPTimeout:   FETCH_TIMEOUT,
	})
	if err != nil {
		log.Fatalf("start: %v", err)
	}

	listener, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatalf("listen %s: %v", *addr, err)
	}
	srv := &http.Server{
		Handler: svc.Handler(news.HTTPOptions{
			Origins:    strings.Split(*origins, ","),
			Hosts:      allowedHosts(*hosts, *addr),
			AdminToken: *adminToken,
		}),
		ReadHeaderTimeout: HEADER_TIMEOUT,
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), SHUTDOWN_TIMEOUT)
		defer cancel()
		srv.Shutdown(shutdownCtx)
	}()
	go func() {
		if err := srv.Serve(listener); !errors.Is(err, http.ErrServerClosed) {
			log.Printf("http: %v", err)
			stop()
		}
	}()

	sources, tags, every := svc.Summary()
	log.Printf("newsd: %d sources, %d watchlist tags, polling every %s, data in %s, API on http://%s",
		sources, tags, every, *dataDir, listener.Addr())
	svc.Run(ctx)
}
