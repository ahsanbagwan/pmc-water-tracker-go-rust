package web

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"pmc/internal/kv"
)

// UpdatesChannel is the shared pub/sub channel. EVERY replica subscribes; whichever replica
// handles a write publishes, and every replica pushes to the browsers connected to IT. No sticky
// sessions needed.
const UpdatesChannel = "locality-updates"

type Hub struct {
	mu      sync.Mutex
	clients map[chan string]struct{}
	max     int
}

func NewHub(maxClients int) *Hub { return &Hub{clients: map[chan string]struct{}{}, max: maxClients} }

func (h *Hub) add() (chan string, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.max > 0 && len(h.clients) >= h.max {
		return nil, false
	}
	c := make(chan string, 16)
	h.clients[c] = struct{}{}
	return c, true
}

func (h *Hub) remove(c chan string) {
	h.mu.Lock()
	delete(h.clients, c)
	h.mu.Unlock()
}

func (h *Hub) Clients() int { h.mu.Lock(); defer h.mu.Unlock(); return len(h.clients) }

// Broadcast never blocks: a client whose buffer is full is too slow and gets dropped (it will
// reconnect and refetch), so one stuck browser can't stall everyone else.
func (h *Hub) Broadcast(msg string) {
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

// Run pumps pub/sub messages into the local hub until ctx ends.
func (h *Hub) Run(ctx context.Context, k kv.KV) {
	ch, err := k.Subscribe(ctx, UpdatesChannel)
	if err != nil {
		slog.Warn("could not subscribe to updates channel", "error", err)
		return
	}
	for msg := range ch {
		h.Broadcast(msg)
	}
}

// Announce publishes a small "this changed" message (id + status). Clients refetch details.
func (h *Hub) Announce(ctx context.Context, k kv.KV, id int64, action, status string) {
	b, _ := json.Marshal(map[string]any{"id": id, "action": action, "status": status})
	if err := k.Publish(ctx, UpdatesChannel, string(b)); err != nil {
		slog.WarnContext(ctx, "could not publish locality update", "error", err)
		h.Broadcast(string(b)) // at least reach clients connected to this replica
	}
}

func (h *handlers) stream(w http.ResponseWriter, r *http.Request) {
	rc := http.NewResponseController(w)
	_ = rc.SetWriteDeadline(time.Time{}) // long-lived: disable the server write timeout
	c, ok := h.Hub.add()
	if !ok {
		writeErr(w, http.StatusServiceUnavailable, "Too many live connections; try again shortly.")
		return
	}
	defer h.Hub.remove(c)

	hd := w.Header()
	hd.Set("Content-Type", "text/event-stream")
	hd.Set("Cache-Control", "no-cache")
	hd.Set("Connection", "keep-alive")
	hd.Set("X-Accel-Buffering", "no") // nginx: don't buffer the stream
	fmt.Fprint(w, "retry: 3000\nevent: hello\ndata: connected\n\n")
	if err := rc.Flush(); err != nil {
		return
	}

	beat := time.NewTicker(20 * time.Second) // keeps idle proxies/gateways from closing the stream
	defer beat.Stop()
	var seq int64
	for {
		select {
		case <-r.Context().Done():
			return
		case msg, open := <-c:
			if !open {
				return
			}
			seq++
			fmt.Fprintf(w, "id: %d\nevent: locality-updated\ndata: %s\n\n", seq, msg)
		case <-beat.C:
			fmt.Fprint(w, ": ping\n\n")
		}
		if err := rc.Flush(); err != nil {
			return
		}
	}
}
