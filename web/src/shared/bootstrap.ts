/**
 * Public page data loading. The public entry uses the hand-written DTO
 * (src/shared/types/public.ts) and plain fetch rather than the generated
 * orval client, keeping the bundle small (docs/m1/contract.md 2.2).
 */
import { isPublicPage, isRecord } from "./types/guards";
import type { PublicPage } from "./types/public";

/** Element id of the server-rendered bootstrap JSON (see internal/webui). */
export const BOOTSTRAP_ELEMENT_ID = "lp-data";

export const BOOTSTRAP_PATH = "/api/v1/public/bootstrap";

/** Extract `data` from a `{"data": ...}` envelope and validate it. */
export function parseEnvelope(json: unknown): PublicPage | null {
  if (!isRecord(json)) return null;
  return isPublicPage(json.data) ? json.data : null;
}

/** Read the bootstrap embedded by the server, if present and valid. */
export function readEmbeddedPage(doc: Document): PublicPage | null {
  const el = doc.getElementById(BOOTSTRAP_ELEMENT_ID);
  if (!el?.textContent) return null;
  try {
    return parseEnvelope(JSON.parse(el.textContent));
  } catch {
    return null;
  }
}

export class BootstrapError extends Error {
  constructor(
    message: string,
    readonly status?: number,
  ) {
    super(message);
    this.name = "BootstrapError";
  }
}

export type Fetch = (input: string, init?: RequestInit) => Promise<Response>;

/** GET /api/v1/public/bootstrap; throws BootstrapError on failure. */
export async function fetchPage(fetcher: Fetch = (i, init) => fetch(i, init)): Promise<PublicPage> {
  const res = await fetcher(BOOTSTRAP_PATH, {
    headers: { Accept: "application/json" },
    credentials: "same-origin",
  });
  if (res.status !== 200) {
    throw new BootstrapError(`bootstrap request failed with status ${res.status}`, res.status);
  }
  let json: unknown;
  try {
    json = await res.json();
  } catch {
    throw new BootstrapError("bootstrap response is not JSON");
  }
  const data = parseEnvelope(json);
  if (!data) throw new BootstrapError("bootstrap response has an unexpected shape");
  return data;
}

/**
 * Embedded data first (production HTML); otherwise fetch it from the API
 * (Vite dev server, or pages the server rendered without data).
 */
export async function loadPage(doc: Document, fetcher?: Fetch): Promise<PublicPage> {
  return readEmbeddedPage(doc) ?? fetchPage(fetcher);
}
