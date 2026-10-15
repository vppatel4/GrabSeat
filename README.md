# GrabSeat

[![CI](https://github.com/vppatel4/GrabSeat/actions/workflows/ci.yml/badge.svg)](https://github.com/vppatel4/GrabSeat/actions/workflows/ci.yml)

GrabSeat is a small seat-booking app I built to work through one tricky problem:
what happens when a lot of people try to grab the same seat at the same time.

When a show goes on sale, two people can click the same seat in the same second.
The app has to make sure only one of them actually gets it, tell the other one
right away that it's taken, and never sell the same seat twice. On top of that,
everyone looking at the seat map should see it update live.

There's no login. When you open the app you get a random id for that browser tab,
and that's enough to pick seats. If you open a second tab, that's a second
"person", so you can race yourself for the same seat and watch one tab win. You
only need Docker to run it.

![GrabSeat seat map](docs/media/preview.svg)

This is the seat map. Grey seats are open, gold is the seat you're holding, amber
means someone else is holding it, and a check mark means it's already booked.

## How it avoids booking the same seat twice

There are three seat states: open, held, and booked. The rule is simple: one seat
can be booked by one person only. I use two databases for two different jobs to
make that happen.

**Redis** keeps track of who is currently holding a seat. Holding a seat is one
Redis command that only works if the seat isn't already held, and the hold deletes
itself after a few minutes. So if two people go for the same seat at once, only one
of them gets it, and a hold that nobody finishes just disappears on its own.

**Postgres** stores the bookings that are final. The bookings table has a rule that
there can only be one booking per seat, so the database itself won't allow the same
seat to be booked twice even if something went wrong in the code.

## Architecture diagram

![Architecture](docs/media/architecture.svg)

The browser talks to an nginx gateway, and the gateway passes the request to a Go
backend. The backend uses Redis to hold seats and Postgres to save bookings. When a
seat changes, the backend sends the update back out through Redis, so every open
seat map updates live, even if more than one backend is running.

## Tech stack

| Part | Tech |
|---|---|
| Backend | Go (HTTP + WebSocket server) |
| Seat holds + live updates | Redis |
| Database | PostgreSQL |
| Frontend | Next.js, TypeScript, Tailwind CSS |
| Gateway | nginx |
| Running it | Docker + Docker Compose |

## How it works

1. You open the app. Your tab gets a random id and connects over a WebSocket, and
   the server sends back the current state of every seat.
2. You click an open seat. The backend tries to hold it in Redis. If you get it,
   it's yours for a few minutes. If someone beat you to it, you're told right away.
3. Every open seat map updates to show that seat as held.
4. You click confirm. The backend saves the booking in Postgres. If the confirm
   gets sent twice (a double click or a retry), it still only books once.
5. If you don't confirm in time, the hold expires on its own and the seat goes back
   to open for everyone.

## Running it

You need Docker Desktop. Nothing else, no accounts.

**Windows (PowerShell):**

```powershell
copy .env.example .env
docker compose up -d --build
docker compose ps
```

**macOS / Linux:**

```bash
cp .env.example .env
docker compose up -d --build
docker compose ps
```

Then open **http://localhost:3000**.

| Service | URL / port |
|---|---|
| Frontend (seat map) | http://localhost:3000 |
| Backend (API + WebSocket) | http://localhost:8080 |
| Postgres | localhost:5432 |
| Redis | localhost:6379 |

Stop it: `docker compose down`
Reset it (wipe the database and all bookings): `docker compose down -v`

If a port is already being used on your machine, change it in `.env` (like
`BACKEND_PORT` or `FRONTEND_PORT`) and start it again.

## Try it

1. Open **http://localhost:3000** in two browser tabs side by side.
2. Click the same open seat in both tabs at about the same time.
3. One tab turns the seat gold (your hold). The other gets a message that someone
   grabbed it first and shows the seat as held.
4. In the tab that won, click **Confirm booking**. The seat shows as booked in both
   tabs.
5. Or just wait: let the hold timer run out and watch the seat go back to open in
   both tabs.
