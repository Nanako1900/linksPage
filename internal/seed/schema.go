package seed

import "time"

// File is the seed.yaml document (schema version 1). YAML keys are
// snake_case; unknown keys are errors. Image paths are relative to the
// directory containing the seed file and must stay inside it.
type File struct {
	Version     int             `yaml:"version"`
	Site        SiteSeed        `yaml:"site"`
	Platforms   []PlatformSeed  `yaml:"platforms"`
	Communities []CommunitySeed `yaml:"communities"`
	Links       []LinkSeed      `yaml:"links"`
	// Blocks lists the page in order. When empty, the default layout is a
	// "社区 / Communities" heading (show_count), every community in file
	// order, every kind=link link, then one social_row with all kind=social
	// links.
	Blocks []BlockSeed `yaml:"blocks"`
}

// Text is a localized string map (locale → text).
type Text map[string]string

// SiteSeed maps onto site.Settings; omitted keys keep the defaults.
type SiteSeed struct {
	DefaultLocale  string   `yaml:"default_locale"`
	Locales        []string `yaml:"locales"`
	Title          Text     `yaml:"title"`
	Description    Text     `yaml:"description"`
	DisplayName    Text     `yaml:"display_name"`
	Bio            Text     `yaml:"bio"`    // Markdown subset
	Avatar         string   `yaml:"avatar"` // image path → media (kind avatar)
	Footer         Text     `yaml:"footer"` // Markdown subset
	ShowPoweredBy  *bool    `yaml:"show_powered_by"`
	Appearance     string   `yaml:"appearance"` // light | dark | auto | visitor-choice
	SearchIndexing string   `yaml:"search_indexing"`
	NotFound       Text     `yaml:"not_found"`
	OG             OGSeed   `yaml:"og"`
	// Copy maps locale → copy key (openInBrowser, openOnDesktop,
	// inviteUnavailable, communityUnavailable) → text.
	Copy map[string]map[string]string `yaml:"copy"`
}

// OGSeed overrides link-preview metadata.
type OGSeed struct {
	Title       Text   `yaml:"title"`
	Description Text   `yaml:"description"`
	Image       string `yaml:"image"` // image path → media (kind og)
}

// PlatformSeed is a custom platform (custom_platforms).
type PlatformSeed struct {
	ID                   string `yaml:"id"`
	Name                 Text   `yaml:"name"`
	Icon                 string `yaml:"icon"` // "si:<slug>" | "builtin:<name>" | image path
	URLPattern           string `yaml:"url_pattern"`
	NeedsExternalBrowser bool   `yaml:"needs_external_browser"`
}

// CommunitySeed is one community. The provider is derived from the
// platform (discord → discord, kook → kook, everything else → static).
type CommunitySeed struct {
	Slug     string `yaml:"slug"`
	Platform string `yaml:"platform"`
	Name     Text   `yaml:"name"` // required
	// Description is plain text.
	Description Text   `yaml:"description"`
	Icon        string `yaml:"icon"` // image path → media (kind icon)
	// GuildID is required for discord (17–20 digits) and kook (1–20 digits).
	GuildID string `yaml:"guild_id"`
	// Invite is the permanent invite / official join link (https only):
	// discord.gg/<code>, kook.top/<code>, qm.qq.com/..., t.me/... .
	Invite      string `yaml:"invite"`
	FallbackURL string `yaml:"fallback_url"`
	// QQGroup is the QQ group number (qq-group, ^\d{5,12}$).
	QQGroup string `yaml:"qq_group"`
	// QR is the QR code (required for wechat-group, optional for qq-group).
	QR *QRSeed `yaml:"qr"`
	// Contact is the fallback contact.
	Contact *ContactSeed `yaml:"contact"`
	// Members is hidden | avatars | avatars_names (default).
	Members         string        `yaml:"members"`
	NameBlocklist   []string      `yaml:"name_blocklist"`
	ShowChannels    *bool         `yaml:"show_channels"`
	ShowOnline      *bool         `yaml:"show_online"`
	MemberLimit     int           `yaml:"member_limit"`
	Embed           bool          `yaml:"embed"` // Discord iframe facade
	RefreshInterval time.Duration `yaml:"refresh_interval"`
	UnavailableText Text          `yaml:"unavailable_text"`
}

// QRSeed is a QR code image with an optional note.
type QRSeed struct {
	Image string `yaml:"image"` // image path (png/jpeg/webp)
	Note  Text   `yaml:"note"`
}

// ContactSeed is a copyable fallback contact.
type ContactSeed struct {
	Label Text   `yaml:"label"`
	Value string `yaml:"value"`
}

// LinkSeed is one link.
type LinkSeed struct {
	Slug  string `yaml:"slug"`
	Kind  string `yaml:"kind"` // link (default) | social
	Label Text   `yaml:"label"`
	URL   string `yaml:"url"`
	Icon  string `yaml:"icon"` // "si:<slug>" | "builtin:<name>" | image path
	RelMe bool   `yaml:"rel_me"`
}

// BlockSeed is one block: exactly one of Heading, Community, Link, Text,
// SocialRow must be set.
type BlockSeed struct {
	Heading   Text     `yaml:"heading"`
	ShowCount bool     `yaml:"show_count"` // heading only
	Community string   `yaml:"community"`  // community slug
	Link      string   `yaml:"link"`       // link slug
	Text      Text     `yaml:"text"`       // Markdown subset
	SocialRow []string `yaml:"social_row"` // link slugs
	// Visible defaults to true.
	Visible     *bool      `yaml:"visible"`
	VisibleFrom *time.Time `yaml:"visible_from"`
	VisibleTo   *time.Time `yaml:"visible_to"`
}
