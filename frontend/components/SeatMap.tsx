"use client";

import { useMemo, useState } from "react";
import type { SeatLiveState } from "@/lib/types";
import { buildLayout, formatPrice, type SeatGeom } from "@/lib/layout";
import type { Seat as SeatType } from "@/lib/types";
import { Seat } from "./Seat";

interface SeatMapProps {
  seats: SeatType[];
  states: Map<number, SeatLiveState>;
  session: string;
  selectedId: number | null;
  onSelect: (id: number) => void;
}

export function SeatMap({ seats, states, session, selectedId, onSelect }: SeatMapProps) {
  const layout = useMemo(() => buildLayout(seats), [seats]);
  const [hoveredId, setHoveredId] = useState<number | null>(null);

  const geomById = useMemo(() => {
    const m = new Map<number, SeatGeom>();
    for (const g of layout.seats) m.set(g.id, g);
    return m;
  }, [layout]);

  const hovered = hoveredId != null ? geomById.get(hoveredId) : undefined;

  return (
    <div className="seatmap-wrap w-full">
      <svg
        viewBox={layout.viewBox}
        preserveAspectRatio="xMidYMid meet"
        className="mx-auto block w-full"
        style={{ maxHeight: "68vh", minWidth: 520 }}
      >
        <defs>
          <linearGradient id="stageGrad" x1="0" y1="0" x2="0" y2="1">
            <stop offset="0%" stopColor="#ffe6ad" />
            <stop offset="100%" stopColor="#d19a3f" />
          </linearGradient>
          <filter id="stageGlow" x="-40%" y="-60%" width="180%" height="260%">
            <feGaussianBlur stdDeviation="6" result="b" />
            <feMerge>
              <feMergeNode in="b" />
              <feMergeNode in="SourceGraphic" />
            </feMerge>
          </filter>
        </defs>

        {/* Stage: a glowing curved band the whole house faces. */}
        <path d={layout.stagePath} fill="none" stroke="url(#stageGrad)" strokeWidth={9} strokeLinecap="round" filter="url(#stageGlow)" />
        <text
          x={layout.stageLabel.x}
          y={layout.stageLabel.y}
          textAnchor="middle"
          className="font-sans"
          fill="#f3d18a"
          fontSize={13}
          letterSpacing={6}
          style={{ textTransform: "uppercase" }}
        >
          Stage
        </text>

        {/* Section labels following the fan on the right edge. */}
        {layout.sectionLabels.map((l) => (
          <text
            key={l.name}
            x={l.x}
            y={l.y}
            fill="rgba(243,209,138,0.75)"
            fontSize={12}
            letterSpacing={1.5}
            dominantBaseline="middle"
            style={{ textTransform: "uppercase" }}
          >
            {l.name}
          </text>
        ))}

        {/* Seats */}
        {layout.seats.map((g) => {
          const st = states.get(g.id);
          const status = st?.status ?? "available";
          const mine = status === "held" && st?.heldBy === session;
          return (
            <Seat
              key={g.id}
              id={g.id}
              x={g.x}
              y={g.y}
              status={status}
              mine={mine}
              selected={selectedId === g.id}
              onSelect={onSelect}
              onHover={setHoveredId}
            />
          );
        })}

        {/* Hover tooltip, drawn in SVG space so it scales with the map. */}
        {hovered && (
          <g pointerEvents="none">
            <rect
              x={hovered.x - 52}
              y={hovered.y - 40}
              width={104}
              height={26}
              rx={6}
              fill="rgba(8,13,15,0.92)"
              stroke="rgba(230,180,92,0.4)"
              strokeWidth={1}
            />
            <text x={hovered.x} y={hovered.y - 27} textAnchor="middle" fill="#e8eef0" fontSize={10.5}>
              {hovered.section} · {hovered.row}
              {hovered.number}
            </text>
            <text x={hovered.x} y={hovered.y - 17} textAnchor="middle" fill="#f3d18a" fontSize={10}>
              {formatPrice(hovered.priceCents)}
            </text>
          </g>
        )}
      </svg>
    </div>
  );
}
