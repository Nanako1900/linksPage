// Normalization of the visitor's language inputs. The normalized values are
// both forwarded to the origin and used in the render cache key, so the
// cached rendering always matches what the origin was asked for.

export const MAX_LANG_LENGTH = 35;
export const MAX_ACCEPT_LANGUAGE_LENGTH = 256;
export const MAX_LANGUAGE_RANGES = 3;

const LANG_RE = /^[A-Za-z]{2,3}(-[A-Za-z0-9]{1,8}){0,3}$/;
const RANGE_RE = /^(\*|[a-z]{1,8}(-[a-z0-9]{1,8})*)$/;
const Q_RE = /^q=(0(\.\d{0,3})?|1(\.0{0,3})?)$/;

/** Returns the ?lang= value when it looks like a BCP 47 tag, otherwise "". */
export function normalizeLang(raw: string | null): string {
  if (raw === null || raw.length > MAX_LANG_LENGTH || !LANG_RE.test(raw)) {
    return "";
  }
  return raw;
}

interface LanguageRange {
  readonly tag: string;
  readonly q: number;
  readonly index: number;
}

/**
 * Reduces Accept-Language to at most MAX_LANGUAGE_RANGES valid, lowercased
 * ranges ordered by quality. Invalid entries and q=0 are dropped.
 */
export function normalizeAcceptLanguage(raw: string | null): string {
  if (raw === null) {
    return "";
  }
  const ranges = boundedEntries(raw)
    .map(parseRange)
    .filter((r): r is LanguageRange => r !== null && r.q > 0)
    .sort((a, b) => b.q - a.q || a.index - b.index)
    .slice(0, MAX_LANGUAGE_RANGES);
  return ranges.map(formatRange).join(",");
}

function boundedEntries(raw: string): readonly string[] {
  if (raw.length <= MAX_ACCEPT_LANGUAGE_LENGTH) {
    return raw.split(",");
  }
  // Drop the entry cut in half by the length limit.
  return raw.slice(0, MAX_ACCEPT_LANGUAGE_LENGTH).split(",").slice(0, -1);
}

function parseRange(entry: string, index: number): LanguageRange | null {
  const [rawTag = "", ...params] = entry.split(";").map((p) => p.trim());
  const tag = rawTag.toLowerCase();
  if (!RANGE_RE.test(tag)) {
    return null;
  }
  let q = 1;
  for (const param of params) {
    const lower = param.toLowerCase().replace(/\s+/g, "");
    if (!Q_RE.test(lower)) {
      return null;
    }
    q = Number(lower.slice(2));
  }
  return { tag, q, index };
}

function formatRange(range: LanguageRange): string {
  return range.q === 1 ? range.tag : `${range.tag};q=${Number(range.q.toFixed(3))}`;
}
