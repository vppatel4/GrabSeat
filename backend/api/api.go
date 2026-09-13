// Package api is the HTTP surface: routing, middleware (trace ids, CORS), the
// hold/confirm endpoints, the websocket entry point, and the snapshot the
// frontend uses to render and to resync after a reconnect.
package api

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"grabseat/booking"
	"grabseat/locking"
	"grabseat/model"
	"grabseat/ratelimit"
	"grabseat/store"
	"grabseat/ws"
)

type Server struct {
	Store         *store.Store
	Lock          *locking.Client
	Bookings      *booking.Service
	Hub           *ws.Hub
	Limiter       *ratelimit.Limiter
	Event         store.Event
	Seats         []model.Seat
	validSeats    map[int]bool
	InstanceID    string
	HoldTTL       time.Duration
	AllowedOrigin string
}

// Handler builds the fully-wired http.Handler with all routes and middleware.
func (s *Server) Handler() http.Handler {
	s.validSeats = make(map[int]bool, len(s.Seats))
	for _, seat := range s.Seats {
		s.validSeats[seat.ID] = true
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.HandleFunc("GET /api/event", s.handleEvent)
	mux.HandleFunc("GET /api/state", s.handleState)
	mux.HandleFunc("GET /ws", s.handleWS)

	// The two endpoints that change state are the only rate-limited ones.
	rl := s.Limiter.Middleware(s.denyRateLimited)
	mux.Handle("POST /api/hold", rl(http.HandlerFunc(s.handleHold)))
	mux.Handle("POST /api/confirm", rl(http.HandlerFunc(s.handleConfirm)))

	// Order matters: trace first (so everything is traced/logged), then CORS.
	return s.traceMiddleware(s.corsMiddleware(mux))
}

// BuildSnapshot assembles the current seat map. Booked seats come from Postgres
// (the durable truth), holds come from Redis. If Redis is unavailable we still
// return a valid snapshot showing booked seats and flag redis_healthy=false, so
// the UI degrades instead of breaking.
func (s *Server) BuildSnapshot(ctx context.Context) ([]byte, error) {
	booked, err := s.Store.BookedSeats(ctx, s.Event.ID)
	if err != nil {
		return nil, err
	}

	redisHealthy := true
	held, herr := s.Lock.HeldSeats(ctx)
	if herr != nil {
		redisHealthy = false
		held = nil
	}

	states := make([]model.SeatState, 0, len(booked)+len(held))
	for seatID, sess := range booked {
		states = append(states, model.SeatState{
			SeatID:   seatID,
			Status:   model.StatusBooked,
			BookedBy: sess,
		})
	}
	for seatID, st := range held {
		if _, isBooked := booked[seatID]; isBooked {
			continue // booked wins over a stale hold
		}
		states = append(states, st)
	}

	snap := model.Snapshot{
		Type:           model.MsgSnapshot,
		InstanceID:     s.InstanceID,
		HoldTTLSeconds: int(s.HoldTTL.Seconds()),
		ServerTimeMs:   time.Now().UnixMilli(),
		Seats:          states,
		RedisHealthy:   redisHealthy,
	}
	return json.Marshal(snap)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
