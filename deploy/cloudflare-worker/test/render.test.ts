import { describe, expect, it } from "vitest";
import {
  EDGE_HOP_HEADER,
  loadRender,
  MAX_RENDER_BYTES,
  PROXY_AUTH_HEADER,
  RENDER_CACHE_PATH,
  type RenderDeps,
  type RenderRequest,
  renderCacheKey,
  renderURL,
} from "../src/render";
import { jsonResponse, ORIGIN, PROXY_SECRET, renderDTO, renderResponse, SITE } from "./helpers";

const request = (overrides: Partial<RenderRequest> = {}): RenderRequest => ({
  origin: ORIGIN,
  cacheOrigin: SITE,
  path: "/c/discord",
  lang: "en",
  acceptLanguage: "zh-cn,en;q=0.8",
  proxyAuth: PROXY_SECRET,
  timeoutMs: 1000,
  ...overrides,
});

class MemoryCache {
  readonly store = new Map<string, string>();
  failPut = false;
  async match(req: Request): Promise<Response | undefined> {
    const body = this.store.get(req.url);
    return body === undefined ? undefined : new Response(body);
  }
  async put(req: Request, res: Response): Promise<void> {
    if (this.failPut) {
      throw new Error("put failed");
    }
    expect(res.headers.get("Cache-Control")).toBe("public, max-age=60");
    this.store.set(req.url, await res.text());
  }
}

function deps(fetchImpl: (r: Request) => Promise<Response>, cache: MemoryCache | null = null) {
  const pending: Promise<unknown>[] = [];
  const calls: Request[] = [];
  const d: RenderDeps = {
    fetch: (r) => {
      calls.push(r);
      return fetchImpl(r);
    },
    cache: cache as unknown as Cache | null,
    waitUntil: (p) => pending.push(p),
  };
  return { d, calls, settle: () => Promise.all(pending) };
}

describe("renderURL / renderCacheKey", () => {
  it("encodes path and lang", () => {
    const url = renderURL(request({ path: "/c/a b" }));
    expect(url.origin).toBe(ORIGIN);
    expect(url.pathname).toBe("/api/v1/public/render");
    expect(url.searchParams.get("path")).toBe("/c/a b");
    expect(url.searchParams.get("lang")).toBe("en");
  });

  it("keys the cache by origin, path, lang and Accept-Language under the site host", () => {
    const key = new URL(renderCacheKey(request()).url);
    expect(`${key.origin}${key.pathname}`).toBe(`${SITE}${RENDER_CACHE_PATH}`);
    expect(Object.fromEntries(key.searchParams)).toEqual({
      origin: ORIGIN,
      path: "/c/discord",
      lang: "en",
      al: "zh-cn,en;q=0.8",
    });
    expect(renderCacheKey(request({ acceptLanguage: "en" })).url).not.toBe(renderCacheKey(request()).url);
  });
});

describe("loadRender", () => {
  it("fetches the origin with auth, language and hop headers, then caches", async () => {
    const cache = new MemoryCache();
    const { d, calls, settle } = deps(async () => renderResponse(renderDTO()), cache);
    const got = await loadRender(request(), d);
    await settle();
    expect(got).toMatchObject({ ok: true, source: "origin" });
    const sent = calls[0];
    expect(sent?.headers.get(PROXY_AUTH_HEADER)).toBe(PROXY_SECRET);
    expect(sent?.headers.get("Accept-Language")).toBe("zh-cn,en;q=0.8");
    expect(sent?.headers.get(EDGE_HOP_HEADER)).toBe("1");
    expect(sent?.redirect).toBe("manual");
    expect(cache.store.size).toBe(1);

    const again = await loadRender(request(), d);
    expect(again).toMatchObject({ ok: true, source: "cache" });
    expect(calls).toHaveLength(1);
  });

  it.each([
    ["https origin", "https://origin.example", true],
    ["http localhost (wrangler dev)", "http://localhost:8080", true],
    ["http loopback IPv6", "http://[::1]:8080", true],
    ["plain http origin", "http://203.0.113.5:8080", false],
  ])("sends the secret only over a secure transport: %s", async (_name, origin, sent) => {
    const { d, calls } = deps(async () => renderResponse(renderDTO()));
    await loadRender(request({ origin }), d);
    expect(calls[0]?.headers.get(PROXY_AUTH_HEADER) ?? null).toBe(sent ? PROXY_SECRET : null);
  });

  it("omits optional headers when unset", async () => {
    const { d, calls } = deps(async () => renderResponse(renderDTO()));
    await loadRender(request({ proxyAuth: "", acceptLanguage: "" }), d);
    expect(calls[0]?.headers.has(PROXY_AUTH_HEADER)).toBe(false);
    expect(calls[0]?.headers.has("Accept-Language")).toBe(false);
  });

  it("ignores a corrupt cache entry and a failing cache write", async () => {
    const cache = new MemoryCache();
    cache.store.set(renderCacheKey(request()).url, "not json");
    cache.failPut = true;
    const { d, settle } = deps(async () => renderResponse(renderDTO()), cache);
    expect(await loadRender(request(), d)).toMatchObject({ ok: true, source: "origin" });
    await expect(settle()).resolves.toBeDefined();
  });

  const failures: ReadonlyArray<readonly [string, () => Promise<Response>, RegExp]> = [
    ["403", async () => new Response("forbidden", { status: 403 }), /status 403/],
    ["redirect", async () => new Response(null, { status: 302, headers: { Location: "/x" } }), /status 302/],
    ["html body", async () => new Response("<html>", { headers: { "Content-Type": "text/html" } }), /acceptable/],
    [
      "declared too large",
      async () => jsonResponse({}, { headers: { "Content-Length": String(MAX_RENDER_BYTES + 1) } }),
      /acceptable/,
    ],
    [
      "actually too large",
      async () =>
        new Response(`"${"a".repeat(MAX_RENDER_BYTES)}"`, { headers: { "Content-Type": "application/json" } }),
      /too large/,
    ],
    ["invalid json", async () => new Response("{", { headers: { "Content-Type": "application/json" } }), /unreadable/],
    ["invalid dto", async () => renderResponse({ status: 200 }), /validation/],
    [
      "network error",
      async () => {
        throw new TypeError("network down");
      },
      /fetch failed: TypeError: network down/,
    ],
  ];
  it.each(failures)("reports %s", async (_name, respond, reason) => {
    const cache = new MemoryCache();
    const { d, settle } = deps(respond, cache);
    const got = await loadRender(request(), d);
    await settle();
    expect(got.ok).toBe(false);
    expect(got.ok ? "" : got.reason).toMatch(reason);
    expect(cache.store.size).toBe(0);
  });

  it("accepts problem-style JSON subtypes", async () => {
    const { d } = deps(async () =>
      jsonResponse({ data: renderDTO() }, { headers: { "Content-Type": "application/vnd.lp+json; charset=utf-8" } }),
    );
    expect((await loadRender(request(), d)).ok).toBe(true);
  });

  it("times out a slow origin", async () => {
    const { d } = deps(
      (r) =>
        new Promise<Response>((_resolve, reject) => {
          r.signal.addEventListener("abort", () => reject(r.signal.reason));
        }),
    );
    const got = await loadRender(request({ timeoutMs: 20 }), d);
    expect(got.ok ? "" : got.reason).toMatch(/fetch failed/);
  });
});
