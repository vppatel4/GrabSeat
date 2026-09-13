package loadtest

import (
	"testing"
)

// TestRateLimitIsolation hammers the hold endpoint from one session and checks
// two things: that abuser gets throttled (429s appear), and — the important
// part — that a separate, well-behaved session is completely unaffected. A rate
// limiter that punished everyone during an attack would be worse than none.
func TestRateLimitIsolation(t *testing.T) {
	requireIntegration(t)
	waitStackReady(t)

	// Two distinct seats so the two sessions never interfere at the seat level;
	// we're measuring throttling, not seat contention.
	seats := availableSeats(t, 2)
	abuserSeat, politeSeat := seats[0], seats[1]

	abuser := newSession()
	polite := newSession()

	abuser429 := 0
	for i := 0; i < 60; i++ {
		code, _, err := hold(abuser, abuserSeat)
		if err != nil {
			t.Fatalf("abuser request errored: %v", err)
		}
		if code == 429 {
			abuser429++
		}
	}
	if abuser429 == 0 {
		t.Errorf("expected the abuser to hit rate limiting, saw no 429s")
	}

	// The polite session makes a handful of requests and must never see a 429.
	polite429 := 0
	for i := 0; i < 5; i++ {
		code, _, err := hold(polite, politeSeat)
		if err != nil {
			t.Fatalf("polite request errored: %v", err)
		}
		if code == 429 {
			polite429++
		}
	}
	if polite429 != 0 {
		t.Errorf("polite session was throttled %d times — limiter is not isolating per session", polite429)
	}
	t.Logf("RATE LIMIT RESULTS: abuser 429s=%d/60, polite 429s=%d/5", abuser429, polite429)
}
