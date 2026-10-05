import { describe, expect, it } from "vitest";
import { absoluteUrl, displayHost, formatCount, padCount, relativeAgo } from "./format";

describe("padCount", () => {
  it.each([
    [0, "00"],
    [3, "03"],
    [12, "12"],
    [120, "120"],
    [-1, "00"],
    [2.7, "02"],
  ])("%d → %s", (n, want) => {
    expect(padCount(n)).toBe(want);
  });
});

describe("formatCount", () => {
  it.each([
    [107345, "en", "107,345"],
    [13, "zh-CN", "13"],
    [1000, "not a locale!", "1,000"],
  ])("%d in %s", (n, locale, want) => {
    expect(formatCount(n, locale)).toBe(want);
  });
});

describe("relativeAgo", () => {
  const now = Date.parse("2026-10-05T13:46:30Z");
  it.each([
    ["2026-10-05T13:46:00Z", "en", "this minute"],
    ["2026-10-05T12:10:00Z", "en", "1 hour ago"],
    ["2026-10-05T13:30:00Z", "en", "16 minutes ago"],
    ["2026-10-05T13:30:00Z", "zh-CN", "16分钟前"],
    ["2026-10-04T01:00:00Z", "en", "36 hours ago"],
    ["2026-10-01T00:00:00Z", "en", "4 days ago"],
    ["2026-10-05T14:00:00Z", "en", "this minute"],
  ])("%s (%s)", (iso, locale, want) => {
    expect(relativeAgo(iso, now, locale)).toBe(want);
  });

  it("returns null for invalid timestamps", () => {
    expect(relativeAgo("yesterday", now, "en")).toBeNull();
  });
});

describe("displayHost", () => {
  it.each([
    ["https://www.blog.example.com/a", "blog.example.com"],
    ["https://mastodon.social/@example", "mastodon.social"],
    ["mailto:hi@example.com", "hi@example.com"],
    ["not a url", "not a url"],
  ])("%s", (url, want) => {
    expect(displayHost(url)).toBe(want);
  });
});

describe("absoluteUrl", () => {
  it.each([
    ["https://links.example.com", "/c/discord", "https://links.example.com/c/discord"],
    ["https://links.example.com/", "/go/x", "https://links.example.com/go/x"],
    ["https://links.example.com", "go/x", "https://links.example.com/go/x"],
  ])("%s + %s", (base, path, want) => {
    expect(absoluteUrl(base, path)).toBe(want);
  });
});
