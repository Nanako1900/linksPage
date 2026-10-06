package kook

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Nanako1900/linksPage/internal/provider"
)

// ProviderKind is the communities.provider value.
const ProviderKind = "kook"

// BadgeInterval is the badge refresh cadence (style 0 and 2 each).
const BadgeInterval = 5 * time.Minute

// Error codes stored in provider_snapshots.err_code.
const (
	CodeNotPublic   = "kook_not_public"
	CodeMalformed   = "kook_malformed"
	CodeRateLimited = "kook_429"
	CodeError       = "kook_error"
)

// maxRetryAfter caps Retry-After hints.
const maxRetryAfter = time.Hour

// Options configure the KOOK (token-free badge) provider.
type Options struct {
	// APIBase replaces https://www.kookapp.cn (config providers.kook.api_base).
	APIBase string
	// Client is provider.NewHTTPClient's client (must not follow redirects).
	Client *http.Client
}

// Config is the validated per-community input.
type Config struct {
	GuildID string
}

// Provider implements provider.Provider for KOOK badges (M1, no token).
type Provider struct {
	opts Options
	base *url.URL
}

var _ provider.Provider = (*Provider)(nil)

// NewProvider returns the KOOK provider.
func NewProvider(opts Options) (*Provider, error) {
	if opts.Client == nil {
		return nil, errors.New("kook: http client is required")
	}
	base, err := provider.ParseAPIBase(opts.APIBase, provider.DefaultKOOKAPIBase)
	if err != nil {
		return nil, fmt.Errorf("kook: %w", err)
	}
	// The badge data lives in the redirect's Location: never follow it,
	// whatever client the caller passed.
	client := *opts.Client
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	opts.Client = &client
	return &Provider{opts: opts, base: base}, nil
}

// Kind implements provider.Provider.
func (p *Provider) Kind() string { return ProviderKind }

// Capabilities implements provider.Provider (name, online and total only).
func (p *Provider) Capabilities() provider.Capabilities {
	return provider.Capabilities{Online: true, Members: true}
}

// ValidateConfig implements provider.Provider (guild id ^\d{1,20}$). The
// id must be validated locally: the badge endpoint answers every invalid
// id with the same "not public" redirect.
func (p *Provider) ValidateConfig(in provider.ConfigInput) (any, error) {
	if !ValidGuildID(in.ExternalID) {
		return nil, fmt.Errorf("%w: kook guild id must be 1-20 digits", provider.ErrInvalidConfig)
	}
	if err := provider.ValidateEmptyConfig(in.Raw); err != nil {
		return nil, err
	}
	return Config{GuildID: in.ExternalID}, nil
}

// MinInterval implements provider.Provider.
func (p *Provider) MinInterval() time.Duration { return BadgeInterval }

// ImageHosts implements provider.Provider (the badge SVG is never proxied).
func (p *Provider) ImageHosts() map[string][]string { return map[string][]string{} }

// BadgeURLAt builds the badge endpoint under base (api_base).
func BadgeURLAt(base *url.URL, guildID string, style Style) (string, error) {
	if !ValidGuildID(guildID) {
		return "", ErrInvalidGuildID
	}
	if style < StyleName || style > StyleOnlineTotal {
		return "", ErrInvalidStyle
	}
	q := url.Values{"guild_id": {guildID}, "style": {strconv.Itoa(int(style))}}
	return provider.EndpointURL(base, "/api/v3/badge/guild", q), nil
}

// Fetch implements provider.Provider: style=0 (name) and style=2
// (online/total) via ParseBadgeResponse; ErrNotPublic → unavailable,
// ErrMalformed → static.
func (p *Provider) Fetch(ctx context.Context, in provider.FetchInput) (*provider.Snapshot, error) {
	cfg, err := configFrom(in.Config)
	if err != nil {
		return nil, err
	}
	now := in.Now
	if now.IsZero() {
		now = time.Now()
	}
	name, err := p.badge(ctx, cfg.GuildID, StyleName)
	if err != nil {
		return logicalOutcome(err, now)
	}
	counts, err := p.badge(ctx, cfg.GuildID, StyleOnlineTotal)
	if err != nil {
		return logicalOutcome(err, now)
	}
	online, total := counts.Online, counts.Total
	snap := provider.SanitizeSnapshot(provider.Snapshot{
		Name:         name.Name,
		Online:       &online,
		Members:      &total,
		OnlineSource: provider.SourceBadge,
		Channels:     []provider.Channel{},
		State:        provider.StateLive,
		FetchedAt:    now,
	})
	return &snap, nil
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
	return Config{}, fmt.Errorf("%w: kook config has type %T", provider.ErrInvalidConfig, v)
}

// badge requests one badge style without following the redirect.
func (p *Provider) badge(ctx context.Context, guildID string, style Style) (Badge, error) {
	endpoint, err := BadgeURLAt(p.base, guildID, style)
	if err != nil {
		return Badge{}, err
	}
	resp, err := provider.Get(ctx, p.opts.Client, endpoint, "")
	if err != nil {
		return Badge{}, &provider.FetchError{Code: CodeError, Err: err}
	}
	switch {
	case resp.Status == http.StatusTooManyRequests:
		return Badge{}, &provider.RetryAfterError{
			After: parseRetryAfter(resp.Header.Get("Retry-After")),
			Err:   &provider.FetchError{Code: CodeRateLimited, Err: fmt.Errorf("%w: 429", ErrUnexpectedStatus)},
		}
	case resp.Status >= http.StatusBadRequest:
		return Badge{}, &provider.FetchError{Code: "kook_" + strconv.Itoa(resp.Status), Err: fmt.Errorf("%w: %d", ErrUnexpectedStatus, resp.Status)}
	}
	return ParseBadgeResponse(resp.Status, resp.Header, style)
}

// logicalOutcome maps badge errors: not public → unavailable; malformed
// labels or unexpected non-error statuses (e.g. a 200 instead of the
// redirect) → static (the format is undocumented, so the card degrades);
// network errors, 4xx and 5xx are transient.
func logicalOutcome(err error, now time.Time) (*provider.Snapshot, error) {
	var snap provider.Snapshot
	switch {
	case errors.Is(err, ErrNotPublic):
		snap = provider.Snapshot{State: provider.StateUnavailable, ErrCode: CodeNotPublic}
	case errors.Is(err, ErrMalformed), errors.Is(err, ErrUnexpectedStatus) && !isFetchError(err):
		snap = provider.Snapshot{State: provider.StateStatic, ErrCode: CodeMalformed}
	default:
		return nil, err
	}
	snap.FetchedAt = now
	snap.Channels = []provider.Channel{}
	clean := provider.SanitizeSnapshot(snap)
	return &clean, nil
}

func isFetchError(err error) bool {
	var fe *provider.FetchError
	return errors.As(err, &fe)
}

// parseRetryAfter accepts delta seconds; HTTP dates and junk yield 0.
func parseRetryAfter(v string) time.Duration {
	secs, err := strconv.Atoi(strings.TrimSpace(v))
	switch {
	case err != nil || secs <= 0:
		return 0
	case secs >= int(maxRetryAfter/time.Second):
		return maxRetryAfter
	}
	return time.Duration(secs) * time.Second
}
