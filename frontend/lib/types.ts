export type SeatStatus = "available" | "held" | "booked";

export interface Seat {
  id: number;
  section: string;
  row_label: string;
  seat_number: number;
  price_cents: number;
  position: number;
}

export interface EventInfo {
  id: number;
  slug: string;
  name: string;
  venue: string;
  starts_at: string;
}

export interface EventResponse {
  event: EventInfo;
  seats: Seat[];
  hold_ttl_seconds: number;
  instance: string;
}

// Live state kept per seat in the browser.
export interface SeatLiveState {
  status: SeatStatus;
  heldBy?: string;
  bookedBy?: string;
  expiresAtMs?: number;
}

// Messages pushed over the WebSocket.
export interface SeatEventMsg {
  type: "held" | "booked" | "freed" | "snapshot";
  seat_id: number;
  status: SeatStatus;
  held_by?: string;
  booked_by?: string;
  expires_at_ms?: number;
  served_by?: string;
}

export interface SnapshotMsg {
  type: "snapshot";
  instance_id: string;
  hold_ttl_seconds: number;
  server_time_ms: number;
  redis_healthy: boolean;
  seats: {
    seat_id: number;
    status: SeatStatus;
    held_by?: string;
    booked_by?: string;
    expires_at_ms?: number;
  }[];
}

export interface ActionResult {
  status: string;
  seat_id?: number;
  expires_at_ms?: number;
  message?: string;
  code?: string;
  error?: string;
}
