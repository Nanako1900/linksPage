import { describe, expect, it } from "vitest";
import { ConfigError, DEFAULT_RENDER_TIMEOUT_MS, type Env, originFor, parseConfig } from "../src/env";
import { PROXY_SECRET } from "./helpers";

const base = { ASSETS: {} as Fetcher } satisfies Env;

describe("parseConfig", () => {
  it("applies defaults", () => {
    const cfg = parseConfig(base);
    expect(cfg).toEqual({ originBase: null, proxyAuth: "", renderTimeoutMs: DEFAULT_RENDER_TIMEOUT_MS });
    expect(Object.isFrozen(cfg)).toBe(true);
  });

  it.each(["http://localhost:8787", "http://127.0.0.1:8080", "http://[::1]:8080", "http://LOCALHOST"])(
    "accepts plain http only for loopback: %s",
    (origin) => {
      expect(parseConfig({ ...base, LP_ORIGIN: origin }).originBase).toBe(new URL(origin).origin);
    },
  );

  it("accepts valid values", () => {
    const cfg = parseConfig({
      ...base,
      LP_ORIGIN: " https://origin.example.com/ ",
      LP_PROXY_AUTH: PROXY_SECRET,
      LP_RENDER_TIMEOUT_MS: "2500",
    });
    expect(cfg).toEqual({ originBase: "https://origin.example.com", proxyAuth: PROXY_SECRET, renderTimeoutMs: 2500 });
  });

  const invalid: ReadonlyArray<readonly [string, Partial<Env>, RegExp]> = [
    ["relative origin", { LP_ORIGIN: "origin.example.com" }, /absolute/],
    ["ftp origin", { LP_ORIGIN: "ftp://origin.example.com" }, /https/],
    ["plain http origin", { LP_ORIGIN: "http://10.0.0.1:8080" }, /https \(http only for localhost\)/],
    ["plain http public host", { LP_ORIGIN: "http://origin.example.com" }, /https/],
    ["origin credentials", { LP_ORIGIN: "https://u:p@origin.example.com" }, /credentials/],
    ["origin path", { LP_ORIGIN: "https://origin.example.com/base" }, /path/],
    ["origin query", { LP_ORIGIN: "https://origin.example.com/?a=1" }, /path/],
    ["short secret", { LP_PROXY_AUTH: "short" }, /32 bytes/],
    ["secret with space", { LP_PROXY_AUTH: `${PROXY_SECRET} x` }, /printable/],
    ["timeout text", { LP_RENDER_TIMEOUT_MS: "4s" }, /integer/],
    ["timeout too small", { LP_RENDER_TIMEOUT_MS: "10" }, /between/],
    ["timeout too large", { LP_RENDER_TIMEOUT_MS: "60000" }, /between/],
  ];
  it.each(invalid)("rejects %s", (_name, overrides, message) => {
    expect(() => parseConfig({ ...base, ...overrides })).toThrow(ConfigError);
    expect(() => parseConfig({ ...base, ...overrides })).toThrow(message);
  });
});

describe("originFor", () => {
  it("falls back to the request origin", () => {
    const url = new URL("https://links.example.com/c/x?lang=en");
    expect(originFor(parseConfig(base), url)).toBe("https://links.example.com");
    expect(originFor(parseConfig({ ...base, LP_ORIGIN: "http://localhost:8080" }), url)).toBe("http://localhost:8080");
  });
});
