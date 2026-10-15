-- Database tables for GrabSeat. Redis keeps the temporary holds; this keeps the
-- permanent data (bookings and the seat/event info) that has to survive a restart.
--
-- This runs on every backend startup (CREATE ... IF NOT EXISTS), so you don't
-- have to run migrations by hand after cloning.

CREATE TABLE IF NOT EXISTS events (
    id          SERIAL PRIMARY KEY,
    slug        TEXT NOT NULL UNIQUE,
    name        TEXT NOT NULL,
    venue       TEXT NOT NULL,
    starts_at   TIMESTAMPTZ NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS seats (
    id           SERIAL PRIMARY KEY,
    event_id     INTEGER NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    section      TEXT NOT NULL,
    row_label    TEXT NOT NULL,
    seat_number  INTEGER NOT NULL,
    price_cents  INTEGER NOT NULL,
    -- ordering position within the whole venue, handy for stable rendering
    position     INTEGER NOT NULL,
    UNIQUE (event_id, section, row_label, seat_number)
);

CREATE INDEX IF NOT EXISTS idx_seats_event ON seats(event_id);

CREATE TABLE IF NOT EXISTS bookings (
    id          SERIAL PRIMARY KEY,
    -- One booking per seat. If two confirms ever got past the Redis lock, this
    -- UNIQUE still stops the second one from being saved.
    seat_id     INTEGER NOT NULL UNIQUE REFERENCES seats(id) ON DELETE CASCADE,
    event_id    INTEGER NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    session_id  TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_bookings_event ON bookings(event_id);
