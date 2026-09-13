"use client";

// Tiny SVG swatches that match the real seat shapes, so the legend teaches the
// shape language (dot / ring / check / glow), not just the colors.
function Swatch({ kind }: { kind: "available" | "held" | "mine" | "booked" }) {
  return (
    <svg width={20} height={20} viewBox="0 0 20 20" aria-hidden>
      {kind === "available" && <circle cx={10} cy={10} r={7} fill="#6f909e" fillOpacity={0.9} />}
      {kind === "held" && (
        <circle cx={10} cy={10} r={6.5} fill="rgba(230,162,58,0.14)" stroke="#e6a23a" strokeWidth={2.4} />
      )}
      {kind === "mine" && (
        <>
          <circle cx={10} cy={10} r={7} fill="#ffd27a" />
          <circle cx={10} cy={10} r={9} fill="none" stroke="#ffd27a" strokeWidth={1.4} opacity={0.8} />
        </>
      )}
      {kind === "booked" && (
        <>
          <circle cx={10} cy={10} r={7} fill="#2f434b" stroke="rgba(159,182,194,0.25)" />
          <path
            d="M 6.6 10.2 L 9.2 13 L 13.8 7"
            fill="none"
            stroke="#9db6c2"
            strokeWidth={1.6}
            strokeLinecap="round"
            strokeLinejoin="round"
          />
        </>
      )}
    </svg>
  );
}

const items: { kind: "available" | "held" | "mine" | "booked"; label: string }[] = [
  { kind: "available", label: "Open" },
  { kind: "mine", label: "Your hold" },
  { kind: "held", label: "Held by others" },
  { kind: "booked", label: "Booked" },
];

export function Legend() {
  return (
    <div className="grid grid-cols-2 gap-x-4 gap-y-2 text-xs text-steel-300">
      {items.map((it) => (
        <div key={it.kind} className="flex items-center gap-2">
          <Swatch kind={it.kind} />
          <span>{it.label}</span>
        </div>
      ))}
    </div>
  );
}
