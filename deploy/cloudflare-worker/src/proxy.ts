// Pass-through for origin routes (/api/*, /go/*, /media/*, /healthz, ...).
// These should be Workers "None" routes (README); this only runs when a
// request reaches the Worker anyway, e.g. before the routes are configured.

import { textResponse } from "./headers";
import { errorMessage, logError } from "./log";
import { EDGE_HOP_HEADER } from "./render";

export type FetchFn = (request: Request) => Promise<Response>;

/** Builds the origin URL for a visitor request, keeping path and query. */
export function originURL(requestUrl: URL, origin: string): URL {
  return new URL(`${requestUrl.pathname}${requestUrl.search}`, origin);
}

export async function proxyToOrigin(request: Request, origin: string, fetchFn: FetchFn): Promise<Response> {
  const url = new URL(request.url);
  const headers = new Headers(request.headers);
  headers.set(EDGE_HOP_HEADER, "1");
  const hasBody = request.method !== "GET" && request.method !== "HEAD";
  try {
    return await fetchFn(
      new Request(originURL(url, origin).toString(), {
        method: request.method,
        headers,
        body: hasBody ? request.body : null,
        // /go/* answers with 302; the visitor must see it, not the Worker.
        redirect: "manual",
      }),
    );
  } catch (err) {
    logError("origin_unreachable", { path: url.pathname, error: errorMessage(err) });
    return textResponse(502, "Bad Gateway");
  }
}
