package main

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
)

const (
	// With --web the runner serves the built pages itself, so the pages, the trading API and the news
	// (forwarded under /news/) share one origin: one local address to open, and one tunnel for a phone.
	ROUTE_WEB         = "GET /"
	ROUTE_NEWS        = "GET /news/"
	NEWS_PREFIX       = "/news"
	WEB_START_PAGE    = "home.html" // what "/" opens; the landing page stays at /index.html
	WEB_ASSETS_PREFIX = "assets/"   // Vite's hashed files: the name changes whenever the content does
	WEB_UNSAFE_CHARS  = `\:`        // a Windows separator or drive letter has no place in a page path
	CACHE_REVALIDATE  = "no-cache"  // keep a copy, but ask first: a new build shows on the next load
	CACHE_FOREVER     = "public, max-age=31536000, immutable"
	NO_SNIFF          = "nosniff"
	NEWS_DOWN         = "the news service is not answering"
	FLUSH_AT_ONCE     = -1 // the proxy flushes every write, so the news stream arrives as it is sent
	SELF_SCHEME       = "http://"
)

// WEB_TYPES are set by hand: Windows takes content types from the registry, where .js is often
// text/plain, and a browser refuses to run a module served as that.
var WEB_TYPES = map[string]string{
	".html":        "text/html; charset=utf-8",
	".js":          "text/javascript; charset=utf-8",
	".css":         "text/css; charset=utf-8",
	".json":        "application/json",
	".webmanifest": "application/manifest+json",
	".svg":         "image/svg+xml",
	".png":         "image/png",
	".jpg":         "image/jpeg",
	".webp":        "image/webp",
	".ico":         "image/x-icon",
	".woff2":       "font/woff2",
	".txt":         "text/plain; charset=utf-8",
}

// webRoutes returns the ServeMux patterns that serve the built pages from dir at "/" and forward
// "/news/" to the news service, to be added to signals.APIOptions.Mount. They sit behind the
// runner's host and origin checks: a page reads its own origin freely, and a write from an origin
// that is not on the allow-list (a phone on a tunnel) is refused there, so remote viewers only read.
func webRoutes(dir, newsURL string) (map[string]http.Handler, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, fmt.Errorf("web pages: %w", err)
	}
	if _, err := root.Stat(WEB_START_PAGE); err != nil {
		root.Close()
		return nil, fmt.Errorf("web pages: %s has no %s; build the pages first (npm run forever does)", dir, WEB_START_PAGE)
	}
	news, err := url.Parse(newsURL)
	if err != nil || news.Host == "" {
		root.Close()
		return nil, fmt.Errorf("news service URL %q: want http://host:port", newsURL)
	}

	pages := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
		if name == "" {
			name = WEB_START_PAGE
		}
		// os.Root already refuses anything that leaves dir; this only turns odd names into a plain 404.
		if strings.ContainsAny(name, WEB_UNSAFE_CHARS) || !filepath.IsLocal(name) {
			http.NotFound(w, r)
			return
		}
		f, err := root.Open(name)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer f.Close()
		info, err := f.Stat()
		if err != nil || info.IsDir() {
			http.NotFound(w, r) // no folder listings
			return
		}
		h := w.Header()
		if t := WEB_TYPES[strings.ToLower(path.Ext(name))]; t != "" {
			h.Set("Content-Type", t)
		}
		h.Set("X-Content-Type-Options", NO_SNIFF)
		h.Set("Cache-Control", CACHE_REVALIDATE)
		if strings.HasPrefix(name, WEB_ASSETS_PREFIX) {
			h.Set("Cache-Control", CACHE_FOREVER)
		}
		http.ServeContent(w, r, name, info.ModTime(), f)
	})

	proxy := &httputil.ReverseProxy{
		// SetURL also makes the outbound Host the news service's own, which is the only one it accepts.
		// The browser's Origin is dropped: to the news service this is the runner asking, not a web page.
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(news)
			pr.Out.Header.Del("Origin")
		},
		FlushInterval: FLUSH_AT_ONCE,
		// A news service that is down is an ordinary state, reported to the page and not logged per request.
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, _ error) {
			http.Error(w, NEWS_DOWN, http.StatusBadGateway)
		},
	}
	return map[string]http.Handler{ROUTE_WEB: pages, ROUTE_NEWS: http.StripPrefix(NEWS_PREFIX, proxy)}, nil
}

// webOrigins are the origins of the pages the runner serves on this machine. Added to the allowed
// origins, the local desk can still paste links and edit rules; a tunnel's origin is left out on
// purpose. Empty when addr is not a loopback address.
func webOrigins(addr string) []string {
	out := []string{}
	for _, h := range loopbackHosts(addr) {
		out = append(out, SELF_SCHEME+h)
	}
	return out
}
