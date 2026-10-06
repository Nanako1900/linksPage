package discord

import (
	"cmp"
	"context"
	"errors"
	"log/slog"
	"slices"
	"time"

	"github.com/Nanako1900/linksPage/internal/provider"
)

// CodeInviteGuildMismatch marks a permanent invite that points to another
// guild than the configured one (treated like an invalid invite).
const CodeInviteGuildMismatch = "discord_invite_guild_mismatch"

// codeUnknownInvite is ErrorCode of the 404/10006 invite response.
const codeUnknownInvite = "discord_404_10006"

// inviteResult is the invite-derived part of a snapshot.
type inviteResult struct {
	name       string // guild name from a fresh invite lookup
	members    *int
	online     *int // approximate presence; nil when unknown
	iconPath   string
	bannerPath string
	invalid    bool
	errCode    string // code for an invalid invite
	fetchedAt  *time.Time
}

// inviteData fetches the invite when due, or carries over the previous
// invite-derived fields. Transient invite failures never fail the whole
// fetch: the previous data is kept (a 429 postpones the next lookup by a
// full InviteInterval, other errors retry on the next fetch).
func (p *Provider) inviteData(ctx context.Context, cfg Config, prev *provider.Snapshot, now time.Time) inviteResult {
	if cfg.InviteCode == "" {
		return inviteResult{}
	}
	carried := carryInvite(prev)
	if prev != nil && prev.InviteFetchedAt != nil && now.Sub(*prev.InviteFetchedAt) < InviteInterval {
		return carried
	}
	inv, err := p.fetchInvite(ctx, cfg.InviteCode)
	fetchedAt := now
	switch {
	case err == nil && inv.Guild.ID != cfg.GuildID:
		return inviteResult{
			invalid: true, errCode: CodeInviteGuildMismatch, fetchedAt: &fetchedAt,
			iconPath: carried.iconPath, bannerPath: carried.bannerPath,
		}
	case err == nil:
		return p.inviteFromResponse(ctx, cfg.GuildID, inv, fetchedAt)
	case errors.Is(err, ErrUnknownInvite):
		return inviteResult{
			invalid: true, errCode: ErrorCode(err), fetchedAt: &fetchedAt,
			iconPath: carried.iconPath, bannerPath: carried.bannerPath,
		}
	}
	p.logger.WarnContext(ctx, "discord invite lookup failed; keeping previous data",
		slog.String("guild_id", cfg.GuildID), slog.String("code", ErrorCode(err)), slog.Any("error", err))
	if errors.Is(err, ErrRateLimited) {
		carried.fetchedAt = &fetchedAt
	}
	return carried
}

// carryInvite copies the invite-derived fields of prev.
func carryInvite(prev *provider.Snapshot) inviteResult {
	if prev == nil {
		return inviteResult{}
	}
	out := inviteResult{
		members:    clonePtr(prev.Members),
		iconPath:   prev.IconPath,
		bannerPath: prev.BannerPath,
		invalid:    prev.InviteInvalid,
		fetchedAt:  clonePtr(prev.InviteFetchedAt),
	}
	if prev.OnlineSource == provider.SourceInvite {
		out.online = clonePtr(prev.Online)
	}
	if prev.InviteInvalid {
		out.errCode = codeUnknownInvite
		if prev.State == provider.StateDegraded && prev.ErrCode != "" {
			out.errCode = prev.ErrCode
		}
	}
	return out
}

func (p *Provider) inviteFromResponse(ctx context.Context, guildID string, inv *Invite, fetchedAt time.Time) inviteResult {
	out := inviteResult{
		name:      inv.Guild.Name,
		members:   clonePtr(inv.ApproximateMemberCount),
		online:    clonePtr(inv.ApproximatePresenceCount),
		fetchedAt: &fetchedAt,
	}
	if hash := deref(inv.Guild.Icon); hash != "" {
		out.iconPath = p.register(ctx, provider.ImageIcon, func() (string, error) { return IconURL(guildID, hash, iconSize) })
	}
	switch {
	case deref(inv.Guild.Banner) != "":
		hash := deref(inv.Guild.Banner)
		out.bannerPath = p.register(ctx, provider.ImageBanner, func() (string, error) { return BannerURL(guildID, hash, bannerSize) })
	case deref(inv.Guild.Splash) != "":
		hash := deref(inv.Guild.Splash)
		out.bannerPath = p.register(ctx, provider.ImageSplash, func() (string, error) { return SplashURL(guildID, hash, bannerSize) })
	}
	return out
}

// register builds an upstream URL and registers it with the media proxy;
// failures are logged and yield "" (the card falls back to other images).
func (p *Provider) register(ctx context.Context, kind string, build func() (string, error)) string {
	if p.opts.Images == nil {
		return ""
	}
	raw, err := build()
	if err == nil {
		var path string
		path, err = p.opts.Images.Register(ctx, ProviderKind, kind, raw)
		if err == nil {
			return path
		}
	}
	p.logger.WarnContext(ctx, "discord image registration failed", slog.String("kind", kind), slog.Any("error", err))
	return ""
}

type assembleInput struct {
	cfg       Config
	widget    *Widget // nil when the widget is disabled
	widgetErr error   // ErrWidgetDisabled or nil
	invite    inviteResult
	prev      *provider.Snapshot
	now       time.Time
}

// assemble merges widget and invite data into an unsanitized snapshot.
func (p *Provider) assemble(ctx context.Context, in assembleInput) provider.Snapshot {
	snap := provider.Snapshot{
		Members:         in.invite.members,
		IconPath:        in.invite.iconPath,
		BannerPath:      in.invite.bannerPath,
		InviteInvalid:   in.invite.invalid,
		InviteFetchedAt: in.invite.fetchedAt,
		Channels:        []provider.Channel{},
		State:           provider.StateLive,
		FetchedAt:       in.now,
	}
	if in.widget != nil {
		snap.Name = in.widget.Name
		snap.Channels = widgetChannels(in.widget.Channels)
		snap.Users = p.widgetUsers(ctx, in.widget.Members)
		snap.InstantInviteURL = instantInvite(in.widget.InstantInvite)
		snap.InviteExpiresAt = instantInviteExpiry(snap.InstantInviteURL, in.prev, in.now)
	}
	if snap.Name == "" {
		snap.Name = in.invite.name
	}
	if snap.Name == "" && in.prev != nil {
		snap.Name = in.prev.Name
	}
	snap.Online, snap.OnlineSource = onlineCount(in.invite, in.widget)
	switch {
	case in.widgetErr != nil:
		snap.State, snap.ErrCode = provider.StateStatic, ErrorCode(in.widgetErr)
	case in.invite.invalid:
		snap.State, snap.ErrCode = provider.StateDegraded, in.invite.errCode
	}
	return snap
}

// onlineCount prefers the invite's approximate presence (doc 5.3) and
// falls back to the widget's presence_count.
func onlineCount(inv inviteResult, w *Widget) (*int, string) {
	if inv.online != nil && !inv.invalid {
		return clonePtr(inv.online), provider.SourceInvite
	}
	if w != nil {
		n := w.PresenceCount
		return &n, provider.SourceWidget
	}
	return nil, ""
}

func widgetChannels(in []WidgetChannel) []provider.Channel {
	out := make([]provider.Channel, 0, len(in))
	for _, c := range in {
		out = append(out, provider.Channel{ID: c.ID, Name: c.Name, Position: c.Position})
	}
	slices.SortStableFunc(out, func(a, b provider.Channel) int {
		return cmp.Or(cmp.Compare(a.Position, b.Position), cmp.Compare(a.ID, b.ID))
	})
	return out
}

// widgetUsers converts widget members (game dropped) and registers their
// avatars; at most provider.MaxUsers are processed.
func (p *Provider) widgetUsers(ctx context.Context, in []WidgetMember) []provider.Member {
	out := make([]provider.Member, 0, min(len(in), provider.MaxUsers))
	for _, m := range in {
		if len(out) == provider.MaxUsers {
			break
		}
		avatar := ""
		if m.AvatarURL != "" {
			raw := m.AvatarURL
			avatar = p.register(ctx, provider.ImageAvatar, func() (string, error) { return raw, nil })
		}
		out = append(out, provider.Member{Name: m.Username, AvatarPath: avatar, Status: m.Status})
	}
	return out
}

// instantInvite normalizes the widget's temporary invite to
// https://discord.com/invite/{code}; anything unparseable is dropped.
func instantInvite(raw *string) string {
	if raw == nil || *raw == "" {
		return ""
	}
	code, err := ExtractInviteCode(*raw)
	if err != nil {
		return ""
	}
	return "https://" + APIHost + "/invite/" + code
}

// instantInviteExpiry dates a widget instant invite: InstantInviteTTL after
// it was first seen. The same invite seen again keeps its earlier expiry,
// so a long outage (stale snapshot) can never extend it; /go and the join
// button stop using it once it passed (provider.JoinTarget).
func instantInviteExpiry(url string, prev *provider.Snapshot, now time.Time) *time.Time {
	if url == "" {
		return nil
	}
	if prev != nil && prev.InstantInviteURL == url && prev.InviteExpiresAt != nil {
		return clonePtr(prev.InviteExpiresAt)
	}
	exp := now.Add(InstantInviteTTL).UTC()
	return &exp
}

func clonePtr[T any](p *T) *T {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}

func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
