package loadtest

import (
	"testing"
	"time"
)

// TestWebSocketReconnect proves a client that drops mid-hold and reconnects
// resyncs to the actual current seat map from a fresh snapshot, rather than
// showing whatever it last saw. This is what keeps a flaky network from leaving
// a browser with a stale view.
func TestWebSocketReconnect(t *testing.T) {
	requireIntegration(t)
	waitStackReady(t)

	seatID := availableSeats(t, 1)[0]

	// Client connects and is watching.
	c1 := connectWS(t)

	// Someone else holds the seat; the live client should see it turn held.
	holder := newSession()
	if code, _, err := hold(holder, seatID); err != nil || code != 200 {
		t.Fatalf("hold failed: code=%d err=%v", code, err)
	}
	c1.waitForEvent(t, seatID, "held", 5*time.Second)

	// Simulate a dropped connection.
	c1.close()

	// Reconnect. The snapshot must reflect the seat as held right now.
	c2 := connectWS(t)
	defer c2.close()

	var seenHeld bool
	for _, s := range c2.snapshot.Seats {
		if s.SeatID == seatID && s.Status == "held" {
			seenHeld = true
			if s.HeldBy != holder {
				t.Errorf("resynced snapshot held_by = %s, want %s", s.HeldBy, holder)
			}
		}
	}
	if !seenHeld {
		t.Errorf("reconnected client did not resync seat %d as held", seatID)
	}
}
