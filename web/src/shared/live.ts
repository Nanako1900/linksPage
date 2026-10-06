/**
 * Live card data (docs/m1/contract.md section 8): poll
 * /api/v1/public/live every 60 s while the page is visible, with
 * If-None-Match, and merge the result immutably. A different revision
 * means the page content changed and the full bootstrap must be reloaded.
 */
import type { Fetch } from "./bootstrap";
import { isLiveDTO, isRecord } from "./types/guards";
import type { LiveDTO, LiveView, PublicPage } from "./types/public";

export const LIVE_PATH = "/api/v1/public/live";
export const POLL_INTERVAL_MS = 60_000;

function sameLive(a: LiveView, b: LiveView): boolean {
  return JSON.stringify(a) === JSON.stringify(b);
}

/**
 * Replace the live data of the communities present in `dto`; missing ones
 * keep their state. Returns `page` itself when nothing changed.
 */
export function mergeLive(page: PublicPage, dto: LiveDTO): PublicPage {
  const updates = Object.entries(dto.communities).filter(([id, live]) => {
    const c = Object.hasOwn(page.communities, id) ? page.communities[id] : undefined;
    return c !== undefined && !sameLive(c.live, live);
  });
  if (updates.length === 0) return page;
  const communities = { ...page.communities };
  for (const [id, live] of updates) {
    communities[id] = { ...(page.communities[id] as PublicPage["communities"][string]), live };
  }
  return { ...page, communities };
}

export type LiveOutcome = { kind: "merged"; page: PublicPage } | { kind: "reload" };

/** Decide how to apply a live response to the current page. */
export function reconcileLive(page: PublicPage, dto: LiveDTO): LiveOutcome {
  if (dto.revision !== page.revision) return { kind: "reload" };
  return { kind: "merged", page: mergeLive(page, dto) };
}

export interface PollerOptions {
  fetch: Fetch;
  doc: Pick<Document, "visibilityState" | "addEventListener" | "removeEventListener">;
  now: () => number;
  /**
   * Apply a 200 response. Resolve to false when the page could not accept
   * it (a revision reload failed): the ETag is then not stored, so the
   * next poll gets a 200 again instead of a 304 and the reload is retried.
   */
  onLive: (dto: LiveDTO) => boolean | Promise<boolean>;
  intervalMs?: number;
}

/** Tolerance so an interval tick right after a visibility poll still runs on schedule. */
const TICK_SLACK_MS = 1_000;

/**
 * Start polling; returns a stop function. Polls only while the document
 * is visible; on becoming visible again, polls at once when the last poll
 * is at least one interval old. Network and parse errors are ignored (the
 * next tick retries); so are responses the page did not accept (onLive
 * resolved false), which are fetched again without If-None-Match.
 */
export function startLivePoller(opts: PollerOptions): () => void {
  const interval = opts.intervalMs ?? POLL_INTERVAL_MS;
  let etag: string | null = null;
  let lastPoll = opts.now();
  let inFlight = false;
  let stopped = false;

  const poll = async (): Promise<void> => {
    if (inFlight || stopped) return;
    inFlight = true;
    lastPoll = opts.now();
    try {
      const headers: Record<string, string> = { Accept: "application/json" };
      if (etag) headers["If-None-Match"] = etag;
      const res = await opts.fetch(LIVE_PATH, { headers, cache: "no-store", credentials: "same-origin" });
      if (res.status !== 200) return;
      const json: unknown = await res.json();
      if (stopped || !isRecord(json) || !isLiveDTO(json.data)) return;
      const nextEtag = res.headers.get("ETag");
      const accepted = await opts.onLive(json.data);
      // Only advance the validator once the page holds this response.
      etag = accepted ? (nextEtag ?? etag) : null;
    } catch {
      // Offline or malformed: try again on the next tick.
    } finally {
      inFlight = false;
    }
  };

  const due = () => opts.doc.visibilityState === "visible" && opts.now() - lastPoll >= interval - TICK_SLACK_MS;
  const timer = setInterval(() => {
    if (due()) void poll();
  }, interval);
  const onVisibility = () => {
    if (due()) void poll();
  };
  opts.doc.addEventListener("visibilitychange", onVisibility);

  return () => {
    stopped = true;
    clearInterval(timer);
    opts.doc.removeEventListener("visibilitychange", onVisibility);
  };
}
