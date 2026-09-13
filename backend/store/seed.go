package store

import (
	"context"
	"fmt"
	"time"
)

// sectionSpec describes one band of the venue. The frontend turns these into
// concentric arcs curving toward the stage, so the order here (front to back)
// is also the visual order from the stage outward.
type sectionSpec struct {
	name       string
	rows       []string
	seatsPer   int
	priceCents int
}

// A deliberately theater-shaped house: a small premium pit up front, a wide
// orchestra, then mezzanine and balcony bands fanning out and back. 320 seats
// total — enough to feel real, small enough to render fast.
var venue = []sectionSpec{
	{name: "Pit", rows: []string{"A", "B"}, seatsPer: 12, priceCents: 18000},
	{name: "Orchestra", rows: []string{"A", "B", "C", "D", "E", "F"}, seatsPer: 18, priceCents: 12500},
	{name: "Mezzanine", rows: []string{"A", "B", "C", "D", "E"}, seatsPer: 20, priceCents: 8500},
	{name: "Balcony", rows: []string{"A", "B", "C", "D"}, seatsPer: 22, priceCents: 5500},
}

// SeedIfEmpty creates the demo event and its seats the first time the database
// comes up. It is idempotent: if the event already exists it just returns its id.
func (s *Store) SeedIfEmpty(ctx context.Context) (int, error) {
	var count int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM events`).Scan(&count); err != nil {
		return 0, err
	}
	if count > 0 {
		e, err := s.PrimaryEvent(ctx)
		return e.ID, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)

	var eventID int
	startsAt := time.Now().Add(21 * 24 * time.Hour)
	err = tx.QueryRow(ctx,
		`INSERT INTO events (slug, name, venue, starts_at)
		 VALUES ($1, $2, $3, $4) RETURNING id`,
		"aurora-live",
		"Aurora — Live in Concert",
		"GrabSeat Arena",
		startsAt,
	).Scan(&eventID)
	if err != nil {
		return 0, fmt.Errorf("seed event: %w", err)
	}

	pos := 0
	for _, sec := range venue {
		for _, row := range sec.rows {
			for n := 1; n <= sec.seatsPer; n++ {
				pos++
				_, err := tx.Exec(ctx,
					`INSERT INTO seats (event_id, section, row_label, seat_number, price_cents, position)
					 VALUES ($1, $2, $3, $4, $5, $6)`,
					eventID, sec.name, row, n, sec.priceCents, pos)
				if err != nil {
					return 0, fmt.Errorf("seed seat %s-%s-%d: %w", sec.name, row, n, err)
				}
			}
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return eventID, nil
}
