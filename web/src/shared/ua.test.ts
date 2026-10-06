import { describe, expect, it } from "vitest";
import samples from "../test/fixtures/user-agents.json";
import { classifyNavigator, classifyUA, isInApp } from "./ua";

describe("classifyUA (shared fixture)", () => {
  it("uses the full corpus shared with internal/uaclass", () => {
    expect(samples.cases.length).toBeGreaterThanOrEqual(40);
    expect(new Set(samples.cases.map((c) => c.name)).size).toBe(samples.cases.length);
  });

  it.each(samples.cases.map((c) => [c.name, c] as const))("%s", (_name, c) => {
    expect(classifyUA(c.ua)).toEqual({ inWeChat: c.inWeChat, inQQ: c.inQQ, mobile: c.mobile });
  });
});

describe("isInApp", () => {
  it.each([
    [{ inWeChat: true, inQQ: false, mobile: true }, true],
    [{ inWeChat: false, inQQ: true, mobile: true }, true],
    [{ inWeChat: false, inQQ: false, mobile: true }, false],
  ])("%o → %s", (c, want) => {
    expect(isInApp(c)).toBe(want);
  });
});

describe("classifyNavigator", () => {
  const mac = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.5";
  it.each([
    ["desktop Mac", mac, 0, false],
    ["iPadOS desktop mode", mac, 5, true],
    ["Android phone", "Mozilla/5.0 (Linux; Android 14) Mobile", 5, true],
  ])("%s", (_name, userAgent, maxTouchPoints, mobile) => {
    expect(classifyNavigator({ userAgent, maxTouchPoints }).mobile).toBe(mobile);
  });
});
