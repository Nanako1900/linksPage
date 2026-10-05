export type Route = { kind: "home" } | { kind: "privacy" } | { kind: "community"; slug: string } | { kind: "notFound" };

const SLUG_RE = /^[a-z0-9][a-z0-9-]{0,63}$/;

/** Mirror of the server's public HTML routes: /, /privacy, /c/{slug}. */
export function matchRoute(pathname: string): Route {
  if (pathname === "/") return { kind: "home" };
  if (pathname === "/privacy") return { kind: "privacy" };
  const m = /^\/c\/([^/]+)$/.exec(pathname);
  if (m?.[1] && SLUG_RE.test(m[1])) return { kind: "community", slug: m[1] };
  return { kind: "notFound" };
}
