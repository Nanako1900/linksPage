// Package provider defines the community provider contract (doc 5.1):
// snapshot model, card states, the Provider interface, the registry, the
// platform presets (platforms.yaml) and the outbound HTTP client.
//
// Implementations live in subpackages (discord, kook); the refresh job
// lives in internal/jobs.
package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// ErrInvalidConfig is wrapped by ValidateConfig errors (mapped to 422 by
// the admin API in M2).
var ErrInvalidConfig = errors.New("provider: invalid community config")

// State is the card state machine value (doc 5.6). It is persisted in
// provider_snapshots.state and sent to browsers unchanged.
type State string

// Card states.
const (
	// StatePending: never fetched yet (admin name + skeleton).
	StatePending State = "pending"
	// StateLive: last fetch succeeded.
	StateLive State = "live"
	// StateStale: last fetch failed; data is from LastOK.
	StateStale State = "stale"
	// StateDegraded: data is fine but the permanent invite is invalid (10006).
	StateDegraded State = "degraded"
	// StateStatic: no live data (static platforms, widget disabled 50004,
	// unparseable KOOK badge).
	StateStatic State = "static"
	// StateQROnly: only a QR code (wechat-group).
	StateQROnly State = "qr-only"
	// StateUnavailable: the community does not exist or is private (10004,
	// KOOK not public).
	StateUnavailable State = "unavailable"
)

// States lists every state in a stable order.
func States() []State {
	return []State{StatePending, StateLive, StateStale, StateDegraded, StateStatic, StateQROnly, StateUnavailable}
}

// Valid reports whether s is a known state.
func (s State) Valid() bool {
	for _, v := range States() {
		if v == s {
			return true
		}
	}
	return false
}

// Online count sources.
const (
	SourceInvite = "invite"
	SourceWidget = "widget"
	SourceBadge  = "badge"
)

// Snapshot is one provider result. The JSON form (Users, State, ErrCode
// and FetchedAt excluded — they are columns or memory-only) is stored in
// provider_snapshots.data. Image fields hold registered /media/p paths.
type Snapshot struct {
	Name         string    `json:"name"`
	IconPath     string    `json:"iconPath"`   // "/media/p/{key}.{ext}" or ""
	BannerPath   string    `json:"bannerPath"` // "/media/p/{key}.{ext}" or ""
	Online       *int      `json:"online"`
	Members      *int      `json:"members"`
	OnlineSource string    `json:"onlineSource"` // SourceInvite | SourceWidget | SourceBadge | ""
	Channels     []Channel `json:"channels"`     // sanitized and truncated
	// Users are online members; kept in memory only (never persisted).
	Users []Member `json:"-"`
	// InstantInviteURL is the widget's temporary invite (absolute) or "".
	InstantInviteURL string `json:"instantInviteUrl"`
	// InviteExpiresAt is the expiry of the instant invite, if known.
	InviteExpiresAt *time.Time `json:"inviteExpiresAt"`
	// InviteInvalid is true when the permanent invite returned 10006.
	InviteInvalid bool `json:"inviteInvalid"`
	// InviteFetchedAt is when the permanent invite was last looked up (the
	// invite endpoint has its own 15 min cadence).
	InviteFetchedAt *time.Time `json:"inviteFetchedAt"`

	State     State     `json:"-"` // provider_snapshots.state
	ErrCode   string    `json:"-"` // provider_snapshots.err_code (code only, never upstream bodies)
	FetchedAt time.Time `json:"-"` // provider_snapshots.fetched_at
}

// Channel is a voice channel chip.
type Channel struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Position int    `json:"position"`
}

// Member is an online member (game/activity is dropped, doc 5.1).
type Member struct {
	Name       string `json:"name"`
	AvatarPath string `json:"avatarPath"` // "/media/p/{key}.{ext}" or ""
	Status     string `json:"status"`     // "online" | "idle" | "dnd"
}

// Capabilities describe what a provider can deliver.
type Capabilities struct {
	Online   bool `json:"online"`
	Members  bool `json:"members"`
	Channels bool `json:"channels"`
	Users    bool `json:"users"`
	Icon     bool `json:"icon"`
	Banner   bool `json:"banner"`
	Invite   bool `json:"invite"`
	Embed    bool `json:"embed"`
}

// ConfigInput is the per-community provider input taken from the
// communities row.
type ConfigInput struct {
	ExternalID string          // communities.external_id (guild id)
	InviteURL  string          // communities.invite_url ("" when unset)
	Raw        json.RawMessage // communities.config ({} in M1)
}

// FetchInput is passed to Provider.Fetch.
type FetchInput struct {
	// Config is the value returned by ValidateConfig.
	Config any
	// Previous is the last snapshot (nil when never fetched); providers use
	// it for cadence bookkeeping such as InviteFetchedAt.
	Previous *Snapshot
	Now      time.Time
}

// Provider fetches live community data (doc 5.1, adapted: Fetch receives
// the previous snapshot; ValidateConfig receives the row fields).
type Provider interface {
	// Kind is the communities.provider value ("discord", "kook").
	Kind() string
	Capabilities() Capabilities
	// ValidateConfig strictly validates the community input; errors wrap
	// ErrInvalidConfig.
	ValidateConfig(in ConfigInput) (any, error)
	// Fetch returns a sanitized snapshot. Logical upstream outcomes
	// (widget disabled, unknown guild, invalid invite, KOOK not public)
	// are returned as a snapshot with the matching State and ErrCode and a
	// nil error. Transient failures (network, 5xx, 429, malformed) return
	// an error; the refresh job then keeps the previous data as stale.
	// Rate limits return a *RetryAfterError.
	Fetch(ctx context.Context, in FetchInput) (*Snapshot, error)
	// MinInterval is the lower bound for the refresh interval.
	MinInterval() time.Duration
	// ImageHosts maps host → allowed path prefixes for media proxy
	// registration (doc 5.2).
	ImageHosts() map[string][]string
}

// RetryAfterError asks the scheduler to wait at least After.
type RetryAfterError struct {
	After time.Duration
	Err   error
}

func (e *RetryAfterError) Error() string {
	return fmt.Sprintf("provider: retry after %s: %v", e.After, e.Err)
}

func (e *RetryAfterError) Unwrap() error { return e.Err }

// RetryAfter extracts the delay from a *RetryAfterError in err's chain.
func RetryAfter(err error) (time.Duration, bool) {
	var ra *RetryAfterError
	if errors.As(err, &ra) {
		return ra.After, true
	}
	return 0, false
}

// Image kinds registered with the media proxy (media_proxy.kind). They
// select the proxy Cache-Control (icons/banners 1 day, avatars 1 hour).
const (
	ImageIcon   = "icon"
	ImageBanner = "banner"
	ImageSplash = "splash"
	ImageAvatar = "avatar"
)

// ImageRegistrar registers an upstream image URL with the media proxy and
// returns its public path "/media/p/{key}.{ext}". Implemented by
// internal/imgproxy.Registrar.
type ImageRegistrar interface {
	Register(ctx context.Context, providerKind, imageKind, rawURL string) (string, error)
}
