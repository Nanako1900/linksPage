// RenderDTO mirror of internal/site/live.go and its runtime guard. The
// origin is trusted but its response still crosses a network boundary, so
// every field that ends up in a header or the document is checked here.

export type PublicPageJSON = Readonly<Record<string, unknown>>;

export interface RenderDTO {
  readonly status: 200 | 404;
  readonly lang: string;
  readonly appearance: string;
  readonly head: string;
  readonly fallback: string;
  /** PublicPage; null exactly when status is 404. */
  readonly data: PublicPageJSON | null;
  readonly csp: string;
  readonly cspReportOnly: string;
  readonly etag: string;
}

const LANG_RE = /^[A-Za-z0-9-]{1,35}$/;
const APPEARANCE_RE = /^[a-z-]{1,32}$/;
/** Visible ASCII plus space: safe as an HTTP header value. */
const HEADER_VALUE_RE = /^[\x20-\x7e]*$/;
export const MAX_HEADER_VALUE_LENGTH = 8192;

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function isHeaderValue(value: unknown): value is string {
  return typeof value === "string" && value.length <= MAX_HEADER_VALUE_LENGTH && HEADER_VALUE_RE.test(value);
}

function hasValidStrings(v: Record<string, unknown>): boolean {
  return (
    typeof v.lang === "string" &&
    LANG_RE.test(v.lang) &&
    typeof v.appearance === "string" &&
    APPEARANCE_RE.test(v.appearance) &&
    typeof v.head === "string" &&
    typeof v.fallback === "string" &&
    isHeaderValue(v.csp) &&
    v.csp.trim() !== "" &&
    isHeaderValue(v.cspReportOnly) &&
    isHeaderValue(v.etag)
  );
}

function hasConsistentData(v: Record<string, unknown>): boolean {
  if (v.status === 200) {
    return isRecord(v.data);
  }
  return v.status === 404 && v.data === null;
}

/** Validates a RenderDTO value and returns a frozen copy, or null. */
export function parseRenderDTO(value: unknown): RenderDTO | null {
  if (!isRecord(value) || !hasValidStrings(value) || !hasConsistentData(value)) {
    return null;
  }
  return Object.freeze({
    status: value.status as 200 | 404,
    lang: value.lang as string,
    appearance: value.appearance as string,
    head: value.head as string,
    fallback: value.fallback as string,
    data: (value.data ?? null) as PublicPageJSON | null,
    csp: value.csp as string,
    cspReportOnly: value.cspReportOnly as string,
    etag: value.etag as string,
  });
}

/** Validates the {"data": RenderDTO} envelope (doc 4.10). */
export function parseRenderEnvelope(value: unknown): RenderDTO | null {
  if (!isRecord(value)) {
    return null;
  }
  return parseRenderDTO(value.data);
}
