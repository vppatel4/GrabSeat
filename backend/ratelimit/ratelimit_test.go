package ratelimit

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// A single session hammering the endpoint should be throttled once its burst is
// spent, while a different session is completely unaffected. That "one abuser
// doesn't hurt anyone else" property is the whole point of keying per session.
func TestPerSessionThrottling(t *testing.T) {
	l := New(60) // 1 req/sec sustained, burst of 10

	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	deny := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}
	h := l.Middleware(deny)(ok)

	countStatuses := func(session string, n int) (allowed, denied int) {
		for i := 0; i < n; i++ {
			req := httptest.NewRequest(http.MethodPost, "/api/hold", nil)
			req.Header.Set("X-Session-Id", session)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code == http.StatusOK {
				allowed++
			} else {
				denied++
			}
		}
		return
	}

	allowedA, deniedA := countStatuses("abuser", 100)
	if deniedA == 0 {
		t.Errorf("expected abuser to be throttled, but nothing was denied")
	}
	if allowedA == 0 {
		t.Errorf("expected some of the abuser's requests to pass, got 0")
	}

	// A second, well-behaved session should sail through its small burst.
	allowedB, deniedB := countStatuses("polite", 10)
	if deniedB != 0 {
		t.Errorf("a separate session was throttled (%d denied) — buckets are not isolated", deniedB)
	}
	if allowedB != 10 {
		t.Errorf("polite session allowed = %d, want 10", allowedB)
	}
}
