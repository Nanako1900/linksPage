// Waits until the seeded page is built and the provider refresh has run
// against the stub, so every test sees deterministic live data.
import { BASE_URL } from "./support/env";

/** Slugs whose cards must leave the "pending" state before tests start. */
const PROVIDER_SLUGS = ["discord", "kook", "discord-nowidget", "discord-gone"] as const;
const DEADLINE_MS = 90_000;
// The public API allows 120 requests/min/IP; stay far below it.
const POLL_MS = 2_000;

interface Envelope<T> {
  data: T;
}
interface Bootstrap {
  communities: Record<string, { slug: string; live: { state: string } }>;
}

async function fetchJSON<T>(path: string): Promise<T | null> {
  try {
    const res = await fetch(new URL(path, BASE_URL));
    if (!res.ok) return null;
    return ((await res.json()) as Envelope<T>).data;
  } catch {
    // Not up yet; the caller polls again.
    return null;
  }
}

function pendingSlugs(page: Bootstrap): string[] {
  const states = new Map(Object.values(page.communities).map((c) => [c.slug, c.live.state]));
  return PROVIDER_SLUGS.filter((slug) => states.get(slug) === undefined || states.get(slug) === "pending");
}

export default async function globalSetup(): Promise<void> {
  const deadline = Date.now() + DEADLINE_MS;
  let pending: string[] = [...PROVIDER_SLUGS];
  while (Date.now() < deadline) {
    const page = await fetchJSON<Bootstrap>("/api/v1/public/bootstrap");
    if (page) {
      pending = pendingSlugs(page);
      if (pending.length === 0) return;
    }
    await new Promise((resolve) => setTimeout(resolve, POLL_MS));
  }
  throw new Error(`E2E stack not ready after ${DEADLINE_MS} ms; still pending: ${pending.join(", ")}`);
}
