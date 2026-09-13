package api

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"grabseat/booking"
	"grabseat/observability"
	"grabseat/ws"
)

type seatRequest struct {
	SeatID int `json:"seat_id"`
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	pgOK := s.Store.Ping(ctx) == nil
	redisOK := s.Lock.Healthy(ctx)

	// The process is alive if it can answer this, so we return 200 even when a
	// dependency is down. That keeps the container from being killed and
	// restarted mid-outage; component health is reported in the body instead.
	writeJSON(w, http.StatusOK, map[string]any{
		"status":   "ok",
		"instance": s.InstanceID,
		"postgres": pgOK,
		"redis":    redisOK,
		"clients":  s.Hub.ClientCount(),
	})
}

func (s *Server) handleEvent(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"event":            s.Event,
		"seats":            s.Seats,
		"hold_ttl_seconds": int(s.HoldTTL.Seconds()),
		"instance":         s.InstanceID,
	})
}

func (s *Server) handleState(w http.ResponseWriter, r *http.Request) {
	snap, err := s.BuildSnapshot(r.Context())
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "could not build seat state",
			"code":  "unavailable",
		})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(snap)
}

func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	ws.Serve(s.Hub, s.AllowedOrigin, s.BuildSnapshot, w, r)
}

func (s *Server) handleHold(w http.ResponseWriter, r *http.Request) {
	session, seatID, ok := s.parseSeatRequest(w, r)
	if !ok {
		return
	}
	outcome, expiresAtMs, err := s.Bookings.Hold(r.Context(), seatID, session)
	if err != nil {
		observability.LoggerFromContext(r.Context()).Error("hold failed", "err", err, "seat_id", seatID)
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "seat service temporarily unavailable, please retry",
			"code":  "unavailable",
		})
		return
	}

	switch outcome {
	case booking.OutcomeHeld:
		writeJSON(w, http.StatusOK, map[string]any{
			"status":        "held",
			"seat_id":       seatID,
			"expires_at_ms": expiresAtMs,
		})
	case booking.OutcomeTaken:
		writeJSON(w, http.StatusConflict, map[string]any{
			"status":  "taken",
			"seat_id": seatID,
			"message": "someone grabbed this seat first",
		})
	case booking.OutcomeSeatBooked:
		writeJSON(w, http.StatusConflict, map[string]any{
			"status":  "seat_booked",
			"seat_id": seatID,
			"message": "this seat is already sold",
		})
	}
}

func (s *Server) handleConfirm(w http.ResponseWriter, r *http.Request) {
	session, seatID, ok := s.parseSeatRequest(w, r)
	if !ok {
		return
	}
	outcome, err := s.Bookings.Confirm(r.Context(), seatID, session)
	if err != nil {
		observability.LoggerFromContext(r.Context()).Error("confirm failed", "err", err, "seat_id", seatID)
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "seat service temporarily unavailable, please retry",
			"code":  "unavailable",
		})
		return
	}

	switch outcome {
	case booking.OutcomeConfirmed:
		writeJSON(w, http.StatusOK, map[string]any{"status": "confirmed", "seat_id": seatID})
	case booking.OutcomeAlreadyMine:
		// Idempotent success: a retried confirm lands here, same 200 as the first.
		writeJSON(w, http.StatusOK, map[string]any{"status": "already_mine", "seat_id": seatID})
	case booking.OutcomeConflict:
		writeJSON(w, http.StatusConflict, map[string]any{
			"status": "conflict", "seat_id": seatID,
			"message": "this seat belongs to someone else",
		})
	case booking.OutcomeExpired:
		writeJSON(w, http.StatusConflict, map[string]any{
			"status": "hold_expired", "seat_id": seatID,
			"message": "your hold expired — the seat is open again",
		})
	}
}

// parseSeatRequest validates the session header and seat id shared by both
// state-changing endpoints. It writes the error response itself and returns
// ok=false when the request is malformed.
func (s *Server) parseSeatRequest(w http.ResponseWriter, r *http.Request) (session string, seatID int, ok bool) {
	session = r.Header.Get("X-Session-Id")
	if session == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "missing X-Session-Id header",
			"code":  "no_session",
		})
		return "", 0, false
	}
	var req seatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "invalid JSON body",
			"code":  "bad_request",
		})
		return "", 0, false
	}
	if !s.validSeats[req.SeatID] {
		writeJSON(w, http.StatusNotFound, map[string]string{
			"error": "unknown seat",
			"code":  "no_such_seat",
		})
		return "", 0, false
	}
	return session, req.SeatID, true
}

func (s *Server) denyRateLimited(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusTooManyRequests, map[string]string{
		"error": "too many requests — slow down",
		"code":  "rate_limited",
	})
}
