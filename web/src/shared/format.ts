/** Display formatting helpers for the public page (locale-aware, never throw). */

const MINUTE = 60_000;
const HOUR = 60 * MINUTE;
const DAY = 24 * HOUR;

/** Two-digit count for headings: 3 → "03", 120 → "120". */
export function padCount(n: number): string {
  return String(Math.max(0, Math.trunc(n))).padStart(2, "0");
}

function safeLocale(locale: string): string {
  try {
    return Intl.getCanonicalLocales(locale)[0] ?? "en";
  } catch {
    return "en";
  }
}

/** Localized integer, e.g. 107345 → "107,345". */
export function formatCount(n: number, locale: string): string {
  return new Intl.NumberFormat(safeLocale(locale), { maximumFractionDigits: 0 }).format(n);
}

/**
 * "5 minutes ago" / "5分钟前" for an RFC 3339 timestamp relative to `now`
 * (ms). Minutes below one hour, hours below two days, days after that.
 * Returns null for unparsable timestamps.
 */
export function relativeAgo(iso: string, now: number, locale: string): string | null {
  const t = Date.parse(iso);
  if (Number.isNaN(t)) return null;
  const diff = Math.max(0, now - t);
  const rtf = new Intl.RelativeTimeFormat(safeLocale(locale), { numeric: "auto" });
  if (diff < HOUR) return rtf.format(-Math.floor(diff / MINUTE), "minute");
  if (diff < 2 * DAY) return rtf.format(-Math.floor(diff / HOUR), "hour");
  return rtf.format(-Math.floor(diff / DAY), "day");
}

/** Host name of an absolute URL for display, or the input when unparsable. */
export function displayHost(url: string): string {
  try {
    const u = new URL(url);
    if (u.protocol === "mailto:") return u.pathname;
    return u.host.replace(/^www\./, "");
  } catch {
    return url;
  }
}

/** Join an absolute base (no trailing slash) and a root-relative path. */
export function absoluteUrl(baseUrl: string, path: string): string {
  return baseUrl.replace(/\/+$/, "") + (path.startsWith("/") ? path : `/${path}`);
}
