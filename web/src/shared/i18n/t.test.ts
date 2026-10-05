import { describe, expect, it } from "vitest";
import { type MessageKey, messages } from "./messages";
import { createT, fmt } from "./t";

describe("createT", () => {
  it("uses the requested locale", () => {
    expect(createT("zh-CN", "en")("privacy")).toBe("隐私");
  });
  it("falls back to the default locale, then en", () => {
    expect(createT("ja", "zh-CN")("privacy")).toBe("隐私");
    expect(createT("ja", "fr")("privacy")).toBe("Privacy");
  });
  it("applies overrides first", () => {
    expect(createT("en", "en", { en: { privacy: "Data use" } })("privacy")).toBe("Data use");
  });
  it("ignores empty overrides", () => {
    const t = createT("en", "en", { en: { privacy: "" } });
    expect(t("privacy")).toBe("Privacy");
  });
  it("prefers any admin override over built-in text (server CopyOverrides.Get order)", () => {
    const copy = { "zh-CN": { openInBrowser: "点右上角" } };
    expect(createT("en", "zh-CN", copy)("openInBrowser")).toBe("点右上角");
    expect(createT("en", "en", copy)("openInBrowser")).toBe(messages.en.openInBrowser);
    expect(createT("ja", "fr", { en: { inviteUnavailable: "Gone" } })("inviteUnavailable")).toBe("Gone");
  });
  it("returns the key when nothing matches", () => {
    expect(createT("en", "en")("missing" as MessageKey)).toBe("missing");
  });
  it("built-in catalogs translate the same keys", () => {
    const keys = (c: object) => Object.keys(c).sort();
    expect(keys(messages["zh-CN"])).toEqual(keys(messages.en));
    for (const v of [...Object.values(messages.en), ...Object.values(messages["zh-CN"])]) expect(v).not.toBe("");
  });
});

describe("fmt", () => {
  it.each([
    ["{n} 在线", { n: 13 }, "13 在线"],
    ["Updated {ago}", { ago: "5 minutes ago" }, "Updated 5 minutes ago"],
    ["{a}{b}{a}", { a: "x", b: 0 }, "x0x"],
    ["keep {unknown}", {}, "keep {unknown}"],
  ])("%s", (template, vars, want) => {
    expect(fmt(template, vars)).toBe(want);
  });
});
