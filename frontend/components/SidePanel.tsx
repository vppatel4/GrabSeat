"use client";

import { useEffect, useState } from "react";
import type { SeatLiveState } from "@/lib/types";
import { formatPrice, type SeatGeom } from "@/lib/layout";
import { Legend } from "./Legend";

interface Counts {
  available: number;
  held: number;
  booked: number;
  total: number;
}

interface SidePanelProps {
  seat: SeatGeom | null;
  state: SeatLiveState | undefined;
  mine: boolean;
  bookedByMe: boolean;
  holdTtlSeconds: number;
  busy: boolean;
  counts: Counts;
  onHold: () => void;
  onConfirm: () => void;
}

function Countdown({ expiresAtMs, ttlMs }: { expiresAtMs: number; ttlMs: number }) {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    const t = setInterval(() => setNow(Date.now()), 200);
    return () => clearInterval(t);
  }, []);

  const remaining = Math.max(0, expiresAtMs - now);
  const secs = Math.ceil(remaining / 1000);
  const mm = Math.floor(secs / 60);
  const ss = secs % 60;
  const pct = Math.max(0, Math.min(100, (remaining / ttlMs) * 100));
  const urgent = remaining < ttlMs * 0.25;

  return (
    <div className="mt-4">
      <div className="mb-1.5 flex items-baseline justify-between">
        <span className="text-xs uppercase tracking-wider text-steel-400">Hold expires in</span>
        <span
          className={`font-display text-2xl font-semibold tabular-nums ${
            urgent ? "animate-pulse-hold text-rose-300" : "text-brass-300"
          }`}
        >
          {mm}:{ss.toString().padStart(2, "0")}
        </span>
      </div>
      <div className="h-2 w-full overflow-hidden rounded-full bg-night-700">
        <div
          className={`h-full rounded-full transition-[width] duration-200 ease-linear ${
            urgent ? "bg-rose-400" : "bg-gradient-to-r from-brass-400 to-brass-300"
          }`}
          style={{ width: `${pct}%` }}
        />
      </div>
    </div>
  );
}

export function SidePanel({
  seat,
  state,
  mine,
  bookedByMe,
  holdTtlSeconds,
  busy,
  counts,
  onHold,
  onConfirm,
}: SidePanelProps) {
  const status = state?.status ?? "available";

  return (
    <aside className="glass flex flex-col gap-4 rounded-2xl p-5">
      {!seat ? (
        <div>
          <h2 className="font-display text-xl text-[#f4ece0]">Pick a seat</h2>
          <p className="mt-1.5 text-sm leading-relaxed text-steel-400">
            Tap any open seat to hold it for {holdTtlSeconds} seconds. Everyone
            else watching sees it turn amber the instant you grab it — first tap
            wins.
          </p>
        </div>
      ) : (
        <div>
          <div className="flex items-start justify-between">
            <div>
              <h2 className="font-display text-xl text-[#f4ece0]">
                {seat.section}
              </h2>
              <p className="text-sm text-steel-400">
                Row {seat.row} · Seat {seat.number}
              </p>
            </div>
            <span className="rounded-md bg-night-700 px-2.5 py-1 text-sm font-semibold text-brass-300">
              {formatPrice(seat.priceCents)}
            </span>
          </div>

          {bookedByMe ? (
            <div className="mt-4 rounded-lg border border-emerald-400/30 bg-emerald-400/5 p-3 text-sm text-emerald-200">
              Booked — this seat is yours. Enjoy the show.
            </div>
          ) : status === "booked" ? (
            <div className="mt-4 rounded-lg border border-steel-500/30 bg-night-700/60 p-3 text-sm text-steel-300">
              This seat was booked by someone else.
            </div>
          ) : mine ? (
            <>
              {state?.expiresAtMs && (
                <Countdown expiresAtMs={state.expiresAtMs} ttlMs={holdTtlSeconds * 1000} />
              )}
              <p className="mt-3 text-xs text-steel-400">
                Others see this seat as held right now. Confirm before the timer
                runs out or it goes back on sale.
              </p>
              <button
                onClick={onConfirm}
                disabled={busy}
                className="mt-3 w-full rounded-lg bg-gradient-to-r from-brass-400 to-brass-300 py-2.5 font-semibold text-night-900 transition hover:brightness-110 disabled:opacity-50"
              >
                {busy ? "Confirming…" : "Confirm booking"}
              </button>
            </>
          ) : status === "held" ? (
            <div className="mt-4 rounded-lg border border-brass-400/30 bg-brass-400/5 p-3 text-sm text-brass-200">
              Someone else is holding this seat. It may open back up if their hold
              expires.
            </div>
          ) : (
            <button
              onClick={onHold}
              disabled={busy}
              className="mt-4 w-full rounded-lg border border-brass-400/50 bg-brass-400/10 py-2.5 font-semibold text-brass-200 transition hover:bg-brass-400/20 disabled:opacity-50"
            >
              {busy ? "Holding…" : "Hold this seat"}
            </button>
          )}
        </div>
      )}

      <div className="border-t border-steel-500/15 pt-4">
        <Legend />
      </div>

      <div className="grid grid-cols-3 gap-2 border-t border-steel-500/15 pt-4 text-center">
        <Stat label="Open" value={counts.available} tone="text-steel-200" />
        <Stat label="Held" value={counts.held} tone="text-brass-300" />
        <Stat label="Booked" value={counts.booked} tone="text-steel-400" />
      </div>
    </aside>
  );
}

function Stat({ label, value, tone }: { label: string; value: number; tone: string }) {
  return (
    <div>
      <div className={`font-display text-2xl font-semibold tabular-nums ${tone}`}>{value}</div>
      <div className="text-[11px] uppercase tracking-wider text-steel-500">{label}</div>
    </div>
  );
}
