package discord

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/Nanako1900/linksPage/internal/provider"
)

// ProviderKind is the communities.provider value.
const ProviderKind = "discord"

// Cadence (doc 4.5): widget every 5 min (matches max-age=300), invite every 15 min.
const (
	WidgetInterval = 5 * time.Minute
	InviteInterval = 15 * time.Minute
	// InstantInviteTTL is how long a widget instant_invite is trusted after
	// it was first seen. Discord creates it with a 24 hour expiry
	// (docs/spikes/providers.md); the hour of margin covers clock skew and
	// invites that already existed when first seen.
	InstantInviteTTL = 23 * time.Hour
)

// Image sizes requested from the CDN.
const (
	iconSize   = 256
	bannerSize = 1024
)

// Options configure the Discord provider.
type Options struct {
	// APIBase replaces https://discord.com (config providers.discord.api_base).
	APIBase string
	// Client is provider.NewHTTPClient's client.
	Client *http.Client
	// Images registers icon/splash/avatar URLs with the media proxy. When
	// nil, snapshots carry no image paths.
	Images provider.ImageRegistrar
	// Logger receives non-fatal problems (image registration, invite
	// lookups that fall back to the previous data). Optional.
	Logger *slog.Logger
}

// Config is the validated per-community input.
type Config struct {
	GuildID    string
	InviteCode string // "" when no permanent invite is configured
}

// Provider implements provider.Provider for Discord (widget.json + invite).
type Provider struct {
	opts   Options
	base   *url.URL
	logger *slog.Logger
}

var _ provider.Provider = (*Provider)(nil)

// NewProvider returns the Discord provider.
func NewProvider(opts Options) (*Provider, error) {
	if opts.Client == nil {
		return nil, errors.New("discord: http client is required")
	}
	base, err := provider.ParseAPIBase(opts.APIBase, provider.DefaultDiscordAPIBase)
	if err != nil {
		return nil, fmt.Errorf("discord: %w", err)
	}
	logger := opts.Logger
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	return &Provider{opts: opts, base: base, logger: logger}, nil
}

// Kind implements provider.Provider.
func (p *Provider) Kind() string { return ProviderKind }

// Capabilities implements provider.Provider.
func (p *Provider) Capabilities() provider.Capabilities {
	return provider.Capabilities{Online: true, Members: true, Channels: true, Users: true, Icon: true, Banner: true, Invite: true, Embed: true}
}

// ValidateConfig implements provider.Provider (guild id ^\d{17,20}$,
// invite code extracted from invite_url via ExtractInviteCode).
func (p *Provider) ValidateConfig(in provider.ConfigInput) (any, error) {
	if !ValidGuildID(in.ExternalID) {
		return nil, fmt.Errorf("%w: discord guild id must be 17-20 digits", provider.ErrInvalidConfig)
	}
	if err := provider.ValidateEmptyConfig(in.Raw); err != nil {
		return nil, err
	}
	cfg := Config{GuildID: in.ExternalID}
	if in.InviteURL != "" {
		code, err := ExtractInviteCode(in.InviteURL)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid discord invite url", provider.ErrInvalidConfig)
		}
		cfg.InviteCode = code
	}
	return cfg, nil
}

// MinInterval implements provider.Provider.
func (p *Provider) MinInterval() time.Duration { return WidgetInterval }

// ImageHosts implements provider.Provider.
func (p *Provider) ImageHosts() map[string][]string { return ImageHosts() }

// Fetch implements provider.Provider. The widget is fetched on every call;
// the invite only when InviteInterval elapsed since
// Previous.InviteFetchedAt (otherwise its data is carried over).
func (p *Provider) Fetch(ctx context.Context, in provider.FetchInput) (*provider.Snapshot, error) {
	cfg, err := configFrom(in.Config)
	if err != nil {
		return nil, err
	}
	now := in.Now
	if now.IsZero() {
		now = time.Now()
	}
	widget, werr := p.fetchWidget(ctx, cfg.GuildID)
	switch {
	case werr == nil, errors.Is(werr, ErrWidgetDisabled):
	case errors.Is(werr, ErrUnknownGuild):
		snap := provider.SanitizeSnapshot(provider.Snapshot{State: provider.StateUnavailable, ErrCode: ErrorCode(werr), FetchedAt: now})
		return &snap, nil
	default:
		return nil, transientError(werr)
	}
	inv := p.inviteData(ctx, cfg, in.Previous, now)
	snap := p.assemble(ctx, assembleInput{cfg: cfg, widget: widget, widgetErr: werr, invite: inv, prev: in.Previous, now: now})
	clean := provider.SanitizeSnapshot(snap)
	return &clean, nil
}

func configFrom(v any) (Config, error) {
	switch c := v.(type) {
	case Config:
		return c, nil
	case *Config:
		if c != nil {
			return *c, nil
		}
	}
	return Config{}, fmt.Errorf("%w: discord config has type %T", provider.ErrInvalidConfig, v)
}

// fetchWidget returns the widget, or an error (typed by ParseWidget).
func (p *Provider) fetchWidget(ctx context.Context, guildID string) (*Widget, error) {
	endpoint, err := WidgetURLAt(p.base, guildID)
	if err != nil {
		return nil, err
	}
	resp, err := provider.Get(ctx, p.opts.Client, endpoint, "application/json")
	if err != nil {
		return nil, err
	}
	w, err := ParseWidget(resp.Status, resp.Header, resp.Body)
	if err != nil {
		return nil, err
	}
	return &w, nil
}

// fetchInvite looks up the permanent invite with counts.
func (p *Provider) fetchInvite(ctx context.Context, code string) (*Invite, error) {
	endpoint, err := InviteURLAt(p.base, code)
	if err != nil {
		return nil, err
	}
	resp, err := provider.Get(ctx, p.opts.Client, endpoint, "application/json")
	if err != nil {
		return nil, err
	}
	inv, err := ParseInvite(resp.Status, resp.Header, resp.Body)
	if err != nil {
		return nil, err
	}
	return &inv, nil
}

// transientError tags err with its Discord code and converts rate limits
// into *provider.RetryAfterError.
func transientError(err error) error {
	fe := &provider.FetchError{Code: ErrorCode(err), Err: err}
	var rl *RateLimitError
	if errors.As(err, &rl) {
		return &provider.RetryAfterError{After: rl.RetryAfter, Err: fe}
	}
	return fe
}
