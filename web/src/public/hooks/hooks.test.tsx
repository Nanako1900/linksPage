import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { POLL_INTERVAL_MS } from "../../shared/live";
import type { LiveDTO } from "../../shared/types/public";
import { IDS, samplePage } from "../../test/fixtures";
import { COPY_FEEDBACK_MS, useCopy } from "./useCopy";
import { useLivePage } from "./useLivePage";
import { useNow } from "./useNow";

afterEach(cleanup);

function liveDTO(revision: string, online: number): LiveDTO {
  const page = samplePage();
  const discord = page.communities[IDS.discord];
  if (!discord) throw new Error("fixture");
  return { revision, generatedAt: "x", communities: { [IDS.discord]: { ...discord.live, online } } };
}

const json = (body: unknown, headers: Record<string, string> = {}) =>
  new Response(JSON.stringify(body), { status: 200, headers });

describe("useLivePage", () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());

  const visibleDoc = () =>
    ({
      visibilityState: "visible",
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
    }) as unknown as Document;

  it("merges live data on the polling interval", async () => {
    const fetch = vi.fn().mockResolvedValue(json({ data: liveDTO("r-5f2c9a1e", 42) }, { ETag: '"e1"' }));
    const initial = samplePage();
    const { result } = renderHook(() => useLivePage(initial, { fetch, doc: visibleDoc(), now: Date.now }));
    expect(result.current).toBe(initial);
    await act(() => vi.advanceTimersByTimeAsync(POLL_INTERVAL_MS));
    expect(result.current.communities[IDS.discord]?.live.online).toBe(42);
    expect(result.current.communities[IDS.kook]).toBe(initial.communities[IDS.kook]);
    expect(initial.communities[IDS.discord]?.live.online).toBe(13);
  });

  it("keeps the same object when nothing changed", async () => {
    const fetch = vi.fn().mockResolvedValue(json({ data: liveDTO("r-5f2c9a1e", 13) }));
    const initial = samplePage();
    const { result } = renderHook(() => useLivePage(initial, { fetch, doc: visibleDoc(), now: Date.now }));
    await act(() => vi.advanceTimersByTimeAsync(POLL_INTERVAL_MS));
    expect(result.current).toBe(initial);
  });

  it("reloads the bootstrap when the revision changed", async () => {
    const next = { ...samplePage(), revision: "r-new", version: 5 };
    const fetch = vi
      .fn()
      .mockImplementation(async (url: string) =>
        url.endsWith("/live") ? json({ data: liveDTO("r-new", 1) }) : json({ data: next }),
      );
    const { result } = renderHook(() => useLivePage(samplePage(), { fetch, doc: visibleDoc(), now: Date.now }));
    await act(() => vi.advanceTimersByTimeAsync(POLL_INTERVAL_MS));
    expect(fetch).toHaveBeenCalledWith("/api/v1/public/bootstrap", expect.anything());
    expect(result.current.revision).toBe("r-new");
    expect(result.current.version).toBe(5);
  });

  it("keeps the page when the reload fails and does not overlap reloads", async () => {
    let bootstrapCalls = 0;
    const fetch = vi.fn().mockImplementation(async (url: string) => {
      if (url.endsWith("/live")) return json({ data: liveDTO("r-new", 1) });
      bootstrapCalls++;
      return new Response("{}", { status: 503 });
    });
    const initial = samplePage();
    const { result } = renderHook(() => useLivePage(initial, { fetch, doc: visibleDoc(), now: Date.now }));
    await act(() => vi.advanceTimersByTimeAsync(POLL_INTERVAL_MS));
    expect(bootstrapCalls).toBe(1);
    expect(result.current).toBe(initial);
  });

  it("retries a failed reload on the next poll without If-None-Match", async () => {
    const next = { ...samplePage(), revision: "r-new", version: 5 };
    let bootstrapCalls = 0;
    const fetch = vi.fn().mockImplementation(async (url: string, init?: RequestInit) => {
      if (url.endsWith("/live")) {
        const headers = (init?.headers ?? {}) as Record<string, string>;
        if (headers["If-None-Match"] === '"e-new"') return new Response(null, { status: 304 });
        return json({ data: liveDTO("r-new", 1) }, { ETag: '"e-new"' });
      }
      bootstrapCalls++;
      return bootstrapCalls === 1 ? new Response("{}", { status: 503 }) : json({ data: next });
    });
    const initial = samplePage();
    const { result } = renderHook(() => useLivePage(initial, { fetch, doc: visibleDoc(), now: Date.now }));
    await act(() => vi.advanceTimersByTimeAsync(POLL_INTERVAL_MS));
    expect(result.current).toBe(initial);
    await act(() => vi.advanceTimersByTimeAsync(POLL_INTERVAL_MS));
    expect(bootstrapCalls).toBe(2);
    expect(result.current.revision).toBe("r-new");
    const liveCalls = fetch.mock.calls.filter(([url]) => String(url).endsWith("/live"));
    const retryHeaders = (liveCalls[1]?.[1]?.headers ?? {}) as Record<string, string>;
    expect(liveCalls).toHaveLength(2);
    expect(retryHeaders["If-None-Match"]).toBeUndefined();
    await act(() => vi.advanceTimersByTimeAsync(POLL_INTERVAL_MS));
    expect(bootstrapCalls).toBe(2);
  });

  it("stops polling on unmount", async () => {
    const fetch = vi.fn().mockResolvedValue(new Response(null, { status: 304 }));
    const { unmount } = renderHook(() => useLivePage(samplePage(), { fetch, doc: visibleDoc(), now: Date.now }));
    unmount();
    await act(() => vi.advanceTimersByTimeAsync(POLL_INTERVAL_MS * 2));
    expect(fetch).not.toHaveBeenCalled();
  });
});

describe("useCopy", () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());

  it.each([
    [true, "copied"],
    [false, "failed"],
  ] as const)("copy ok=%s → %s, then idle", async (ok, want) => {
    const copy = vi.fn().mockResolvedValue(ok);
    const { result } = renderHook(() => useCopy(copy));
    expect(result.current[0]).toBe("idle");
    await act(async () => result.current[1]("text"));
    expect(copy).toHaveBeenCalledWith("text");
    expect(result.current[0]).toBe(want);
    await act(() => vi.advanceTimersByTimeAsync(COPY_FEEDBACK_MS));
    expect(result.current[0]).toBe("idle");
  });
});

describe("useNow", () => {
  beforeEach(() => vi.useFakeTimers({ now: 1_000 }));
  afterEach(() => vi.useRealTimers());

  it("ticks every interval", async () => {
    const { result } = renderHook(() => useNow(5, 1_000));
    expect(result.current).toBe(5);
    await act(() => vi.advanceTimersByTimeAsync(1_000));
    expect(result.current).toBe(2_000);
  });
});
