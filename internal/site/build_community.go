package site

import (
	"errors"
	"fmt"
	"maps"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/Nanako1900/linksPage/internal/provider"
	"github.com/Nanako1900/linksPage/internal/store/dbq"
)

// ProviderIconSize is the rendered size of provider icons (/media/p): the
// providers request 128px guild icons.
const ProviderIconSize = 128

// communityInput bundles the rows one community card is built from.
type communityInput struct {
	row      dbq.Community
	display  CommunityDisplay
	platform provider.Platform
	snap     *liveSnapshot // nil when never fetched (or static)
	qr       *dbq.ListQRCodesRow
	icon     *dbq.Medium
	now      time.Time
}

// liveSnapshot is the snapshot chosen for a card plus its last success.
type liveSnapshot struct {
	snap   provider.Snapshot
	lastOK *time.Time
}

// pickSnapshot prefers the in-memory snapshot (it carries Users) unless the
// database row is newer (written by another instance).
func pickSnapshot(id string, row *dbq.ProviderSnapshot, live provider.LiveSource) (*liveSnapshot, error) {
	var out *liveSnapshot
	if row != nil {
		s, err := decodeSnapshot(*row)
		if err != nil {
			return nil, err
		}
		out = &liveSnapshot{snap: s}
		if row.LastOkAt.Valid {
			t := row.LastOkAt.Time.UTC()
			out.lastOK = &t
		}
	}
	mem, ok := live.Live(id)
	if !ok || !mem.State.Valid() || (out != nil && mem.FetchedAt.Before(out.snap.FetchedAt)) {
		return out, nil
	}
	merged := &liveSnapshot{snap: mem}
	if out != nil {
		merged.lastOK = out.lastOK
	}
	if succeeded(mem.State) && !mem.FetchedAt.IsZero() && (merged.lastOK == nil || mem.FetchedAt.After(*merged.lastOK)) {
		t := mem.FetchedAt.UTC()
		merged.lastOK = &t
	}
	return merged, nil
}

func succeeded(s provider.State) bool {
	return s == provider.StateLive || s == provider.StateDegraded
}

// decodeDisplay strictly decodes communities.display and applies defaults.
func decodeDisplay(raw []byte) (CommunityDisplay, error) {
	var d CommunityDisplay
	if err := DecodeStrict(raw, &d); err != nil {
		return CommunityDisplay{}, err
	}
	if len(d.Name) == 0 {
		return CommunityDisplay{}, errors.New("display.name is required")
	}
	if d.MemberDisplay == "" {
		d.MemberDisplay = MemberAvatarsNames
	}
	if !memberDisplays[d.MemberDisplay] {
		return CommunityDisplay{}, fmt.Errorf("display.memberDisplay %q unknown", d.MemberDisplay)
	}
	if d.MemberLimit <= 0 {
		d.MemberLimit = DefaultMemberLimit
	}
	d.MemberLimit = min(d.MemberLimit, MaxMemberLimit)
	return d, nil
}

// buildCommunity assembles one card (validated by the caller).
func buildCommunity(in communityInput) CommunityView {
	r, d := in.row, in.display
	card := CardKind(in.platform.Card)
	state := effectiveState(in, cardState(r.Provider, card, in.snap))
	var snap provider.Snapshot
	if in.snap != nil {
		snap = in.snap.snap
	}
	v := CommunityView{
		ID:              r.ID.String(),
		Slug:            r.Slug,
		Provider:        r.Provider,
		Platform:        r.Platform,
		Card:            card,
		Name:            maps.Clone(d.Name),
		Description:     cloneText(d.Description),
		Icon:            communityIcon(in.icon, snap.IconPath),
		SharePath:       PathCommunity + r.Slug,
		InviteURL:       inviteURL(r.InviteUrl, snap.InviteInvalid),
		MemberDisplay:   d.MemberDisplay,
		Embed:           embedView(r, d),
		QQ:              qqView(card, d),
		QR:              qrView(in.qr),
		Contact:         contactView(d.Contact),
		UnavailableText: cloneText(d.UnavailableText),
	}
	v.Live = buildLive(in, state, snap)
	return v
}

// cardState derives the card state: wechat-group is always qr-only,
// static platforms are static, provider cards follow their snapshot.
func cardState(providerKind string, card CardKind, snap *liveSnapshot) CardState {
	switch {
	case card == CardWeChatGroup:
		return provider.StateQROnly
	case providerKind == "static":
		return provider.StateStatic
	case snap == nil || !snap.snap.State.Valid():
		return provider.StatePending
	}
	return snap.snap.State
}

// effectiveState shows live/degraded data as stale once the last success
// is older than provider.StaleAfter (3 refresh intervals, at least 15
// minutes): a dead leader or a stuck refresh job records no failure, so the
// stored state alone would stay live forever. The page-rebuild tick (every
// minute) re-evaluates it without a database write. The 15 minute floor
// covers every provider's MinInterval (5 minutes), so the row's own
// refresh_interval is enough here.
func effectiveState(in communityInput, state CardState) CardState {
	var lastOK *time.Time
	if in.snap != nil {
		lastOK = in.snap.lastOK
	}
	return provider.EffectiveState(state, lastOK, refreshInterval(in.row.RefreshInterval), in.now)
}

// refreshInterval converts communities.refresh_interval (months count as
// 30 days; NULL is 0, i.e. the provider default).
func refreshInterval(iv pgtype.Interval) time.Duration {
	if !iv.Valid {
		return 0
	}
	const day = 24 * time.Hour
	return time.Duration(iv.Microseconds)*time.Microsecond +
		time.Duration(iv.Days)*day + time.Duration(iv.Months)*30*day
}

func communityIcon(m *dbq.Medium, proxyPath string) *ImageView {
	if m != nil {
		return &ImageView{URL: PathUploads + m.Key, Width: int(m.Width), Height: int(m.Height)}
	}
	if proxyPathRe.MatchString(proxyPath) {
		return &ImageView{URL: proxyPath, Width: ProviderIconSize, Height: ProviderIconSize}
	}
	return nil
}

func inviteURL(raw *string, invalid bool) *string {
	if raw == nil || *raw == "" || invalid {
		return nil
	}
	v := *raw
	return &v
}

func embedView(r dbq.Community, d CommunityDisplay) *EmbedView {
	if !d.Embed || r.Provider != "discord" || r.ExternalID == nil {
		return nil
	}
	return &EmbedView{Kind: "discord", Src: DiscordWidgetSrc + *r.ExternalID}
}

func qqView(card CardKind, d CommunityDisplay) *QQView {
	if card != CardQQGroup {
		return nil
	}
	return &QQView{GroupNumber: d.QQGroupNumber}
}

func qrView(q *dbq.ListQRCodesRow) *QRView {
	if q == nil {
		return nil
	}
	note := LocalizedText{}
	if len(q.Note) > 0 {
		if err := DecodeStrict(q.Note, &note); err != nil {
			note = LocalizedText{}
		}
	}
	return &QRView{URL: PathQR + q.ID.String(), Width: int(q.Width), Height: int(q.Height), Note: note}
}

func contactView(c *ContactView) *ContactView {
	if c == nil {
		return nil
	}
	return &ContactView{Label: maps.Clone(c.Label), Value: c.Value}
}

// buildLive derives the live part. Static, pending, qr-only and
// unavailable cards carry no live data.
func buildLive(in communityInput, state CardState, snap provider.Snapshot) LiveView {
	l := LiveView{State: state, Channels: []ChannelView{}, Users: []UserView{}}
	if in.row.Provider != "static" && in.snap != nil {
		l.UpdatedAt = in.snap.lastOK
	}
	if hasLiveData(state) && in.snap != nil {
		fillLiveData(&l, in.display, snap)
	}
	if joinTarget(in, state, snap) != "" {
		u := PathGo + in.row.Slug
		l.JoinURL = &u
	}
	return l
}

func hasLiveData(s CardState) bool {
	switch s {
	case provider.StateLive, provider.StateStale, provider.StateDegraded, provider.StateStatic:
		return true
	}
	return false
}

func fillLiveData(l *LiveView, d CommunityDisplay, snap provider.Snapshot) {
	if d.ShowOnline == nil || *d.ShowOnline {
		l.Members = clonePtr(snap.Members)
		if snap.Online != nil && validSource(snap.OnlineSource) {
			l.Online = clonePtr(snap.Online)
			src := snap.OnlineSource
			l.OnlineSource = &src
		}
	}
	if d.ShowChannels == nil || *d.ShowChannels {
		for _, c := range snap.Channels {
			l.Channels = append(l.Channels, ChannelView{ID: c.ID, Name: c.Name})
		}
	}
	l.Users = buildUsers(snap.Users, d)
}

func validSource(s string) bool {
	return s == provider.SourceInvite || s == provider.SourceWidget || s == provider.SourceBadge
}

// buildUsers applies the display mode, the name block list and the limit.
func buildUsers(members []provider.Member, d CommunityDisplay) []UserView {
	out := []UserView{}
	if d.MemberDisplay == MemberHidden {
		return out
	}
	for _, m := range members {
		if len(out) >= d.MemberLimit {
			break
		}
		if blocked(m.Name, d.NameBlocklist) {
			continue
		}
		u := UserView{Status: normalizeStatus(m.Status)}
		if d.MemberDisplay == MemberAvatarsNames {
			name := m.Name
			u.Name = &name
		}
		if proxyPathRe.MatchString(m.AvatarPath) {
			p := m.AvatarPath
			u.AvatarURL = &p
		}
		out = append(out, u)
	}
	return out
}

func blocked(name string, blocklist []string) bool {
	lower := strings.ToLower(name)
	for _, b := range blocklist {
		if b != "" && strings.Contains(lower, strings.ToLower(b)) {
			return true
		}
	}
	return false
}

func normalizeStatus(s string) string {
	switch s {
	case "idle", "dnd":
		return s
	}
	return "online"
}

// joinTarget uses the shared decision function so the button and /go
// never disagree.
func joinTarget(in communityInput, state CardState, snap provider.Snapshot) string {
	return provider.JoinTarget(provider.JoinInput{
		Provider:               in.row.Provider,
		Card:                   in.platform.Card,
		State:                  state,
		InviteURL:              deref(in.row.InviteUrl),
		FallbackURL:            deref(in.row.FallbackUrl),
		InviteInvalid:          snap.InviteInvalid,
		InstantInviteURL:       snap.InstantInviteURL,
		InstantInviteExpiresAt: snap.InviteExpiresAt,
	}, in.now)
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func clonePtr[T any](p *T) *T {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}
