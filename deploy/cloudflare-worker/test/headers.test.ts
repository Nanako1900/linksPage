import { describe, expect, it } from "vitest";
import {
  combineETag,
  FALLBACK_CSP,
  HTML_CACHE_CONTROL,
  htmlHeaders,
  matchesIfNoneMatch,
  textResponse,
} from "../src/headers";

describe("htmlHeaders", () => {
  it("sets the HTML cache, vary, CSP and security headers", () => {
    const h = htmlHeaders({ csp: "default-src 'self'", cspReportOnly: "ro", etag: 'W/"x"' });
    expect(Object.fromEntries(h)).toMatchObject({
      "cache-control": HTML_CACHE_CONTROL,
      vary: "Accept-Language",
      "content-type": "text/html; charset=utf-8",
      "content-security-policy": "default-src 'self'",
      "content-security-policy-report-only": "ro",
      etag: 'W/"x"',
      "x-content-type-options": "nosniff",
      "referrer-policy": "strict-origin-when-cross-origin",
      "cross-origin-opener-policy": "same-origin",
    });
  });

  it("omits empty report-only and missing etag", () => {
    const h = htmlHeaders({ csp: FALLBACK_CSP, cspReportOnly: "", etag: null });
    expect(h.has("Content-Security-Policy-Report-Only")).toBe(false);
    expect(h.has("ETag")).toBe(false);
  });
});

describe("textResponse", () => {
  it("is no-store plain text with extra headers", async () => {
    const res = textResponse(405, "nope", { Allow: "GET" });
    expect(res.status).toBe(405);
    expect(res.headers.get("Cache-Control")).toBe("no-store");
    expect(res.headers.get("Allow")).toBe("GET");
    expect(await res.text()).toBe("nope");
  });
});

describe("combineETag", () => {
  const cases: ReadonlyArray<readonly [string, string | null, string | null]> = [
    ['W/"r1"', '"s1"', 'W/"r1.s1"'],
    ['"r1"', 'W/"s1"', 'W/"r1.s1"'],
    ["", '"s1"', null],
    ['W/"r1"', null, null],
    ['W/""', '"s1"', null],
    ["r1", '"s1"', null],
    ['W/"r1"', '"s"1"', null],
  ];
  it.each(cases)("%s + %s → %s", (render, shell, want) => {
    expect(combineETag(render, shell)).toBe(want);
  });
});

describe("matchesIfNoneMatch", () => {
  const cases: ReadonlyArray<readonly [string | null, boolean]> = [
    [null, false],
    ['W/"a.b"', true],
    ['"a.b"', true],
    ['"x", W/"a.b"', true],
    ["*", true],
    ['"other"', false],
    ["garbage", false],
  ];
  it.each(cases)("%s → %s", (header, want) => {
    expect(matchesIfNoneMatch(header, 'W/"a.b"')).toBe(want);
  });
});
