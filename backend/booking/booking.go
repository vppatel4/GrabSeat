// Package booking ties the ephemeral lock (Redis) and the durable record
// (Postgres) together into the two operations a user actually performs: hold a
// seat, then confirm it. The confirm path is deliberately idempotent so a
// retried request never double-books and never errors.
package booking

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"grabseat/locking"
	"grabseat/model"
	"grabseat/observability"
	"grabseat/store"
)

// Outcome codes returned to the API layer, which maps them to HTTP responses.
const (
	OutcomeHeld        = "held"          // hold acquired
	OutcomeTaken       = "taken"         // held by someone else
	OutcomeSeatBooked  = "seat_booked"   // already sold, can't hold
	OutcomeConfirmed   = "confirmed"     // booking written just now
	OutcomeAlreadyMine = "already_mine"  // idempotent: this session already booked it
	OutcomeConflict    = "conflict"      // someone else owns it
	OutcomeExpired     = "hold_expired"  // no valid hold to confirm
)

type Service struct {
	store      *store.Store
	lock       *locking.Client
	eventID    int
	instanceID string
	holdTTL    time.Duration
}

func NewService(st *store.Store, lk *locking.Client, eventID int, instanceID string, holdTTL time.Duration) *Service {
	return &Service{store: st, lock: lk, eventID: eventID, instanceID: instanceID, holdTTL: holdTTL}
}

// Hold tries to grab a seat for a session. On success it broadcasts a "held"
// event so every other connected client sees the seat turn amber instantly.
func (s *Service) Hold(ctx context.Context, seatID int, session string) (outcome string, expiresAtMs int64, err error) {
	ctx, span := observability.Tracer.Start(ctx, "acquire_hold")
	defer span.End()
	span.SetAttributes(attribute.Int("seat.id", seatID), attribute.String("session.id", session))
	log := observability.LoggerFromContext(ctx)

	res, expiresAtMs, err := s.lock.TryHold(ctx, seatID, session, s.holdTTL)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "redis unavailable")
		return "", 0, fmt.Errorf("hold: %w", err)
	}

	switch res {
	case locking.HoldAcquired:
		span.SetAttributes(attribute.String("hold.result", "acquired"))
		ev := model.SeatEvent{
			Type:        model.MsgHeld,
			SeatID:      seatID,
			Status:      model.StatusHeld,
			HeldBy:      session,
			ExpiresAtMs: expiresAtMs,
			ServedBy:    s.instanceID,
		}
		if perr := s.lock.Publish(ctx, ev); perr != nil {
			// The lock is held regardless; a failed broadcast only delays the
			// live update, so log it rather than failing the request.
			log.Warn("publish held event failed", "err", perr, "seat_id", seatID)
		}
		log.Info("seat held", "seat_id", seatID, "session", session)
		return OutcomeHeld, expiresAtMs, nil
	case locking.HoldBooked:
		span.SetAttributes(attribute.String("hold.result", "already_booked"))
		return OutcomeSeatBooked, 0, nil
	default: // HoldTaken
		span.SetAttributes(attribute.String("hold.result", "taken"))
		return OutcomeTaken, 0, nil
	}
}

// Confirm turns a hold into a permanent booking. It is safe to call more than
// once with the same (seat, session): the first call books, every later call
// returns the same "already yours" answer with no extra side effects.
func (s *Service) Confirm(ctx context.Context, seatID int, session string) (outcome string, err error) {
	ctx, span := observability.Tracer.Start(ctx, "confirm_booking")
	defer span.End()
	span.SetAttributes(attribute.Int("seat.id", seatID), attribute.String("session.id", session))
	log := observability.LoggerFromContext(ctx)

	holder, held, err := s.lock.HoldOwner(ctx, seatID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "redis unavailable")
		return "", fmt.Errorf("check hold: %w", err)
	}

	// Not the current holder: resolve the outcome from the durable record
	// without inserting anything, so we can't accidentally book a seat we don't
	// hold. This also handles the idempotent-retry case cleanly.
	if !held || holder != session {
		owner, found, derr := s.store.BookingOwner(ctx, seatID)
		if derr != nil {
			span.RecordError(derr)
			return "", fmt.Errorf("lookup booking: %w", derr)
		}
		switch {
		case found && owner == session:
			span.SetAttributes(attribute.String("confirm.result", "already_mine"))
			return OutcomeAlreadyMine, nil
		case found:
			span.SetAttributes(attribute.String("confirm.result", "conflict"))
			return OutcomeConflict, nil
		case held: // held by someone else, mid-purchase
			span.SetAttributes(attribute.String("confirm.result", "conflict"))
			return OutcomeConflict, nil
		default:
			span.SetAttributes(attribute.String("confirm.result", "expired"))
			return OutcomeExpired, nil
		}
	}

	// We hold the seat. The DB insert is the authoritative, atomic step.
	inserted, owner, err := s.store.ConfirmBooking(ctx, s.eventID, seatID, session)
	if err != nil {
		span.RecordError(err)
		return "", fmt.Errorf("confirm booking: %w", err)
	}

	if inserted {
		// Flip the Redis cache from held -> booked and tell everyone.
		if merr := s.lock.MarkBooked(ctx, seatID, session); merr != nil {
			log.Warn("mark booked in redis failed", "err", merr, "seat_id", seatID)
		}
		ev := model.SeatEvent{
			Type:     model.MsgBooked,
			SeatID:   seatID,
			Status:   model.StatusBooked,
			BookedBy: session,
			ServedBy: s.instanceID,
		}
		if perr := s.lock.Publish(ctx, ev); perr != nil {
			log.Warn("publish booked event failed", "err", perr, "seat_id", seatID)
		}
		span.SetAttributes(attribute.String("confirm.result", "confirmed"))
		log.Info("seat booked", "seat_id", seatID, "session", session)
		return OutcomeConfirmed, nil
	}

	// Not inserted: a row already existed. If it's ours, this was a concurrent
	// double-confirm and we treat it as success; otherwise we lost the race.
	if owner == session {
		span.SetAttributes(attribute.String("confirm.result", "already_mine"))
		return OutcomeAlreadyMine, nil
	}
	span.SetAttributes(attribute.String("confirm.result", "conflict"))
	return OutcomeConflict, nil
}
