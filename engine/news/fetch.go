package news

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"
)

const (
	MAX_BODY_BYTES     = 16 << 20
	GDELT_SPACING      = 20 * time.Second // GDELT asks for 5 s between requests but throttles harder in practice
	DIAL_TIMEOUT       = 10 * time.Second
	DIAL_KEEP_ALIVE    = 30 * time.Second
	ENCODING_GZIP      = "gzip"
	ENCODING_IDENTITY  = "identity"
	REQUEST_LINE_EXTRA = len("  HTTP/1.1\r\n") + len("\r\n") // spaces, version, final blank line
	HEADER_LINE_EXTRA  = len(": \r\n")
)

var ERR_ENCODING = errors.New("unsupported content encoding")

// Fetcher does polite, metered GETs: one User-Agent, conditional requests, gzip, bounded bodies.
type Fetcher struct {
	client     *http.Client
	userAgent  string
	monitor    *Monitor
	mu         sync.Mutex
	validators map[string][2]string // url -> {ETag, Last-Modified}
	gdeltMu    sync.Mutex
	gdeltLast  time.Time
}

// NewFetcher builds a Fetcher whose every byte, on the wire and per request, lands in monitor.
func NewFetcher(timeout time.Duration, userAgent string, monitor *Monitor) *Fetcher {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	dialer := &net.Dialer{Timeout: DIAL_TIMEOUT, KeepAlive: DIAL_KEEP_ALIVE}
	transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		conn, err := dialer.DialContext(ctx, network, addr)
		if err != nil {
			return nil, err
		}
		return &countingConn{Conn: conn, monitor: monitor}, nil
	}
	return &Fetcher{
		client:     &http.Client{Timeout: timeout, Transport: transport},
		userAgent:  userAgent,
		monitor:    monitor,
		validators: map[string][2]string{},
	}
}

// countingConn tallies raw socket bytes, TLS handshakes included.
type countingConn struct {
	net.Conn
	monitor *Monitor
}

func (c *countingConn) Read(p []byte) (int, error) {
	n, err := c.Conn.Read(p)
	c.monitor.wireIn.Add(int64(n))
	return n, err
}

func (c *countingConn) Write(p []byte) (int, error) {
	n, err := c.Conn.Write(p)
	c.monitor.wireOut.Add(int64(n))
	return n, err
}

// get returns the decoded body, or nil when the server says nothing changed (304).
func (f *Fetcher) get(ctx context.Context, sourceID, rawURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", f.userAgent)
	// Asking for gzip ourselves stops Go from decompressing silently, so we can meter the real size.
	req.Header.Set("Accept-Encoding", ENCODING_GZIP)
	f.mu.Lock()
	cached := f.validators[rawURL]
	f.mu.Unlock()
	if cached[0] != "" {
		req.Header.Set("If-None-Match", cached[0])
	}
	if cached[1] != "" {
		req.Header.Set("If-Modified-Since", cached[1])
	}

	usage := Usage{Out: requestSize(req)}
	started := time.Now()
	status := 0
	defer func() { f.monitor.recordRequest(sourceID, rawURL, usage, status, time.Since(started), err) }()

	res, err := f.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	status = res.StatusCode
	raw, err := io.ReadAll(io.LimitReader(res.Body, MAX_BODY_BYTES))
	usage.In = responseHeaderSize(res) + int64(len(raw))
	if err != nil {
		return nil, err
	}
	if res.StatusCode == http.StatusNotModified {
		return nil, nil
	}
	body, err := decode(raw, res.Header.Get("Content-Encoding"))
	if err != nil {
		return nil, err
	}
	if res.StatusCode != http.StatusOK {
		err = fmt.Errorf("HTTP %d: %s", res.StatusCode, snippet(body))
		return nil, err
	}
	f.mu.Lock()
	f.validators[rawURL] = [2]string{res.Header.Get("ETag"), res.Header.Get("Last-Modified")}
	f.mu.Unlock()
	return body, nil
}

// getGDELT takes turns with every other GDELT source, because they share one rate limit.
func (f *Fetcher) getGDELT(ctx context.Context, sourceID, rawURL string) ([]byte, error) {
	f.gdeltMu.Lock()
	defer f.gdeltMu.Unlock()
	if wait := GDELT_SPACING - time.Since(f.gdeltLast); wait > 0 {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(wait):
		}
	}
	defer func() { f.gdeltLast = time.Now() }()
	return f.get(ctx, sourceID, rawURL)
}

func decode(raw []byte, encoding string) ([]byte, error) {
	if encoding == "" || encoding == ENCODING_IDENTITY {
		return raw, nil
	}
	if encoding != ENCODING_GZIP {
		return nil, fmt.Errorf("%w: %q", ERR_ENCODING, encoding)
	}
	zr, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	return io.ReadAll(io.LimitReader(zr, MAX_BODY_BYTES))
}

// requestSize estimates the bytes a request puts on the wire (HTTP/1.1 framing; HTTP/2 is smaller).
func requestSize(req *http.Request) int64 {
	size := len(req.Method) + len(req.URL.RequestURI()) + REQUEST_LINE_EXTRA
	size += len("Host") + len(req.URL.Host) + HEADER_LINE_EXTRA
	for key, values := range req.Header {
		for _, v := range values {
			size += len(key) + len(v) + HEADER_LINE_EXTRA
		}
	}
	return int64(size)
}

func responseHeaderSize(res *http.Response) int64 {
	size := len(res.Proto) + len(res.Status) + REQUEST_LINE_EXTRA
	for key, values := range res.Header {
		for _, v := range values {
			size += len(key) + len(v) + HEADER_LINE_EXTRA
		}
	}
	return int64(size)
}
