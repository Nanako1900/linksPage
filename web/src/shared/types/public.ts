/**
 * Public page DTO — hand-written mirror of internal/site (Go). Contract:
 * docs/m1/contract.md. Canonical sample: src/test/fixtures/public-page.json.
 *
 * Conventions: every field is always present (nullable fields are `null`,
 * never omitted) except the kind-specific fields of {@link Block}; arrays
 * and objects are never null; timestamps are RFC 3339 UTC strings; URLs are
 * root-relative (/media/..., /go/..., /c/...) unless documented absolute.
 *
 * Owned by the contract; request changes instead of editing in place.
 */

/** BCP-47 locale → text. May be empty ({}). */
export type LocalizedText = Record<string, string>;

/** Card state machine (doc 5.6). */
export type CardState = "pending" | "live" | "stale" | "degraded" | "static" | "qr-only" | "unavailable";

export type Appearance = "light" | "dark" | "auto" | "visitor-choice";

export type BlockKind = "community" | "link" | "heading" | "text" | "social_row";

export type CardKind = "discord" | "kook" | "qq-group" | "wechat-group" | "static";

export type ProviderKind = "discord" | "kook" | "static";

export type MemberDisplay = "hidden" | "avatars" | "avatars_names";

export type OnlineSource = "invite" | "widget" | "badge";

export type UserStatus = "online" | "idle" | "dnd";

export type IconKind = "simple" | "builtin" | "media";

/** Overridable copy keys (settings.copy). */
export type CopyKey = "openInBrowser" | "openOnDesktop" | "inviteUnavailable" | "communityUnavailable";

/** locale → copy key → text ("" or missing = built-in text). */
export type CopyOverrides = Record<string, Partial<Record<CopyKey, string>>>;

export interface Palette {
  bg: string;
  fg: string;
  muted: string;
  card: string;
  border: string;
  accent: string;
  accentFg: string;
}

export interface Theme {
  preset: string;
  light: Palette;
  dark: Palette;
  radius: string;
  fontSans: string;
  fontDisplay: string;
}

export interface Page {
  id: number;
  slug: string;
}

/** Sized image; always render width/height. */
export interface ImageView {
  url: string;
  width: number;
  height: number;
}

/** `url` is non-null only for kind "media" (/media/u/{name}). */
export interface IconView {
  kind: IconKind;
  name: string;
  url: string | null;
}

export interface PublicSite {
  /** config base_url: absolute origin, no trailing slash. */
  baseUrl: string;
  defaultLocale: string;
  locales: string[];
  title: LocalizedText;
  description: LocalizedText;
  displayName: LocalizedText;
  /** Markdown subset. */
  bio: LocalizedText;
  avatar: ImageView | null;
  appearance: Appearance;
  theme: Theme;
  /** Markdown subset. */
  footer: LocalizedText;
  showPoweredBy: boolean;
  notFound: LocalizedText;
  copy: CopyOverrides;
}

interface BlockBase {
  id: string;
}

export interface CommunityBlock extends BlockBase {
  kind: "community";
  communityId: string;
}

export interface LinkBlock extends BlockBase {
  kind: "link";
  linkId: string;
}

export interface HeadingBlock extends BlockBase {
  kind: "heading";
  text: LocalizedText;
  /** Present only when the heading shows a count ("社区 — 03"). */
  count?: number;
}

export interface TextBlock extends BlockBase {
  kind: "text";
  /** Markdown subset. */
  markdown: LocalizedText;
}

export interface SocialRowBlock extends BlockBase {
  kind: "social_row";
  linkIds: string[];
}

export type Block = CommunityBlock | LinkBlock | HeadingBlock | TextBlock | SocialRowBlock;

/** Discord click-to-load iframe: append `&theme=light|dark` to `src`. */
export interface EmbedView {
  kind: "discord";
  /** https://discord.com/widget?id={guildId} */
  src: string;
}

export interface QQView {
  groupNumber: string;
}

export interface QRView {
  /** /media/q/{uuid} */
  url: string;
  width: number;
  height: number;
  note: LocalizedText;
}

export interface ContactView {
  label: LocalizedText;
  value: string;
}

export interface ChannelView {
  id: string;
  name: string;
}

export interface UserView {
  /** null in "avatars" mode. */
  name: string | null;
  /** /media/p/{key}.{ext} or null. */
  avatarUrl: string | null;
  status: UserStatus;
}

/** Frequently changing card data; also served by /api/v1/public/live. */
export interface LiveView {
  state: CardState;
  online: number | null;
  members: number | null;
  /** null exactly when `online` is null. */
  onlineSource: OnlineSource | null;
  channels: ChannelView[];
  users: UserView[];
  /** Last successful fetch (for "updated N minutes ago"). */
  updatedAt: string | null;
  /** "/go/{slug}" or null (no target; always null for unavailable / wechat-group). */
  joinUrl: string | null;
}

export interface CommunityView {
  id: string;
  slug: string;
  provider: ProviderKind;
  /** Key into PublicPage.platforms. */
  platform: string;
  card: CardKind;
  name: LocalizedText;
  description: LocalizedText;
  /** /media/u/... or /media/p/...; null → platform icon. */
  icon: ImageView | null;
  /** "/c/{slug}" */
  sharePath: string;
  /** Absolute permanent invite for "copy invite", or null. */
  inviteUrl: string | null;
  memberDisplay: MemberDisplay;
  embed: EmbedView | null;
  /** Set exactly for qq-group cards. */
  qq: QQView | null;
  /** Always set for wechat-group cards. */
  qr: QRView | null;
  contact: ContactView | null;
  /** {} → copy.communityUnavailable / built-in text. */
  unavailableText: LocalizedText;
  live: LiveView;
}

export interface LinkView {
  id: string;
  slug: string;
  kind: "link" | "social";
  label: LocalizedText;
  /** Absolute destination (display / copy). */
  url: string;
  /** Use for <a href>: "/go/{slug}", or `url` when relMe. */
  href: string;
  icon: IconView | null;
  relMe: boolean;
}

export interface PlatformView {
  id: string;
  name: LocalizedText;
  icon: IconView | null;
  /** In WeChat/QQ show the "open in browser" overlay instead of navigating. */
  needsExternalBrowser: boolean;
}

/** GET /api/v1/public/bootstrap → { data: PublicPage }; also #lp-data. */
export interface PublicPage {
  version: number;
  revision: string;
  page: Page;
  site: PublicSite;
  blocks: Block[];
  communities: Record<string, CommunityView>;
  links: Record<string, LinkView>;
  platforms: Record<string, PlatformView>;
  generatedAt: string;
  nextBoundary: string | null;
}

/** GET /api/v1/public/live → { data: LiveDTO } (poll 60 s while visible, If-None-Match). */
export interface LiveDTO {
  revision: string;
  communities: Record<string, LiveView>;
  generatedAt: string;
}

/** GET /api/v1/public/render?path=&lang= → { data: RenderDTO } (topology C Worker). */
export interface RenderDTO {
  status: number;
  lang: string;
  appearance: Appearance;
  head: string;
  fallback: string;
  data: PublicPage | null;
  csp: string;
  cspReportOnly: string;
  etag: string;
}

/** `{ "data": T }` success envelope (doc 4.10). */
export interface Envelope<T> {
  data: T;
}
