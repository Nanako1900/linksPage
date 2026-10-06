import { act, cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { samplePage } from "../../test/fixtures";
import { renderWithEnv } from "../../test/render";
import { BlockList, LinkRow, SectionHeading } from "./Blocks";
import { applyAppearance, Footer, homeHref } from "./Footer";
import { Identity } from "./Identity";
import { Icon, loadSimpleIcons, resetSimpleIcons } from "./icons/Icon";
import { Markdown } from "./Markdown";

afterEach(() => {
  cleanup();
  window.localStorage.clear();
});

describe("Markdown", () => {
  it("renders the subset as elements with safe links", () => {
    const { container } = renderWithEnv(
      <Markdown
        source={"Hi **there**, see [blog](https://b.example) and `code`\n\n- a\n- b\n\n3. c\n\n> q ~~x~~ *e*"}
      />,
    );
    const link = screen.getByRole("link", { name: "blog" });
    expect(link.getAttribute("rel")).toBe("nofollow noopener noreferrer");
    expect(link.getAttribute("target")).toBe("_blank");
    expect(container.querySelector("strong")?.textContent).toBe("there");
    expect(container.querySelector("code")?.textContent).toBe("code");
    expect(container.querySelectorAll("ul > li")).toHaveLength(2);
    expect(container.querySelector("ol")?.getAttribute("start")).toBe("3");
    expect(container.querySelector("blockquote del")?.textContent).toBe("x");
    expect(container.querySelector("blockquote em")?.textContent).toBe("e");
  });

  it("never creates HTML from the source", () => {
    const { container } = renderWithEnv(
      <Markdown source={'<img src=x onerror="alert(1)"> [x](javascript:alert)\nline'} />,
    );
    expect(container.querySelector("img")).toBeNull();
    expect(container.querySelector("a")).toBeNull();
    expect(container.querySelector("br")).toBeTruthy();
    expect(container.textContent).toContain("<img src=x");
  });

  it("renders nothing for blank input", () => {
    const { container } = renderWithEnv(<Markdown source="  " />);
    expect(container.innerHTML).toBe("");
  });

  it("omits start for lists beginning at 1", () => {
    const { container } = renderWithEnv(<Markdown source="1. a" />);
    expect(container.querySelector("ol")?.hasAttribute("start")).toBe(false);
  });
});

describe("SectionHeading", () => {
  it("pads the count with tabular numerals and keeps an accessible number", () => {
    renderWithEnv(<SectionHeading label="社区" count={3} />);
    const h = screen.getByRole("heading", { level: 2 });
    expect(h.querySelector(".tabular-nums")?.textContent).toBe("03");
    expect(h.querySelector(".tabular-nums")?.getAttribute("aria-hidden")).toBe("true");
    expect(h.textContent).toContain("(3)");
  });

  it("omits the count when absent", () => {
    renderWithEnv(<SectionHeading label="链接" />);
    expect(screen.getByRole("heading").textContent).toBe("链接");
  });
});

describe("BlockList", () => {
  it("renders every block kind in order with staggered entrance", () => {
    const page = samplePage();
    const { container } = renderWithEnv(<BlockList page={page} blocks={page.blocks} />);
    const wrappers = container.querySelectorAll(".lp-enter");
    expect(wrappers).toHaveLength(page.blocks.length);
    expect((wrappers[2] as HTMLElement).style.getPropertyValue("--i")).toBe("2");
    expect(screen.getAllByRole("article")).toHaveLength(8);
    expect(screen.getByRole("heading", { name: /链接/ })).toBeTruthy();
    expect(screen.getByRole("link", { name: "博客" }).getAttribute("href")).toBe("https://blog.example.com");
    expect(screen.getByRole("link", { name: /博客\s*blog\.example\.com/ }).getAttribute("href")).toBe("/go/blog");
    expect(screen.getByRole("link", { name: "Mastodon" }).getAttribute("rel")).toBe("me noopener");
  });

  it("skips blocks whose references are missing", () => {
    const page = samplePage();
    const blocks = [
      { id: "a", kind: "community" as const, communityId: "missing" },
      { id: "b", kind: "link" as const, linkId: "missing" },
    ];
    const { container } = renderWithEnv(<BlockList page={page} blocks={blocks} />);
    expect(container.querySelectorAll(".lp-enter")).toHaveLength(0);
  });
});

describe("LinkRow", () => {
  it("falls back to the host when the label is empty", () => {
    const link = { ...Object.values(samplePage().links)[0], label: {} } as Parameters<typeof LinkRow>[0]["link"];
    renderWithEnv(<LinkRow link={link} />);
    expect(screen.getByRole("link").textContent).toContain("blog.example.com");
    expect(screen.getByRole("link").getAttribute("rel")).toBeNull();
  });
});

describe("Identity", () => {
  it("shows avatar, display name, bio markdown and social links", () => {
    const page = samplePage();
    const social = Object.values(page.links).filter((l) => l.kind === "social");
    renderWithEnv(<Identity social={social} />);
    const img = document.querySelector("header img");
    expect(img?.getAttribute("width")).toBe("96");
    expect(screen.getByRole("heading", { level: 1 }).textContent).toBe("猎人小屋");
    expect(document.querySelector("header strong")?.textContent).toBe("周五晚上");
    expect(screen.getByRole("link", { name: "GitHub" })).toBeTruthy();
  });

  it("falls back to title and description", () => {
    const site = { ...samplePage().site, displayName: {}, bio: {}, avatar: null };
    renderWithEnv(<Identity social={[]} />, { site });
    expect(screen.getByRole("heading", { level: 1 }).textContent).toBe("猎人小屋");
    expect(screen.getByText("我们的 Discord、KOOK、QQ 和微信社区。")).toBeTruthy();
    expect(document.querySelector("header img")).toBeNull();
    expect(screen.queryByRole("list")).toBeNull();
  });
});

describe("Footer", () => {
  it("renders footer markdown, languages, privacy and powered-by", () => {
    renderWithEnv(<Footer />, { locale: "en" });
    expect(screen.getByRole("link", { name: "email" }).getAttribute("href")).toBe("mailto:hi@example.com");
    const nav = screen.getByRole("navigation", { name: "Language" });
    expect(within(nav).getByRole("link", { current: true }).getAttribute("hreflang")).toBe("en");
    expect(screen.getByRole("link", { name: "Privacy" }).getAttribute("href")).toBe("/privacy?lang=en");
    expect(screen.getByText("Powered by LinksPage")).toBeTruthy();
    expect(screen.queryByRole("group", { name: "Appearance" })).toBeNull();
  });

  it("hides powered-by and uses the plain privacy path for the default locale", () => {
    const site = { ...samplePage().site, showPoweredBy: false, footer: {} };
    renderWithEnv(<Footer />, { site });
    expect(screen.queryByText(/LinksPage/)).toBeNull();
    expect(screen.getByRole("link", { name: "隐私" }).getAttribute("href")).toBe("/privacy");
  });

  it("offers an appearance toggle for visitor-choice", () => {
    vi.stubGlobal("matchMedia", () => ({ matches: false }));
    const site = { ...samplePage().site, appearance: "visitor-choice" as const };
    renderWithEnv(<Footer />, { site });
    const group = screen.getByRole("group", { name: "外观" });
    expect(within(group).getByRole("button", { name: "自动" }).getAttribute("aria-pressed")).toBe("true");
    fireEvent.click(within(group).getByRole("button", { name: "深色" }));
    expect(document.documentElement.getAttribute("data-appearance")).toBe("dark");
    expect(window.localStorage.getItem("lp_appearance")).toBe("dark");
    expect(within(group).getByRole("button", { name: "深色" }).getAttribute("aria-pressed")).toBe("true");
    fireEvent.click(within(group).getByRole("button", { name: "自动" }));
    expect(window.localStorage.getItem("lp_appearance")).toBeNull();
    expect(document.documentElement.getAttribute("data-appearance")).toBe("light");
  });

  it("reads a stored appearance and survives storage errors", () => {
    window.localStorage.setItem("lp_appearance", "light");
    const site = { ...samplePage().site, appearance: "visitor-choice" as const };
    renderWithEnv(<Footer />, { site });
    expect(screen.getByRole("button", { name: "浅色" }).getAttribute("aria-pressed")).toBe("true");
    vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
      throw new Error("denied");
    });
    vi.stubGlobal("matchMedia", undefined);
    expect(() => applyAppearance("dark")).not.toThrow();
    expect(document.documentElement.getAttribute("data-appearance")).toBe("dark");
    cleanup();
    vi.spyOn(Storage.prototype, "getItem").mockImplementation(() => {
      throw new Error("denied");
    });
    renderWithEnv(<Footer />, { site });
    expect(screen.getByRole("button", { name: "自动" }).getAttribute("aria-pressed")).toBe("true");
  });

  it("homeHref keeps non-default languages", () => {
    expect(homeHref("zh-CN", "zh-CN")).toBe("/");
    expect(homeHref("en", "zh-CN")).toBe("/?lang=en");
  });
});

describe("Icon", () => {
  afterEach(() => resetSimpleIcons());

  it("renders simple-icons paths once the chunk loads", async () => {
    const { container } = renderWithEnv(
      <Icon icon={{ kind: "simple", name: "discord", url: null }} label="D" size={20} />,
    );
    expect(container.querySelector("svg")?.getAttribute("width")).toBe("20");
    await waitFor(() => expect(container.querySelector("svg path")).toBeTruthy());
  });

  it("falls back to a monogram for unknown slugs and missing icons", async () => {
    const { container } = renderWithEnv(
      <Icon icon={{ kind: "simple", name: "nope", url: null }} label="heybox" size={20} />,
    );
    await waitFor(() => expect(container.querySelector(".lp-monogram")?.textContent).toBe("H"));
    cleanup();
    const again = renderWithEnv(<Icon icon={null} label="" size={20} />);
    expect(again.container.textContent).toBe("·");
  });

  it("renders builtin marks and uploaded images", () => {
    const { container } = renderWithEnv(
      <>
        <Icon icon={{ kind: "builtin", name: "kook", url: null }} label="K" size={24} />
        <Icon icon={{ kind: "builtin", name: "unknown", url: null }} label="u" size={24} />
        <Icon icon={{ kind: "media", name: "a.png", url: "/media/u/a.png" }} label="M" size={24} />
      </>,
    );
    expect(container.querySelector("svg path")?.getAttribute("fill-rule")).toBe("evenodd");
    expect(container.querySelector(".lp-monogram")?.textContent).toBe("U");
    expect(container.querySelector("img")?.getAttribute("src")).toBe("/media/u/a.png");
  });

  it("caches the chunk and tolerates load failures", async () => {
    const failing = vi.fn().mockRejectedValue(new Error("offline"));
    await expect(loadSimpleIcons(failing)).resolves.toEqual({});
    await loadSimpleIcons(failing);
    expect(failing).toHaveBeenCalledOnce();
  });

  it("ignores results after unmount", async () => {
    let resolve: (m: { SIMPLE_ICONS: Record<string, string> }) => void = () => {};
    void loadSimpleIcons(() => new Promise((r) => (resolve = r)));
    const { unmount } = renderWithEnv(<Icon icon={{ kind: "simple", name: "x", url: null }} label="x" size={16} />);
    unmount();
    await act(async () => resolve({ SIMPLE_ICONS: { x: "M0 0" } }));
  });
});
