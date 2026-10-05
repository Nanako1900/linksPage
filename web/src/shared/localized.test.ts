import { describe, expect, it } from "vitest";
import { localeName, pickLocalized, resolveLocale } from "./localized";

describe("pickLocalized", () => {
  const text = { "zh-CN": "你好", en: "Hello", ja: "" };
  it.each([
    ["zh-CN", "zh-CN", "你好"],
    ["fr", "zh-CN", "你好"],
    ["ja", "fr", "Hello"],
  ])("locale %s default %s → %s", (locale, def, want) => {
    expect(pickLocalized(text, locale, def)).toBe(want);
  });
  it("handles missing text", () => {
    expect(pickLocalized(undefined, "en", "en")).toBe("");
    expect(pickLocalized({ fr: "Salut" }, "en", "de")).toBe("");
  });
});

describe("resolveLocale", () => {
  const base = { enabled: ["zh-CN", "en"], defaultLocale: "zh-CN" };
  it("prefers ?lang=", () => {
    expect(resolveLocale({ ...base, query: "en", stored: "zh-CN" })).toBe("en");
  });
  it("ignores disabled query and uses storage", () => {
    expect(resolveLocale({ ...base, query: "fr", stored: "en" })).toBe("en");
  });
  it("matches browser languages by primary subtag", () => {
    expect(resolveLocale({ ...base, browser: ["fr-FR", "en-GB"] })).toBe("en");
    expect(resolveLocale({ ...base, browser: ["zh-CN"] })).toBe("zh-CN");
  });
  it("falls back to the default", () => {
    expect(resolveLocale({ ...base, browser: ["fr"] })).toBe("zh-CN");
    expect(resolveLocale(base)).toBe("zh-CN");
  });
});

describe("localeName", () => {
  it("returns the endonym", () => {
    expect(localeName("en")).toBe("English");
    expect(localeName("zh-CN")).toMatch(/^中文/);
  });
  it("falls back to the tag for invalid locales", () => {
    expect(localeName("not a locale")).toBe("not a locale");
  });
});
