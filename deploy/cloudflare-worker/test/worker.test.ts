import { SELF } from "cloudflare:test";
import { afterEach, describe, expect, it, vi } from "vitest";
import { FALLBACK_CSP, TRUSTED_TYPES_REPORT_ONLY } from "../src/headers";
import { EDGE_HOP_HEADER, PROXY_AUTH_HEADER, renderCacheKey } from "../src/render";
import { NOT_FOUND_RENDER_PATH } from "../src/routes";
import { HEAD_FRAGMENT, mockOrigin, ORIGIN, PAGE, renderDTO, renderResponse, SITE, uniquePath } from "./helpers";

afterEach(() => {
  vi.restoreAllMocks();
});

function extractData(html: string): unknown {
  const match = /<script id="lp-data" type="application\/json">(.*?)<\/script>/s.exec(html);
  expect(match).not.toBeNull();
  return JSON.parse(match?.[1] ?? "null");
}

describe("public HTML", () => {
  it("rewrites the static shell with the origin rendering", async () => {
    const path = uniquePath();
    const { calls } = mockOrigin(() => renderResponse(renderDTO()));
    const res = await SELF.fetch(`${SITE}${path}?lang=en`, { headers: { "Accept-Language": "zh-CN,en;q=0.5" } });
    const html = await res.text();

    expect(res.status).toBe(200);
    expect(res.headers.get("Content-Security-Policy")).toBe(renderDTO().csp);
    expect(res.headers.get("Content-Security-Policy-Report-Only")).toBe(renderDTO().cspReportOnly);
    expect(res.headers.get("Cache-Control")).toBe("no-cache, no-transform");
    expect(res.headers.get("Vary")).toBe("Accept-Language");
    expect(res.headers.get("ETag")).toMatch(/^W\/"render-1\..+"$/);

    expect(html).toContain('<html lang="en" data-appearance="dark">');
    expect(html).not.toContain("<title>LinksPage</title>");
    expect(html).not.toContain("shell description");
    expect(html).not.toContain("Dev-only shell");
    expect(html).toContain(`${HEAD_FRAGMENT}</head>`);
    expect(html).toContain('<div id="root"><main class="lp-fallback">');
    expect(html).not.toContain("shell placeholder");
    expect(html).toContain('src="/assets/public-test.js"');
    expect(html).toContain('rel="modulepreload"');
    expect(html).not.toContain("</script><!--");
    expect(html).not.toMatch(/[\u2028\u2029]/);
    expect(extractData(html)).toEqual({ data: PAGE });

    const sent = calls[0];
    const url = new URL(sent?.url ?? "");
    expect(url.origin).toBe(ORIGIN);
    expect(url.searchParams.get("path")).toBe(path);
    expect(url.searchParams.get("lang")).toBe("en");
    expect(sent?.headers.get("Accept-Language")).toBe("zh-cn,en;q=0.5");
    expect(sent?.headers.has(PROXY_AUTH_HEADER)).toBe(false);
  });

  it("serves the origin 404 rendering without lp-data or ETag", async () => {
    mockOrigin(() => renderResponse(renderDTO({ status: 404, data: null })));
    const res = await SELF.fetch(`${SITE}${uniquePath()}`);
    const html = await res.text();
    expect(res.status).toBe(404);
    expect(res.headers.has("ETag")).toBe(false);
    expect(html).not.toContain("lp-data");
    expect(html).toContain("lp-fallback");
    // Like the origin 404 page: no SPA entry, so the app never boots.
    expect(html).not.toContain('type="module"');
    expect(html).not.toContain("modulepreload");
    expect(html).toContain('rel="stylesheet"');
  });

  it("collapses unknown paths to one 404 render path", async () => {
    const { calls } = mockOrigin(() => renderResponse(renderDTO({ status: 404, data: null, lang: "fr" })));
    const res = await SELF.fetch(`${SITE}/wp-login.php?lang=fr`);
    expect(res.status).toBe(404);
    expect(new URL(calls[0]?.url ?? "").searchParams.get("path")).toBe(NOT_FOUND_RENDER_PATH);
  });

  it("caches the rendering for repeated requests", async () => {
    const path = uniquePath();
    const { calls } = mockOrigin(() => renderResponse(renderDTO()));
    await (await SELF.fetch(`${SITE}${path}`)).text();
    const key = renderCacheKey({
      origin: ORIGIN,
      cacheOrigin: SITE,
      path,
      lang: "",
      acceptLanguage: "",
      proxyAuth: "",
      timeoutMs: 0,
    });
    // ctx.waitUntil(cache.put) may finish after the response.
    await vi.waitFor(async () => expect(await caches.default.match(key)).toBeDefined());
    await (await SELF.fetch(`${SITE}${path}`)).text();
    expect(calls).toHaveLength(1);
  });

  it("answers If-None-Match with 304", async () => {
    const path = uniquePath();
    mockOrigin(() => renderResponse(renderDTO()));
    const first = await SELF.fetch(`${SITE}${path}`);
    await first.text();
    const etag = first.headers.get("ETag") ?? "";
    const res = await SELF.fetch(`${SITE}${path}`, { headers: { "If-None-Match": etag } });
    expect(res.status).toBe(304);
    expect(res.headers.get("ETag")).toBe(etag);
    expect(res.headers.get("Content-Security-Policy")).toBe(renderDTO().csp);
    expect(await res.text()).toBe("");
  });

  it("answers HEAD without a body", async () => {
    mockOrigin(() => renderResponse(renderDTO()));
    const res = await SELF.fetch(`${SITE}${uniquePath()}`, { method: "HEAD" });
    expect(res.status).toBe(200);
    expect(res.headers.get("Content-Type")).toBe("text/html; charset=utf-8");
    expect(await res.text()).toBe("");
  });

  it("rejects other methods", async () => {
    const { calls } = mockOrigin(() => renderResponse(renderDTO()));
    const res = await SELF.fetch(`${SITE}/`, { method: "POST", body: "x" });
    expect(res.status).toBe(405);
    expect(res.headers.get("Allow")).toBe("GET, HEAD");
    expect(calls).toHaveLength(0);
  });
});

describe("origin failure fallback", () => {
  const failures: ReadonlyArray<readonly [string, () => Response]> = [
    ["5xx", () => new Response("down", { status: 502 })],
    ["invalid body", () => renderResponse({ status: 200 })],
    [
      "network error",
      () => {
        throw new TypeError("connection refused");
      },
    ],
  ];
  it.each(failures)("serves the shell with the generic CSP on %s", async (_name, respond) => {
    const errors = vi.spyOn(console, "error").mockImplementation(() => undefined);
    mockOrigin(respond);
    const res = await SELF.fetch(`${SITE}${uniquePath()}`);
    const html = await res.text();
    expect(res.status).toBe(200);
    expect(res.headers.get("Content-Security-Policy")).toBe(FALLBACK_CSP);
    expect(res.headers.get("Content-Security-Policy-Report-Only")).toBe(TRUSTED_TYPES_REPORT_ONLY);
    expect(res.headers.get("Cache-Control")).toBe("no-cache, no-transform");
    expect(res.headers.has("ETag")).toBe(false);
    expect(html).toContain("<title>LinksPage</title>");
    expect(html).toContain("shell placeholder");
    expect(html).not.toContain("Dev-only shell");
    expect(html).not.toContain("lp-data");
    expect(errors).toHaveBeenCalledWith(expect.stringContaining('"event":"render_unavailable"'));
  });

  it("keeps 404 for paths that can never be pages", async () => {
    vi.spyOn(console, "error").mockImplementation(() => undefined);
    mockOrigin(() => new Response("down", { status: 503 }));
    const res = await SELF.fetch(`${SITE}/no/such/page`);
    expect(res.status).toBe(404);
    expect(res.headers.get("Content-Security-Policy")).toBe(FALLBACK_CSP);
  });

  it("answers HEAD on fallback without a body", async () => {
    vi.spyOn(console, "error").mockImplementation(() => undefined);
    mockOrigin(() => new Response("down", { status: 503 }));
    const res = await SELF.fetch(`${SITE}${uniquePath()}`, { method: "HEAD" });
    expect(res.status).toBe(200);
    expect(await res.text()).toBe("");
  });
});

describe("origin pass-through", () => {
  it.each(["/api/v1/public/live", "/go/discord", "/media/u/0123.webp", "/healthz", "/favicon.ico", "/admin/"])(
    "forwards %s unchanged",
    async (path) => {
      const { calls } = mockOrigin(() => new Response("origin", { status: 200, headers: { "X-From": "origin" } }));
      const res = await SELF.fetch(`${SITE}${path}?q=1`, { headers: { "If-None-Match": '"e"' } });
      expect(await res.text()).toBe("origin");
      expect(res.headers.get("X-From")).toBe("origin");
      expect(calls[0]?.url).toBe(`${ORIGIN}${path}?q=1`);
      expect(calls[0]?.headers.get("If-None-Match")).toBe('"e"');
      expect(calls[0]?.headers.get(EDGE_HOP_HEADER)).toBe("1");
    },
  );

  it("does not follow /go redirects", async () => {
    const { calls } = mockOrigin(
      () => new Response(null, { status: 302, headers: { Location: "https://discord.gg/abc" } }),
    );
    const res = await SELF.fetch(`${SITE}/go/discord`, { redirect: "manual" });
    expect(res.status).toBe(302);
    expect(res.headers.get("Location")).toBe("https://discord.gg/abc");
    expect(calls[0]?.redirect).toBe("manual");
  });

  it("forwards request bodies", async () => {
    const { calls } = mockOrigin(async (r) => new Response(await r.text(), { status: 201 }));
    const res = await SELF.fetch(`${SITE}/api/v1/thing`, { method: "POST", body: "payload" });
    expect(res.status).toBe(201);
    expect(await res.text()).toBe("payload");
    expect(calls[0]?.method).toBe("POST");
  });

  it("returns 502 when the origin is unreachable", async () => {
    const errors = vi.spyOn(console, "error").mockImplementation(() => undefined);
    mockOrigin(() => {
      throw new TypeError("connection refused");
    });
    const res = await SELF.fetch(`${SITE}/api/v1/public/live`);
    expect(res.status).toBe(502);
    expect(errors).toHaveBeenCalledWith(expect.stringContaining('"event":"origin_unreachable"'));
  });
});

describe("guards", () => {
  it("returns a plain 404 for missing static assets", async () => {
    const { calls } = mockOrigin(() => renderResponse(renderDTO()));
    const res = await SELF.fetch(`${SITE}/assets/missing-abc.js`);
    expect(res.status).toBe(404);
    expect(res.headers.get("Cache-Control")).toBe("no-store");
    expect(res.headers.get("Content-Type")).toBe("text/plain; charset=utf-8");
    expect(calls).toHaveLength(0);
  });

  it("refuses requests that already went through the Worker", async () => {
    vi.spyOn(console, "error").mockImplementation(() => undefined);
    const { calls } = mockOrigin(() => renderResponse(renderDTO()));
    const res = await SELF.fetch(`${SITE}/`, { headers: { [EDGE_HOP_HEADER]: "1" } });
    expect(res.status).toBe(508);
    expect(calls).toHaveLength(0);
  });
});
