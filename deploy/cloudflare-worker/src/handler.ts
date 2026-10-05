// Top-level request dispatch.

import { ConfigError, type Env, originFor, parseConfig, type WorkerConfig } from "./env";
import { textResponse } from "./headers";
import { handleHTML } from "./html";
import { errorMessage, logError } from "./log";
import { type FetchFn, proxyToOrigin } from "./proxy";
import { EDGE_HOP_HEADER } from "./render";
import { classify } from "./routes";

export interface HandlerDeps {
  readonly fetch: FetchFn;
  /** Returns the edge cache, or null where the Cache API is unavailable. */
  readonly cache: () => Cache | null;
}

export async function handleRequest(
  request: Request,
  env: Env,
  ctx: ExecutionContext,
  deps: HandlerDeps,
): Promise<Response> {
  if (request.headers.has(EDGE_HOP_HEADER)) {
    // A Worker subrequest came back to the Worker: the routes are misconfigured.
    logError("edge_loop", { path: new URL(request.url).pathname });
    return textResponse(508, "Loop Detected");
  }
  let config: WorkerConfig;
  try {
    config = parseConfig(env);
  } catch (err) {
    if (!(err instanceof ConfigError)) {
      throw err;
    }
    logError("config_invalid", { error: errorMessage(err) });
    return textResponse(500, "Worker misconfigured");
  }
  return dispatch(request, env, ctx, deps, config);
}

function dispatch(
  request: Request,
  env: Env,
  ctx: ExecutionContext,
  deps: HandlerDeps,
  config: WorkerConfig,
): Promise<Response> {
  const url = new URL(request.url);
  const origin = originFor(config, url);
  const route = classify(url.pathname);
  switch (route.kind) {
    case "origin":
      return proxyToOrigin(request, origin, deps.fetch);
    case "asset-miss":
      return Promise.resolve(textResponse(404, "Not Found"));
    case "html":
      return handleHTML({
        request,
        url,
        origin,
        renderPath: route.renderPath,
        knownNotFound: route.knownNotFound,
        proxyAuth: config.proxyAuth,
        renderTimeoutMs: config.renderTimeoutMs,
        assets: env.ASSETS,
        render: { fetch: deps.fetch, cache: deps.cache(), waitUntil: (p) => ctx.waitUntil(p) },
      });
  }
}

/** Runtime dependencies used in production. */
export function defaultDeps(): HandlerDeps {
  return {
    fetch: (request) => fetch(request),
    cache: () => (typeof caches === "undefined" ? null : caches.default),
  };
}
