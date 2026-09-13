import type { Seat } from "./types";

// Turns the flat seat list into amphitheater geometry: concentric arcs curving
// up toward the stage. Sections sit at growing radii (Pit closest, Balcony
// furthest), and seats within a row are spaced by constant arc length so the
// house fans out naturally instead of looking like a grid.

export interface SeatGeom {
  id: number;
  x: number;
  y: number;
  section: string;
  row: string;
  number: number;
  priceCents: number;
}

export interface SectionLabel {
  name: string;
  x: number;
  y: number;
}

export interface VenueLayout {
  seats: SeatGeom[];
  viewBox: string;
  stagePath: string;
  stageLabel: { x: number; y: number };
  sectionLabels: SectionLabel[];
}

const R0 = 205; // radius of the first (front) row
const ROW_GAP = 26; // radial gap between rows
const SECTION_GAP = 22; // extra radial gap between sections
const SEAT_ARC = 23; // arc-length spacing between adjacent seats

export function buildLayout(seats: Seat[]): VenueLayout {
  // Group into sections (in encounter order) → rows (in encounter order).
  const sectionOrder: string[] = [];
  const sections = new Map<string, Map<string, Seat[]>>();
  for (const s of seats) {
    if (!sections.has(s.section)) {
      sections.set(s.section, new Map());
      sectionOrder.push(s.section);
    }
    const rows = sections.get(s.section)!;
    if (!rows.has(s.row_label)) rows.set(s.row_label, []);
    rows.get(s.row_label)!.push(s);
  }

  const geom: SeatGeom[] = [];
  const sectionLabels: SectionLabel[] = [];
  let radius = R0;
  let minX = Infinity,
    maxX = -Infinity,
    minY = Infinity,
    maxY = -Infinity;

  for (let si = 0; si < sectionOrder.length; si++) {
    const name = sectionOrder[si];
    const rows = sections.get(name)!;
    if (si > 0) radius += SECTION_GAP;

    const rowLabels = [...rows.keys()];
    const sectionInnerRadius = radius;
    let sectionMaxHalfAngle = 0;

    for (const rl of rowLabels) {
      const rowSeats = [...rows.get(rl)!].sort(
        (a, b) => a.seat_number - b.seat_number
      );
      const n = rowSeats.length;
      const step = SEAT_ARC / radius; // radians between seats
      const halfWidth = (step * (n - 1)) / 2;
      sectionMaxHalfAngle = Math.max(sectionMaxHalfAngle, halfWidth);

      rowSeats.forEach((s, i) => {
        const a = -halfWidth + i * step;
        const x = radius * Math.sin(a);
        const y = radius * Math.cos(a);
        geom.push({
          id: s.id,
          x,
          y,
          section: name,
          row: s.row_label,
          number: s.seat_number,
          priceCents: s.price_cents,
        });
        if (x < minX) minX = x;
        if (x > maxX) maxX = x;
        if (y < minY) minY = y;
        if (y > maxY) maxY = y;
      });
      radius += ROW_GAP;
    }

    // Section label just outside the fan on the right, at the section's mid depth.
    const midRadius = (sectionInnerRadius + (radius - ROW_GAP)) / 2;
    const la = sectionMaxHalfAngle + 0.11;
    sectionLabels.push({
      name,
      x: midRadius * Math.sin(la) + 14,
      y: midRadius * Math.cos(la),
    });
  }

  // Stage: a bright curved band just above the front row.
  const rs = R0 - 70;
  const stageHalf = 0.52;
  const sx0 = rs * Math.sin(-stageHalf);
  const sy0 = rs * Math.cos(-stageHalf);
  const sx1 = rs * Math.sin(stageHalf);
  const sy1 = rs * Math.cos(stageHalf);
  const stagePath = `M ${sx0.toFixed(1)} ${sy0.toFixed(1)} A ${rs} ${rs} 0 0 1 ${sx1.toFixed(
    1
  )} ${sy1.toFixed(1)}`;
  const stageLabel = { x: 0, y: rs * Math.cos(stageHalf) - 26 };

  minY = Math.min(minY, stageLabel.y - 20);
  for (const l of sectionLabels) {
    maxX = Math.max(maxX, l.x + 70);
    minX = Math.min(minX, l.x);
  }

  const pad = 42;
  const vbX = minX - pad;
  const vbY = minY - pad;
  const vbW = maxX - minX + pad * 2;
  const vbH = maxY - minY + pad * 2;

  return {
    seats: geom,
    viewBox: `${vbX.toFixed(1)} ${vbY.toFixed(1)} ${vbW.toFixed(1)} ${vbH.toFixed(1)}`,
    stagePath,
    stageLabel,
    sectionLabels,
  };
}

export function formatPrice(cents: number): string {
  return "$" + (cents / 100).toFixed(0);
}
