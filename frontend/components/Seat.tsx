"use client";

import { memo } from "react";
import type { SeatStatus } from "@/lib/types";

interface SeatProps {
  id: number;
  x: number;
  y: number;
  status: SeatStatus; // available | held | booked
  mine: boolean; // this seat is held by *me*
  selected: boolean; // currently focused in the side panel
  onSelect: (id: number) => void;
  onHover: (id: number | null) => void;
}

const R = 8.5;

// Each state is distinguishable by color AND shape, so it reads even without
// color vision: available = solid dot, held = hollow ring, booked = dot with a
// check, mine = glowing solid dot with a bright ring.
function SeatBase({ id, x, y, status, mine, selected, onSelect, onHover }: SeatProps) {
  const clickable = status === "available" || mine;

  const handleClick = () => {
    if (clickable) onSelect(id);
  };

  let body: React.ReactNode;
  if (mine) {
    body = (
      <g className="animate-glow-mine">
        <circle cx={x} cy={y} r={R} fill="#ffd27a" />
        <circle cx={x} cy={y} r={R + 3} fill="none" stroke="#ffd27a" strokeWidth={1.6} opacity={0.8} />
      </g>
    );
  } else if (status === "held") {
    body = (
      <circle
        cx={x}
        cy={y}
        r={R - 0.5}
        fill="rgba(230,162,58,0.14)"
        stroke="#e6a23a"
        strokeWidth={2.4}
        className="animate-pulse-hold"
      />
    );
  } else if (status === "booked") {
    body = (
      <g opacity={0.85}>
        <circle cx={x} cy={y} r={R} fill="#2f434b" stroke="rgba(159,182,194,0.25)" strokeWidth={1} />
        <path
          d={`M ${x - 3.4} ${y + 0.2} L ${x - 0.8} ${y + 3} L ${x + 3.8} ${y - 3}`}
          fill="none"
          stroke="#9db6c2"
          strokeWidth={1.6}
          strokeLinecap="round"
          strokeLinejoin="round"
        />
      </g>
    );
  } else {
    body = (
      <circle
        cx={x}
        cy={y}
        r={R}
        fill="#6f909e"
        fillOpacity={0.9}
        stroke="rgba(230,240,244,0.18)"
        strokeWidth={1}
      />
    );
  }

  return (
    <g
      className={clickable ? "seat-hit" : "seat-static"}
      onClick={handleClick}
      onMouseEnter={() => onHover(id)}
      onMouseLeave={() => onHover(null)}
      role={clickable ? "button" : undefined}
      aria-label={`seat ${id} ${mine ? "your hold" : status}`}
    >
      {selected && (
        <circle cx={x} cy={y} r={R + 5} fill="none" stroke="#f3d18a" strokeWidth={1.4} opacity={0.9} />
      )}
      {body}
    </g>
  );
}

export const Seat = memo(SeatBase);
