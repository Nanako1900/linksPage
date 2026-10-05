import { act } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { LANG_STORAGE_KEY } from "../shared/localized";
import { sampleBootstrap } from "../test/fixtures";
import { localeFor, rememberLocaleChoice, start } from "./start";

function embed(json: string) {
  const el = document.createElement("script");
  el.id = "lp-data";
  el.type = "application/json";
  el.textContent = json;
  document.body.append(el);
}

const loc = (search: string) => ({ search }) as Location;

describe("start", () => {
  let root: HTMLElement;

  beforeEach(() => {
    root = document.createElement("div");
    root.innerHTML = '<main class="lp-shell"><h1>fallback</h1></main>';
    document.body.append(root);
    window.history.replaceState(null, "", "/?lang=en");
  });

  afterEach(() => {
    document.body.replaceChildren();
    document.documentElement.lang = "";
    window.localStorage.clear();
  });

  it("replaces the fallback markup with the app and remembers ?lang", async () => {
    embed(JSON.stringify({ data: sampleBootstrap() }));
    await act(() => start(root));
    expect(root.querySelector("h1")?.textContent).toBe("My Communities");
    expect(root.querySelector(".lp-shell")).toBeNull();
    expect(document.documentElement.lang).toBe("en");
    expect(window.localStorage.getItem(LANG_STORAGE_KEY)).toBe("en");
  });

  it("keeps the document title in the resolved locale", async () => {
    document.title = "我的社区";
    embed(JSON.stringify({ data: sampleBootstrap() }));
    await act(() => start(root));
    expect(document.title).toBe("My Communities");
  });

  it("reports the failure and keeps the server fallback when bootstrap cannot be loaded", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response("{}", { status: 503 })));
    const report = vi.fn();
    vi.stubGlobal("reportError", report);
    document.documentElement.lang = "zh-CN";
    await act(() => start(root));
    expect(root.querySelector('[role="alert"]')?.textContent).toBe("页面数据加载失败，请刷新重试。");
    expect(root.querySelector(".lp-shell h1")?.textContent).toBe("fallback");
    expect(report).toHaveBeenCalledOnce();
  });

  it("falls back to English copy when <html lang> is unset", async () => {
    vi.stubGlobal("fetch", vi.fn().mockRejectedValue(new TypeError("network")));
    vi.stubGlobal("reportError", vi.fn());
    await act(() => start(root));
    expect(root.querySelector('[role="alert"]')?.textContent).toBe(
      "The page data could not be loaded. Please refresh.",
    );
  });
});

describe("localeFor", () => {
  it("reads ?lang and falls back to the default locale list", () => {
    const data = { ...sampleBootstrap(), site: { ...sampleBootstrap().site, locales: null } };
    expect(localeFor(data, loc("?lang=en"), { languages: [] })).toBe("zh-CN");
    expect(localeFor(sampleBootstrap(), loc("?lang=en"), { languages: [] })).toBe("en");
  });

  it("prefers the stored choice over browser languages", () => {
    window.localStorage.setItem(LANG_STORAGE_KEY, "en");
    expect(localeFor(sampleBootstrap(), loc(""), { languages: ["zh-CN"] })).toBe("en");
    window.localStorage.clear();
  });

  it("tolerates storage that throws", () => {
    vi.spyOn(Storage.prototype, "getItem").mockImplementation(() => {
      throw new Error("denied");
    });
    expect(localeFor(sampleBootstrap(), loc(""), { languages: ["en-US"] })).toBe("en");
  });
});

describe("rememberLocaleChoice", () => {
  afterEach(() => window.localStorage.clear());

  it("stores only enabled locales from ?lang", () => {
    rememberLocaleChoice(sampleBootstrap(), loc("?lang=fr"));
    expect(window.localStorage.getItem(LANG_STORAGE_KEY)).toBeNull();
    rememberLocaleChoice(sampleBootstrap(), loc(""));
    expect(window.localStorage.getItem(LANG_STORAGE_KEY)).toBeNull();
    rememberLocaleChoice(sampleBootstrap(), loc("?lang=zh-CN"));
    expect(window.localStorage.getItem(LANG_STORAGE_KEY)).toBe("zh-CN");
  });

  it("tolerates storage that throws", () => {
    vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
      throw new Error("quota");
    });
    expect(() => rememberLocaleChoice(sampleBootstrap(), loc("?lang=en"))).not.toThrow();
  });
});
