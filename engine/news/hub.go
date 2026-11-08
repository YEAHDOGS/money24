package news

import "sync"

const CLIENT_BUFFER = 1024

// Message is one Server-Sent Event. ID is set only for stored events, so clients resume from it.
type Message struct {
	Event string
	ID    int64
	Data  []byte
}

// Hub fans each message out to every open stream.
type Hub struct {
	mu      sync.Mutex
	clients map[chan Message]struct{}
}

// NewHub returns an empty hub.
func NewHub() *Hub { return &Hub{clients: map[chan Message]struct{}{}} }

func (h *Hub) add() chan Message {
	c := make(chan Message, CLIENT_BUFFER)
	h.mu.Lock()
	h.clients[c] = struct{}{}
	h.mu.Unlock()
	return c
}

func (h *Hub) remove(c chan Message) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.clients[c]; !ok {
		return
	}
	delete(h.clients, c)
	close(c)
}

// broadcast never blocks. A client too slow to keep up is cut off; its browser reconnects
// with Last-Event-ID and the stream backfills what it missed.
func (h *Hub) broadcast(msg Message) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for c := range h.clients {
		select {
		case c <- msg:
		default:
			delete(h.clients, c)
			close(c)
		}
	}
}

func (h *Hub) count() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.clients)
}
