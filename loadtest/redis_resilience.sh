#!/usr/bin/env bash
# Redis resilience test (section 5, #6).
#
# Stops Redis mid-flight and checks the backend degrades cleanly instead of
# crashing: hold requests return a clear 503 while Redis is down, /healthz keeps
# answering 200 (the process never dies), and once Redis is back the system
# recovers on its own and holds succeed again.
#
# Run from the repo root with the stack already up:
#   docker compose up -d --build
#   bash loadtest/redis_resilience.sh
set -u

BASE="${BASE_URL:-http://localhost:8080}"
SESSION="resilience-probe"
SEAT=1
PASS=0
FAIL=0

say()  { printf '\n=== %s ===\n' "$1"; }
ok()   { printf '  PASS: %s\n' "$1"; PASS=$((PASS+1)); }
bad()  { printf '  FAIL: %s\n' "$1"; FAIL=$((FAIL+1)); }

hold_code() {
  curl -s -o /dev/null -w '%{http_code}' -X POST "$BASE/api/hold" \
    -H 'Content-Type: application/json' \
    -H "X-Session-Id: $SESSION" \
    -d "{\"seat_id\": $SEAT}"
}

health_code() {
  curl -s -o /dev/null -w '%{http_code}' "$BASE/healthz"
}

say "Baseline (Redis up)"
code=$(hold_code)
# 200 (held) or 409 (already taken) are both fine — anything but 503 means Redis
# is doing its job.
if [ "$code" != "503" ]; then ok "hold returned $code (Redis healthy)"; else bad "hold returned 503 while Redis should be up"; fi

say "Stopping Redis"
docker compose stop redis >/dev/null 2>&1
sleep 3

code=$(health_code)
if [ "$code" = "200" ]; then ok "/healthz still 200 — backend did not crash"; else bad "/healthz returned $code — backend may have crashed"; fi

code=$(hold_code)
if [ "$code" = "503" ]; then ok "hold returned a clean 503 while Redis is down"; else bad "expected 503 with Redis down, got $code"; fi

say "Restarting Redis"
docker compose start redis >/dev/null 2>&1

# Give it a few seconds and retry until it recovers.
recovered=0
for i in $(seq 1 15); do
  sleep 2
  code=$(hold_code)
  if [ "$code" != "503" ]; then recovered=1; break; fi
done
if [ "$recovered" = "1" ]; then ok "hold recovered (returned $code) after Redis came back"; else bad "hold still failing after Redis restart"; fi

say "Summary"
printf '  %d passed, %d failed\n' "$PASS" "$FAIL"
[ "$FAIL" -eq 0 ]
