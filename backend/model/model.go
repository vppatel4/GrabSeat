// Package model holds the small set of types shared across the backend so the
// locking, booking, websocket, and API packages don't have to import each other.
package model

// Seat status values, sent to the browser exactly as-is.
const (
	StatusAvailable = "available"
	StatusHeld      = "held"
	StatusBooked    = "booked"
)

// Event message types pushed over the websocket / published on Redis pub/sub.
const (
	MsgSnapshot = "snapshot" // full seat map, sent right after a client connects
	MsgHeld     = "held"     // a seat was just held
	MsgBooked   = "booked"   // a seat was permanently booked
	MsgFreed    = "freed"    // a hold expired (or was released) and the seat is open again
)

// Seat is the static description of a seat, used to render the venue.
type Seat struct {
	ID         int    `json:"id"`
	Section    string `json:"section"`
	RowLabel   string `json:"row_label"`
	SeatNumber int    `json:"seat_number"`
	PriceCents int    `json:"price_cents"`
	Position   int    `json:"position"`
}

// SeatState is the live status of one seat at a moment in time.
type SeatState struct {
	SeatID int    `json:"seat_id"`
	Status string `json:"status"`
	// HeldBy / BookedBy carry the session id so a browser can tell "my hold"
	// apart from "someone else's". These are random per-tab ids, not real users.
	HeldBy      string `json:"held_by,omitempty"`
	BookedBy    string `json:"booked_by,omitempty"`
	ExpiresAtMs int64  `json:"expires_at_ms,omitempty"`
}

// SeatEvent is a single live change broadcast to every connected client.
type SeatEvent struct {
	Type        string `json:"type"`
	SeatID      int    `json:"seat_id"`
	Status      string `json:"status"`
	HeldBy      string `json:"held_by,omitempty"`
	BookedBy    string `json:"booked_by,omitempty"`
	ExpiresAtMs int64  `json:"expires_at_ms,omitempty"`
	// ServedBy names the backend instance that produced the event. It exists so
	// the multi-instance test can prove a booking on instance A reached a client
	// connected to instance B.
	ServedBy string `json:"served_by,omitempty"`
}

// Snapshot is the first message a client receives: the whole seat map plus a
// little context so it can render countdowns correctly and resync after a
// reconnect without trusting anything it saw before the drop.
type Snapshot struct {
	Type           string      `json:"type"` // always MsgSnapshot
	InstanceID     string      `json:"instance_id"`
	HoldTTLSeconds int         `json:"hold_ttl_seconds"`
	ServerTimeMs   int64       `json:"server_time_ms"`
	Seats          []SeatState `json:"seats"`
	RedisHealthy   bool        `json:"redis_healthy"`
}
