import { describe, expect, it } from "vitest";
import { contrastRatio, luminance, resolveAppearance } from "./theme";

describe("resolveAppearance", () => {
  it.each([
    ["light", true, null, "light"],
    ["dark", false, null, "dark"],
    ["auto", true, null, "dark"],
    ["auto", false, null, "light"],
    ["visitor-choice", false, "dark", "dark"],
    ["visitor-choice", true, "light", "light"],
    ["visitor-choice", true, "junk", "dark"],
    ["unknown", false, null, "light"],
  ] as const)("%s prefersDark=%s stored=%s → %s", (mode, dark, stored, want) => {
    expect(resolveAppearance(mode, dark, stored)).toBe(want);
  });
});

describe("contrast", () => {
  it("computes luminance bounds", () => {
    expect(luminance("#000")).toBe(0);
    expect(luminance("#ffffff")).toBeCloseTo(1);
    expect(luminance("red")).toBeNull();
  });
  it("matches WCAG reference values", () => {
    expect(contrastRatio("#000000", "#ffffff")).toBeCloseTo(21);
    expect(contrastRatio("#ffffff", "#000000")).toBeCloseTo(21);
    // Signal Paper accent with white text must pass AA (≥4.5:1).
    expect(contrastRatio("#5b4ad8", "#ffffff")).toBeGreaterThan(4.5);
  });
  it("returns null for invalid input", () => {
    expect(contrastRatio("#12", "#fff")).toBeNull();
  });
});
