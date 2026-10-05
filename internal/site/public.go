package site

import (
	"time"

	"github.com/Nanako1900/linksPage/internal/provider"
)

// Public DTO (contract: docs/m1/contract.md, TS mirror:
// web/src/shared/types/public.ts, canonical sample:
// web/src/test/fixtures/public-page.json).
//
// JSON conventions:
//   - field names are camelCase;
//   - every field is always present; nullable fields are encoded as null
//     (never omitted). The only exception is BlockView, whose kind-specific
//     fields appear only for their kind;
//   - arrays and objects are never null (empty [] / {});
//   - timestamps are RFC 3339 strings in UTC;
//   - URLs are root-relative paths (/media/..., /go/..., /c/...) except
//     PublicSite.BaseURL, LinkView.URL, CommunityView.InviteURL and
//     EmbedView.Src, which are absolute.

// URL path prefixes produced by the public DTO.
const (
	PathUploads      = "/media/u/"                      // + media key, e.g. /media/u/<32 hex>.webp
	PathProxy        = "/media/p/"                      // + proxy key + "." + ext
	PathQR           = "/media/q/"                      // + QR code uuid
	PathGo           = "/go/"                           // + community or link slug
	PathCommunity    = "/c/"                            // + community slug
	DiscordWidgetSrc = "https://discord.com/widget?id=" // + guild id (EmbedView.Src)
)

// CardState is the card state machine value (doc 5.6).
type CardState = provider.State

// BlockKind is a page block type.
type BlockKind string

// Block kinds.
const (
	BlockCommunity BlockKind = "community"
	BlockLink      BlockKind = "link"
	BlockHeading   BlockKind = "heading"
	BlockText      BlockKind = "text"
	BlockSocialRow BlockKind = "social_row"
)

// CardKind selects the community card component.
type CardKind string

// Card kinds.
const (
	CardDiscord     CardKind = "discord"
	CardKOOK        CardKind = "kook"
	CardQQGroup     CardKind = "qq-group"
	CardWeChatGroup CardKind = "wechat-group"
	CardStatic      CardKind = "static"
)

// MemberDisplay is the online member display mode (doc 5.3; default
// MemberAvatarsNames, Q3).
type MemberDisplay string

// Member display modes.
const (
	MemberHidden       MemberDisplay = "hidden"
	MemberAvatars      MemberDisplay = "avatars"
	MemberAvatarsNames MemberDisplay = "avatars_names"
)

// IconKind tells the frontend where an icon comes from.
type IconKind string

// Icon kinds.
const (
	// IconSimple is a simple-icons slug (Name), rendered from the bundle.
	IconSimple IconKind = "simple"
	// IconBuiltin is a LinksPage built-in icon (Name), e.g. "kook".
	IconBuiltin IconKind = "builtin"
	// IconMedia is an uploaded image (URL under /media/u/).
	IconMedia IconKind = "media"
)

// PublicPage is the complete public DTO: GET /api/v1/public/bootstrap
// (envelope {"data": PublicPage}), the #lp-data script and RenderDTO.Data.
type PublicPage struct {
	// Version is the site settings version.
	Version int64 `json:"version"`
	// Revision changes whenever anything except live data changes; LiveDTO
	// carries the same value so clients know when to reload.
	Revision string     `json:"revision"`
	Page     Page       `json:"page"`
	Site     PublicSite `json:"site"`
	// Blocks are the blocks visible at GeneratedAt, in display order.
	Blocks []BlockView `json:"blocks"`
	// Communities referenced by Blocks, keyed by community id.
	Communities map[string]CommunityView `json:"communities"`
	// Links referenced by Blocks, keyed by link id.
	Links map[string]LinkView `json:"links"`
	// Platforms referenced by Communities, keyed by platform id.
	Platforms   map[string]PlatformView `json:"platforms"`
	GeneratedAt time.Time               `json:"generatedAt"`
	// NextBoundary is the earliest future visible_from/visible_to; the page
	// must be rebuilt when it is reached. Null when none.
	NextBoundary *time.Time `json:"nextBoundary"`
}

// PublicSite is the browser-visible part of Settings.
type PublicSite struct {
	// BaseURL is config base_url (absolute, no trailing slash), used for
	// "open on desktop" and share URLs.
	BaseURL       string        `json:"baseUrl"`
	DefaultLocale string        `json:"defaultLocale"`
	Locales       []string      `json:"locales"`
	Title         LocalizedText `json:"title"`
	Description   LocalizedText `json:"description"`
	DisplayName   LocalizedText `json:"displayName"`
	// Bio and Footer are Markdown subset sources (render with
	// markdown-to-jsx, raw HTML disabled, links through the URL allow-list).
	Bio           LocalizedText `json:"bio"`
	Avatar        *ImageView    `json:"avatar"`
	Appearance    string        `json:"appearance"`
	Theme         Theme         `json:"theme"`
	Footer        LocalizedText `json:"footer"`
	ShowPoweredBy bool          `json:"showPoweredBy"`
	NotFound      LocalizedText `json:"notFound"`
	Copy          CopyOverrides `json:"copy"`
}

// BlockView is one page block. Kind-specific fields are present only for
// their kind (TS discriminated union on "kind"):
//
//	community:  communityId
//	link:       linkId
//	heading:    text, count (count present only when showCount is set)
//	text:       markdown
//	social_row: linkIds
type BlockView struct {
	ID          string        `json:"id"`
	Kind        BlockKind     `json:"kind"`
	CommunityID string        `json:"communityId,omitempty"`
	LinkID      string        `json:"linkId,omitempty"`
	Text        LocalizedText `json:"text,omitempty"`
	Count       *int          `json:"count,omitempty"`
	Markdown    LocalizedText `json:"markdown,omitempty"`
	LinkIDs     []string      `json:"linkIds,omitempty"`
}

// ImageView is a sized image (explicit width/height avoid layout shift).
type ImageView struct {
	URL    string `json:"url"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

// IconView is an icon reference. URL is non-null only for IconMedia.
type IconView struct {
	Kind IconKind `json:"kind"`
	Name string   `json:"name"`
	URL  *string  `json:"url"`
}

// PlatformView is a platform preset or custom platform.
type PlatformView struct {
	ID   string        `json:"id"`
	Name LocalizedText `json:"name"`
	Icon *IconView     `json:"icon"`
	// NeedsExternalBrowser: inside WeChat/QQ, joining shows the
	// "open in browser" overlay instead of navigating (doc 5.7).
	NeedsExternalBrowser bool `json:"needsExternalBrowser"`
}

// LinkView is a link (link block or social row entry).
type LinkView struct {
	ID    string        `json:"id"`
	Slug  string        `json:"slug"`
	Kind  string        `json:"kind"` // "link" | "social"
	Label LocalizedText `json:"label"`
	// URL is the destination (absolute; for display and copying).
	URL string `json:"url"`
	// Href is what <a href> uses: "/go/{slug}", or URL itself when RelMe.
	Href  string    `json:"href"`
	Icon  *IconView `json:"icon"`
	RelMe bool      `json:"relMe"`
}
