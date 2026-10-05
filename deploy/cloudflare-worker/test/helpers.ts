import { vi } from "vitest";
import publicPage from "../../../web/src/test/fixtures/public-page.json";
import type { RenderDTO } from "../src/dto";

export const ORIGIN = "https://origin.test";
export const SITE = "https://links.test";
export const PROXY_SECRET = "0123456789abcdef0123456789abcdef";

/** Contract fixture (web/src/test/fixtures/public-page.json) plus hostile strings. */
export const PAGE: Readonly<Record<string, unknown>> = {
  ...publicPage,
  revision: "r-</script><!--\u2028\u2029&",
};

export const HEAD_FRAGMENT =
  '<title>Hunter\'s Lodge</title><meta name="description" content="Our communities">' +
  '<style id="lp-critical">body{color:#111}</style><script>/*boot*/</script>';

export function renderDTO(overrides: Partial<RenderDTO> = {}): RenderDTO {
  return {
    status: 200,
    lang: "en",
    appearance: "dark",
    head: HEAD_FRAGMENT,
    fallback: '<main class="lp-fallback"><h1>Hunter&#39;s Lodge</h1></main>',
    data: PAGE,
    csp: "default-src 'self'; script-src 'self' 'sha256-abc'",
    cspReportOnly: "require-trusted-types-for 'script'; trusted-types 'none'",
    etag: 'W/"render-1"',
    ...overrides,
  };
}

export function jsonResponse(body: unknown, init: ResponseInit = {}): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    ...init,
    headers: { "Content-Type": "application/json", ...(init.headers ?? {}) },
  });
}

export function renderResponse(dto: unknown): Response {
  return jsonResponse({ data: dto });
}

export type OriginHandler = (request: Request) => Response | Promise<Response>;

/** Replaces global fetch for the Worker (same isolate as the tests). */
export function mockOrigin(handler: OriginHandler) {
  const calls: Request[] = [];
  const spy = vi.spyOn(globalThis, "fetch").mockImplementation(async (input, init) => {
    const request = new Request(input as RequestInfo, init as RequestInit);
    calls.push(request);
    return handler(request);
  });
  return { calls, spy };
}

let counter = 0;
/** Unique community path so the shared edge cache never leaks between tests. */
export function uniquePath(): string {
  counter += 1;
  return `/c/t${Date.now().toString(36)}-${counter}`;
}
