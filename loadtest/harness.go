// Package loadtest holds the adversarial, end-to-end tests that exercise the
// running system over real HTTP and WebSocket connections — the same path a
// browser uses. Everything here talks to the stack through the gateway, so the
// tests also implicitly cover the reverse proxy and (when scaled) multiple
// backend instances.
//
// These tests need the compose stack running. They are gated behind
// RUN_INTEGRATION=1 so a plain `go test ./...` elsewhere doesn't try to reach a
// server that isn't there. See loadtest/README.md.
package loadtest

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func baseURL() string {
	if v := os.Getenv("BASE_URL"); v != "" {
		return v
	}
	return "http://localhost:8080"
}

func wsURL() string {
	if v := os.Getenv("WS_URL"); v != "" {
		return v
	}
	return "ws://localhost:8080/ws"
}

// requireIntegration skips the test unless we've been told the stack is up.
func requireIntegration(t *testing.T) {
	t.Helper()
	if os.Getenv("RUN_INTEGRATION") != "1" {
		t.Skip("set RUN_INTEGRATION=1 and start the stack (docker compose up) to run integration tests")
	}
}

func newSession() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return "sess_" + hex.EncodeToString(b)
}

// --- API response shapes we care about (mirrors of the backend's JSON) ---

type seatState struct {
	SeatID      int    `json:"seat_id"`
	Status      string `json:"status"`
	HeldBy      string `json:"held_by"`
	BookedBy    string `json:"booked_by"`
	ExpiresAtMs int64  `json:"expires_at_ms"`
}

type snapshot struct {
	Type         string      `json:"type"`
	InstanceID   string      `json:"instance_id"`
	Seats        []seatState `json:"seats"`
	RedisHealthy bool        `json:"redis_healthy"`
}

type seat struct {
	ID int `json:"id"`
}

type eventResponse struct {
	Seats          []seat `json:"seats"`
	HoldTTLSeconds int    `json:"hold_ttl_seconds"`
}

type actionResponse struct {
	Status      string `json:"status"`
	SeatID      int    `json:"seat_id"`
	ExpiresAtMs int64  `json:"expires_at_ms"`
	Code        string `json:"code"`
	Error       string `json:"error"`
}

// hold and confirm return the HTTP status plus the decoded body.
func hold(session string, seatID int) (int, actionResponse, error) {
	return postSeat("/api/hold", session, seatID)
}

func confirm(session string, seatID int) (int, actionResponse, error) {
	return postSeat("/api/confirm", session, seatID)
}

func postSeat(path, session string, seatID int) (int, actionResponse, error) {
	body, _ := json.Marshal(map[string]int{"seat_id": seatID})
	req, err := http.NewRequest(http.MethodPost, baseURL()+path, bytes.NewReader(body))
	if err != nil {
		return 0, actionResponse{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Session-Id", session)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, actionResponse{}, err
	}
	defer resp.Body.Close()
	var ar actionResponse
	raw, _ := io.ReadAll(resp.Body)
	_ = json.Unmarshal(raw, &ar)
	return resp.StatusCode, ar, nil
}

func getEvent(t *testing.T) eventResponse {
	t.Helper()
	resp, err := http.Get(baseURL() + "/api/event")
	if err != nil {
		t.Fatalf("get event: %v", err)
	}
	defer resp.Body.Close()
	var er eventResponse
	if err := json.NewDecoder(resp.Body).Decode(&er); err != nil {
		t.Fatalf("decode event: %v", err)
	}
	return er
}

func getState(t *testing.T) snapshot {
	t.Helper()
	resp, err := http.Get(baseURL() + "/api/state")
	if err != nil {
		t.Fatalf("get state: %v", err)
	}
	defer resp.Body.Close()
	var s snapshot
	if err := json.NewDecoder(resp.Body).Decode(&s); err != nil {
		t.Fatalf("decode state: %v", err)
	}
	return s
}

// availableSeats returns n seat ids that are currently neither held nor booked,
// so tests are safe to re-run against the same database until the venue fills.
func availableSeats(t *testing.T, n int) []int {
	t.Helper()
	ev := getEvent(t)
	st := getState(t)
	taken := make(map[int]bool)
	for _, s := range st.Seats {
		if s.Status == "held" || s.Status == "booked" {
			taken[s.SeatID] = true
		}
	}
	out := make([]int, 0, n)
	for _, s := range ev.Seats {
		if !taken[s.ID] {
			out = append(out, s.ID)
			if len(out) == n {
				break
			}
		}
	}
	if len(out) < n {
		t.Fatalf("needed %d available seats but only %d free; reset with 'docker compose down -v'", n, len(out))
	}
	return out
}

// wsClient wraps a WebSocket connection and pumps decoded events onto a channel.
type wsClient struct {
	conn     *websocket.Conn
	events   chan map[string]any
	snapshot snapshot
}

func connectWS(t *testing.T) *wsClient {
	t.Helper()
	conn, _, err := websocket.DefaultDialer.Dial(wsURL(), nil)
	if err != nil {
		t.Fatalf("ws dial: %v", err)
	}
	c := &wsClient{conn: conn, events: make(chan map[string]any, 256)}

	// The first message is always the snapshot.
	_, raw, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read snapshot: %v", err)
	}
	if err := json.Unmarshal(raw, &c.snapshot); err != nil {
		t.Fatalf("decode snapshot: %v", err)
	}

	go func() {
		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				close(c.events)
				return
			}
			var m map[string]any
			if json.Unmarshal(msg, &m) == nil {
				c.events <- m
			}
		}
	}()
	return c
}

func (c *wsClient) close() { _ = c.conn.Close() }

// waitForEvent blocks until an event matching seatID and type arrives, or fails.
func (c *wsClient) waitForEvent(t *testing.T, seatID int, evType string, timeout time.Duration) {
	t.Helper()
	deadline := time.After(timeout)
	for {
		select {
		case m, ok := <-c.events:
			if !ok {
				t.Fatalf("ws closed before seeing %s for seat %d", evType, seatID)
			}
			if int(numField(m, "seat_id")) == seatID && m["type"] == evType {
				return
			}
		case <-deadline:
			t.Fatalf("timed out waiting for %s on seat %d", evType, seatID)
		}
	}
}

func numField(m map[string]any, k string) float64 {
	if v, ok := m[k].(float64); ok {
		return v
	}
	return -1
}

func waitStackReady(t *testing.T) {
	t.Helper()
	for i := 0; i < 30; i++ {
		resp, err := http.Get(baseURL() + "/healthz")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(time.Second)
	}
	t.Fatal("stack did not become ready at " + baseURL())
}

func init() {
	// Keep the HTTP client from pooling too few connections under heavy fan-out.
	http.DefaultClient.Timeout = 15 * time.Second
	if tr, ok := http.DefaultTransport.(*http.Transport); ok {
		tr.MaxIdleConns = 500
		tr.MaxIdleConnsPerHost = 500
	}
	fmt.Fprintln(io.Discard, "loadtest harness loaded")
}
