// Package ws is the real-time transport. A Hub keeps the set of connected
// browsers for one backend instance and fans every seat change out to them.
// Cross-instance delivery is handled upstream by Redis pub/sub; the Hub only
// ever talks to the clients attached to this one process.
package ws

import (
	"log/slog"
	"sync"
)

type Hub struct {
	mu      sync.RWMutex
	clients map[*Client]struct{}
	log     *slog.Logger
}

func NewHub(log *slog.Logger) *Hub {
	return &Hub{
		clients: make(map[*Client]struct{}),
		log:     log,
	}
}

func (h *Hub) add(c *Client) {
	h.mu.Lock()
	h.clients[c] = struct{}{}
	n := len(h.clients)
	h.mu.Unlock()
	h.log.Debug("ws client connected", "clients", n)
}

func (h *Hub) remove(c *Client) {
	h.mu.Lock()
	if _, ok := h.clients[c]; ok {
		delete(h.clients, c)
		close(c.send)
	}
	n := len(h.clients)
	h.mu.Unlock()
	h.log.Debug("ws client disconnected", "clients", n)
}

// Broadcast delivers a raw message to every connected client. A client whose
// buffer is full is assumed stuck and is dropped rather than allowed to block
// everyone else — its browser will reconnect and resync from a fresh snapshot.
func (h *Hub) Broadcast(msg []byte) {
	h.mu.RLock()
	stuck := make([]*Client, 0)
	for c := range h.clients {
		select {
		case c.send <- msg:
		default:
			stuck = append(stuck, c)
		}
	}
	h.mu.RUnlock()

	for _, c := range stuck {
		h.log.Warn("dropping slow ws client")
		h.remove(c)
	}
}

// ClientCount is used by tests and health output.
func (h *Hub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}
