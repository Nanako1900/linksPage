import { describe, expect, it } from "vitest";
import { type MessageKey, messages } from "./messages";
import { createT } from "./t";

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
  it("returns the key when nothing matches", () => {
    expect(createT("en", "en")("missing" as MessageKey)).toBe("missing");
  });
  it("built-in catalogs translate the same keys", () => {
    const keys = (c: object) => Object.keys(c).sort();
    expect(keys(messages["zh-CN"])).toEqual(keys(messages.en));
    for (const v of [...Object.values(messages.en), ...Object.values(messages["zh-CN"])]) expect(v).not.toBe("");
  });
});
