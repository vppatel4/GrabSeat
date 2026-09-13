# Deployment

## Local (the supported, zero-signup way)

Everything runs in Docker. You need Docker Desktop (or Docker Engine +
Compose v2) and nothing else — no accounts, no cloud services.

### First run

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

Open http://localhost:3000.

### What comes up

| Service | Container | Host port | Purpose |
|---|---|---|---|
| frontend | grabseat-frontend-1 | 3000 | Next.js seat-map UI |
| gateway | grabseat-gateway-1 | 8080 | nginx, load-balances the backend |
| backend | grabseat-backend-N | (internal) | Go API + WebSocket server |
| postgres | grabseat-postgres-1 | 5432 | durable bookings |
| redis | grabseat-redis-1 | 6379 | ephemeral holds + pub/sub |

The backend has no host port on purpose — the gateway is the single front door,
which is what lets you run more than one backend instance.

### Everyday commands

```bash
docker compose logs -f backend      # watch structured logs + traces
docker compose up -d --scale backend=2   # run two backend instances
docker compose down                 # stop everything (keeps the database)
docker compose down -v              # stop and wipe the database (full reset)
```

### Configuration

All settings come from `.env` (copied from `.env.example`). Nothing is hardcoded.
The ones you're most likely to touch:

| Variable | Default | Notes |
|---|---|---|
| `HOLD_TTL_SECONDS` | 120 | How long a hold lasts. Set to `5` to make the expiry demo quick. |
| `RATE_LIMIT_PER_MINUTE` | 60 | Per-session limit on hold/confirm. |
| `OTEL_TRACES_EXPORTER` | stdout | `stdout` prints real spans; `none` for quiet high-throughput runs. |
| `BACKEND_PORT` / `FRONTEND_PORT` | 8080 / 3000 | Change these if a port is already in use. |

If a port clashes with something already running on your machine, edit the port in
`.env` and run `docker compose up -d` again.

### Running the tests

See [../loadtest/README.md](../loadtest/README.md) for the full adversarial suite
(concurrency, idempotency, expiry boundary, reconnect, rate limiting, Redis
resilience, multi-instance broadcast) and the k6 load test.

---

## Optional: getting a live, shareable link

**This is entirely optional. It is not required for the demo, the tests, or the
screen recording** — the whole project runs and is fully testable locally with
Docker alone. This section only exists if you want a public URL to share.

Everything below has a free tier. You'd swap the local Redis and Postgres for
managed ones and point the backend and frontend at them via environment variables.

| Piece | Free-tier option | What it replaces |
|---|---|---|
| Frontend | Vercel | the `frontend` container |
| Backend | Render or Fly.io (Docker deploy) | the `backend` container |
| Redis | Upstash | the `redis` container |
| Postgres | Supabase or Neon | the `postgres` container |

Rough shape:

1. **Postgres (Supabase/Neon):** create a database, copy its connection string
   into the backend's `DATABASE_URL`. The schema and seed run automatically on
   first boot.
2. **Redis (Upstash):** create a database, set the backend's `REDIS_ADDR` and
   `REDIS_PASSWORD`. Upstash supports the keyspace-expiry notifications this
   project relies on — enable them for the database.
3. **Backend (Render/Fly.io):** deploy `backend/Dockerfile`. Set `DATABASE_URL`,
   `REDIS_ADDR`, `REDIS_PASSWORD`, `ALLOWED_ORIGIN` (your Vercel URL), and
   `BACKEND_PORT`. Expose the service publicly.
4. **Frontend (Vercel):** deploy `frontend/`. Set `NEXT_PUBLIC_API_URL` to the
   backend's public URL and `NEXT_PUBLIC_WS_URL` to its `wss://…/ws` URL. These are
   build-time values, so redeploy after changing them.

The one thing to watch in a hosted setup is that your backend host must support
long-lived WebSocket connections (Render and Fly.io both do).
