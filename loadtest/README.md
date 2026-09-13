# Load & correctness tests

These are the adversarial tests from the project's hardest-test-cases list. They
run against the live stack over real HTTP and WebSocket connections, so they
cover the gateway and (when scaled) multiple backend instances too.

Start the stack first:

```bash
cp .env.example .env        # Windows: copy .env.example .env
docker compose up -d --build
```

## The seven tests

| # | Test | Where | How to run |
|---|------|-------|------------|
| 1 | Concurrency stress (100+ req for 10 seats, zero double-booking) | `race_condition_test.go` → `TestConcurrencyStress` | `go test` (see below) |
| 2 | Idempotent confirm (5 rapid retries → 1 booking) | `idempotency_test.go` | `go test` |
| 3 | Expiry-boundary race (confirm right at TTL) | `expiry_test.go` | `go test` with short TTL |
| 4 | WebSocket disconnect / reconnect resync | `ws_reconnect_test.go` | `go test` |
| 5 | Rate-limit abuse isolation | `rate_limit_test.go` | `go test` |
| 6 | Redis resilience (stop Redis mid-flight, recover) | `redis_resilience.sh` | `bash loadtest/redis_resilience.sh` |
| 7 | Multi-instance pub/sub broadcast | `multi_instance_test.go` | `go test` with `--scale backend=2` |

## Running the Go tests

They're gated behind `RUN_INTEGRATION=1` so they don't run when the stack is
down. Easiest is inside a Go container on the compose network:

```bash
docker run --rm -i --network=grabseat_default \
  -e RUN_INTEGRATION=1 \
  -e BASE_URL=http://gateway:8080 \
  -e WS_URL=ws://gateway:8080/ws \
  -v "$(pwd)/loadtest:/src" -w /src \
  golang:1.23-alpine go test -v ./...
```

On the host directly (if Go is installed), point at the published port instead:

```bash
cd loadtest
RUN_INTEGRATION=1 BASE_URL=http://localhost:8080 WS_URL=ws://localhost:8080/ws go test -v ./...
```

The tests pick currently-available seats each run, so they're safe to re-run
until the venue fills. To start fresh: `docker compose down -v && docker compose up -d`.

### Test 3 (expiry) — use a short TTL

The boundary test waits for a real hold to expire. Run the stack with a short
hold window so it's quick:

```bash
HOLD_TTL_SECONDS=5 docker compose up -d
```

At the default 120s TTL the test skips itself with that advice.

### Test 7 (multi-instance) — scale first

```bash
docker compose up -d --scale backend=2
```

With one instance the test skips (there's nothing cross-instance to prove).

## Load numbers (k6)

```bash
docker run --rm -i --network host -e BASE_URL=http://localhost:8080 \
  grafana/k6 run - < loadtest/stress_test.js
```

The summary reports p50/p95 latency and the status breakdown. The `server_errors`
threshold fails the run if any request ever returns a 500.
