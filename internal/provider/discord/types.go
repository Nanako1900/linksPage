// Package discord parses responses from Discord's unauthenticated public
// endpoints (guild widget and invite lookup). It performs no network I/O;
// fetching belongs to the provider implementation (M1).
package discord

import "time"

// Widget is the body of GET /api/guilds/{id}/widget.json.
type Widget struct {
	ID            string          `json:"id"`
	Name          string          `json:"name"`
	InstantInvite *string         `json:"instant_invite"` // null when no invite channel is set
	Channels      []WidgetChannel `json:"channels"`
	Members       []WidgetMember  `json:"members"`
	PresenceCount int             `json:"presence_count"`
}

// WidgetChannel is a voice channel listed by the widget.
type WidgetChannel struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Position int    `json:"position"`
}

// WidgetMember is an anonymised online member. IDs are widget-local ("0".."N").
type WidgetMember struct {
	ID            string      `json:"id"`
	Username      string      `json:"username"`
	Discriminator string      `json:"discriminator"`
	Avatar        *string     `json:"avatar"`
	Status        string      `json:"status"`
	AvatarURL     string      `json:"avatar_url"`
	Game          *WidgetGame `json:"game,omitempty"` // dropped before persisting (doc 5.1)
	ChannelID     *string     `json:"channel_id,omitempty"`
}

// WidgetGame is the activity a member is playing.
type WidgetGame struct {
	Name string `json:"name"`
}

// Invite is the body of GET /api/v10/invites/{code}?with_counts=true.
type Invite struct {
	Type                     int            `json:"type"`
	Code                     string         `json:"code"`
	ExpiresAt                *time.Time     `json:"expires_at"` // null for permanent invites
	GuildID                  string         `json:"guild_id"`
	Guild                    *InviteGuild   `json:"guild"`
	Channel                  *InviteChannel `json:"channel"`
	ApproximateMemberCount   *int           `json:"approximate_member_count"`
	ApproximatePresenceCount *int           `json:"approximate_presence_count"`
}

// InviteGuild is the partial guild embedded in an invite.
type InviteGuild struct {
	ID                       string   `json:"id"`
	Name                     string   `json:"name"`
	Splash                   *string  `json:"splash"`
	Banner                   *string  `json:"banner"`
	Description              *string  `json:"description"`
	Icon                     *string  `json:"icon"`
	Features                 []string `json:"features"`
	VerificationLevel        int      `json:"verification_level"`
	VanityURLCode            *string  `json:"vanity_url_code"`
	NSFWLevel                int      `json:"nsfw_level"`
	PremiumSubscriptionCount int      `json:"premium_subscription_count"`
}

// InviteChannel is the partial channel embedded in an invite.
type InviteChannel struct {
	ID   string `json:"id"`
	Type int    `json:"type"`
	Name string `json:"name"`
}

// IsPermanent reports whether the invite never expires.
func (i Invite) IsPermanent() bool {
	return i.ExpiresAt == nil
}
