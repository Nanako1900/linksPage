import { describe, expect, it } from "vitest";
import raw from "../wrangler.jsonc?raw";

/** Strips // line comments that are not inside strings (enough for wrangler.jsonc). */
function parseJSONC(text: string): Record<string, unknown> {
  const stripped = text
    .split("\n")
    .map((line) => line.replace(/^(\s*)\/\/.*$/, "$1"))
    .join("\n");
  return JSON.parse(stripped) as Record<string, unknown>;
}

type Assets = Record<string, unknown>;

describe("wrangler.jsonc", () => {
  const config = parseJSONC(raw);
  const assets = config.assets as Assets;
  const testAssets = ((config.env as Record<string, Record<string, unknown>>).test?.assets ?? {}) as Assets;

  it("runs the Worker first only for HTML (and the stale admin shell)", () => {
    expect(assets.run_worker_first).toEqual(["/", "/privacy", "/c/*", "/admin", "/admin/*"]);
    expect(assets.directory).toBe("../../web/dist");
    expect(assets.binding).toBe("ASSETS");
    expect(assets.not_found_handling).toBe("none");
  });

  it("keeps the test environment's assets identical except the directory", () => {
    const { directory: _a, ...prod } = assets;
    const { directory: _b, ...test } = testAssets;
    expect(test).toEqual(prod);
  });

  it("does not expose workers.dev (unreachable from mainland China)", () => {
    expect(config.workers_dev).toBe(false);
  });
});
