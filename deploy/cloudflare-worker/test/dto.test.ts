import { describe, expect, it } from "vitest";
import { MAX_HEADER_VALUE_LENGTH, parseRenderDTO, parseRenderEnvelope } from "../src/dto";
import { renderDTO } from "./helpers";

describe("parseRenderDTO", () => {
  it("accepts a 200 rendering and returns a frozen copy", () => {
    const input = renderDTO();
    const got = parseRenderDTO(input);
    expect(got).toEqual(input);
    expect(got).not.toBe(input);
    expect(Object.isFrozen(got)).toBe(true);
  });

  it("accepts a 404 rendering with null data", () => {
    expect(parseRenderDTO(renderDTO({ status: 404, data: null }))?.status).toBe(404);
  });

  const invalid: ReadonlyArray<readonly [string, unknown]> = [
    ["null", null],
    ["array", []],
    ["string", "x"],
    ["status 500", { ...renderDTO(), status: 500 }],
    ["200 with null data", { ...renderDTO(), data: null }],
    ["200 with array data", { ...renderDTO(), data: [] }],
    ["404 with data", { ...renderDTO(), status: 404 }],
    ["missing lang", { ...renderDTO(), lang: undefined }],
    ["bad lang", { ...renderDTO(), lang: "en US" }],
    ["bad appearance", { ...renderDTO(), appearance: "Dark!" }],
    ["head not string", { ...renderDTO(), head: 1 }],
    ["fallback not string", { ...renderDTO(), fallback: null }],
    ["empty csp", { ...renderDTO(), csp: "  " }],
    ["csp with newline", { ...renderDTO(), csp: "default-src 'self'\r\nX-Evil: 1" }],
    ["csp too long", { ...renderDTO(), csp: "a".repeat(MAX_HEADER_VALUE_LENGTH + 1) }],
    ["csp non-ascii", { ...renderDTO(), csp: "default-src 'self' é" }],
    ["report-only not string", { ...renderDTO(), cspReportOnly: null }],
    ["etag with newline", { ...renderDTO(), etag: 'W/"a"\n' }],
  ];
  it.each(invalid)("rejects %s", (_name, value) => {
    expect(parseRenderDTO(value)).toBeNull();
  });
});

describe("parseRenderEnvelope", () => {
  it("unwraps {data}", () => {
    expect(parseRenderEnvelope({ data: renderDTO() })?.lang).toBe("en");
  });
  it.each([null, [], { dto: renderDTO() }, { data: null }])("rejects %j", (value) => {
    expect(parseRenderEnvelope(value)).toBeNull();
  });
});
