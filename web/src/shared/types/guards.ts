/**
 * Runtime guards for the public DTO (src/shared/types/public.ts). The
 * public entry cannot use zod (bundle budget), so untrusted JSON (#lp-data,
 * API responses) is narrowed with these small structural checks. They
 * verify every field the public page reads; semantic invariants are
 * enforced server-side (internal/site PublicPage.Validate).
 *
 * Initial version by the contract; the frontend builder owns and extends it.
 */
import type {
  Block,
  CardState,
  CommunityView,
  IconView,
  ImageView,
  LinkView,
  LiveDTO,
  LiveView,
  PlatformView,
  PublicPage,
  PublicSite,
  UserView,
} from "./public";

type Rec = Record<string, unknown>;

const CARD_STATES: ReadonlySet<string> = new Set<CardState>([
  "pending",
  "live",
  "stale",
  "degraded",
  "static",
  "qr-only",
  "unavailable",
]);
const CARDS: ReadonlySet<string> = new Set(["discord", "kook", "qq-group", "wechat-group", "static"]);
const PROVIDERS: ReadonlySet<string> = new Set(["discord", "kook", "static"]);
const MEMBER_DISPLAYS: ReadonlySet<string> = new Set(["hidden", "avatars", "avatars_names"]);
const ONLINE_SOURCES: ReadonlySet<string> = new Set(["invite", "widget", "badge"]);
const USER_STATUSES: ReadonlySet<string> = new Set(["online", "idle", "dnd"]);
const ICON_KINDS: ReadonlySet<string> = new Set(["simple", "builtin", "media"]);
const APPEARANCES: ReadonlySet<string> = new Set(["light", "dark", "auto", "visitor-choice"]);
const PALETTE_KEYS = ["bg", "fg", "muted", "card", "border", "accent", "accentFg"] as const;
const THEME_KEYS = ["preset", "radius", "fontSans", "fontDisplay"] as const;

export function isRecord(v: unknown): v is Rec {
  return typeof v === "object" && v !== null && !Array.isArray(v);
}

const isString = (v: unknown): v is string => typeof v === "string";
const isNonEmptyString = (v: unknown): v is string => isString(v) && v !== "";
const isNumber = (v: unknown): v is number => typeof v === "number" && Number.isFinite(v);
const isNullableString = (v: unknown): v is string | null => v === null || isString(v);
const isNullableNumber = (v: unknown): v is number | null => v === null || isNumber(v);
const isIn = (set: ReadonlySet<string>, v: unknown): boolean => isString(v) && set.has(v);

// URL shapes (contract section 3). Same-origin paths start with exactly one
// "/" (never "//" or "/\", which browsers treat as another host); the
// few absolute URLs are http(s), plus mailto for links. Defence in depth:
// the server already validates them, and the CSP limits what runs.
const ROOT_PATH = /^\/(?![/\\])/;
const WEB_URL = /^https?:\/\//i;
const LINK_URL = /^(?:https?:\/\/|mailto:)/i;
const GO_PATH = /^\/go\/[a-z0-9][a-z0-9-]{0,63}$/;
const EMBED_SRC = /^https:\/\/discord\.com\/widget\?id=\d{17,20}$/;
const FORBIDDEN_KEYS: ReadonlySet<string> = new Set(["__proto__", "constructor", "prototype"]);

const isRootPath = (v: unknown): v is string => isString(v) && ROOT_PATH.test(v);
const isWebURL = (v: unknown): v is string => isString(v) && WEB_URL.test(v);
const isLinkURL = (v: unknown): v is string => isString(v) && LINK_URL.test(v);
const isNullableRootPath = (v: unknown): v is string | null => v === null || isRootPath(v);

export function isLocalizedText(v: unknown): v is Record<string, string> {
  return isRecord(v) && Object.values(v).every(isString);
}

function isArrayOf<T>(v: unknown, guard: (x: unknown) => x is T): v is T[] {
  return Array.isArray(v) && v.every(guard);
}

/** A JSON object map; prototype-polluting keys such as "__proto__" are rejected. */
function isMapOf<T>(v: unknown, guard: (x: unknown, key: string) => x is T): v is Record<string, T> {
  return isRecord(v) && Object.entries(v).every(([k, x]) => !FORBIDDEN_KEYS.has(k) && guard(x, k));
}

function isImage(v: unknown): v is ImageView {
  return isRecord(v) && isRootPath(v.url) && isNumber(v.width) && isNumber(v.height);
}

function isIcon(v: unknown): v is IconView {
  return isRecord(v) && isIn(ICON_KINDS, v.kind) && isString(v.name) && isNullableRootPath(v.url);
}

function isTheme(v: unknown): boolean {
  if (!isRecord(v) || !THEME_KEYS.every((k) => isString(v[k]))) return false;
  return [v.light, v.dark].every((p) => isRecord(p) && PALETTE_KEYS.every((k) => isString(p[k])));
}

function isCopy(v: unknown): boolean {
  return isRecord(v) && Object.values(v).every(isLocalizedText);
}

export function isPublicSite(v: unknown): v is PublicSite {
  if (!isRecord(v)) return false;
  const texts = [v.title, v.description, v.displayName, v.bio, v.footer, v.notFound];
  return (
    (v.baseUrl === "" || isWebURL(v.baseUrl)) &&
    isString(v.defaultLocale) &&
    v.defaultLocale !== "" &&
    isArrayOf(v.locales, isNonEmptyString) &&
    texts.every(isLocalizedText) &&
    (v.avatar === null || isImage(v.avatar)) &&
    isIn(APPEARANCES, v.appearance) &&
    isTheme(v.theme) &&
    typeof v.showPoweredBy === "boolean" &&
    isCopy(v.copy)
  );
}

export function isBlock(v: unknown): v is Block {
  if (!isRecord(v) || !isString(v.id)) return false;
  switch (v.kind) {
    case "community":
      return isString(v.communityId);
    case "link":
      return isString(v.linkId);
    case "heading":
      return isLocalizedText(v.text) && (v.count === undefined || isNumber(v.count));
    case "text":
      return isLocalizedText(v.markdown);
    case "social_row":
      return isArrayOf(v.linkIds, isString);
    default:
      return false;
  }
}

function isUser(v: unknown): v is UserView {
  return isRecord(v) && isNullableString(v.name) && isNullableRootPath(v.avatarUrl) && isIn(USER_STATUSES, v.status);
}

function isChannel(v: unknown): v is { id: string; name: string } {
  return isRecord(v) && isString(v.id) && isString(v.name);
}

export function isLiveView(v: unknown): v is LiveView {
  return (
    isRecord(v) &&
    isIn(CARD_STATES, v.state) &&
    isNullableNumber(v.online) &&
    isNullableNumber(v.members) &&
    (v.onlineSource === null || isIn(ONLINE_SOURCES, v.onlineSource)) &&
    isArrayOf(v.channels, isChannel) &&
    isArrayOf(v.users, isUser) &&
    isNullableString(v.updatedAt) &&
    (v.joinUrl === null || (isString(v.joinUrl) && GO_PATH.test(v.joinUrl)))
  );
}

function isCardExtras(v: Rec): boolean {
  const embedOk =
    v.embed === null ||
    (isRecord(v.embed) && v.embed.kind === "discord" && isString(v.embed.src) && EMBED_SRC.test(v.embed.src));
  const qqOk = v.qq === null || (isRecord(v.qq) && isString(v.qq.groupNumber));
  const qrOk =
    v.qr === null ||
    (isRecord(v.qr) &&
      isRootPath(v.qr.url) &&
      isNumber(v.qr.width) &&
      isNumber(v.qr.height) &&
      isLocalizedText(v.qr.note));
  const contactOk =
    v.contact === null || (isRecord(v.contact) && isLocalizedText(v.contact.label) && isString(v.contact.value));
  return embedOk && qqOk && qrOk && contactOk;
}

export function isCommunity(v: unknown, key?: string): v is CommunityView {
  if (!isRecord(v) || (key !== undefined && v.id !== key)) return false;
  return (
    isString(v.id) &&
    isString(v.slug) &&
    isIn(PROVIDERS, v.provider) &&
    isString(v.platform) &&
    isIn(CARDS, v.card) &&
    isLocalizedText(v.name) &&
    isLocalizedText(v.description) &&
    (v.icon === null || isImage(v.icon)) &&
    isRootPath(v.sharePath) &&
    (v.inviteUrl === null || isWebURL(v.inviteUrl)) &&
    isIn(MEMBER_DISPLAYS, v.memberDisplay) &&
    isCardExtras(v) &&
    isLocalizedText(v.unavailableText) &&
    isLiveView(v.live)
  );
}

function isLink(v: unknown, key?: string): v is LinkView {
  if (!isRecord(v) || (key !== undefined && v.id !== key)) return false;
  return (
    isString(v.id) &&
    isString(v.slug) &&
    (v.kind === "link" || v.kind === "social") &&
    isLocalizedText(v.label) &&
    isLinkURL(v.url) &&
    (isRootPath(v.href) || isLinkURL(v.href)) &&
    (v.icon === null || isIcon(v.icon)) &&
    typeof v.relMe === "boolean"
  );
}

function isPlatform(v: unknown, key?: string): v is PlatformView {
  if (!isRecord(v) || (key !== undefined && v.id !== key)) return false;
  return (
    isString(v.id) &&
    isLocalizedText(v.name) &&
    (v.icon === null || isIcon(v.icon)) &&
    typeof v.needsExternalBrowser === "boolean"
  );
}

/** Narrow untrusted JSON to a PublicPage. */
export function isPublicPage(v: unknown): v is PublicPage {
  if (!isRecord(v) || !isRecord(v.page)) return false;
  return (
    isNumber(v.version) &&
    isString(v.revision) &&
    isNumber(v.page.id) &&
    isString(v.page.slug) &&
    isPublicSite(v.site) &&
    isArrayOf(v.blocks, isBlock) &&
    isMapOf(v.communities, isCommunity) &&
    isMapOf(v.links, isLink) &&
    isMapOf(v.platforms, isPlatform) &&
    isString(v.generatedAt) &&
    isNullableString(v.nextBoundary)
  );
}

/** Narrow untrusted JSON to a LiveDTO. */
export function isLiveDTO(v: unknown): v is LiveDTO {
  return (
    isRecord(v) &&
    isString(v.revision) &&
    isString(v.generatedAt) &&
    isMapOf(v.communities, (x): x is LiveView => isLiveView(x))
  );
}
