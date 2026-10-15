# Architecture

This goes a bit deeper into why I made the choices I did. The README covers what
the app does and how to run it; this is the reasoning.

## The one thing this project is about

Almost everything here is in service of one thing: when a lot of people go for the
same seat at once, only one should get it, no seat should ever be booked twice, and
everyone watching should see the change live.

The rest of the app is kept plain on purpose so that part stays easy to follow.

## Why Redis and Postgres instead of just one database

The easy option is to use one database for everything: a seats table with a status
column, and a transaction or row lock to stop two people grabbing the same seat.
That works, but it means your busiest, most fought-over operation runs straight
against the database, and you also have to clean up holds that nobody finished.

I split it up:

- **Redis holds the temporary stuff (the holds).** A hold is short-lived and
  should expire on its own. Redis's `SET key value NX EX seconds` does both in one
  command: grab the seat only if it's free, and forget it after a timeout. No
  cleanup job, no cron. If someone starts a hold and closes their laptop, the hold
  just disappears when the timer runs out.
- **Postgres holds the permanent stuff (the bookings).** A booking has to survive a
  restart and must never be duplicated. A `UNIQUE(seat_id)` on the bookings table
  takes care of that no matter what the code does.

So the hot, temporary path runs on Redis, which is built for it, and the thing that
has to be correct forever lives in Postgres with a rule protecting it. Redis never
stores a booking, and Postgres never decides a live hold.

## Why one Redis command instead of a database lock

The whole race comes down to two requests for the same seat arriving at basically
the same time. `SET hold:42 <session> NX EX 300` is a single command, and Redis
runs commands one at a time, so one request gets it and the other doesn't. There's
no gap between reading and writing where a second request could sneak in, because
there's no separate read.

You can do the same thing in Postgres with a row lock or `SELECT ... FOR UPDATE`,
but then you're holding a database transaction open on your most contended rows,
and you still have to build something to expire old holds. Redis gives me the
pick-a-winner behavior and the auto-expiry in one step.

In the code the hold is a small Lua script (`locking/locking.go`) that also checks
a "booked" marker first, so "is it already sold?" and "grab it" happen together
with no gap in between.

## Why WebSockets instead of polling

The feeling I wanted is: you grab a seat and everyone else's screen updates right
away. Polling adds a delay equal to how often you poll, and wastes requests when
nothing changed. WebSockets just push the change the moment it happens.

When a browser connects (or reconnects), the server first sends the full seat map.
Applying a seat change is written so that doing it twice is the same as doing it
once, so even if a live update and the first snapshot arrive out of order, the
browser still ends up showing the right thing.

## Why Redis pub/sub instead of keeping it in memory

If there were only ever one backend, it could just keep the list of connected
browsers in memory and send updates to them directly. But then I could only ever
run one backend. The second I run two, a booking made on backend A wouldn't reach
someone connected to backend B.

Redis pub/sub fixes that. When any backend changes a seat, it publishes the change
to a Redis channel. Every backend listens to that channel and forwards the change
to its own browsers. That's what lets me run more than one backend. The
`TestMultiInstanceBroadcast` test connects clients to two backends and checks a
change on one shows up on the other.

Hold expiry works the same way. Redis can fire an event when a key expires, every
backend listens for it, and each one frees the seat for its own browsers. No
polling for expired holds.

## Why confirm is safe to call more than once

Confirm is the step that matters most, and it's also the one most likely to get
retried: a flaky connection, an impatient double click, a client retry. If it
wasn't safe to retry, a retry could error out or, worse, try to book twice.

I use `INSERT ... ON CONFLICT (seat_id) DO NOTHING RETURNING session_id`:

- The first confirm inserts the row and sees that it inserted.
- A second confirm hits the conflict, inserts nothing, and reads back who owns the
  seat. If it's the same person, that's fine, it's just a repeat. If it's someone
  else, they simply lost and get a clear message.

Because the uniqueness is enforced by the database, this holds up even if two
confirms run at the exact same time.

## What happens when Redis goes down

Redis is needed for holds, so I made sure a Redis outage fails cleanly instead of
crashing:

- Hold and confirm requests that can't reach Redis return a clear `503` asking the
  client to retry, not a crash and not a 500.
- `/healthz` keeps returning 200 (the process is still alive), so the container
  doesn't get killed and restarted in a loop during a short outage.
- The pub/sub and expiry listeners reconnect by themselves when Redis comes back,
  and the "booked" cache is refilled from Postgres.
- The seat snapshot always reads booked seats from Postgres, so even if Redis lost
  everything, booked seats still show correctly.

The `redis_resilience.sh` test does exactly this: stop Redis mid-run (clean 503s,
backend still up), start it again, and holds start working again.

## Logging and tracing

I wanted to be able to follow one booking all the way through. Every request has a
trace id that shows up on each log line, and the hold and confirm steps are wrapped
in OpenTelemetry spans with the seat id, session, and result attached. With the
stdout exporter I can point at one trace and see where the time went, which is handy
when explaining a latency number from the load test.

## The data

Three tables (`backend/db/schema.sql`):

- `events` — the show(s).
- `seats` — every seat with its section, row, number and price. The frontend turns
  these into the curved seat map.
- `bookings` — one row per confirmed booking, with `UNIQUE(seat_id)`. That rule is
  the final thing stopping a double booking.

The Redis keys are simple: `hold:<seatId>` (the session holding it, with a timeout)
and `booked:<seatId>` (a cache of what's sold, refilled from Postgres). Changes go
out on the `seat_events` channel.
