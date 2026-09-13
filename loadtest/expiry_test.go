package loadtest

import (
	"testing"
	"time"
)

// TestExpiryBoundary confirms a seat right as its hold is expiring. The outcome
// must be deterministic and clean either way — booked because it arrived just in
// time, or a clear "hold_expired" rejection — never a 500 or an ambiguous state.
// Then it proves the seat is genuinely free again by re-holding it from a
// different session.
//
// The test schedules itself off the real expiry time reported by the hold, so it
// works at any TTL; it skips (with advice) if the TTL is too long to wait on.
func TestExpiryBoundary(t *testing.T) {
	requireIntegration(t)
	waitStackReady(t)

	seatID := availableSeats(t, 1)[0]
	sessionA := newSession()

	code, resp, err := hold(sessionA, seatID)
	if err != nil || code != 200 {
		t.Fatalf("setup hold failed: code=%d err=%v", code, err)
	}
	expiresAt := time.UnixMilli(resp.ExpiresAtMs)
	wait := time.Until(expiresAt)
	if wait > 30*time.Second {
		t.Skipf("hold TTL too long to wait on (%s). Re-run the stack with HOLD_TTL_SECONDS=5 for this test.", wait)
	}

	// Aim the confirm to land right at the expiry instant.
	if d := time.Until(expiresAt); d > 0 {
		time.Sleep(d)
	}
	code, resp, err = confirm(sessionA, seatID)
	if err != nil {
		t.Fatalf("boundary confirm errored: %v", err)
	}
	switch resp.Status {
	case "confirmed":
		if code != 200 {
			t.Errorf("confirmed but code=%d, want 200", code)
		}
		t.Logf("boundary outcome: confirmed (arrived just in time)")
	case "hold_expired":
		if code != 409 {
			t.Errorf("hold_expired but code=%d, want 409", code)
		}
		t.Logf("boundary outcome: hold_expired (clean rejection)")
	default:
		t.Fatalf("ambiguous boundary outcome: code=%d status=%q — must be confirmed or hold_expired", code, resp.Status)
	}

	// If it expired rather than confirmed, the seat must be freely holdable again.
	if resp.Status == "hold_expired" {
		// Give the expiry event a moment to settle.
		time.Sleep(500 * time.Millisecond)
		sessionB := newSession()
		code, resp2, err := hold(sessionB, seatID)
		if err != nil {
			t.Fatalf("re-hold errored: %v", err)
		}
		if code != 200 || resp2.Status != "held" {
			t.Errorf("expected freed seat to be re-holdable, got code=%d status=%s", code, resp2.Status)
		}
	}
}
