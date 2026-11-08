package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/lib/pq"
)

const (
	NOTIFY_CHANNEL      = "events"
	LISTENER_MIN_RETRY  = 2 * time.Second
	LISTENER_MAX_RETRY  = time.Minute
	LISTENER_PING_EVERY = 90 * time.Second
	SSE_HEARTBEAT       = 25 * time.Second
	CLIENT_BUFFER       = 64
	DEFAULT_LIMIT       = 100
)

//go:embed web
var webFiles embed.FS

// Hub fans each new event out to every open browser stream.
type Hub struct {
	mu      sync.Mutex
	clients map[chan []byte]struct{}
}

// listen relays Postgres NOTIFY messages to the hub, reconnecting on its own.
func (h *Hub) listen(ctx context.Context, dbURL string) {
	listener := pq.NewListener(dbURL, LISTENER_MIN_RETRY, LISTENER_MAX_RETRY, func(ev pq.ListenerEventType, err error) {
		if err != nil {
			log.Printf("[listen] %v", err)
		}
	})
	defer listener.Close()
	if err := listener.Listen(NOTIFY_CHANNEL); err != nil {
		log.Printf("[listen] %v", err)
		return
	}
	for {
		select {
		case <-ctx.Done():
			return
		case n := <-listener.Notify:
			if n == nil {
				continue // connection was re-established
			}
			h.broadcast([]byte(n.Extra))
		case <-time.After(LISTENER_PING_EVERY):
			go listener.Ping()
		}
	}
}

func (h *Hub) broadcast(msg []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for c := range h.clients {
		select {
		case c <- msg:
		default: // slow client: drop rather than stall everyone
		}
	}
}

func (h *Hub) add() chan []byte {
	c := make(chan []byte, CLIENT_BUFFER)
	h.mu.Lock()
	h.clients[c] = struct{}{}
	h.mu.Unlock()
	return c
}

func (h *Hub) remove(c chan []byte) {
	h.mu.Lock()
	delete(h.clients, c)
	h.mu.Unlock()
}

func routes(store *Store, hub *Hub) http.Handler {
	mux := http.NewServeMux()
	static, _ := fs.Sub(webFiles, "web")
	mux.Handle("GET /", http.FileServerFS(static))

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})

	mux.HandleFunc("GET /api/events", func(w http.ResponseWriter, r *http.Request) {
		limit, err := strconv.Atoi(r.URL.Query().Get("limit"))
		if err != nil {
			limit = DEFAULT_LIMIT
		}
		events, err := store.recentEvents(r.Context(), limit, r.URL.Query().Get("tag"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(events)
	})

	mux.HandleFunc("GET /api/stream", func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		c := hub.add()
		defer hub.remove(c)
		heartbeat := time.NewTicker(SSE_HEARTBEAT)
		defer heartbeat.Stop()
		fmt.Fprint(w, ": connected\n\n")
		flusher.Flush()
		for {
			select {
			case <-r.Context().Done():
				return
			case msg := <-c:
				fmt.Fprintf(w, "data: %s\n\n", msg)
				flusher.Flush()
			case <-heartbeat.C:
				fmt.Fprint(w, ": ping\n\n")
				flusher.Flush()
			}
		}
	})
	return mux
}
