import { describe, expect, it, vi } from "vitest";
import { installPreloadRecovery } from "./preloadRecovery";

function setup(storage: () => Pick<Storage, "getItem" | "setItem">) {
  let handler: ((e: Event) => void) | undefined;
  const reload = vi.fn();
  installPreloadRecovery({
    addEventListener: (_type, fn) => {
      handler = fn;
    },
    storage,
    reload,
  });
  const fire = () => {
    const e = new Event("vite:preloadError", { cancelable: true });
    handler?.(e);
    return e;
  };
  return { fire, reload };
}

function memoryStorage(): Pick<Storage, "getItem" | "setItem"> {
  const m = new Map<string, string>();
  return { getItem: (k) => m.get(k) ?? null, setItem: (k, v) => void m.set(k, v) };
}

describe("installPreloadRecovery", () => {
  it("reloads once per session", () => {
    const s = memoryStorage();
    const { fire, reload } = setup(() => s);
    expect(fire().defaultPrevented).toBe(true);
    fire();
    expect(reload).toHaveBeenCalledOnce();
  });

  it("does not reload without usable storage", () => {
    const { fire, reload } = setup(() => {
      throw new Error("denied");
    });
    expect(fire().defaultPrevented).toBe(false);
    expect(reload).not.toHaveBeenCalled();
  });
});
