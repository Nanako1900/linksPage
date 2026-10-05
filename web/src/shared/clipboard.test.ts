import { afterEach, describe, expect, it, vi } from "vitest";
import { copyText } from "./clipboard";

function stubClipboard(writeText: ((t: string) => Promise<void>) | undefined) {
  Object.defineProperty(window.navigator, "clipboard", {
    configurable: true,
    value: writeText ? { writeText } : undefined,
  });
}

function stubExecCommand(impl: ((cmd: string) => boolean) | undefined) {
  Object.defineProperty(document, "execCommand", { configurable: true, writable: true, value: impl });
}

afterEach(() => {
  stubClipboard(undefined);
  stubExecCommand(undefined);
  document.body.replaceChildren();
});

describe("copyText", () => {
  it("uses the Clipboard API when available", async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    stubClipboard(writeText);
    await expect(copyText("hello")).resolves.toBe(true);
    expect(writeText).toHaveBeenCalledWith("hello");
  });

  it("falls back to execCommand when the Clipboard API rejects", async () => {
    stubClipboard(vi.fn().mockRejectedValue(new Error("denied")));
    let selected = "";
    stubExecCommand((cmd) => {
      const area = document.querySelector("textarea");
      selected = area?.value ?? "";
      return cmd === "copy";
    });
    await expect(copyText("123456789")).resolves.toBe(true);
    expect(selected).toBe("123456789");
    expect(document.querySelector("textarea")).toBeNull();
  });

  it("restores focus after the legacy copy", async () => {
    const button = document.createElement("button");
    document.body.append(button);
    button.focus();
    stubExecCommand(() => true);
    await copyText("x");
    expect(document.activeElement).toBe(button);
  });

  it.each([
    ["no API at all", undefined],
    ["execCommand returns false", () => false],
    [
      "execCommand throws",
      () => {
        throw new Error("nope");
      },
    ],
  ])("returns false when %s", async (_name, impl) => {
    stubExecCommand(impl);
    await expect(copyText("x")).resolves.toBe(false);
    expect(document.querySelector("textarea")).toBeNull();
  });
});
