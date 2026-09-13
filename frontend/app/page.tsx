"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { fetchEvent, holdSeat, confirmSeat } from "@/lib/api";
import { getSessionId } from "@/lib/session";
import { useSeatSocket } from "@/lib/useSeatSocket";
import type { EventResponse } from "@/lib/types";
import type { SeatGeom } from "@/lib/layout";
import { SeatMap } from "@/components/SeatMap";
import { SidePanel } from "@/components/SidePanel";
import { Header } from "@/components/Header";
import { Toasts, type Toast, type ToastKind } from "@/components/Toasts";

export default function Page() {
  const [event, setEvent] = useState<EventResponse | null>(null);
  const [session, setSession] = useState("");
  const [selectedId, setSelectedId] = useState<number | null>(null);
  const [busy, setBusy] = useState(false);
  const [toasts, setToasts] = useState<Toast[]>([]);
  const toastSeq = useRef(0);

  const { states, connected, redisHealthy, instanceId } = useSeatSocket();

  const pushToast = useCallback((kind: ToastKind, text: string) => {
    const id = ++toastSeq.current;
    setToasts((t) => [...t, { id, kind, text }]);
    setTimeout(() => setToasts((t) => t.filter((x) => x.id !== id)), 3600);
  }, []);

  useEffect(() => {
    setSession(getSessionId());
    fetchEvent()
      .then(setEvent)
      .catch(() => pushToast("error", "Couldn't reach the box office. Is the backend up?"));
  }, [pushToast]);

  const seatById = useMemo(() => {
    const m = new Map<number, SeatGeom>();
    if (event) {
      for (const s of event.seats) {
        m.set(s.id, {
          id: s.id,
          x: 0,
          y: 0,
          section: s.section,
          row: s.row_label,
          number: s.seat_number,
          priceCents: s.price_cents,
        });
      }
    }
    return m;
  }, [event]);

  const selectedSeat = selectedId != null ? seatById.get(selectedId) ?? null : null;
  const selectedState = selectedId != null ? states.get(selectedId) : undefined;
  const mine = selectedState?.status === "held" && selectedState.heldBy === session;
  const bookedByMe = selectedState?.status === "booked" && selectedState.bookedBy === session;

  // Notice when a seat we were holding slips away (hold expired), and say so.
  const wasMine = useRef(false);
  useEffect(() => {
    if (wasMine.current && !mine && !bookedByMe && selectedState?.status !== "booked") {
      pushToast("info", "Your hold expired — the seat is open again.");
    }
    wasMine.current = mine;
  }, [mine, bookedByMe, selectedState, pushToast]);

  const counts = useMemo(() => {
    const total = event?.seats.length ?? 0;
    let held = 0,
      booked = 0;
    for (const s of states.values()) {
      if (s.status === "held") held++;
      else if (s.status === "booked") booked++;
    }
    return { total, held, booked, available: Math.max(0, total - held - booked) };
  }, [event, states]);

  const doHold = useCallback(
    async (seatId: number) => {
      if (!session) return;
      setBusy(true);
      try {
        const { ok, status, body } = await holdSeat(session, seatId);
        if (ok && body.status === "held") {
          pushToast("info", "Seat held — confirm before the timer runs out.");
        } else if (body.status === "taken") {
          pushToast("error", "Someone grabbed that seat a split second first.");
        } else if (body.status === "seat_booked") {
          pushToast("error", "That seat is already sold.");
        } else if (status === 429) {
          pushToast("error", "Slow down a moment — too many taps.");
        } else {
          pushToast("error", "Couldn't hold that seat. Try again.");
        }
      } catch {
        pushToast("error", "Network hiccup holding the seat.");
      } finally {
        setBusy(false);
      }
    },
    [session, pushToast]
  );

  const onSelect = useCallback(
    (seatId: number) => {
      setSelectedId(seatId);
      const st = states.get(seatId);
      const isMine = st?.status === "held" && st.heldBy === session;
      // Clicking an open seat is the "grab" — hold it immediately.
      if (!st && !isMine) doHold(seatId);
    },
    [states, session, doHold]
  );

  const doConfirm = useCallback(async () => {
    if (selectedId == null || !session) return;
    setBusy(true);
    try {
      const { ok, status, body } = await confirmSeat(session, selectedId);
      if (ok && (body.status === "confirmed" || body.status === "already_mine")) {
        pushToast("success", "Booked! That seat is yours.");
      } else if (body.status === "conflict") {
        pushToast("error", "That seat belongs to someone else now.");
      } else if (body.status === "hold_expired") {
        pushToast("error", "Too late — your hold had expired.");
      } else if (status === 429) {
        pushToast("error", "Slow down a moment — too many taps.");
      } else {
        pushToast("error", "Couldn't confirm. Try again.");
      }
    } catch {
      pushToast("error", "Network hiccup confirming the seat.");
    } finally {
      setBusy(false);
    }
  }, [selectedId, session, pushToast]);

  return (
    <main className="mx-auto max-w-6xl px-4 py-6 sm:py-9">
      <Header event={event?.event ?? null} session={session} connected={connected} instanceId={instanceId} />

      {!redisHealthy && (
        <div className="mb-4 rounded-lg border border-rose-400/40 bg-rose-500/10 px-4 py-2.5 text-sm text-rose-200">
          Live holds are paused — the lock service is briefly unavailable. Booked
          seats are still shown correctly, and holds resume automatically once
          it&apos;s back.
        </div>
      )}

      <div className="grid gap-5 lg:grid-cols-[1fr_340px]">
        <section className="glass rounded-2xl p-3 sm:p-5">
          {event ? (
            <SeatMap
              seats={event.seats}
              states={states}
              session={session}
              selectedId={selectedId}
              onSelect={onSelect}
            />
          ) : (
            <div className="flex h-[60vh] items-center justify-center text-steel-500">
              Loading the house…
            </div>
          )}
        </section>

        <SidePanel
          seat={selectedSeat}
          state={selectedState}
          mine={mine}
          bookedByMe={bookedByMe}
          holdTtlSeconds={event?.hold_ttl_seconds ?? 120}
          busy={busy}
          counts={counts}
          onHold={() => selectedId != null && doHold(selectedId)}
          onConfirm={doConfirm}
        />
      </div>

      <footer className="mt-8 text-center text-xs text-steel-500">
        Open a second browser tab and race yourself for the same seat — exactly
        one tab wins, every time.
      </footer>

      <Toasts toasts={toasts} />
    </main>
  );
}
