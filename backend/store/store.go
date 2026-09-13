// Package store is the durable side of GrabSeat: Postgres. It owns the
// connection pool, runs the schema on startup, seeds the demo venue, and holds
// the one query that actually makes a double-booking impossible.
package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"grabseat/model"
)

type Store struct {
	pool *pgxpool.Pool
}

// Event is the metadata for the one seeded show.
type Event struct {
	ID       int       `json:"id"`
	Slug     string    `json:"slug"`
	Name     string    `json:"name"`
	Venue    string    `json:"venue"`
	StartsAt time.Time `json:"starts_at"`
}

// New opens a pooled connection and verifies it works before returning.
func New(ctx context.Context, url string) (*Store, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("parse database url: %w", err)
	}
	cfg.MaxConns = 20
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("connect postgres: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	return &Store{pool: pool}, nil
}

func (s *Store) Close() { s.pool.Close() }

func (s *Store) Ping(ctx context.Context) error { return s.pool.Ping(ctx) }

// RunMigrations executes the schema. It is written with IF NOT EXISTS, so
// running it on every boot is safe and means a clean clone needs no manual step.
func (s *Store) RunMigrations(ctx context.Context, schema string) error {
	_, err := s.pool.Exec(ctx, schema)
	return err
}

// ConfirmBooking is the durable half of the confirm flow. The INSERT ... ON
// CONFLICT DO NOTHING against the UNIQUE(seat_id) constraint is atomic: the
// first caller inserts and gets inserted=true; any later caller (a retry, a
// double-click, or a genuine racing request) conflicts and gets back the
// session that actually owns the seat. That single query gives us both
// no-double-booking and idempotency for free.
func (s *Store) ConfirmBooking(ctx context.Context, eventID, seatID int, session string) (inserted bool, owner string, err error) {
	err = s.pool.QueryRow(ctx,
		`INSERT INTO bookings (seat_id, event_id, session_id)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (seat_id) DO NOTHING
		 RETURNING session_id`,
		seatID, eventID, session,
	).Scan(&owner)

	if err == nil {
		return true, owner, nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		// Conflict: the row already existed, so nothing was returned. Read back
		// who owns it so the caller can decide (same session = idempotent OK,
		// different session = lost the race).
		err = s.pool.QueryRow(ctx,
			`SELECT session_id FROM bookings WHERE seat_id = $1`, seatID,
		).Scan(&owner)
		if err != nil {
			return false, "", fmt.Errorf("read existing booking: %w", err)
		}
		return false, owner, nil
	}
	return false, "", fmt.Errorf("insert booking: %w", err)
}

// BookingOwner reports who, if anyone, has booked a seat. Used by the confirm
// path to resolve a retry that arrived after the Redis hold was already gone,
// without touching (and possibly creating) a booking.
func (s *Store) BookingOwner(ctx context.Context, seatID int) (owner string, found bool, err error) {
	err = s.pool.QueryRow(ctx,
		`SELECT session_id FROM bookings WHERE seat_id = $1`, seatID).Scan(&owner)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return owner, true, nil
}

// BookedSeats returns seat_id -> owning session for every confirmed booking.
// This is the authoritative source for "which seats are taken" — the snapshot
// uses it, so even if Redis is wiped the UI still shows booked seats correctly.
func (s *Store) BookedSeats(ctx context.Context, eventID int) (map[int]string, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT seat_id, session_id FROM bookings WHERE event_id = $1`, eventID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[int]string)
	for rows.Next() {
		var id int
		var sess string
		if err := rows.Scan(&id, &sess); err != nil {
			return nil, err
		}
		out[id] = sess
	}
	return out, rows.Err()
}

// Seats returns every seat for the event, ordered for stable rendering.
func (s *Store) Seats(ctx context.Context, eventID int) ([]model.Seat, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, section, row_label, seat_number, price_cents, position
		   FROM seats WHERE event_id = $1 ORDER BY position`, eventID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var seats []model.Seat
	for rows.Next() {
		var st model.Seat
		if err := rows.Scan(&st.ID, &st.Section, &st.RowLabel, &st.SeatNumber, &st.PriceCents, &st.Position); err != nil {
			return nil, err
		}
		seats = append(seats, st)
	}
	return seats, rows.Err()
}

// PrimaryEvent returns the single seeded event.
func (s *Store) PrimaryEvent(ctx context.Context) (Event, error) {
	var e Event
	err := s.pool.QueryRow(ctx,
		`SELECT id, slug, name, venue, starts_at FROM events ORDER BY id LIMIT 1`,
	).Scan(&e.ID, &e.Slug, &e.Name, &e.Venue, &e.StartsAt)
	return e, err
}
