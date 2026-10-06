// Fetches GET /api/v1/public/render from the origin, with a 60 second
// edge cache (caches.default) keyed by path + lang + Accept-Language.

import { parseRenderEnvelope, type RenderDTO } from "./dto";
import { secureTransport } from "./env";
import { errorMessage } from "./log";

export const RENDER_API_PATH = "/api/v1/public/render";
export const RENDER_CACHE_TTL_SECONDS = 60;
/** Cache keys live under the request's own host on this reserved path. */
export const RENDER_CACHE_PATH = "/__lp/render-cache";
export const MAX_RENDER_BYTES = 2 * 1024 * 1024;
export const PROXY_AUTH_HEADER = "X-LP-Proxy-Auth";
/** Marks Worker subrequests so a misrouted request cannot loop. */
export const EDGE_HOP_HEADER = "X-LP-Edge-Hop";

export interface RenderRequest {
  readonly origin: string;
  /** Host whose cache namespace holds the entry (the visitor-facing origin). */
  readonly cacheOrigin: string;
  readonly path: string;
  readonly lang: string;
  readonly acceptLanguage: string;
  readonly proxyAuth: string;
  readonly timeoutMs: number;
}

export interface RenderDeps {
  readonly fetch: (request: Request) => Promise<Response>;
  readonly cache: Cache | null;
  readonly waitUntil: (promise: Promise<unknown>) => void;
}

export type RenderResult =
  | { readonly ok: true; readonly dto: RenderDTO; readonly source: "cache" | "origin" }
  | { readonly ok: false; readonly reason: string };

export function renderURL(req: RenderRequest): URL {
  const url = new URL(RENDER_API_PATH, req.origin);
  url.searchParams.set("path", req.path);
  url.searchParams.set("lang", req.lang);
  return url;
}

export function renderCacheKey(req: RenderRequest): Request {
  const url = new URL(RENDER_CACHE_PATH, req.cacheOrigin);
  url.searchParams.set("origin", req.origin);
  url.searchParams.set("path", req.path);
  url.searchParams.set("lang", req.lang);
  url.searchParams.set("al", req.acceptLanguage);
  return new Request(url.toString(), { method: "GET" });
}

export async function loadRender(req: RenderRequest, deps: RenderDeps): Promise<RenderResult> {
  const cached = await readCache(req, deps.cache);
  if (cached !== null) {
    return { ok: true, dto: cached, source: "cache" };
  }
  const result = await fetchRender(req, deps.fetch);
  if (result.ok && deps.cache !== null) {
    deps.waitUntil(writeCache(req, result.dto, deps.cache));
  }
  return result;
}

async function readCache(req: RenderRequest, cache: Cache | null): Promise<RenderDTO | null> {
  if (cache === null) {
    return null;
  }
  try {
    const hit = await cache.match(renderCacheKey(req));
    return hit === undefined ? null : parseRenderEnvelope(await hit.json());
  } catch {
    // A broken cache entry only costs one origin request.
    return null;
  }
}

async function writeCache(req: RenderRequest, dto: RenderDTO, cache: Cache): Promise<void> {
  const body = JSON.stringify({ data: dto });
  const response = new Response(body, {
    headers: {
      "Content-Type": "application/json",
      "Cache-Control": `public, max-age=${RENDER_CACHE_TTL_SECONDS}`,
    },
  });
  try {
    await cache.put(renderCacheKey(req), response);
  } catch {
    // Caching is best effort; the next request fetches the origin again.
  }
}

/** Subrequest headers; the shared secret is only sent over https (or to localhost). */
export function renderHeaders(req: RenderRequest, url: URL): Headers {
  const headers = new Headers({ Accept: "application/json", [EDGE_HOP_HEADER]: "1" });
  if (req.acceptLanguage !== "") {
    headers.set("Accept-Language", req.acceptLanguage);
  }
  if (req.proxyAuth !== "" && secureTransport(url)) {
    headers.set(PROXY_AUTH_HEADER, req.proxyAuth);
  }
  return headers;
}

async function fetchRender(req: RenderRequest, fetchFn: RenderDeps["fetch"]): Promise<RenderResult> {
  let response: Response;
  const url = renderURL(req);
  try {
    response = await fetchFn(
      new Request(url.toString(), {
        method: "GET",
        headers: renderHeaders(req, url),
        redirect: "manual",
        signal: AbortSignal.timeout(req.timeoutMs),
      }),
    );
  } catch (err) {
    return { ok: false, reason: `fetch failed: ${errorMessage(err)}` };
  }
  return readRenderResponse(response);
}

async function readRenderResponse(response: Response): Promise<RenderResult> {
  if (response.status !== 200) {
    await response.body?.cancel();
    return { ok: false, reason: `origin status ${response.status}` };
  }
  const contentType = response.headers.get("Content-Type") ?? "";
  const declared = Number(response.headers.get("Content-Length") ?? "0");
  if (!/^application\/(?:[\w.+-]+\+)?json\b/i.test(contentType) || declared > MAX_RENDER_BYTES) {
    await response.body?.cancel();
    return { ok: false, reason: "origin response is not acceptable JSON" };
  }
  try {
    const text = await response.text();
    if (text.length > MAX_RENDER_BYTES) {
      return { ok: false, reason: "origin response too large" };
    }
    const dto = parseRenderEnvelope(JSON.parse(text));
    return dto === null
      ? { ok: false, reason: "origin response failed validation" }
      : { ok: true, dto, source: "origin" };
  } catch (err) {
    return { ok: false, reason: `origin response unreadable: ${errorMessage(err)}` };
  }
}
