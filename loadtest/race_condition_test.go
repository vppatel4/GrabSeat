package loadtest

import (
	"sort"
	"sync"
	"testing"
	"time"
)

// TestConcurrencyStress is the heart of the project. Many requests pile onto a
// small pool of seats at the same instant; we assert that exactly one request
// wins each seat, every loser gets a clean rejection, and nothing double-books —
// then we confirm all the winners and re-check the durable record.
//
// It also prints the real numbers (success/rejection counts, p50/p95 hold
// latency) that go into the README.
func TestConcurrencyStress(t *testing.T) {
	requireIntegration(t)
	waitStackReady(t)

	const (
		seatCount = 10
		workers   = 150 // 15 requests fighting over each of the 10 seats
	)
	seats := availableSeats(t, seatCount)

	type result struct {
		seatID  int
		session string
		ok      bool
		latency time.Duration
	}
	results := make([]result, workers)

	var wg sync.WaitGroup
	start := make(chan struct{}) // release everyone at once for a real thundering herd
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			seatID := seats[i%seatCount]
			session := newSession()
			<-start
			t0 := time.Now()
			code, resp, err := hold(session, seatID)
			lat := time.Since(t0)
			if err != nil {
				t.Errorf("hold request errored: %v", err)
				return
			}
			results[i] = result{
				seatID:  seatID,
				session: session,
				ok:      code == 200 && resp.Status == "held",
				latency: lat,
			}
		}(i)
	}
	close(start)
	wg.Wait()

	// --- Assert exactly one winner per seat, no double-holds ---
	winners := make(map[int]string)
	winsPerSeat := make(map[int]int)
	successes, rejections := 0, 0
	latencies := make([]time.Duration, 0, workers)
	for _, r := range results {
		latencies = append(latencies, r.latency)
		if r.ok {
			successes++
			winsPerSeat[r.seatID]++
			winners[r.seatID] = r.session
		} else {
			rejections++
		}
	}

	for _, seatID := range seats {
		if winsPerSeat[seatID] != 1 {
			t.Errorf("seat %d had %d winning holds, want exactly 1 (DOUBLE-HOLD)", seatID, winsPerSeat[seatID])
		}
	}
	if successes != seatCount {
		t.Errorf("total successful holds = %d, want %d", successes, seatCount)
	}
	if rejections != workers-seatCount {
		t.Errorf("total rejections = %d, want %d", rejections, workers-seatCount)
	}

	// --- Confirm every winner; assert the durable record has no double-bookings ---
	var confirmed int
	for _, seatID := range seats {
		code, resp, err := confirm(winners[seatID], seatID)
		if err != nil {
			t.Fatalf("confirm errored: %v", err)
		}
		if code != 200 || (resp.Status != "confirmed" && resp.Status != "already_mine") {
			t.Errorf("winner confirm for seat %d = %d/%s, want 200/confirmed", seatID, code, resp.Status)
			continue
		}
		confirmed++
	}
	if confirmed != seatCount {
		t.Errorf("confirmed bookings = %d, want %d", confirmed, seatCount)
	}

	// A loser trying to confirm a seat it never held must be cleanly rejected.
	for _, r := range results {
		if !r.ok && r.session != winners[r.seatID] {
			code, resp, _ := confirm(r.session, r.seatID)
			if code == 200 {
				t.Errorf("a loser confirmed seat %d (status %s) — DOUBLE BOOKING", r.seatID, resp.Status)
			}
			break // one probe is enough
		}
	}

	// --- Final durable check via the snapshot ---
	st := getState(t)
	bookedByWinner := 0
	for _, s := range st.Seats {
		if w, ok := winners[s.SeatID]; ok {
			if s.Status != "booked" {
				t.Errorf("seat %d final status = %s, want booked", s.SeatID, s.Status)
			}
			if s.BookedBy != w {
				t.Errorf("seat %d booked_by = %s, want winner %s", s.SeatID, s.BookedBy, w)
			}
			bookedByWinner++
		}
	}
	if bookedByWinner != seatCount {
		t.Errorf("seats booked by their winner = %d, want %d", bookedByWinner, seatCount)
	}

	p50, p95 := percentile(latencies, 50), percentile(latencies, 95)
	t.Logf("CONCURRENCY STRESS RESULTS: workers=%d seats=%d successes=%d rejections=%d double_bookings=0",
		workers, seatCount, successes, rejections)
	t.Logf("hold latency p50=%s p95=%s", p50, p95)
}

func percentile(d []time.Duration, p int) time.Duration {
	if len(d) == 0 {
		return 0
	}
	sort.Slice(d, func(i, j int) bool { return d[i] < d[j] })
	idx := (p * len(d)) / 100
	if idx >= len(d) {
		idx = len(d) - 1
	}
	return d[idx]
}
