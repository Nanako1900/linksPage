// Response headers for HTML served by the Worker (doc 4.8, 12.4).

/**
 * CSP for the static shell served when the origin render is unavailable.
 * The shell only loads same-origin module scripts and stylesheets and has
 * no inline blocks, so no hashes are needed. frame-src allows the Discord
 * widget because the SPA may still show it after fetching bootstrap.
 */
export const FALLBACK_CSP = [
  "default-src 'self'",
  "script-src 'self'",
  "style-src 'self'",
  "img-src 'self' data: blob:",
  "font-src 'self'",
  "connect-src 'self'",
  "frame-src https://discord.com",
  "frame-ancestors 'none'",
  "base-uri 'self'",
  "form-action 'self'",
  "object-src 'none'",
].join("; ");

/** Same value as webui.TrustedTypesReportOnly. */
export const TRUSTED_TYPES_REPORT_ONLY = "require-trusted-types-for 'script'; trusted-types 'none'";

export const HTML_CACHE_CONTROL = "no-cache, no-transform";

const SECURITY_HEADERS: Readonly<Record<string, string>> = {
  "X-Content-Type-Options": "nosniff",
  "Referrer-Policy": "strict-origin-when-cross-origin",
  "Permissions-Policy": "camera=(), microphone=(), geolocation=()",
  "Cross-Origin-Opener-Policy": "same-origin",
};

export interface HTMLHeaderInput {
  readonly csp: string;
  readonly cspReportOnly: string;
  readonly etag: string | null;
}

export function htmlHeaders(input: HTMLHeaderInput): Headers {
  const headers = new Headers(SECURITY_HEADERS);
  headers.set("Content-Type", "text/html; charset=utf-8");
  headers.set("Cache-Control", HTML_CACHE_CONTROL);
  headers.set("Vary", "Accept-Language");
  headers.set("Content-Security-Policy", input.csp);
  if (input.cspReportOnly !== "") {
    headers.set("Content-Security-Policy-Report-Only", input.cspReportOnly);
  }
  if (input.etag !== null) {
    headers.set("ETag", input.etag);
  }
  return headers;
}

/** Plain-text response for errors that never reach the HTML pipeline. */
export function textResponse(status: number, body: string, extra: Readonly<Record<string, string>> = {}): Response {
  const headers = new Headers({ ...SECURITY_HEADERS, ...extra });
  headers.set("Content-Type", "text/plain; charset=utf-8");
  headers.set("Cache-Control", "no-store");
  return new Response(body, { status, headers });
}

const OPAQUE_TAG_RE = /^(?:W\/)?"([\x21\x23-\x7e]*)"$/;

function opaqueTag(etag: string | null): string | null {
  if (etag === null) {
    return null;
  }
  const match = OPAQUE_TAG_RE.exec(etag.trim());
  return match?.[1] ?? null;
}

/**
 * Weak ETag for the rewritten document: it changes when either the origin
 * rendering or the deployed static shell changes. Null if either is unknown.
 */
export function combineETag(renderETag: string, shellETag: string | null): string | null {
  const render = opaqueTag(renderETag);
  const shell = opaqueTag(shellETag);
  if (render === null || shell === null || render === "" || shell === "") {
    return null;
  }
  return `W/"${render}.${shell}"`;
}

/** Weak comparison of If-None-Match against an ETag (RFC 9110 13.1.2). */
export function matchesIfNoneMatch(header: string | null, etag: string): boolean {
  if (header === null) {
    return false;
  }
  const target = opaqueTag(etag);
  return header.split(",").some((candidate) => {
    const value = candidate.trim();
    return value === "*" || (target !== null && opaqueTag(value) === target);
  });
}
