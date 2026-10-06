package provider

import (
	"strings"

	"github.com/Nanako1900/linksPage/internal/content"
)

// Snapshot cleaning limits (doc 5.1), in runes.
const (
	MaxNameRunes        = 100
	MaxUserNameRunes    = 32
	MaxChannelNameRunes = 100
	MaxChannels         = 50
	MaxUsers            = 100
	// maxIDRunes bounds upstream ids (snowflakes are ≤20 digits).
	maxIDRunes = 32
)

// Member statuses accepted in snapshots.
const (
	StatusOnline = "online"
	StatusIdle   = "idle"
	StatusDND    = "dnd"
)

// MediaProxyPrefix is the public prefix of registered proxy images.
const MediaProxyPrefix = "/media/p/"

// SanitizeSnapshot returns a cleaned copy: names truncated, bidi and
// zero-width characters stripped (content.CleanText), list sizes capped,
// unknown statuses and sources normalized, image paths restricted to the
// media proxy and negative counts dropped. It must be applied before a
// snapshot is stored or published; it is idempotent.
func SanitizeSnapshot(in Snapshot) Snapshot {
	out := cloneSnapshot(in)
	out.Name = content.CleanText(in.Name, MaxNameRunes)
	out.IconPath = cleanImagePath(in.IconPath)
	out.BannerPath = cleanImagePath(in.BannerPath)
	out.Online = cleanCount(in.Online)
	out.Members = cleanCount(in.Members)
	out.OnlineSource = cleanSource(in.OnlineSource)
	if out.Online == nil {
		out.OnlineSource = ""
	}
	out.Channels = cleanChannels(in.Channels)
	out.Users = cleanUsers(in.Users)
	if !strings.HasPrefix(in.InstantInviteURL, "https://") {
		out.InstantInviteURL = ""
		out.InviteExpiresAt = nil
	}
	if !ErrCodeRe.MatchString(in.ErrCode) {
		out.ErrCode = ""
	}
	return out
}

func cleanChannels(in []Channel) []Channel {
	out := make([]Channel, 0, min(len(in), MaxChannels))
	for _, c := range in {
		if len(out) == MaxChannels {
			break
		}
		name := content.CleanText(c.Name, MaxChannelNameRunes)
		if name == "" {
			continue
		}
		out = append(out, Channel{ID: content.CleanText(c.ID, maxIDRunes), Name: name, Position: c.Position})
	}
	return out
}

func cleanUsers(in []Member) []Member {
	out := make([]Member, 0, min(len(in), MaxUsers))
	for _, m := range in {
		if len(out) == MaxUsers {
			break
		}
		out = append(out, Member{
			Name:       content.CleanText(m.Name, MaxUserNameRunes),
			AvatarPath: cleanImagePath(m.AvatarPath),
			Status:     cleanStatus(m.Status),
		})
	}
	return out
}

func cleanStatus(s string) string {
	switch s {
	case StatusIdle, StatusDND:
		return s
	default:
		return StatusOnline
	}
}

func cleanSource(s string) string {
	switch s {
	case SourceInvite, SourceWidget, SourceBadge:
		return s
	default:
		return ""
	}
}

func cleanCount(p *int) *int {
	if p == nil || *p < 0 {
		return nil
	}
	v := *p
	return &v
}

func cleanImagePath(p string) string {
	rest, ok := strings.CutPrefix(p, MediaProxyPrefix)
	if !ok || rest == "" || strings.ContainsAny(rest, "/?#%\\") {
		return ""
	}
	return p
}
