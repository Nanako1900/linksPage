import { getPublicBootstrap } from "./api/gen/linkspage";
import type { Bootstrap } from "./api/gen/model";

export type { Bootstrap };

/** Element id of the server-rendered bootstrap JSON (see internal/webui). */
export const BOOTSTRAP_ELEMENT_ID = "lp-data";

const APPEARANCES = new Set(["light", "dark", "auto", "visitor-choice"]);

function isRecord(v: unknown): v is Record<string, unknown> {
  return typeof v === "object" && v !== null && !Array.isArray(v);
}

const PALETTE_KEYS = ["bg", "fg", "muted", "card", "border", "accent", "accentFg"] as const;
const THEME_STRING_KEYS = ["preset", "radius", "fontSans", "fontDisplay"] as const;

function isNonEmptyString(v: unknown): v is string {
  return typeof v === "string" && v !== "";
}

function isStringMap(v: unknown): v is Record<string, string> {
  return isRecord(v) && Object.values(v).every((x) => typeof x === "string");
}

function isLocaleList(v: unknown): v is string[] | null {
  return v === null || (Array.isArray(v) && v.every(isNonEmptyString));
}

function isPalette(v: unknown): boolean {
  return isRecord(v) && PALETTE_KEYS.every((k) => typeof v[k] === "string");
}

function isTheme(v: unknown): boolean {
  return (
    isRecord(v) && isPalette(v.light) && isPalette(v.dark) && THEME_STRING_KEYS.every((k) => typeof v[k] === "string")
  );
}

function isSite(v: unknown): boolean {
  return (
    isRecord(v) &&
    isNonEmptyString(v.defaultLocale) &&
    isLocaleList(v.locales) &&
    typeof v.appearance === "string" &&
    APPEARANCES.has(v.appearance) &&
    isStringMap(v.title) &&
    isStringMap(v.description) &&
    isTheme(v.theme)
  );
}

/** Narrow untrusted JSON to a Bootstrap, checking every field the public entry reads. */
export function isBootstrap(v: unknown): v is Bootstrap {
  if (!isRecord(v) || typeof v.version !== "number") return false;
  const { page } = v;
  if (!isRecord(page) || typeof page.slug !== "string" || typeof page.id !== "number") return false;
  return isSite(v.site);
}

/** Extract `data` from a `{"data": ...}` envelope and validate it. */
export function parseEnvelope(json: unknown): Bootstrap | null {
  if (!isRecord(json)) return null;
  return isBootstrap(json.data) ? json.data : null;
}

/** Read the bootstrap embedded by the Go server, if present and valid. */
export function readEmbeddedBootstrap(doc: Document): Bootstrap | null {
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

type Fetcher = typeof getPublicBootstrap;

/**
 * Embedded data first (production HTML); otherwise fetch it from the API
 * (Vite dev server, which serves its own index.html).
 */
export async function loadBootstrap(doc: Document, fetcher: Fetcher = getPublicBootstrap): Promise<Bootstrap> {
  const embedded = readEmbeddedBootstrap(doc);
  if (embedded) return embedded;
  const res = await fetcher({ headers: { Accept: "application/json" } });
  if (res.status !== 200) {
    throw new BootstrapError(`bootstrap request failed with status ${res.status}`, res.status);
  }
  const data = parseEnvelope(res.data);
  if (!data) throw new BootstrapError("bootstrap response has an unexpected shape");
  return data;
}
