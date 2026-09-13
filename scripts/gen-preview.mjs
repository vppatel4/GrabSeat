// Generates docs/media/preview.svg — a static poster of the seat map, drawn with
// the same amphitheater geometry the app uses, so the README shows the real
// design before a screen recording is added. Run: node scripts/gen-preview.mjs
import { writeFileSync, mkdirSync } from "node:fs";

const venue = [
  { name: "Pit", rows: ["A", "B"], seatsPer: 12 },
  { name: "Orchestra", rows: ["A", "B", "C", "D", "E", "F"], seatsPer: 18 },
  { name: "Mezzanine", rows: ["A", "B", "C", "D", "E"], seatsPer: 20 },
  { name: "Balcony", rows: ["A", "B", "C", "D"], seatsPer: 22 },
];

const R0 = 205, ROW_GAP = 26, SECTION_GAP = 22, SEAT_ARC = 23;

let id = 0;
const seats = [];
let radius = R0;
const sectionLabels = [];
for (let si = 0; si < venue.length; si++) {
  const sec = venue[si];
  if (si > 0) radius += SECTION_GAP;
  const inner = radius;
  let maxHalf = 0;
  for (const row of sec.rows) {
    const n = sec.seatsPer;
    const step = SEAT_ARC / radius;
    const half = (step * (n - 1)) / 2;
    maxHalf = Math.max(maxHalf, half);
    for (let i = 0; i < n; i++) {
      const a = -half + i * step;
      seats.push({
        id: ++id,
        x: radius * Math.sin(a),
        y: radius * Math.cos(a),
        section: sec.name,
      });
    }
    radius += ROW_GAP;
  }
  const mid = (inner + (radius - ROW_GAP)) / 2;
  const la = maxHalf + 0.11;
  sectionLabels.push({ name: sec.name, x: mid * Math.sin(la) + 14, y: mid * Math.cos(la) });
}

// A few states to show the shape language: booked cluster up front, one "your
// hold", a scattering of holds.
const booked = new Set([1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11]);
const mine = 70;
const held = new Set([64, 88, 140, 205, 260, 300]);

let minX = Infinity, maxX = -Infinity, minY = Infinity, maxY = -Infinity;
for (const s of seats) {
  minX = Math.min(minX, s.x); maxX = Math.max(maxX, s.x);
  minY = Math.min(minY, s.y); maxY = Math.max(maxY, s.y);
}
const rs = R0 - 70, stageHalf = 0.52;
minY = Math.min(minY, rs * Math.cos(stageHalf) - 46);
for (const l of sectionLabels) { maxX = Math.max(maxX, l.x + 70); }

const pad = 46;
const vbX = minX - pad, vbY = minY - pad;
const vbW = maxX - minX + pad * 2, vbH = maxY - minY + pad * 2;

const seatSVG = (s) => {
  const r = 7.5;
  if (booked.has(s.id))
    return `<g opacity="0.85"><circle cx="${s.x.toFixed(1)}" cy="${s.y.toFixed(1)}" r="${r}" fill="#2f434b" stroke="rgba(159,182,194,0.25)"/><path d="M ${(s.x-3).toFixed(1)} ${(s.y+0.2).toFixed(1)} L ${(s.x-0.7).toFixed(1)} ${(s.y+2.6).toFixed(1)} L ${(s.x+3.3).toFixed(1)} ${(s.y-2.6).toFixed(1)}" fill="none" stroke="#9db6c2" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/></g>`;
  if (s.id === mine)
    return `<g><circle cx="${s.x.toFixed(1)}" cy="${s.y.toFixed(1)}" r="${r}" fill="#ffd27a"/><circle cx="${s.x.toFixed(1)}" cy="${s.y.toFixed(1)}" r="${(r+3).toFixed(1)}" fill="none" stroke="#ffd27a" stroke-width="1.5" opacity="0.85"/></g>`;
  if (held.has(s.id))
    return `<circle cx="${s.x.toFixed(1)}" cy="${s.y.toFixed(1)}" r="${(r-0.5).toFixed(1)}" fill="rgba(230,162,58,0.14)" stroke="#e6a23a" stroke-width="2.3"/>`;
  return `<circle cx="${s.x.toFixed(1)}" cy="${s.y.toFixed(1)}" r="${r}" fill="#6f909e" fill-opacity="0.9" stroke="rgba(230,240,244,0.18)"/>`;
};

const svg = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="${vbX.toFixed(1)} ${vbY.toFixed(1)} ${vbW.toFixed(1)} ${vbH.toFixed(1)}" width="1000" font-family="'Space Grotesk', system-ui, sans-serif">
  <defs>
    <radialGradient id="bg" cx="50%" cy="2%" r="80%">
      <stop offset="0%" stop-color="#122127"/>
      <stop offset="60%" stop-color="#0b1418"/>
      <stop offset="100%" stop-color="#080d0f"/>
    </radialGradient>
    <linearGradient id="stage" x1="0" y1="0" x2="0" y2="1">
      <stop offset="0%" stop-color="#ffe6ad"/><stop offset="100%" stop-color="#d19a3f"/>
    </linearGradient>
    <filter id="glow" x="-40%" y="-60%" width="180%" height="260%">
      <feGaussianBlur stdDeviation="6" result="b"/><feMerge><feMergeNode in="b"/><feMergeNode in="SourceGraphic"/></feMerge>
    </filter>
  </defs>
  <rect x="${vbX.toFixed(1)}" y="${vbY.toFixed(1)}" width="${vbW.toFixed(1)}" height="${vbH.toFixed(1)}" fill="url(#bg)"/>
  <path d="M ${(rs*Math.sin(-stageHalf)).toFixed(1)} ${(rs*Math.cos(-stageHalf)).toFixed(1)} A ${rs} ${rs} 0 0 1 ${(rs*Math.sin(stageHalf)).toFixed(1)} ${(rs*Math.cos(stageHalf)).toFixed(1)}" fill="none" stroke="url(#stage)" stroke-width="9" stroke-linecap="round" filter="url(#glow)"/>
  <text x="0" y="${(rs*Math.cos(stageHalf)-30).toFixed(1)}" text-anchor="middle" fill="#f3d18a" font-size="13" letter-spacing="6">STAGE</text>
  ${sectionLabels.map((l) => `<text x="${l.x.toFixed(1)}" y="${l.y.toFixed(1)}" fill="rgba(243,209,138,0.75)" font-size="12" letter-spacing="1.5" dominant-baseline="middle">${l.name.toUpperCase()}</text>`).join("\n  ")}
  ${seats.map(seatSVG).join("\n  ")}
</svg>
`;

mkdirSync("docs/media", { recursive: true });
writeFileSync("docs/media/preview.svg", svg);
console.log("wrote docs/media/preview.svg (%d seats)", seats.length);
