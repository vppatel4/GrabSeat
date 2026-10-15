// k6 test that proves the core guarantee under concurrent load: no seat is ever
// sold twice. Unlike stress_test.js (which measures raw hold throughput), this
// drives the *full booking flow* — many virtual users stampede a small pool of
// seats trying to hold AND confirm them at the same time — then verifies against
// the server's durable source of truth that zero double-bookings occurred.
//
// How the proof works: the backend exposes /api/stats with the total number of
// booking rows and the number of distinct seats booked. The UNIQUE(seat_id)
// constraint means those two numbers must be equal; if a double-booking ever
// slipped through, total would exceed distinct. teardown() asserts they match and
// fails the test otherwise.
//
// Run (stack must be up):
//   docker run --rm -i --network=grabseat_default -e BASE_URL=http://gateway:8080 \
//     grafana/k6 run - < loadtest/k6_double_booking.js

import http from "k6/http";
import { check } from "k6";
import { Counter } from "k6/metrics";

const BASE = __ENV.BASE_URL || "http://localhost:8080";
const POOL = parseInt(__ENV.POOL || "25", 10); // seats everyone fights over
const VUS = parseInt(__ENV.VUS || "50", 10);
const DURATION = __ENV.DURATION || "20s";

http.setResponseCallback(http.expectedStatuses(200, 409, 429));

const confirmed = new Counter("bookings_confirmed");
const conflicts = new Counter("confirm_conflicts");
const holdRejected = new Counter("holds_rejected");
const serverErrors = new Counter("server_errors");

export const options = {
  scenarios: {
    stampede: {
      executor: "ramping-vus",
      startVUs: 0,
      stages: [
        { duration: "3s", target: VUS },
        { duration: DURATION, target: VUS },
        { duration: "3s", target: 0 },
      ],
    },
  },
  thresholds: {
    server_errors: ["count==0"],
  },
};

// Pick a pool of currently-available seats and record the booking count before
// we start, so the check is valid even if earlier runs already booked some seats.
export function setup() {
  const ev = http.get(`${BASE}/api/event`).json();
  const st = http.get(`${BASE}/api/state`).json();

  const taken = {};
  for (const s of st.seats || []) {
    if (s.status === "held" || s.status === "booked") taken[s.seat_id] = true;
  }
  const pool = [];
  for (const s of ev.seats) {
    if (!taken[s.id]) {
      pool.push(s.id);
      if (pool.length >= POOL) break;
    }
  }
  const stats = http.get(`${BASE}/api/stats`).json();
  console.log(`[SETUP] pool=${pool.length} seats, baseline bookings=${stats.bookings_total}`);
  return { pool, baselineTotal: stats.bookings_total };
}

export default function (data) {
  const seat = data.pool[Math.floor(Math.random() * data.pool.length)];
  const session = `k6dbl-${__VU}-${__ITER}-${Math.random().toString(36).slice(2)}`;
  const headers = { "Content-Type": "application/json", "X-Session-Id": session };

  const h = http.post(`${BASE}/api/hold`, JSON.stringify({ seat_id: seat }), { headers });
  check(h, { "hold: no server error": (r) => r.status < 500 });
  if (h.status >= 500) {
    serverErrors.add(1);
    return;
  }

  let heldOk = false;
  try {
    heldOk = h.status === 200 && h.json().status === "held";
  } catch (e) {
    heldOk = false;
  }

  if (!heldOk) {
    holdRejected.add(1); // someone else already holds/booked it — real contention
    return;
  }

  // We won the hold; now try to turn it into a permanent booking.
  const c = http.post(`${BASE}/api/confirm`, JSON.stringify({ seat_id: seat }), { headers });
  check(c, { "confirm: no server error": (r) => r.status < 500 });
  if (c.status >= 500) {
    serverErrors.add(1);
    return;
  }
  let cs = "";
  try {
    cs = c.json().status;
  } catch (e) {
    cs = "";
  }
  if (cs === "confirmed" || cs === "already_mine") confirmed.add(1);
  else if (cs === "conflict") conflicts.add(1);
}

// The authoritative check, read straight from Postgres via /api/stats.
export function teardown(data) {
  const stats = http.get(`${BASE}/api/stats`).json();
  const newlyBooked = stats.bookings_total - data.baselineTotal;
  console.log(
    `[VERIFY] bookings_total=${stats.bookings_total} distinct_seats=${stats.distinct_seats} ` +
      `double_bookings=${stats.double_bookings} newly_booked_this_run=${newlyBooked} pool=${data.pool.length}`
  );

  if (stats.double_bookings !== 0) {
    throw new Error(`DOUBLE BOOKING DETECTED: ${stats.double_bookings} (total != distinct seats)`);
  }
  if (newlyBooked > data.pool.length) {
    throw new Error(`booked more seats (${newlyBooked}) than the pool size (${data.pool.length})`);
  }
  console.log("[VERIFY] PASS — zero double bookings across the run");
}
