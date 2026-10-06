// Public HTML pipeline: static shell + origin render → rewritten document.

import type { RenderDTO } from "./dto";
import {
  combineETag,
  FALLBACK_CSP,
  htmlHeaders,
  matchesIfNoneMatch,
  TRUSTED_TYPES_REPORT_ONLY,
  textResponse,
} from "./headers";
import { normalizeAcceptLanguage, normalizeLang } from "./lang";
import { logError } from "./log";
import { loadRender, type RenderDeps, type RenderRequest } from "./render";
import { plainShell, rewriteShell } from "./rewrite";

export interface HTMLContext {
  readonly request: Request;
  readonly url: URL;
  readonly origin: string;
  readonly renderPath: string;
  readonly knownNotFound: boolean;
  readonly proxyAuth: string;
  readonly renderTimeoutMs: number;
  readonly assets: Fetcher;
  readonly render: RenderDeps;
}

const ALLOWED_METHODS = "GET, HEAD";

export async function handleHTML(ctx: HTMLContext): Promise<Response> {
  const method = ctx.request.method;
  if (method !== "GET" && method !== "HEAD") {
    return textResponse(405, "Method Not Allowed", { Allow: ALLOWED_METHODS });
  }
  const [shell, render] = await Promise.all([
    loadShell(ctx.assets, ctx.url),
    loadRender(renderRequest(ctx), ctx.render),
  ]);
  if (shell === null) {
    logError("shell_unavailable", { path: ctx.url.pathname });
    return textResponse(503, "Service Unavailable", { "Retry-After": "30" });
  }
  if (render.ok) {
    return renderedResponse(ctx.request, shell, render.dto);
  }
  logError("render_unavailable", { path: ctx.renderPath, reason: render.reason });
  return fallbackResponse(ctx.request, shell, ctx.knownNotFound ? 404 : 200);
}

function renderRequest(ctx: HTMLContext): RenderRequest {
  return {
    origin: ctx.origin,
    cacheOrigin: ctx.url.origin,
    path: ctx.renderPath,
    lang: normalizeLang(ctx.url.searchParams.get("lang")),
    acceptLanguage: normalizeAcceptLanguage(ctx.request.headers.get("Accept-Language")),
    proxyAuth: ctx.proxyAuth,
    timeoutMs: ctx.renderTimeoutMs,
  };
}

/** Fetches index.html from Static Assets without visitor conditionals. */
async function loadShell(assets: Fetcher, url: URL): Promise<Response | null> {
  try {
    const response = await assets.fetch(new Request(new URL("/", url).toString(), { method: "GET" }));
    if (response.status === 200 && response.body !== null) {
      return response;
    }
    await response.body?.cancel();
    return null;
  } catch {
    return null;
  }
}

async function renderedResponse(request: Request, shell: Response, dto: RenderDTO): Promise<Response> {
  const etag = dto.status === 200 ? combineETag(dto.etag, shell.headers.get("ETag")) : null;
  const headers = htmlHeaders({ csp: dto.csp, cspReportOnly: dto.cspReportOnly, etag });
  if (etag !== null && matchesIfNoneMatch(request.headers.get("If-None-Match"), etag)) {
    await shell.body?.cancel();
    headers.delete("Content-Type");
    return new Response(null, { status: 304, headers });
  }
  return finish(request, rewriteShell(shell, dto), dto.status, headers);
}

async function fallbackResponse(request: Request, shell: Response, status: number): Promise<Response> {
  const headers = htmlHeaders({ csp: FALLBACK_CSP, cspReportOnly: TRUSTED_TYPES_REPORT_ONLY, etag: null });
  return finish(request, plainShell(shell), status, headers);
}

async function finish(request: Request, document: Response, status: number, headers: Headers): Promise<Response> {
  if (request.method === "HEAD") {
    await document.body?.cancel();
    return new Response(null, { status, headers });
  }
  return new Response(document.body, { status, headers });
}
