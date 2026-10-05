package provider

import "time"

// JoinInput holds the fields that decide a community's join target. The
// page builder (LiveView.JoinURL) and /go/{slug} both use JoinTarget so
// the button and the redirect can never disagree.
type JoinInput struct {
	// Provider is "discord", "kook" or "static".
	Provider string
	// Card is one of the Card* constants.
	Card string
	// State is the snapshot state ("" when there is no snapshot row).
	State State
	// InviteURL is communities.invite_url (permanent invite / official
	// join link), "" when unset.
	InviteURL string
	// FallbackURL is communities.fallback_url, "" when unset.
	FallbackURL string
	// InviteInvalid is the snapshot's inviteInvalid flag (Discord 10006).
	InviteInvalid bool
	// InstantInviteURL / InstantInviteExpiresAt come from the snapshot.
	InstantInviteURL       string
	InstantInviteExpiresAt *time.Time
}

// JoinTarget returns the URL /go/{slug} redirects to, or "" when there is
// none (doc 5.6):
//   - unavailable communities and wechat-group cards have no target;
//   - provider platforms: permanent invite unless it returned 10006, then
//     the unexpired widget instant invite, then fallback_url;
//   - static platforms (QQ groups included): invite_url, then fallback_url.
func JoinTarget(in JoinInput, now time.Time) string {
	if in.State == StateUnavailable || in.Card == CardWeChatGroup {
		return ""
	}
	if in.Provider == "static" {
		return firstNonEmpty(in.InviteURL, in.FallbackURL)
	}
	permanent := in.InviteURL
	if in.InviteInvalid {
		permanent = ""
	}
	instant := in.InstantInviteURL
	if in.InstantInviteExpiresAt != nil && !now.Before(*in.InstantInviteExpiresAt) {
		instant = ""
	}
	return firstNonEmpty(permanent, instant, in.FallbackURL)
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
