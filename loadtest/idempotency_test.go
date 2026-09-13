package loadtest

import (
	"sync"
	"testing"
)

// TestIdempotentConfirm fires the same confirm five times at once, the way a
// flaky connection or a double-clicking user would. The rule: exactly one
// booking, no errors, no duplicate side effects. Every call must return a 200
// ("confirmed" for the first to land, "already_mine" for the rest).
func TestIdempotentConfirm(t *testing.T) {
	requireIntegration(t)
	waitStackReady(t)

	seatID := availableSeats(t, 1)[0]
	session := newSession()

	if code, resp, err := hold(session, seatID); err != nil || code != 200 {
		t.Fatalf("setup hold failed: code=%d status=%s err=%v", code, resp.Status, err)
	}

	const tries = 5
	codes := make([]int, tries)
	statuses := make([]string, tries)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < tries; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			code, resp, err := confirm(session, seatID)
			if err != nil {
				t.Errorf("confirm %d errored: %v", i, err)
				return
			}
			codes[i] = code
			statuses[i] = resp.Status
		}(i)
	}
	close(start)
	wg.Wait()

	confirmedCount := 0
	for i := 0; i < tries; i++ {
		if codes[i] != 200 {
			t.Errorf("confirm attempt %d returned %d (%s), want 200", i, codes[i], statuses[i])
		}
		switch statuses[i] {
		case "confirmed":
			confirmedCount++
		case "already_mine":
			// idempotent success
		default:
			t.Errorf("confirm attempt %d status = %q, want confirmed/already_mine", i, statuses[i])
		}
	}
	if confirmedCount != 1 {
		t.Errorf("got %d 'confirmed' responses, want exactly 1 (the rest should be already_mine)", confirmedCount)
	}

	// The durable record must show the seat booked exactly once, by this session.
	st := getState(t)
	found := 0
	for _, s := range st.Seats {
		if s.SeatID == seatID {
			found++
			if s.Status != "booked" || s.BookedBy != session {
				t.Errorf("seat %d = %s booked_by %s, want booked by %s", seatID, s.Status, s.BookedBy, session)
			}
		}
	}
	if found != 1 {
		t.Errorf("seat %d appears %d times in snapshot, want 1", seatID, found)
	}
}
