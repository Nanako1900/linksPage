package site

import "time"

// CommunityView is one community card. Static parts change only when the
// page is rebuilt; Live is also served by GET /api/v1/public/live.
type CommunityView struct {
	ID       string   `json:"id"`
	Slug     string   `json:"slug"`
	Provider string   `json:"provider"` // "discord" | "kook" | "static"
	Platform string   `json:"platform"` // key into PublicPage.Platforms
	Card     CardKind `json:"card"`
	// Name is the admin display name (required; never the provider name).
	Name        LocalizedText `json:"name"`
	Description LocalizedText `json:"description"`
	// Icon is the uploaded icon (/media/u/...), else the provider icon
	// (/media/p/...), else null (render the platform icon).
	Icon *ImageView `json:"icon"`
	// SharePath is "/c/{slug}".
	SharePath string `json:"sharePath"`
	// InviteURL is the permanent invite (absolute, e.g. https://discord.gg/x)
	// for "copy invite"; null when there is none or it is known to be invalid.
	InviteURL     *string       `json:"inviteUrl"`
	MemberDisplay MemberDisplay `json:"memberDisplay"`
	// Embed is set when the Discord iframe facade is enabled.
	Embed *EmbedView `json:"embed"`
	// QQ is set for qq-group cards.
	QQ *QQView `json:"qq"`
	// QR is set when a QR code was uploaded (required for wechat-group).
	QR *QRView `json:"qr"`
	// Contact is the fallback contact (e.g. admin WeChat id), or null.
	Contact *ContactView `json:"contact"`
	// UnavailableText overrides copy.communityUnavailable for this card ({}
	// = use the site copy / built-in text).
	UnavailableText LocalizedText `json:"unavailableText"`
	Live            LiveView      `json:"live"`
}

// EmbedView describes the click-to-load Discord iframe. The frontend
// appends "&theme=light" or "&theme=dark" to Src; CSP allows
// frame-src https://discord.com only when some card has Embed.
type EmbedView struct {
	Kind string `json:"kind"` // "discord"
	Src  string `json:"src"`  // "https://discord.com/widget?id={guildId}"
}

// QQView holds the QQ group number (5–12 digits) for "copy group number".
// The join link, when configured, is LiveView.JoinURL ("/go/{slug}").
type QQView struct {
	GroupNumber string `json:"groupNumber"`
}

// QRView is the current QR code image (PNG/JPEG, ≥240px rendered size).
type QRView struct {
	URL    string        `json:"url"` // "/media/q/{uuid}"
	Width  int           `json:"width"`
	Height int           `json:"height"`
	Note   LocalizedText `json:"note"` // e.g. "updated 10-05, valid 7 days"; {} when none
}

// ContactView is a copyable fallback contact.
type ContactView struct {
	Label LocalizedText `json:"label"`
	Value string        `json:"value"`
}

// LiveView is the frequently changing part of a card.
type LiveView struct {
	State CardState `json:"state"`
	// Online and Members are null when unknown or hidden by the admin.
	Online  *int `json:"online"`
	Members *int `json:"members"`
	// OnlineSource is "invite" | "widget" | "badge", null when Online is null.
	OnlineSource *string `json:"onlineSource"`
	// Channels is [] when unknown or hidden.
	Channels []ChannelView `json:"channels"`
	// Users are online members after the display mode, block list and
	// member limit were applied: [] for "hidden"; Name is null for "avatars".
	Users []UserView `json:"users"`
	// UpdatedAt is the time of the last successful fetch (for "updated N
	// minutes ago" when State is stale), null when never fetched.
	UpdatedAt *time.Time `json:"updatedAt"`
	// JoinURL is "/go/{slug}" when /go has a target (doc 5.6), else null.
	// Always null for unavailable and wechat-group cards.
	JoinURL *string `json:"joinUrl"`
}

// ChannelView is a channel chip.
type ChannelView struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// UserView is one online member (avatars are aria-hidden; render a text
// summary next to them).
type UserView struct {
	Name      *string `json:"name"`
	AvatarURL *string `json:"avatarUrl"` // "/media/p/{key}.{ext}" or null
	Status    string  `json:"status"`    // "online" | "idle" | "dnd"
}
