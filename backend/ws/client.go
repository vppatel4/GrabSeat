package ws

import (
	"context"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = (pongWait * 9) / 10
	sendBufferSize = 64
)

// SnapshotProvider builds the full-seat-map message sent to a client the moment
// it connects (and again after any reconnect), so it never trusts stale state.
type SnapshotProvider func(ctx context.Context) ([]byte, error)

type Client struct {
	hub  *Hub
	conn *websocket.Conn
	send chan []byte
}

func newUpgrader(allowedOrigin string) websocket.Upgrader {
	return websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 4096,
		CheckOrigin: func(r *http.Request) bool {
			origin := r.Header.Get("Origin")
			// Empty origin = a non-browser client (our own tests, k6). Otherwise
			// only the configured frontend origin is allowed.
			return origin == "" || origin == allowedOrigin
		},
	}
}

// Serve upgrades an HTTP request to a WebSocket, registers it, sends the initial
// snapshot, and starts the read/write pumps.
func Serve(hub *Hub, allowedOrigin string, snapshot SnapshotProvider, w http.ResponseWriter, r *http.Request) {
	up := newUpgrader(allowedOrigin)
	conn, err := up.Upgrade(w, r, nil)
	if err != nil {
		hub.log.Warn("ws upgrade failed", "err", err)
		return
	}
	c := &Client{hub: hub, conn: conn, send: make(chan []byte, sendBufferSize)}
	hub.add(c)

	go c.writePump()
	go c.readPump()

	// Build and enqueue the snapshot as the first message. Live deltas are
	// applied idempotently on the client, so even if a delta races ahead of the
	// snapshot the final state is correct.
	if snap, err := snapshot(r.Context()); err == nil {
		select {
		case c.send <- snap:
		default:
		}
	} else {
		hub.log.Warn("snapshot build failed", "err", err)
	}
}

func (c *Client) readPump() {
	defer c.hub.remove(c)
	c.conn.SetReadLimit(4096)
	_ = c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		_ = c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})
	for {
		// Clients don't send us anything meaningful; this loop exists to notice
		// disconnects and keep the read deadline fresh via pong handling.
		if _, _, err := c.conn.ReadMessage(); err != nil {
			return
		}
	}
}

func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()
	for {
		select {
		case msg, ok := <-c.send:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				_ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
