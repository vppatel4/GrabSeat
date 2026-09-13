// Package ratelimit throttles the hold and confirm endpoints per session (or
// per IP when no session is given). One scripted client hammering the API gets
// its own bucket and is slowed down without touching anyone else's requests.
package ratelimit

import (
	"net"
	"net/http"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

type bucket struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

type Limiter struct {
	mu       sync.Mutex
	buckets  map[string]*bucket
	rate     rate.Limit
	burst    int
}

// New builds a limiter allowing perMinute requests per key over time, with a
// small burst so normal clicking is never throttled but scripted hammering is.
func New(perMinute int) *Limiter {
	if perMinute <= 0 {
		perMinute = 60
	}
	l := &Limiter{
		buckets: make(map[string]*bucket),
		rate:    rate.Limit(float64(perMinute) / 60.0),
		burst:   10,
	}
	go l.cleanupLoop()
	return l
}

func (l *Limiter) get(key string) *rate.Limiter {
	l.mu.Lock()
	defer l.mu.Unlock()
	b, ok := l.buckets[key]
	if !ok {
		b = &bucket{limiter: rate.NewLimiter(l.rate, l.burst)}
		l.buckets[key] = b
	}
	b.lastSeen = time.Now()
	return b.limiter
}

// cleanupLoop drops buckets that have been idle so the map can't grow forever
// under many distinct sessions.
func (l *Limiter) cleanupLoop() {
	t := time.NewTicker(5 * time.Minute)
	defer t.Stop()
	for range t.C {
		l.mu.Lock()
		for k, b := range l.buckets {
			if time.Since(b.lastSeen) > 10*time.Minute {
				delete(l.buckets, k)
			}
		}
		l.mu.Unlock()
	}
}

// Middleware wraps a handler, rejecting over-limit callers with 429 before any
// work is done. deny is called to write the rejection so the caller controls the
// response shape.
func (l *Limiter) Middleware(deny http.HandlerFunc) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := clientKey(r)
			if !l.get(key).Allow() {
				deny(w, r)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// clientKey prefers the per-tab session id; if absent it falls back to the real
// client IP (the gateway forwards it in X-Forwarded-For).
func clientKey(r *http.Request) string {
	if s := r.Header.Get("X-Session-Id"); s != "" {
		return "sess:" + s
	}
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return "ip:" + xff
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	return "ip:" + host
}
