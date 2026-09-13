"use client";

import type { EventInfo } from "@/lib/types";
import { shortSession } from "@/lib/session";

interface HeaderProps {
  event: EventInfo | null;
  session: string;
  connected: boolean;
  instanceId: string;
}

export function Header({ event, session, connected, instanceId }: HeaderProps) {
  const dateLabel = event
    ? new Date(event.starts_at).toLocaleDateString(undefined, {
        weekday: "short",
        month: "long",
        day: "numeric",
        year: "numeric",
      })
    : "";

  return (
    <header className="mb-5 flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
      <div>
        <p className="mb-1 text-xs uppercase tracking-[0.35em] text-brass-500">
          Now on sale
        </p>
        <h1 className="font-display text-4xl font-semibold leading-none text-[#f4ece0] sm:text-5xl">
          {event?.name ?? "Loading…"}
        </h1>
        <p className="mt-2 text-sm text-steel-400">
          {event?.venue}
          {dateLabel && <span className="text-steel-500"> · {dateLabel}</span>}
        </p>
      </div>

      <div className="flex items-center gap-2.5">
        <span className="glass rounded-full px-3 py-1.5 text-xs text-steel-300">
          You are <span className="font-semibold text-brass-300">#{shortSession(session)}</span>
        </span>
        <span
          className="glass flex items-center gap-2 rounded-full px-3 py-1.5 text-xs text-steel-300"
          title={instanceId ? `served by instance ${instanceId}` : undefined}
        >
          <span
            className={`inline-block h-2 w-2 rounded-full ${
              connected ? "bg-emerald-400" : "bg-brass-500"
            }`}
            style={connected ? { boxShadow: "0 0 8px rgba(52,211,153,0.9)" } : undefined}
          />
          {connected ? "Live" : "Reconnecting…"}
        </span>
      </div>
    </header>
  );
}
