import { describe, expect, it } from "vitest";
import { serializeScriptJSON } from "../src/json";

describe("serializeScriptJSON", () => {
  const cases: ReadonlyArray<readonly [unknown, string]> = [
    [{ a: "</script>" }, '{"a":"\\u003c/script\\u003e"}'],
    [{ a: "<!--" }, '{"a":"\\u003c!--"}'],
    [{ a: "&amp;" }, '{"a":"\\u0026amp;"}'],
    [{ a: "\u2028\u2029" }, '{"a":"\\u2028\\u2029"}'],
    [["plain", 1, null], '["plain",1,null]'],
  ];
  it.each(cases)("%j", (value, want) => {
    const out = serializeScriptJSON(value);
    expect(out).toBe(want);
    expect(JSON.parse(out)).toEqual(value);
    expect(out).not.toMatch(/[<>&\u2028\u2029]/);
  });

  it("rejects values JSON cannot represent", () => {
    expect(() => serializeScriptJSON(undefined)).toThrow(TypeError);
  });
});
