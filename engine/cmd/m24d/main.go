// m24d is Money 24's trading runner. This first cut has no broker code at all:
// it takes callouts on a webhook, plans each entry with its stop, ladder and time exits, and appends
// every plan and rejection to a journal. The same binary runs headless on a Linux or Windows
// server, or as the desktop app's sidecar behind the trading UI (trading.html).
//
//	m24d [--data DIR] [--addr 127.0.0.1:8025] [--origins a,b] [--hosts h] [--sources FILE] [--web DIR] [--keep-awake] [--exit-with-stdin]
//	m24d [--data DIR] --replay
//	m24d [--data DIR] --crypto-backtest DAYS
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
	"encoding/json"
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

	"github.com/YEAHDOGS/money24/engine/alpaca"
	"github.com/YEAHDOGS/money24/engine/calendar"
	"github.com/YEAHDOGS/money24/engine/coinbase"
	"github.com/YEAHDOGS/money24/engine/dexscreener"
	"github.com/YEAHDOGS/money24/engine/market"
	"github.com/YEAHDOGS/money24/engine/plays"
	"github.com/YEAHDOGS/money24/engine/predict"
	"github.com/YEAHDOGS/money24/engine/signals"
	"github.com/YEAHDOGS/money24/engine/tape"
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
	WATCH_INTERVAL   = 30 * time.Second // how often open positions are priced; one quote request each
	ENV_SECRET       = "M24D_WEBHOOK_SECRET"
	PLACEHOLDER      = "change-me" // the value old copies of .env.example shipped with
	SECRET_MIN_CHARS = 16
	LOOPBACK_NAME    = "localhost"
	MAX_TICKER_CHARS = 6 // longer than this is a full option symbol, not a ticker
	BEARER           = "Bearer "
	PLAYS_SUBDIR     = "plays"
	ENV_NEWSD        = "M24D_NEWSD" // newsd base URL, for headlines on a play
	DEFAULT_NEWSD    = "http://127.0.0.1:8024"
	ENV_WEB          = "M24D_WEB"  // folder of built pages to serve, same as --web
	ENV_BASE         = "M24D_BASE" // the path the pages were built for, same as --base
	ROUTE_ACCOUNT    = "GET /api/account"
	ROUTE_TAPE       = "GET /api/tape"
	ROUTE_BROKER     = "GET /api/broker"
	MIRROR_FILE      = "alpaca-mirror.json"    // the last journal line placed at the broker
	TAPE_SUBDIR      = "tape"                  // every minute's 0DTE and weekly chains, for backtests
	PREDICT_FILE     = "predict-journal.jsonl" // the prediction desk's paper book
	PREDICT_TIMEOUT  = 30 * time.Second        // a page of a thousand Kalshi markets is a few megabytes
	SCOUT_AT_LAYOUT  = "2006-01-02 15:04"
	BACKTEST_TIMEOUT = 10 * time.Minute // a replay pages months of hourly candles for every coin
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

// settlement is when a sale's cash can be spent again: at once for coins, at the next trading
// day's open for stocks and options, which is what keeps a cash account out of trouble.
func settlement(sig signals.Signal, sold time.Time) time.Time {
	if sig.Market == signals.MARKET_CRYPTO || sig.Address != "" {
		return sold
	}
	return calendar.NextSettlement(sold)
}

// scoutDryRun shows what the scout would have found at one past minute of the latest session, without
// writing its record. at is New York time, "2006-01-02 15:04".
func scoutDryRun(playsDir, at string) error {
	asOf, err := time.ParseInLocation(SCOUT_AT_LAYOUT, at, calendar.NEW_YORK)
	if err != nil {
		return fmt.Errorf("--scout-at wants %q in New York time: %w", SCOUT_AT_LAYOUT, err)
	}
	store, err := plays.OpenStore(playsDir)
	if err != nil {
		return err
	}
	prices := market.NewClient()
	service := &plays.Service{Store: store, HTTP: http.DefaultClient, Resolver: plays.Resolver{HTTP: http.DefaultClient, Market: prices},
		NewsURL: cmp.Or(os.Getenv(ENV_NEWSD), DEFAULT_NEWSD), ScoutDry: true}
	ctx, cancel := context.WithTimeout(context.Background(), BACKTEST_TIMEOUT)
	defer cancel()
	board := service.ScanAt(ctx, asOf, asOf)
	out := json.NewEncoder(os.Stdout)
	out.SetIndent("", "  ")
	return out.Encode(board)
}

// replayCrypto runs the crypto desk's rule over past candles with the rulebook in the data folder
// and the catalog's starting capital, and prints what it would have done.
func replayCrypto(playsDir string, options plays.CryptoReplayOptions, capitalUSD float64) error {
	store, err := plays.OpenStore(playsDir)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), BACKTEST_TIMEOUT)
	defer cancel()
	return plays.BacktestCrypto(ctx, &http.Client{Timeout: QUOTE_TIMEOUT}, store.Rules(), options, capitalUSD, os.Stdout)
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
	coin := flag.String("coin", "", "print the quote for a listed coin (BTC, PEPE) or an on-chain token address and exit")
	web := flag.String("web", os.Getenv(ENV_WEB), "folder of built pages to serve at / with the news under /news/, so one address shows everything")
	base := flag.String("base", os.Getenv(ENV_BASE), "path the runner also answers under, the one the pages were built for (/desk)")
	awake := flag.Bool("keep-awake", false, "stop this machine from idle-sleeping while the runner runs (Windows)")
	scoutAt := flag.String("scout-at", "", "show what the scout would have found at a past minute of the latest session (\"2026-11-20 11:00\", New York time) and exit")
	cryptoBacktest := flag.Int("crypto-backtest", 0, "replay the crypto desk's rule over this many days of hourly candles, print the result and exit")
	cryptoBacktestEntry := flag.String("crypto-backtest-entry", plays.REPLAY_MARKET, "offline crypto replay entry: market, limit or both")
	cryptoBacktestCompare := flag.Bool("crypto-backtest-all", false, "compare all crypto strategies and also replay 90 days when the requested window is longer")
	cryptoBacktestEnd := flag.String("crypto-backtest-end", "", "exclusive end of the offline crypto replay as an RFC3339 hour")
	flag.Parse()

	if dexscreener.ADDRESS.MatchString(*coin) {
		pair, err := dexscreener.Fetch(context.Background(), &http.Client{Timeout: QUOTE_TIMEOUT}, *coin)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("%s (%s) on %s  price $%g  pool $%.0f  24h volume $%.0f\n", pair.Base.Symbol, pair.Base.Name, pair.Venue, pair.PriceUSD, pair.Liquidity.USD, pair.Volume.Day)
		return
	}
	if *coin != "" {
		ticker, err := coinbase.Fetch(context.Background(), &http.Client{Timeout: QUOTE_TIMEOUT}, *coin)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("%s  bid %g  ask %g  last %g  24h volume %g\n", strings.ToUpper(*coin), ticker.Bid, ticker.Ask, ticker.Price, ticker.Volume)
		return
	}

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
	if *scoutAt != "" {
		if err := scoutDryRun(filepath.Join(*dataDir, PLAYS_SUBDIR), *scoutAt); err != nil {
			log.Fatal(err)
		}
		return
	}
	if *cryptoBacktest > 0 {
		options := plays.CryptoReplayOptions{Days: *cryptoBacktest, Entry: *cryptoBacktestEntry, Compare: *cryptoBacktestCompare}
		if *cryptoBacktestEnd != "" {
			options.End, err = time.Parse(time.RFC3339, *cryptoBacktestEnd)
			if err != nil {
				log.Fatal(err)
			}
		}
		if err := replayCrypto(filepath.Join(*dataDir, PLAYS_SUBDIR), options, catalog.GoLive.PaperCapitalUSD); err != nil {
			log.Fatal(err)
		}
		return
	}
	if *awake {
		release, err := keepAwake()
		if err != nil {
			log.Fatalf("keep awake: %v", err)
		}
		defer release()
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

	// Coins are always priced live. Without a Webull key, stocks and options are priced from Yahoo's
	// delayed quotes and option chain.
	prices := market.NewClient()
	log.Printf("live quotes: Coinbase for %d coins, DEX Screener for token addresses", len(catalog.CryptoSymbols))
	if quotes != nil {
		log.Printf("live quotes: Webull (%s)", cmp.Or(os.Getenv(ENV_WEBULL_ENV), webull.ENV_SANDBOX))
	}
	runner := signals.NewRunner(catalog, journal, liveQuotes(quotes, prices))
	runner.SetSettlement(settlement)
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

	// The exit watcher: prices open positions and closes them on the stop, a target or the clock.
	go func() {
		tick := time.NewTicker(WATCH_INTERVAL)
		defer tick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-tick.C:
				closed, err := runner.Tick(now)
				if err != nil {
					log.Printf("watcher: %v", err)
				}
				for _, e := range closed {
					log.Printf("%s %s %s %+v %s", e.Signal.SourceID, e.Action, e.Signal.Symbol, e.Result, e.Note)
				}
			}
		}
	}()

	allowedHosts := loopbackHosts(*addr)
	if *hosts != "" {
		allowedHosts = strings.Split(*hosts, ",")
	}
	opts := signals.APIOptions{Origins: strings.Split(*origins, ","), Hosts: allowedHosts}
	tracker := signals.NewTracker(recorder, catalog, priceHistory(prices))
	// Loading a source's history reaches out to X, so it takes the local desk or the webhook secret.
	mayLoad := func(r *http.Request) bool {
		return opts.TrustsLocalDesk(r) || r.Header.Get("Authorization") == BEARER+secret
	}
	opts.Mount = tracker.Routes(http.DefaultClient, mayLoad)
	opts.Mount[market.ROUTE_PREFIX] = market.Handler(prices)
	opts.Mount[calendar.ROUTE] = calendar.Handler()
	opts.Mount[ROUTE_ACCOUNT] = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(runner.Account(time.Now()))
	})
	newsURL := cmp.Or(os.Getenv(ENV_NEWSD), DEFAULT_NEWSD)
	if *web != "" {
		routes, err := webRoutes(*web, newsURL)
		if err != nil {
			log.Fatal(err)
		}
		for pattern, h := range routes {
			opts.Mount[pattern] = h
		}
		opts.Origins = append(opts.Origins, webOrigins(*addr)...)
		log.Printf("pages served from %s", *web)
	}
	playStore, err := plays.OpenStore(filepath.Join(*dataDir, PLAYS_SUBDIR))
	if err != nil {
		log.Fatal(err)
	}
	playService := &plays.Service{Store: playStore, HTTP: http.DefaultClient, MayWrite: mayLoad,
		Resolver: plays.Resolver{HTTP: http.DefaultClient, Market: prices},
		Paper:    func(sig signals.Signal, plan signals.Plan) (int64, error) { return runner.Adopt(sig, plan, time.Now()) },
		Lines:    journal.Lines, NewsURL: newsURL,
		Funds: func(now time.Time) plays.Funds {
			account := runner.Account(now)
			return plays.Funds{EquityUSD: account.EquityUSD, BuyingPowerUSD: account.BuyingPowerUSD}
		}}
	for pattern, h := range playService.Routes() {
		opts.Mount[pattern] = h
	}
	// The finder opens plays on its own from the stories newsd groups; the rulebook can turn it off.
	go playService.RunFinder(ctx)
	// The crypto desk does the same for coins from their own candles, around the clock.
	go playService.RunCrypto(ctx)
	// The flow desk follows the most premium traded in the most traded options, through the stock session.
	go playService.RunFlow(ctx)
	// The scout looks for short-term option plays every minute of the session and follows each to its outcome.
	go playService.RunScout(ctx)
	// The broker: with Alpaca paper keys in .env, every desk trade is also placed as a real paper order,
	// so the desk's prices can be checked against what the market actually filled.
	broker, err := alpaca.FromEnv()
	if err != nil {
		log.Fatal(err)
	}
	if broker != nil {
		startup, cancel := context.WithTimeout(ctx, QUOTE_TIMEOUT)
		if _, err := broker.CashOnly(startup); err != nil {
			log.Printf("alpaca: could not set the account to cash only: %v", err)
		}
		cancel()
		mirror := &alpaca.Mirror{Client: broker, Journal: journal, StateFile: filepath.Join(*dataDir, MIRROR_FILE)}
		go mirror.Run(ctx)
		opts.Mount[ROUTE_BROKER] = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(mirror.Status())
		})
		log.Printf("broker: Alpaca %s, cash only; desk trades are placed there too", broker.Mode)
	}
	// The tape records the same-day and weekly option chains every minute of the session: the
	// history short-term option plays are backtested on, which no free source has.
	tapeRecorder := &tape.Recorder{Dir: filepath.Join(*dataDir, TAPE_SUBDIR), Market: prices}
	go tapeRecorder.Run(ctx)
	opts.Mount[ROUTE_TAPE] = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(tapeRecorder.Status(time.Now()))
	})
	// The prediction desk: a paper book of Kalshi event contracts at Kalshi's live prices, filled against
	// the real order book with the real fee and settled at the real result. It places no order anywhere.
	// Every strategy it runs is in predict-strategies.json in the data folder: edit it to add, retune or
	// switch one off, and the desk picks it up within a minute.
	predictBook, err := predict.OpenBook(filepath.Join(*dataDir, PREDICT_FILE), predict.DEFAULT_CAPITAL_USD)
	if err != nil {
		log.Fatal(err)
	}
	defer predictBook.Close()
	predictHTTP := &http.Client{Timeout: PREDICT_TIMEOUT}
	predictDesk := &predict.Desk{Book: predictBook, Kalshi: &predict.Kalshi{HTTP: predictHTTP}, Env: &predict.Env{HTTP: predictHTTP},
		RulebookPath: filepath.Join(*dataDir, predict.RULEBOOK_FILE), MayWrite: mayLoad}
	go predictDesk.Run(ctx)
	for pattern, h := range predictDesk.Routes() {
		opts.Mount[pattern] = h
	}
	for _, p := range plays.Providers() {
		if p.Connected {
			log.Printf("AI: %s (%s) connected for play analysis", p.Name, p.Model)
		}
	}
	resolver := func(ctx context.Context, statusURL string) (signals.Tweet, error) {
		return signals.ResolveTweet(ctx, http.DefaultClient, statusURL)
	}
	handler := signals.API(journal, catalog, signals.Webhook(secret, opts.TrustsLocalDesk, resolver, record, queue), opts)
	// Request contexts end with ctx, so open UI streams close on shutdown instead of holding it up.
	srv := &http.Server{Addr: *addr, Handler: withBase(*base, handler), ReadHeaderTimeout: HEADER_TIMEOUT,
		BaseContext: func(net.Listener) context.Context { return ctx }}
	go func() {
		log.Printf("m24d runner on %s, %d sources, data %s", *addr, len(catalog.Sources), *dataDir)
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
