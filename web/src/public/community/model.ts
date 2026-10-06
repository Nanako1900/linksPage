/**
 * Pure card logic: which notice and actions a community card shows for a
 * given state and visitor (docs/m1/contract.md sections 6 and 7, doc 5.5
 * and 5.7). Components render this model; tests pin it per state.
 */
import { absoluteUrl } from "../../shared/format";
import type { MessageKey } from "../../shared/i18n/messages";
import type { CommunityView, PlatformView, QRView } from "../../shared/types/public";
import { isInApp, type UAClass } from "../../shared/ua";

export type CardAction =
  /** Navigate to /go/{slug}; `intercept` shows the open-in-browser overlay instead. */
  | { kind: "join"; href: string; label: "join" | "joinGroup"; intercept: boolean; copyUrl: string }
  | { kind: "copy"; value: string; label: MessageKey }
  /** Uploaded QR image in a dialog. */
  | { kind: "qr"; qr: QRView }
  /** Client-generated QR of the absolute join URL (desktop visitors scan with a phone). */
  | { kind: "qrGenerate"; url: string }
  /** "Open on desktop" dialog with the share URL. */
  | { kind: "desktop"; url: string };

export interface CardModel {
  /** Greyed out (unavailable). */
  dimmed: boolean;
  /** Plain-text notice (unavailable text or "invite unavailable"). */
  notice: string | null;
  primary: CardAction | null;
  secondary: CardAction[];
  /** QR shown inline (WeChat groups; QQ groups inside WeChat). */
  inlineQr: QRView | null;
  /** Discord iframe facade is allowed. */
  embed: boolean;
  /** The embed must show the open-in-browser overlay instead of loading. */
  embedIntercept: boolean;
}

export interface ModelEnv {
  ua: UAClass;
  baseUrl: string;
  platform: PlatformView | undefined;
  /** Already-resolved text (site.copy → built-in). */
  texts: { inviteUnavailable: string; communityUnavailable: string; unavailableText: string };
}

function contactAction(c: CommunityView): CardAction[] {
  if (!c.contact) return [];
  return [{ kind: "copy", value: c.contact.value, label: c.card === "wechat-group" ? "copyWeChatId" : "copyContact" }];
}

const EMPTY: CardModel = {
  dimmed: false,
  notice: null,
  primary: null,
  secondary: [],
  inlineQr: null,
  embed: false,
  embedIntercept: false,
};

function joinAction(c: CommunityView, env: ModelEnv, label: "join" | "joinGroup"): CardAction | null {
  if (!c.live.joinUrl) return null;
  const intercept = (env.platform?.needsExternalBrowser ?? false) && isInApp(env.ua);
  return { kind: "join", href: c.live.joinUrl, label, intercept, copyUrl: absoluteUrl(env.baseUrl, c.live.joinUrl) };
}

function qqModel(c: CommunityView, env: ModelEnv): CardModel {
  const groupNumber = c.qq?.groupNumber ?? "";
  const copyGroup: CardAction = { kind: "copy", value: groupNumber, label: "copyGroupNumber" };
  // WeChat blocks qm.qq.com: never navigate there from inside WeChat.
  const join = env.ua.inWeChat ? null : joinAction(c, env, "joinGroup");
  const qrInline = env.ua.inWeChat ? c.qr : null;
  const qrAction: CardAction[] = c.qr && !qrInline ? [{ kind: "qr", qr: c.qr }] : [];
  return {
    ...EMPTY,
    primary: join ?? copyGroup,
    secondary: [...(join ? [copyGroup] : []), ...qrAction, ...contactAction(c)],
    inlineQr: qrInline,
  };
}

function providerModel(c: CommunityView, env: ModelEnv): CardModel {
  const join = joinAction(c, env, "join");
  const joinUrl = c.live.joinUrl;
  const secondary: CardAction[] = [];
  if (c.inviteUrl) secondary.push({ kind: "copy", value: c.inviteUrl, label: "copyInvite" });
  if (c.qr) secondary.push({ kind: "qr", qr: c.qr });
  else if (joinUrl && !env.ua.mobile) secondary.push({ kind: "qrGenerate", url: absoluteUrl(env.baseUrl, joinUrl) });
  if (join && env.ua.mobile && c.card === "discord") {
    secondary.push({ kind: "desktop", url: absoluteUrl(env.baseUrl, c.sharePath) });
  }
  secondary.push(...contactAction(c));
  const embedIntercept = (env.platform?.needsExternalBrowser ?? false) && isInApp(env.ua);
  return {
    ...EMPTY,
    notice: join || c.live.state === "pending" ? null : env.texts.inviteUnavailable,
    primary: join,
    secondary,
    embed: c.embed !== null,
    embedIntercept,
  };
}

/** Build the card model for one community and visitor. */
export function cardModel(c: CommunityView, env: ModelEnv): CardModel {
  if (c.live.state === "unavailable") {
    return {
      ...EMPTY,
      dimmed: true,
      notice: env.texts.unavailableText || env.texts.communityUnavailable,
      secondary: contactAction(c),
    };
  }
  switch (c.card) {
    case "wechat-group":
      return { ...EMPTY, inlineQr: c.qr, secondary: contactAction(c) };
    case "qq-group":
      return qqModel(c, env);
    default:
      return providerModel(c, env);
  }
}
