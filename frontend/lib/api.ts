import type { ActionResult, EventResponse } from "./types";

const API_URL = process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080";

export function apiBase(): string {
  return API_URL;
}

export async function fetchEvent(): Promise<EventResponse> {
  const res = await fetch(`${API_URL}/api/event`, { cache: "no-store" });
  if (!res.ok) throw new Error(`event fetch failed: ${res.status}`);
  return res.json();
}

async function postSeat(
  path: string,
  session: string,
  seatId: number
): Promise<{ ok: boolean; status: number; body: ActionResult }> {
  const res = await fetch(`${API_URL}${path}`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      "X-Session-Id": session,
    },
    body: JSON.stringify({ seat_id: seatId }),
  });
  const body = (await res.json().catch(() => ({}))) as ActionResult;
  return { ok: res.ok, status: res.status, body };
}

export function holdSeat(session: string, seatId: number) {
  return postSeat("/api/hold", session, seatId);
}

export function confirmSeat(session: string, seatId: number) {
  return postSeat("/api/confirm", session, seatId);
}
