import { act, cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { PublicPage } from "../shared/types/public";
import { samplePage } from "../test/fixtures";
import { DESKTOP, PHONE, WECHAT } from "../test/render";
import { App, findCommunity, pageTitle, splitSocial } from "./App";
import { ErrorNotice } from "./ErrorNotice";
import type { Route } from "./route";

afterEach(() => {
  cleanup();
  document.title = "";
});

/** Live polling never fires within a test; keep it off the network anyway. */
const liveDeps = {
  fetch: vi.fn().mockResolvedValue(new Response(null, { status: 304 })),
  doc: document,
  now: Date.now,
};

function renderApp(route: Route, opts: { locale?: string; page?: PublicPage; ua?: typeof DESKTOP } = {}) {
  return render(
    <App
      page={opts.page ?? samplePage()}
      route={route}
      locale={opts.locale ?? "zh-CN"}
      ua={opts.ua ?? DESKTOP}
      liveDeps={liveDeps}
    />,
  );
}

describe("App — home", () => {
  it("renders identity, blocks and footer", () => {
    renderApp({ kind: "home" });
    expect(screen.getByRole("heading", { level: 1 }).textContent).toBe("猎人小屋");
    const main = screen.getByRole("main");
    expect(within(main).getAllByRole("article")).toHaveLength(8);
    expect(within(main).getByRole("heading", { name: /社区/ }).textContent).toContain("08");
    expect(screen.queryByRole("link", { name: /返回首页/ })).toBeNull();
    expect(screen.getByRole("contentinfo")).toBeTruthy();
    expect(document.title).toBe("猎人小屋");
  });

  it("moves social rows into the identity column", () => {
    renderApp({ kind: "home" }, { locale: "en" });
    const header = screen.getByRole("banner");
    expect(within(header).getByRole("link", { name: "GitHub" }).getAttribute("href")).toBe("/go/github");
    expect(within(screen.getByRole("main")).queryByRole("link", { name: "GitHub" })).toBeNull();
    expect(document.title).toBe("Hunter's Lodge");
  });

  it("shows the coming-soon placeholder without blocks", () => {
    renderApp({ kind: "home" }, { page: { ...samplePage(), blocks: [] } });
    expect(screen.getByText("配置社区后会显示在这里。")).toBeTruthy();
  });

  it("opens and closes the open-in-browser overlay inside WeChat", async () => {
    renderApp({ kind: "home" }, { ua: WECHAT });
    const join = within(screen.getAllByRole("article")[0] as HTMLElement).getByRole("link", { name: /加入/ });
    fireEvent.click(join);
    const overlay = screen.getByRole("dialog", { name: "请在浏览器中打开" });
    expect(overlay.textContent).toContain("https://links.example.com/go/discord");
    await act(async () => {
      fireEvent.click(within(overlay).getByRole("button", { name: "关闭" }));
    });
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("opens the open-on-desktop dialog on phones", () => {
    renderApp({ kind: "home" }, { ua: PHONE });
    fireEvent.click(screen.getAllByRole("button", { name: /在电脑上打开/ })[0] as HTMLElement);
    expect(screen.getByRole("dialog", { name: "在电脑上打开" }).textContent).toContain(
      "https://links.example.com/c/discord",
    );
  });
});

describe("App — other routes", () => {
  it("pins the shared community first on /c/{slug}", () => {
    renderApp({ kind: "community", slug: "wechat" });
    const pinned = screen.getByRole("region", { name: "分享给你的社区" });
    expect(within(pinned).getByRole("article").textContent).toContain("微信群");
    const articles = screen.getAllByRole("article");
    expect(articles).toHaveLength(8);
    expect(articles[0]?.textContent).toContain("微信群");
    expect(screen.getByRole("link", { name: /返回首页/ }).getAttribute("href")).toBe("/");
    expect(document.title).toBe("微信群 · 猎人小屋");
  });

  it("shows the 404 view for unknown community slugs", () => {
    renderApp({ kind: "community", slug: "nope" }, { locale: "en" });
    expect(screen.getByRole("region").textContent).toContain("Nothing here.");
    expect(screen.getByRole("link", { name: /Back to home/ }).getAttribute("href")).toBe("/?lang=en");
    expect(document.title).toBe("Hunter's Lodge");
  });

  it("falls back to the built-in 404 copy", () => {
    const page = samplePage();
    renderApp({ kind: "notFound" }, { page: { ...page, site: { ...page.site, notFound: {} } } });
    expect(screen.getByText("页面不存在。")).toBeTruthy();
  });

  it("renders the privacy placeholder", () => {
    renderApp({ kind: "privacy" });
    expect(screen.getByRole("region", { name: "隐私" }).textContent).toContain("这里将说明访问统计的方式。");
  });
});

describe("helpers", () => {
  it("findCommunity matches by slug", () => {
    expect(findCommunity(samplePage(), "kook")?.platform).toBe("kook");
    expect(findCommunity(samplePage(), "missing")).toBeUndefined();
  });

  it("splitSocial dedupes link ids and drops unknown ones", () => {
    const page = samplePage();
    const blocks = [
      ...page.blocks,
      { id: "x", kind: "social_row" as const, linkIds: ["01920000-0000-7000-8000-000000000202", "missing"] },
    ];
    const { content, social } = splitSocial({ ...page, blocks });
    expect(social.map((l) => l.slug)).toEqual(["github", "mastodon"]);
    expect(content.blocks.some((b) => b.kind === "social_row")).toBe(false);
  });

  it("pageTitle uses the community name on /c pages", () => {
    expect(pageTitle(samplePage(), { kind: "community", slug: "kook" }, "en")).toBe("KOOK voice · Hunter's Lodge");
    expect(pageTitle(samplePage(), { kind: "privacy" }, "zh-CN")).toBe("猎人小屋");
  });

  it("renders the error notice as an alert", () => {
    render(<ErrorNotice message="boom" />);
    expect(screen.getByRole("alert").textContent).toBe("boom");
  });
});
