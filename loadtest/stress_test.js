// k6 load test — produces the real throughput and latency numbers reported in
// the README. It simulates a crowd of distinct users all trying to grab seats
// at once, then reports p50/p95 latency and the status-code breakdown.
//
// Every iteration uses a fresh session id (a distinct "user"), so per-session
// rate limiting doesn't distort the capacity measurement — this measures how the
// system holds up under load, not how it throttles one abuser (that's covered by
// TestRateLimitIsolation and the rate-limit test).
//
// Run (stack must be up):
//   docker compose up -d --build
//   docker run --rm -i --network host -e BASE_URL=http://localhost:8080 \
//     grafana/k6 run - < loadtest/stress_test.js
//
// Tune with env vars: VUS (default 50), DURATION (default 30s), SEAT_MAX (320).

import http from 'k6/http';
import { check } from 'k6';
import { Counter } from 'k6/metrics';

const BASE = __ENV.BASE_URL || 'http://localhost:8080';
const SEAT_MAX = parseInt(__ENV.SEAT_MAX || '320', 10);
const VUS = parseInt(__ENV.VUS || '50', 10);
const DURATION = __ENV.DURATION || '30s';

// 409 (seat taken/sold) and 429 (rate limited) are correct, expected outcomes,
// not failures — tell k6 so http_req_failed only counts genuine errors (5xx).
http.setResponseCallback(http.expectedStatuses(200, 409, 429));

const held = new Counter('seats_held');
const taken = new Counter('seats_taken');
const booked_full = new Counter('seats_already_booked');
const rate_limited = new Counter('rate_limited');
const server_errors = new Counter('server_errors');

export const options = {
  scenarios: {
    grab: {
      executor: 'ramping-vus',
      startVUs: 0,
      stages: [
        { duration: '5s', target: VUS },
        { duration: DURATION, target: VUS },
        { duration: '5s', target: 0 },
      ],
    },
  },
  thresholds: {
    // The whole point: no request should ever 500. A booked/held/taken/limited
    // response is a clean outcome; a 500 is a bug.
    server_errors: ['count==0'],
    http_req_duration: ['p(95)<800'],
  },
};

function sessionId() {
  return `k6-${__VU}-${__ITER}-${Math.random().toString(36).slice(2)}`;
}

export default function () {
  const seat = 1 + Math.floor(Math.random() * SEAT_MAX);
  const res = http.post(`${BASE}/api/hold`, JSON.stringify({ seat_id: seat }), {
    headers: { 'Content-Type': 'application/json', 'X-Session-Id': sessionId() },
  });

  check(res, { 'no server error': (r) => r.status < 500 });

  if (res.status >= 500) server_errors.add(1);
  else if (res.status === 429) rate_limited.add(1);
  else if (res.status === 200) held.add(1);
  else if (res.status === 409) {
    // Distinguish "someone else holds it" from "already sold" by the body.
    try {
      const b = res.json();
      if (b.status === 'seat_booked') booked_full.add(1);
      else taken.add(1);
    } catch (e) {
      taken.add(1);
    }
  }
}
