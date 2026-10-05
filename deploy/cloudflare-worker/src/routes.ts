// Request classification. Public HTML only matches "/", "/privacy" and
// "/c/{slug}" (doc 4.2); every other unmatched path renders the 404 page.

export type Route =
  | { readonly kind: "html"; readonly renderPath: string; readonly knownNotFound: boolean }
  | { readonly kind: "origin" }
  | { readonly kind: "asset-miss" };

/** Same regex as /go/{slug} and /c/{slug} on the origin (contract 3). */
export const SLUG_RE = /^[a-z0-9][a-z0-9-]{0,63}$/;

/**
 * Render path sent for paths that can never be a page. Collapsing them
 * keeps the render cache at one entry instead of one per probed URL.
 */
export const NOT_FOUND_RENDER_PATH = "/404";

const ORIGIN_EXACT: ReadonlySet<string> = new Set([
  "/healthz",
  "/readyz",
  "/favicon.ico",
  "/robots.txt",
  "/site.webmanifest",
  "/admin",
]);

const ORIGIN_PREFIXES: readonly string[] = ["/api/", "/go/", "/media/", "/admin/"];

/** Static asset prefixes: a miss here is a plain 404, never the HTML page. */
const ASSET_PREFIXES: readonly string[] = ["/assets/", "/fonts/", "/ext/"];

const COMMUNITY_PREFIX = "/c/";

export function classify(pathname: string): Route {
  if (ORIGIN_EXACT.has(pathname) || ORIGIN_PREFIXES.some((p) => pathname.startsWith(p))) {
    return { kind: "origin" };
  }
  if (ASSET_PREFIXES.some((p) => pathname.startsWith(p))) {
    return { kind: "asset-miss" };
  }
  if (pathname === "/" || pathname === "/privacy") {
    return { kind: "html", renderPath: pathname, knownNotFound: false };
  }
  if (pathname.startsWith(COMMUNITY_PREFIX) && SLUG_RE.test(pathname.slice(COMMUNITY_PREFIX.length))) {
    return { kind: "html", renderPath: pathname, knownNotFound: false };
  }
  return { kind: "html", renderPath: NOT_FOUND_RENDER_PATH, knownNotFound: true };
}
