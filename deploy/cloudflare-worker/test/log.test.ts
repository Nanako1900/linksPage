import { afterEach, describe, expect, it, vi } from "vitest";
import { errorMessage, logError } from "../src/log";

afterEach(() => {
  vi.restoreAllMocks();
});

describe("errorMessage", () => {
  it.each([
    [new TypeError("bad"), "TypeError: bad"],
    ["text", "text"],
    [42, "42"],
  ])("%s", (err, want) => {
    expect(errorMessage(err)).toBe(want);
  });
});

describe("logError", () => {
  it("writes one JSON line", () => {
    const spy = vi.spyOn(console, "error").mockImplementation(() => undefined);
    logError("evt");
    logError("evt2", { path: "/", n: 1 });
    expect(spy.mock.calls.map((c) => JSON.parse(String(c[0])))).toEqual([
      { level: "error", event: "evt" },
      { level: "error", event: "evt2", path: "/", n: 1 },
    ]);
  });
});
