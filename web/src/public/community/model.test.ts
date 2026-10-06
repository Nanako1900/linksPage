import { describe, expect, it } from "vitest";
import type { CommunityView } from "../../shared/types/public";
import type { UAClass } from "../../shared/ua";
import { IDS, sampleCommunity, samplePage } from "../../test/fixtures";
import { type CardModel, cardModel, type ModelEnv } from "./model";

const DESKTOP: UAClass = { inWeChat: false, inQQ: false, mobile: false };
const PHONE: UAClass = { inWeChat: false, inQQ: false, mobile: true };
const WECHAT: UAClass = { inWeChat: true, inQQ: false, mobile: true };
const QQ: UAClass = { inWeChat: false, inQQ: true, mobile: true };

function model(c: CommunityView, ua: UAClass, unavailableText = ""): CardModel {
  const env: ModelEnv = {
    ua,
    baseUrl: "https://links.example.com",
    platform: samplePage().platforms[c.platform],
    texts: { inviteUnavailable: "INVITE_UNAVAILABLE", communityUnavailable: "COMMUNITY_UNAVAILABLE", unavailableText },
  };
  return cardModel(c, env);
}

const kinds = (m: CardModel) => m.secondary.map((a) => (a.kind === "copy" ? `copy:${a.label}` : a.kind));

describe("cardModel — discord", () => {
  const discord = sampleCommunity(IDS.discord);

  it("desktop: join, copy invite, generated QR, embed", () => {
    const m = model(discord, DESKTOP);
    expect(m.primary).toEqual({
      kind: "join",
      href: "/go/discord",
      label: "join",
      intercept: false,
      copyUrl: "https://links.example.com/go/discord",
    });
    expect(kinds(m)).toEqual(["copy:copyInvite", "qrGenerate"]);
    expect(m.embed).toBe(true);
    expect(m.embedIntercept).toBe(false);
    expect(m.notice).toBeNull();
  });

  it("phone browser: open on desktop instead of QR", () => {
    const m = model(discord, PHONE);
    expect(kinds(m)).toEqual(["copy:copyInvite", "desktop"]);
    expect(m.secondary[1]).toEqual({ kind: "desktop", url: "https://links.example.com/c/discord" });
  });

  it.each([
    ["WeChat", WECHAT],
    ["QQ", QQ],
  ])("inside %s: join and embed are intercepted", (_name, ua) => {
    const m = model(discord, ua);
    expect(m.primary?.kind === "join" && m.primary.intercept).toBe(true);
    expect(m.embedIntercept).toBe(true);
    expect(kinds(m)).toContain("desktop");
  });

  it("degraded without a target shows the invite-unavailable notice", () => {
    const m = model(sampleCommunity(IDS.discordDegraded), DESKTOP);
    expect(m.primary).toBeNull();
    expect(m.notice).toBe("INVITE_UNAVAILABLE");
    expect(kinds(m)).toEqual([]);
    expect(kinds(model(sampleCommunity(IDS.discordDegraded), PHONE))).toEqual([]);
  });

  it("degraded with a fallback target still joins", () => {
    const m = model(sampleCommunity(IDS.discordDegraded, { live: { joinUrl: "/go/discord-old" } }), DESKTOP);
    expect(m.primary?.kind).toBe("join");
    expect(m.notice).toBeNull();
  });

  it("pending without a target shows no notice yet", () => {
    const m = model(sampleCommunity(IDS.discordDegraded, { live: { state: "pending" } }), DESKTOP);
    expect(m.notice).toBeNull();
  });
});

describe("cardModel — kook", () => {
  it("is never intercepted (no external browser needed)", () => {
    const m = model(sampleCommunity(IDS.kook), WECHAT);
    expect(m.primary?.kind === "join" && m.primary.intercept).toBe(false);
    expect(kinds(m)).toEqual(["copy:copyInvite"]);
  });

  it("unavailable: dimmed with the admin text and no actions", () => {
    const m = model(sampleCommunity(IDS.kookUnavailable), DESKTOP, "Closed");
    expect(m).toMatchObject({ dimmed: true, notice: "Closed", primary: null, secondary: [], embed: false });
  });

  it("unavailable without admin text uses communityUnavailable and keeps the contact", () => {
    const c = sampleCommunity(IDS.kookUnavailable, { contact: { label: {}, value: "admin" } });
    const m = model(c, DESKTOP);
    expect(m.notice).toBe("COMMUNITY_UNAVAILABLE");
    expect(kinds(m)).toEqual(["copy:copyContact"]);
  });
});

describe("cardModel — qq-group", () => {
  it("with a join link: join + copy group number + QR dialog", () => {
    const m = model(sampleCommunity(IDS.qqWithLink), PHONE);
    expect(m.primary).toMatchObject({ kind: "join", label: "joinGroup", href: "/go/qq-fans" });
    expect(kinds(m)).toEqual(["copy:copyGroupNumber", "qr"]);
    expect(m.inlineQr).toBeNull();
  });

  it("inside WeChat: copy group number is primary and the QR is inline", () => {
    const m = model(sampleCommunity(IDS.qqWithLink), WECHAT);
    expect(m.primary).toEqual({ kind: "copy", value: "123456789", label: "copyGroupNumber" });
    expect(m.inlineQr?.url).toBe("/media/q/01920000-0000-7000-8000-000000000301");
    expect(kinds(m)).toEqual([]);
  });

  it("inside QQ: joins directly", () => {
    expect(model(sampleCommunity(IDS.qqWithLink), QQ).primary?.kind).toBe("join");
  });

  it("without a link: copy group number is primary", () => {
    const m = model(sampleCommunity(IDS.qqNoLink), DESKTOP);
    expect(m.primary).toEqual({ kind: "copy", value: "87654321", label: "copyGroupNumber" });
    expect(kinds(m)).toEqual([]);
  });
});

describe("cardModel — wechat-group", () => {
  it.each([
    ["desktop", DESKTOP],
    ["WeChat", WECHAT],
  ])("%s: inline QR and copy WeChat ID, no primary", (_name, ua) => {
    const m = model(sampleCommunity(IDS.wechat), ua);
    expect(m.primary).toBeNull();
    expect(m.inlineQr?.width).toBe(430);
    expect(kinds(m)).toEqual(["copy:copyWeChatId"]);
  });
});

describe("cardModel — static platforms", () => {
  it("uses an uploaded QR when present", () => {
    const c = sampleCommunity(IDS.kook, {
      card: "static",
      provider: "static",
      platform: "telegram",
      inviteUrl: null,
      qr: { url: "/media/q/x", width: 300, height: 300, note: {} },
    });
    const m = model(c, DESKTOP);
    expect(kinds(m)).toEqual(["qr"]);
    expect(m.primary?.kind === "join" && m.primary.intercept).toBe(false);
  });
});
