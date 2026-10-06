// Worker bindings and the validated configuration derived from them.

export interface Env {
  /** Workers Static Assets binding serving web/dist. */
  readonly ASSETS: Fetcher;
  /** Shared secret sent as X-LP-Proxy-Auth (wrangler secret put LP_PROXY_AUTH). */
  readonly LP_PROXY_AUTH?: string;
  /** Origin base URL, e.g. https://links.example.com. Empty = same host as the request. */
  readonly LP_ORIGIN?: string;
  /** Timeout for GET /api/v1/public/render in milliseconds. */
  readonly LP_RENDER_TIMEOUT_MS?: string;
}

export interface WorkerConfig {
  /** Origin base without trailing slash, or null to use the request's own origin. */
  readonly originBase: string | null;
  readonly proxyAuth: string;
  readonly renderTimeoutMs: number;
}

export const DEFAULT_RENDER_TIMEOUT_MS = 4000;
export const MIN_RENDER_TIMEOUT_MS = 250;
export const MAX_RENDER_TIMEOUT_MS = 15000;
/** The origin rejects shorter secrets (config edge.proxy_auth, doc 12.3). */
export const MIN_PROXY_AUTH_BYTES = 32;

export class ConfigError extends Error {
  override readonly name = "ConfigError";
}

export function parseConfig(env: Env): WorkerConfig {
  return Object.freeze({
    originBase: parseOrigin(env.LP_ORIGIN ?? ""),
    proxyAuth: parseProxyAuth(env.LP_PROXY_AUTH ?? ""),
    renderTimeoutMs: parseTimeout(env.LP_RENDER_TIMEOUT_MS ?? ""),
  });
}

const LOOPBACK_HOSTS = new Set(["localhost", "127.0.0.1", "[::1]"]);

/** True for the loopback host names used by `wrangler dev`. */
export function isLoopback(hostname: string): boolean {
  return LOOPBACK_HOSTS.has(hostname.toLowerCase());
}

/**
 * True when a secret may be sent to `url`: https, or plain http to a
 * loopback host (local development only).
 */
export function secureTransport(url: URL): boolean {
  return url.protocol === "https:" || (url.protocol === "http:" && isLoopback(url.hostname));
}

/** Returns the origin base URL (no trailing slash) used for subrequests. */
export function originFor(config: WorkerConfig, requestUrl: URL): string {
  return config.originBase ?? requestUrl.origin;
}

function parseOrigin(raw: string): string | null {
  const value = raw.trim();
  if (value === "") {
    return null;
  }
  let url: URL;
  try {
    url = new URL(value);
  } catch {
    throw new ConfigError("LP_ORIGIN is not an absolute URL");
  }
  if (url.protocol !== "https:" && !(url.protocol === "http:" && isLoopback(url.hostname))) {
    // Render subrequests carry the LP_PROXY_AUTH secret: never in cleartext.
    throw new ConfigError("LP_ORIGIN must use https (http only for localhost)");
  }
  if (url.username !== "" || url.password !== "") {
    throw new ConfigError("LP_ORIGIN must not contain credentials");
  }
  if (url.pathname !== "/" || url.search !== "" || url.hash !== "") {
    throw new ConfigError("LP_ORIGIN must not contain a path, query or fragment");
  }
  return url.origin;
}

function parseProxyAuth(raw: string): string {
  if (raw === "") {
    return "";
  }
  if (new TextEncoder().encode(raw).length < MIN_PROXY_AUTH_BYTES) {
    throw new ConfigError(`LP_PROXY_AUTH must be at least ${MIN_PROXY_AUTH_BYTES} bytes`);
  }
  if (!/^[\x21-\x7e]+$/.test(raw)) {
    throw new ConfigError("LP_PROXY_AUTH must be printable ASCII without spaces");
  }
  return raw;
}

function parseTimeout(raw: string): number {
  const value = raw.trim();
  if (value === "") {
    return DEFAULT_RENDER_TIMEOUT_MS;
  }
  if (!/^\d+$/.test(value)) {
    throw new ConfigError("LP_RENDER_TIMEOUT_MS must be an integer");
  }
  const ms = Number(value);
  if (ms < MIN_RENDER_TIMEOUT_MS || ms > MAX_RENDER_TIMEOUT_MS) {
    throw new ConfigError(`LP_RENDER_TIMEOUT_MS must be between ${MIN_RENDER_TIMEOUT_MS} and ${MAX_RENDER_TIMEOUT_MS}`);
  }
  return ms;
}
