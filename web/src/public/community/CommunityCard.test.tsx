import { act, cleanup, fireEvent, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { IDS, sampleCommunity } from "../../test/fixtures";
import { PHONE, QQ_APP, renderWithEnv, WECHAT } from "../../test/render";
import { CommunityCard } from "./CommunityCard";

afterEach(cleanup);

const card = () => screen.getByRole("article");

describe("CommunityCard — live Discord", () => {
  it("renders name, platform tag, status line, channels and actions", () => {
    renderWithEnv(<CommunityCard c={sampleCommunity(IDS.discord)} />);
    const el = card();
    expect(within(el).getByRole("heading", { level: 3 }).textContent).toContain("Discord 主服务器");
    expect(within(el).getByText("Discord", { selector: ".lp-tag" })).toBeTruthy();
    expect(el.textContent).toContain("13 在线");
    expect(el.textContent).toContain("125 成员");
    expect(el.querySelector(".lp-dot")?.nextSibling?.textContent).toBe("13 在线");
    expect(within(el).getByRole("list", { name: "频道" }).textContent).toContain("开黑 1 号");
    const join = within(el).getByRole("link", { name: /加入/ });
    expect(join.getAttribute("href")).toBe("/go/discord");
    expect(within(el).getByRole("button", { name: /复制邀请/ })).toBeTruthy();
    expect(within(el).getByRole("button", { name: /二维码/ })).toBeTruthy();
    expect(el.getAttribute("data-state")).toBe("live");
  });

  it("renders the 48px icon with explicit dimensions", () => {
    renderWithEnv(<CommunityCard c={sampleCommunity(IDS.discord)} />);
    const img = card().querySelector("img");
    expect(img?.getAttribute("width")).toBe("48");
    expect(img?.getAttribute("height")).toBe("48");
  });

  it("opens the generated QR dialog", () => {
    const { env } = renderWithEnv(<CommunityCard c={sampleCommunity(IDS.discord)} />);
    fireEvent.click(screen.getByRole("button", { name: /二维码/ }));
    expect(env.openDialog).toHaveBeenCalledWith({
      kind: "qrGenerate",
      title: "Discord 主服务器",
      url: "https://links.example.com/go/discord",
    });
  });

  it.each([
    ["WeChat", WECHAT],
    ["QQ", QQ_APP],
  ])("inside %s intercepts join with the open-in-browser overlay", (_name, ua) => {
    const { env } = renderWithEnv(<CommunityCard c={sampleCommunity(IDS.discord)} />, { ua });
    const join = screen.getByRole("link", { name: /加入/ });
    const event = new MouseEvent("click", { bubbles: true, cancelable: true });
    act(() => {
      join.dispatchEvent(event);
    });
    expect(event.defaultPrevented).toBe(true);
    expect(env.openDialog).toHaveBeenCalledWith({ kind: "browser", url: "https://links.example.com/go/discord" });
  });

  it("intercepts the iframe facade in-app", () => {
    const { env } = renderWithEnv(<CommunityCard c={sampleCommunity(IDS.discord)} />, { ua: WECHAT });
    fireEvent.click(screen.getByRole("button", { name: "加载 Discord 小组件" }));
    expect(env.openDialog).toHaveBeenCalledWith({ kind: "browser", url: "https://links.example.com/c/discord" });
    expect(document.querySelector("iframe")).toBeNull();
  });

  it("loads the Discord iframe on click with the page theme and sandbox", () => {
    document.documentElement.setAttribute("data-appearance", "dark");
    renderWithEnv(<CommunityCard c={sampleCommunity(IDS.discord)} />);
    expect(document.querySelector("iframe")).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "加载 Discord 小组件" }));
    const frame = document.querySelector("iframe");
    expect(frame?.getAttribute("src")).toBe("https://discord.com/widget?id=1114391825336250432&theme=dark");
    expect(frame?.getAttribute("sandbox")).toBe(
      "allow-popups allow-popups-to-escape-sandbox allow-same-origin allow-scripts",
    );
    expect(frame?.getAttribute("title")).toBe("Discord 主服务器");
    document.documentElement.removeAttribute("data-appearance");
  });

  it("offers open-on-desktop on phones", () => {
    const { env } = renderWithEnv(<CommunityCard c={sampleCommunity(IDS.discord)} />, { ua: PHONE });
    expect(screen.queryByRole("button", { name: /二维码/ })).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: /在电脑上打开/ }));
    expect(env.openDialog).toHaveBeenCalledWith({ kind: "desktop", url: "https://links.example.com/c/discord" });
  });
});

describe("CommunityCard — states", () => {
  it("stale shows the relative update time", () => {
    renderWithEnv(<CommunityCard c={sampleCommunity(IDS.discordStale)} />, { locale: "en" });
    expect(card().textContent).toContain("Updated 1 hour ago");
    expect(card().textContent).toContain("4 online");
  });

  it("degraded without a target shows the invite-unavailable copy", () => {
    renderWithEnv(<CommunityCard c={sampleCommunity(IDS.discordDegraded)} />);
    expect(card().textContent).toContain("邀请暂不可用。");
    expect(screen.queryByRole("link", { name: /加入/ })).toBeNull();
  });

  it("pending shows a skeleton with an accessible label", () => {
    renderWithEnv(<CommunityCard c={sampleCommunity(IDS.kook, { live: { state: "pending", online: null } })} />);
    expect(card().querySelector(".lp-skeleton")).toBeTruthy();
    expect(screen.getByText("正在获取实时数据")).toBeTruthy();
    expect(screen.getByRole("link", { name: /加入/ })).toBeTruthy();
  });

  it("unavailable is dimmed with the admin text and no join", () => {
    renderWithEnv(<CommunityCard c={sampleCommunity(IDS.kookUnavailable)} />);
    expect(card().getAttribute("data-dimmed")).toBe("true");
    expect(card().textContent).toContain("服务器已关闭，请加入新的 KOOK。");
    expect(screen.queryByRole("link")).toBeNull();
  });

  it("unavailable without admin text uses site.copy, then built-in text", () => {
    const c = sampleCommunity(IDS.kookUnavailable, { unavailableText: {} });
    renderWithEnv(<CommunityCard c={c} />, { locale: "en" });
    expect(card().textContent).toContain("This community is currently unavailable.");
  });

  it("members-only status line without online count", () => {
    renderWithEnv(<CommunityCard c={sampleCommunity(IDS.kook, { live: { online: null, onlineSource: null } })} />);
    expect(card().querySelector(".lp-dot")).toBeNull();
    expect(card().textContent).toContain("107,345 成员");
  });

  it("static card without counts has no status line", () => {
    renderWithEnv(<CommunityCard c={sampleCommunity(IDS.qqNoLink)} />);
    expect(card().querySelector("p.tabular-nums")).toBeNull();
  });
});

describe("CommunityCard — QQ and WeChat", () => {
  it("QQ group with link: join group, copy number, QR dialog", () => {
    const { env } = renderWithEnv(<CommunityCard c={sampleCommunity(IDS.qqWithLink)} />);
    expect(screen.getByRole("link", { name: /加群/ }).getAttribute("href")).toBe("/go/qq-fans");
    expect(screen.getByText("123456789")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: /二维码/ }));
    expect(env.openDialog).toHaveBeenCalledWith(expect.objectContaining({ kind: "qr", title: "QQ 粉丝群" }));
  });

  it("QQ group inside WeChat: copy number primary, inline QR, no link", () => {
    renderWithEnv(<CommunityCard c={sampleCommunity(IDS.qqWithLink)} />, { ua: WECHAT });
    expect(screen.queryByRole("link")).toBeNull();
    expect(screen.getByRole("button", { name: /复制群号/ }).className).toContain("lp-btn-primary");
    const qr = screen.getByRole("img", { name: /二维码/ });
    expect(qr.getAttribute("src")).toBe("/media/q/01920000-0000-7000-8000-000000000301");
  });

  it("copies the group number", async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(navigator, "clipboard", { configurable: true, value: { writeText } });
    renderWithEnv(<CommunityCard c={sampleCommunity(IDS.qqNoLink)} />);
    await act(async () => {
      fireEvent.click(screen.getByRole("button", { name: /复制群号/ }));
    });
    expect(writeText).toHaveBeenCalledWith("87654321");
    expect(screen.getByRole("button", { name: /已复制/ })).toBeTruthy();
    Object.defineProperty(navigator, "clipboard", { configurable: true, value: undefined });
  });

  it("WeChat group: inline QR ≥240px with note, contact copy, no primary", () => {
    renderWithEnv(<CommunityCard c={sampleCommunity(IDS.wechat)} />);
    const qr = screen.getByRole("img", { name: /二维码/ });
    expect(qr.className).toContain("lp-qr-img");
    expect(qr.getAttribute("width")).toBe("430");
    expect(card().textContent).toContain("长按识别二维码");
    expect(card().textContent).toContain("更新于 10-05，7 天内有效");
    expect(card().textContent).toContain("群满或二维码失效时，加我拉你进群");
    expect(screen.getByRole("button", { name: /复制微信号/ })).toBeTruthy();
    expect(screen.queryByRole("link")).toBeNull();
  });
});
