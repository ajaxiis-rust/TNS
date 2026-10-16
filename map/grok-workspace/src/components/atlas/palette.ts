/** Choropleth ramp. Keep in step with `--color-ramp-*` in styles.css. */
export const RAMP = ["#f4e7d4", "#efd0a6", "#e29a64", "#c25436", "#7a291c"] as const;

function hexToRgb(hex: string): [number, number, number] {
  const h = hex.slice(1);
  return [
    Number.parseInt(h.slice(0, 2), 16),
    Number.parseInt(h.slice(2, 4), 16),
    Number.parseInt(h.slice(4, 6), 16),
  ];
}

function mixHex(a: string, b: string, t: number): string {
  const A = hexToRgb(a);
  const B = hexToRgb(b);
  const c = A.map((v, i) => Math.round(v + (B[i] - v) * t));
  return `#${c.map((v) => v.toString(16).padStart(2, "0")).join("")}`;
}

export function rampColor(t: number): string {
  const x = Math.min(1, Math.max(0, t));
  const scaled = x * (RAMP.length - 1);
  const i = Math.min(RAMP.length - 2, Math.floor(scaled));
  return mixHex(RAMP[i], RAMP[i + 1], scaled - i);
}

export function logT(value: number, min: number, max: number): number {
  if (!(value > 0) || !(min > 0) || !(max > min)) return 0.5;
  const t = (Math.log(value) - Math.log(min)) / (Math.log(max) - Math.log(min));
  return Math.min(1, Math.max(0, t));
}

export function colorFor(value: number, min: number, max: number): string {
  return rampColor(logT(value, min, max));
}
