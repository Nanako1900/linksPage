import { describe, expect, it } from "vitest";
import { MAX_ACCEPT_LANGUAGE_LENGTH, normalizeAcceptLanguage, normalizeLang } from "../src/lang";

describe("normalizeLang", () => {
  const cases: ReadonlyArray<readonly [string | null, string]> = [
    [null, ""],
    ["", ""],
    ["en", "en"],
    ["zh-CN", "zh-CN"],
    ["zh-Hant-TW", "zh-Hant-TW"],
    ["e", ""],
    ["en_US", ""],
    ["en-", ""],
    ["<script>", ""],
    ["a-b-c-d-e", ""],
    [`en-${"a".repeat(40)}`, ""],
  ];
  it.each(cases)("%s → %s", (raw, want) => {
    expect(normalizeLang(raw)).toBe(want);
  });
});

describe("normalizeAcceptLanguage", () => {
  const cases: ReadonlyArray<readonly [string | null, string]> = [
    [null, ""],
    ["", ""],
    ["en", "en"],
    ["zh-CN,zh;q=0.9,en;q=0.8", "zh-cn,zh;q=0.9,en;q=0.8"],
    ["en;q=0.5, zh-CN", "zh-cn,en;q=0.5"],
    ["fr;q=0.8, de;q=0.8, en", "en,fr;q=0.8,de;q=0.8"],
    ["a, b, c, d", "a,b,c"],
    ["en;q=0, fr", "fr"],
    ["en;q=1.000, fr;q=0.123", "en,fr;q=0.123"],
    ["en; q=0.7", "en;q=0.7"],
    ["en;q=2, fr", "fr"],
    ["en;q=abc, fr", "fr"],
    ["en;level=1, fr", "fr"],
    ["*", "*"],
    ["en_US, de", "de"],
    ["\r\nX-Evil: 1", ""],
  ];
  it.each(cases)("%j → %s", (raw, want) => {
    expect(normalizeAcceptLanguage(raw)).toBe(want);
  });

  it("drops the entry cut by the length limit", () => {
    const head = `en,${"x".repeat(MAX_ACCEPT_LANGUAGE_LENGTH - 6)}`;
    expect(normalizeAcceptLanguage(`${head},fr-ca`)).toBe("en");
  });
});
