import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { IDS, samplePage } from "../test/fixtures";
import { LIVE_PATH, mergeLive, POLL_INTERVAL_MS, reconcileLive, startLivePoller } from "./live";
import type { LiveDTO, LiveView } from "./types/public";

function liveDTO(patch: Record<string, Partial<LiveView>> = {}, revision = "r-5f2c9a1e"): LiveDTO {
  const page = samplePage();
  const communities = Object.fromEntries(
    Object.entries(page.communities).map(([id, c]) => [id, { ...c.live, ...patch[id] }]),
  );
  return { revision, communities, generatedAt: "2026-10-05T13:47:30Z" };
}

describe("mergeLive", () => {
  it("returns the same page when nothing changed", () => {
    const page = samplePage();
    expect(mergeLive(page, liveDTO())).toBe(page);
  });

  it("replaces changed communities immutably", () => {
    const page = samplePage();
    const before = structuredClone(page);
    const next = mergeLive(page, liveDTO({ [IDS.discord]: { online: 20 } }));
    expect(next).not.toBe(page);
    expect(next.communities[IDS.discord]?.live.online).toBe(20);
    expect(next.communities[IDS.kook]).toBe(page.communities[IDS.kook]);
    expect(page).toEqual(before);
  });

  it("ignores unknown communities and keeps missing ones", () => {
    const page = samplePage();
    const dto: LiveDTO = { revision: page.revision, generatedAt: "x", communities: {} };
    const extra = liveDTO().communities[IDS.discord] as LiveView;
    expect(mergeLive(page, { ...dto, communities: { unknown: extra } })).toBe(page);
  });

  it.each(["toString", "hasOwnProperty"])("ignores inherited names such as %s", (id) => {
    const page = samplePage();
    const extra = liveDTO().communities[IDS.discord] as LiveView;
    const next = mergeLive(page, { revision: page.revision, generatedAt: "x", communities: { [id]: extra } });
    expect(next).toBe(page);
    expect(Object.hasOwn(next.communities, id)).toBe(false);
  });
});

describe("reconcileLive", () => {
  it("requests a reload when the revision differs", () => {
    expect(reconcileLive(samplePage(), liveDTO({}, "r-new"))).toEqual({ kind: "reload" });
  });
  it("merges otherwise", () => {
    const out = reconcileLive(samplePage(), liveDTO({ [IDS.kook]: { state: "stale" } }));
    expect(out.kind === "merged" && out.page.communities[IDS.kook]?.live.state).toBe("stale");
  });
});

describe("startLivePoller", () => {
  let visibility: DocumentVisibilityState;
  let listeners: Array<() => void>;
  const doc = {
    get visibilityState() {
      return visibility;
    },
    addEventListener: (_t: string, fn: () => void) => listeners.push(fn),
    removeEventListener: (_t: string, fn: () => void) => {
      listeners = listeners.filter((l) => l !== fn);
    },
  } as unknown as Document;

  const ok = (body: unknown, etag = '"abc"') =>
    new Response(JSON.stringify(body), { status: 200, headers: { ETag: etag } });

  beforeEach(() => {
    vi.useFakeTimers();
    visibility = "visible";
    listeners = [];
  });
  afterEach(() => vi.useRealTimers());

  async function advance(ms: number) {
    await vi.advanceTimersByTimeAsync(ms);
  }

  it("polls every interval while visible and sends If-None-Match", async () => {
    const fetch = vi
      .fn()
      .mockResolvedValueOnce(ok({ data: liveDTO() }))
      .mockResolvedValue(new Response(null, { status: 304 }));
    const onLive = vi.fn().mockReturnValue(true);
    const stop = startLivePoller({ fetch, doc, now: Date.now, onLive });
    await advance(POLL_INTERVAL_MS - 1);
    expect(fetch).not.toHaveBeenCalled();
    await advance(1);
    expect(fetch).toHaveBeenCalledTimes(1);
    expect(fetch.mock.calls[0]?.[0]).toBe(LIVE_PATH);
    expect(onLive).toHaveBeenCalledTimes(1);
    await advance(POLL_INTERVAL_MS);
    expect(fetch.mock.calls[1]?.[1].headers["If-None-Match"]).toBe('"abc"');
    expect(onLive).toHaveBeenCalledTimes(1);
    stop();
    await advance(POLL_INTERVAL_MS * 3);
    expect(fetch).toHaveBeenCalledTimes(2);
    expect(listeners).toHaveLength(0);
  });

  it.each([
    ["false", () => false],
    ["a promise of false", () => Promise.resolve(false)],
  ])("does not store the ETag when onLive returns %s", async (_name, result) => {
    const fetch = vi.fn().mockImplementation(() => Promise.resolve(ok({ data: liveDTO() })));
    const onLive = vi.fn().mockImplementation(result);
    startLivePoller({ fetch, doc, now: Date.now, onLive });
    await advance(POLL_INTERVAL_MS * 2);
    expect(fetch).toHaveBeenCalledTimes(2);
    expect(fetch.mock.calls[1]?.[1].headers["If-None-Match"]).toBeUndefined();
    expect(onLive).toHaveBeenCalledTimes(2);
  });

  it("stores the ETag once onLive resolves true and keeps it on a later failure-free 304", async () => {
    const fetch = vi
      .fn()
      .mockResolvedValueOnce(ok({ data: liveDTO() }, '"one"'))
      .mockResolvedValueOnce(ok({ data: liveDTO() }, '"two"'))
      .mockResolvedValue(new Response(null, { status: 304 }));
    const onLive = vi.fn().mockResolvedValueOnce(false).mockResolvedValue(true);
    startLivePoller({ fetch, doc, now: Date.now, onLive });
    await advance(POLL_INTERVAL_MS * 3);
    expect(fetch.mock.calls[1]?.[1].headers["If-None-Match"]).toBeUndefined();
    expect(fetch.mock.calls[2]?.[1].headers["If-None-Match"]).toBe('"two"');
  });

  it("skips ticks while hidden and catches up when visible again", async () => {
    const fetch = vi.fn().mockResolvedValue(new Response(null, { status: 304 }));
    startLivePoller({ fetch, doc, now: Date.now, onLive: vi.fn() });
    visibility = "hidden";
    await advance(POLL_INTERVAL_MS * 3);
    expect(fetch).not.toHaveBeenCalled();
    visibility = "visible";
    for (const l of listeners) l();
    await advance(0);
    expect(fetch).toHaveBeenCalledTimes(1);
  });

  it("does not poll on visibility when the last poll is recent", async () => {
    const fetch = vi.fn().mockResolvedValue(new Response(null, { status: 304 }));
    startLivePoller({ fetch, doc, now: Date.now, onLive: vi.fn() });
    await advance(10_000);
    for (const l of listeners) l();
    await advance(0);
    expect(fetch).not.toHaveBeenCalled();
  });

  it.each([
    ["network error", () => Promise.reject(new TypeError("offline"))],
    ["server error", () => Promise.resolve(new Response("{}", { status: 500 }))],
    ["malformed JSON", () => Promise.resolve(new Response("nope", { status: 200 }))],
    ["wrong shape", () => Promise.resolve(ok({ data: { revision: 1 } }))],
  ])("ignores %s and retries next tick", async (_name, impl) => {
    const fetch = vi.fn().mockImplementation(impl);
    const onLive = vi.fn();
    startLivePoller({ fetch, doc, now: Date.now, onLive });
    await advance(POLL_INTERVAL_MS * 2);
    expect(fetch).toHaveBeenCalledTimes(2);
    expect(onLive).not.toHaveBeenCalled();
  });

  it("never overlaps requests", async () => {
    let resolve: (r: Response) => void = () => {};
    const fetch = vi.fn().mockImplementation(() => new Promise<Response>((r) => (resolve = r)));
    startLivePoller({ fetch, doc, now: Date.now, onLive: vi.fn(), intervalMs: 5_000 });
    await advance(5_000);
    await advance(5_000);
    expect(fetch).toHaveBeenCalledTimes(1);
    resolve(new Response(null, { status: 304 }));
    await advance(5_000);
    expect(fetch).toHaveBeenCalledTimes(2);
  });

  it("drops responses that arrive after stop", async () => {
    let resolve: (r: Response) => void = () => {};
    const fetch = vi.fn().mockImplementation(() => new Promise<Response>((r) => (resolve = r)));
    const onLive = vi.fn();
    const stop = startLivePoller({ fetch, doc, now: Date.now, onLive });
    await advance(POLL_INTERVAL_MS);
    stop();
    resolve(ok({ data: liveDTO() }));
    await advance(0);
    expect(onLive).not.toHaveBeenCalled();
  });
});
