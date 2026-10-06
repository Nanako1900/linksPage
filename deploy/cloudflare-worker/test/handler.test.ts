import { createExecutionContext, env as testBindings, waitOnExecutionContext } from "cloudflare:test";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { Env } from "../src/env";
import { defaultDeps, type HandlerDeps, handleRequest } from "../src/handler";
import { PROXY_AUTH_HEADER } from "../src/render";
import { PROXY_SECRET, renderDTO, renderResponse, SITE } from "./helpers";

/** Bindings from wrangler.jsonc env.test (see vitest.config.ts). */
const env = testBindings as unknown as Env;

afterEach(() => {
  vi.restoreAllMocks();
});

function harness(overrides: Partial<Env>, deps: Partial<HandlerDeps> = {}) {
  const calls: Request[] = [];
  const full: HandlerDeps = {
    fetch: async (r) => {
      calls.push(r);
      return renderResponse(renderDTO());
    },
    cache: () => null,
    ...deps,
  };
  const testEnv: Env = { ...env, ...overrides, ASSETS: overrides.ASSETS ?? env.ASSETS };
  return {
    calls,
    async run(url: string, init?: RequestInit): Promise<Response> {
      const ctx = createExecutionContext();
      const res = await handleRequest(new Request(url, init), testEnv, ctx, full);
      await waitOnExecutionContext(ctx);
      return res;
    },
  };
}

describe("handleRequest", () => {
  it("sends the proxy secret to the configured origin", async () => {
    const h = harness({ LP_PROXY_AUTH: PROXY_SECRET, LP_ORIGIN: "https://backend.test" });
    const res = await h.run(`${SITE}/`);
    await res.text();
    expect(res.status).toBe(200);
    expect(h.calls[0]?.headers.get(PROXY_AUTH_HEADER)).toBe(PROXY_SECRET);
    expect(new URL(h.calls[0]?.url ?? "").origin).toBe("https://backend.test");
  });

  it("uses the visitor host when LP_ORIGIN is empty", async () => {
    const h = harness({ LP_ORIGIN: "" });
    await (await h.run(`${SITE}/privacy`)).text();
    expect(new URL(h.calls[0]?.url ?? "").origin).toBe(SITE);
  });

  it("fails closed on invalid configuration", async () => {
    const errors = vi.spyOn(console, "error").mockImplementation(() => undefined);
    const h = harness({ LP_ORIGIN: "not a url" });
    const res = await h.run(`${SITE}/`);
    expect(res.status).toBe(500);
    expect(h.calls).toHaveLength(0);
    expect(errors).toHaveBeenCalledWith(expect.stringContaining('"event":"config_invalid"'));
  });

  it("rethrows unexpected errors", async () => {
    const broken = Object.defineProperty({ ...env }, "LP_ORIGIN", {
      get(): string {
        throw new RangeError("boom");
      },
    }) as Env;
    const deps: HandlerDeps = { fetch: async () => new Response(), cache: () => null };
    await expect(handleRequest(new Request(`${SITE}/`), broken, createExecutionContext(), deps)).rejects.toThrow(
      RangeError,
    );
  });

  it.each([
    ["missing shell", async () => new Response("nope", { status: 404 })],
    [
      "assets binding error",
      async () => {
        throw new Error("binding down");
      },
    ],
  ])("returns 503 when the static shell is unavailable (%s)", async (_name, assetFetch) => {
    const errors = vi.spyOn(console, "error").mockImplementation(() => undefined);
    const assets = { fetch: assetFetch, connect: () => undefined } as unknown as Fetcher;
    const res = await harness({ ASSETS: assets }).run(`${SITE}/`);
    expect(res.status).toBe(503);
    expect(res.headers.get("Retry-After")).toBe("30");
    expect(errors).toHaveBeenCalledWith(expect.stringContaining('"event":"shell_unavailable"'));
  });
});

describe("defaultDeps", () => {
  it("exposes the global fetch and the default cache", async () => {
    const spy = vi.spyOn(globalThis, "fetch").mockResolvedValue(new Response("ok"));
    const deps = defaultDeps();
    expect(await (await deps.fetch(new Request("https://x.test/"))).text()).toBe("ok");
    expect(spy).toHaveBeenCalledOnce();
    expect(deps.cache()).toBe(caches.default);
  });
});
