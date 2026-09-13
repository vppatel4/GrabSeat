# Architecture

This document explains *why* GrabSeat is built the way it is. The README covers
what it does and how to run it; this is the reasoning behind the design decisions.

## The one problem this system exists to solve

Everything here is in service of a single guarantee: **one seat, one winner, no
double-bookings, resolved correctly even when requests arrive at the same
millisecond — and everyone watching sees the result live.**

Most of the interesting complexity is concentrated in that guarantee. The rest of
the system is deliberately plain so the concurrency story stays legible.

## Why Redis and Postgres, split by job

A natural first instinct is "just use the database for everything." You could hold
seats with a row and a status column, and lock it with a transaction. That works,
but it puts your hottest, most contended operation — everyone grabbing for the
same few seats — directly onto your durable database, and it ties the lifetime of
a temporary hold to a row you now have to clean up.

GrabSeat splits the two concerns:

- **Redis owns the ephemeral state: holds.** A hold is inherently short-lived and
  self-expiring. Redis's `SET key value NX EX seconds` does the whole thing in one
  atomic operation: claim the seat only if unclaimed, and forget the claim
  automatically after the TTL. There's no cleanup job, no cron, no "sweep for
  abandoned holds" — a crashed or distracted client's hold evaporates on its own.
- **Postgres owns the permanent state: bookings.** A confirmed booking must be
  durable and must never be duplicated. A `UNIQUE(seat_id)` constraint on the
  bookings table enforces that at the storage layer, independent of any
  application logic.

The payoff is that the high-frequency contended path (holds) runs against an
in-memory store built for exactly that, while the thing that must survive a
restart (bookings) lives in a database with a hard constraint protecting it. The
two are kept deliberately separate — Redis never stores a booking, Postgres never
arbitrates a live hold.

## Why an atomic Redis command instead of a database lock

The heart of the race is this: two requests for the same seat, effectively
simultaneous. `SET hold:42 <session> NX EX 300` is a single command that Redis
executes atomically. When two of them race, Redis serializes them internally —
one returns OK, the other returns nil. There is no read-then-write window for a
race to slip through, because there is no separate read.

A Postgres row lock or `SELECT ... FOR UPDATE` could also serialize this, but it
means holding a database transaction open for the duration of contention on your
most-fought-over rows, and you still need a mechanism to expire abandoned holds.
The Redis approach makes the winner-selection *and* the auto-expiry a single
built-in behavior.

The hold is actually done as a tiny Lua script (`locking/locking.go`) that also
checks a "booked" marker first, so "is it already sold?" and "grab it" happen in
one atomic step with no gap between them.

## Why WebSockets instead of polling or SSE

The product feeling that matters is: you grab a seat, and everyone else's screen
turns that seat amber *now*. Polling adds latency equal to the poll interval and
wastes requests when nothing changes. Server-Sent Events would work for the
server-to-client push, but the connection is naturally two-way here (the client
also needs a clean reconnect-and-resync story), and WebSockets are the simplest
fit for "push every state change to every viewer immediately."

On connect — and on every reconnect — the server sends a full snapshot before any
live deltas. Combined with the fact that applying a seat state is idempotent
(setting a seat to "held" twice is the same as once), a dropped-and-restored
connection always converges to the truth instead of showing stale state.

## Why Redis pub/sub instead of in-process broadcast

If GrabSeat ran as a single process, it could keep the list of connected clients
in memory and broadcast to them directly. But then you could only ever run one
instance — the moment you scale to two, a booking handled by instance A has no way
to reach a browser connected to instance B.

Redis pub/sub fixes that. When any instance changes a seat, it publishes the event
to a Redis channel. Every instance subscribes to that channel, so every instance
hears about every change and forwards it to *its own* connected browsers. This is
what makes the design horizontally scalable rather than a single point that holds
all the connections. The `TestMultiInstanceBroadcast` test proves it: it connects
clients across two instances and confirms a change on one reaches clients on the
other.

Hold expiry uses the same idea a slightly different way. Redis keyspace
notifications fire an event when a hold key hits its TTL. Every instance is
subscribed, so each one independently frees the seat for its own clients — no
single "expiry manager," no polling for expired holds.

## Why idempotency matters here specifically

Confirm is the operation where money would change hands in a real system, and it's
exactly the operation most likely to be retried: a flaky connection, an impatient
double-click, client-side retry logic. If confirm weren't idempotent, a retry
could either error out confusingly or, worse, try to book twice.

The confirm path is built so that calling it twice with the same (seat, session)
has the same result as calling it once. The mechanism is
`INSERT ... ON CONFLICT (seat_id) DO NOTHING RETURNING session_id`:

- The first confirm inserts the row and learns it inserted.
- A second confirm conflicts, inserts nothing, and reads back the existing owner.
  If that owner is the same session, it's a successful idempotent no-op; if it's a
  different session, that session simply lost the race and gets a clear conflict.

Because the uniqueness is enforced by the database, this holds even if two confirm
requests for the same seat run in genuinely parallel transactions.

## Graceful degradation when Redis is down

Redis is on the critical path for holds, so what happens when it's briefly
unavailable matters. The backend is written to fail cleanly rather than crash or
corrupt anything:

- Hold and confirm requests that can't reach Redis return a clear `503` with a
  retry hint, not a hang and not a 500.
- `/healthz` keeps returning 200 (the process is alive), so the container isn't
  killed and restart-looped during a blip; component health is reported in the
  body instead.
- The pub/sub and keyspace subscriptions reconnect on their own when Redis comes
  back, and the "booked" cache is rehydrated from Postgres so the fast path is
  correct again.
- The snapshot always reads *booked* seats from Postgres, so even if Redis were
  wiped, the UI still shows sold seats correctly.

`TestRedisResilience` (the `redis_resilience.sh` script) exercises exactly this:
stop Redis mid-flight, confirm clean 503s and a still-alive backend, restart
Redis, confirm recovery.

## Observability

A booking attempt is worth being able to trace end to end. Each request gets a
trace id that appears on every structured log line it produces, and the
lock-acquisition and confirm code paths are wrapped in OpenTelemetry spans with
attributes (seat id, session, outcome). With the stdout exporter you can point at
a single trace and see where the time went — lock acquisition versus the Postgres
write — which is the concrete thing you'd want when explaining a latency number
from the load test.

## Data model

Three tables (`backend/db/schema.sql`):

- `events` — the show(s).
- `seats` — every seat, with section / row / number / price and a stable position
  for rendering. The frontend turns these into the fanned amphitheater geometry.
- `bookings` — one row per confirmed booking, with `UNIQUE(seat_id)`. This single
  constraint is the ultimate guarantee against a double-booking.

Redis keys are simple: `hold:<seatId>` (value = session, with a TTL) and
`booked:<seatId>` (a cache of what's sold, rehydrated from Postgres). Events are
published on the `seat_events` channel.
