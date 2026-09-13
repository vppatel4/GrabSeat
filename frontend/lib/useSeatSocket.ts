"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import type { SeatLiveState, SeatEventMsg, SnapshotMsg } from "./types";

const WS_URL = process.env.NEXT_PUBLIC_WS_URL || "ws://localhost:8080/ws";

export interface SocketState {
  // Only non-available seats are stored; a seat absent from the map is available.
  states: Map<number, SeatLiveState>;
  connected: boolean;
  redisHealthy: boolean;
  instanceId: string;
}

// useSeatSocket owns the live connection. It rebuilds the whole seat map from a
// snapshot on every (re)connect, so a dropped-and-restored connection always
// resyncs to the truth rather than trusting stale local state. Live deltas are
// applied idempotently on top.
export function useSeatSocket(): SocketState {
  const [states, setStates] = useState<Map<number, SeatLiveState>>(new Map());
  const [connected, setConnected] = useState(false);
  const [redisHealthy, setRedisHealthy] = useState(true);
  const [instanceId, setInstanceId] = useState("");

  const wsRef = useRef<WebSocket | null>(null);
  const retryRef = useRef(0);
  const closedRef = useRef(false);

  const applySnapshot = useCallback((msg: SnapshotMsg) => {
    const next = new Map<number, SeatLiveState>();
    for (const s of msg.seats) {
      if (s.status === "available") continue;
      next.set(s.seat_id, {
        status: s.status,
        heldBy: s.held_by,
        bookedBy: s.booked_by,
        expiresAtMs: s.expires_at_ms,
      });
    }
    setStates(next);
    setRedisHealthy(msg.redis_healthy);
    setInstanceId(msg.instance_id);
  }, []);

  const applyDelta = useCallback((msg: SeatEventMsg) => {
    setStates((prev) => {
      const next = new Map(prev);
      if (msg.type === "freed") {
        next.delete(msg.seat_id);
      } else if (msg.type === "held") {
        next.set(msg.seat_id, {
          status: "held",
          heldBy: msg.held_by,
          expiresAtMs: msg.expires_at_ms,
        });
      } else if (msg.type === "booked") {
        next.set(msg.seat_id, { status: "booked", bookedBy: msg.booked_by });
      }
      return next;
    });
  }, []);

  useEffect(() => {
    closedRef.current = false;

    const connect = () => {
      if (closedRef.current) return;
      const ws = new WebSocket(WS_URL);
      wsRef.current = ws;

      ws.onopen = () => {
        retryRef.current = 0;
        setConnected(true);
      };
      ws.onmessage = (ev) => {
        try {
          const msg = JSON.parse(ev.data);
          if (msg.type === "snapshot") applySnapshot(msg as SnapshotMsg);
          else applyDelta(msg as SeatEventMsg);
        } catch {
          /* ignore malformed frames */
        }
      };
      ws.onclose = () => {
        setConnected(false);
        if (closedRef.current) return;
        // Exponential-ish backoff, capped, so a downed backend doesn't spin us.
        const delay = Math.min(1000 * 2 ** retryRef.current, 8000);
        retryRef.current += 1;
        setTimeout(connect, delay);
      };
      ws.onerror = () => ws.close();
    };

    connect();
    return () => {
      closedRef.current = true;
      wsRef.current?.close();
    };
  }, [applySnapshot, applyDelta]);

  return { states, connected, redisHealthy, instanceId };
}
