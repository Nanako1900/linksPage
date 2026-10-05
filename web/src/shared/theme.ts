export type AppearanceMode = "light" | "dark" | "auto" | "visitor-choice";
export type Appearance = "light" | "dark";

/** Storage key read by the inline boot script (internal/webui/static/boot.js). */
export const APPEARANCE_STORAGE_KEY = "lp_appearance";

/**
 * Same rules as the inline boot script: visitor-choice reads the stored
 * preference, auto follows prefers-color-scheme.
 */
export function resolveAppearance(mode: string, prefersDark: boolean, stored: string | null): Appearance {
  let a: string = mode;
  if (mode === "visitor-choice") a = stored === "light" || stored === "dark" ? stored : "auto";
  if (a === "light" || a === "dark") return a;
  return prefersDark ? "dark" : "light";
}

function channel(c: number): number {
  const s = c / 255;
  return s <= 0.04045 ? s / 12.92 : ((s + 0.055) / 1.055) ** 2.4;
}

function parseHex(hex: string): [number, number, number] | null {
  const m = /^#([0-9a-f]{3}|[0-9a-f]{6})$/i.exec(hex);
  if (!m?.[1]) return null;
  const h = m[1].length === 3 ? [...m[1]].map((c) => c + c).join("") : m[1];
  return [0, 2, 4].map((i) => Number.parseInt(h.slice(i, i + 2), 16)) as [number, number, number];
}

/** WCAG 2.x relative luminance of a hex color, or null if invalid. */
export function luminance(hex: string): number | null {
  const rgb = parseHex(hex);
  if (!rgb) return null;
  const [r, g, b] = rgb.map(channel) as [number, number, number];
  return 0.2126 * r + 0.7152 * g + 0.0722 * b;
}

/** WCAG contrast ratio between two hex colors, or null if either is invalid. */
export function contrastRatio(a: string, b: string): number | null {
  const la = luminance(a);
  const lb = luminance(b);
  if (la === null || lb === null) return null;
  const [hi, lo] = la > lb ? [la, lb] : [lb, la];
  return (hi + 0.05) / (lo + 0.05);
}
