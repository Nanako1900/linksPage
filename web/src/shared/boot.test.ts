import { afterEach, describe, expect, it, vi } from "vitest";
// The inline boot script is owned by the Go renderer; keep it in lockstep
// with resolveAppearance (used by the future visitor-choice toggle).
import bootJs from "../../../internal/webui/static/boot.js?raw";
import { APPEARANCE_STORAGE_KEY, resolveAppearance } from "./theme";

function runBoot(mode: string, prefersDark: boolean, stored: string | null): string | null {
  const html = document.documentElement;
  html.setAttribute("data-appearance", mode);
  window.localStorage.clear();
  if (stored !== null) window.localStorage.setItem(APPEARANCE_STORAGE_KEY, stored);
  vi.stubGlobal("matchMedia", (q: string) => ({ matches: prefersDark && q === "(prefers-color-scheme: dark)" }));
  new Function(bootJs)();
  return html.getAttribute("data-appearance");
}

describe("boot.js parity", () => {
  afterEach(() => {
    document.documentElement.removeAttribute("data-appearance");
    document.documentElement.removeAttribute("data-appearance-mode");
    window.localStorage.clear();
  });

  const modes = ["light", "dark", "auto", "visitor-choice"];
  const stored = [null, "light", "dark", "junk"];
  for (const mode of modes) {
    for (const s of stored) {
      for (const dark of [false, true]) {
        it(`${mode} stored=${s} prefersDark=${dark}`, () => {
          expect(runBoot(mode, dark, s)).toBe(resolveAppearance(mode, dark, s));
          expect(document.documentElement.getAttribute("data-appearance-mode")).toBe(mode);
        });
      }
    }
  }
});
