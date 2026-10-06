package golink

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Nanako1900/linksPage/internal/provider"
	"github.com/Nanako1900/linksPage/internal/site"
	"github.com/Nanako1900/linksPage/internal/store/dbq"
)

// providerStatic is communities.provider for static platforms.
const providerStatic = "static"

// GoQuerier is the subset of dbq.Queries used by DBSource.
type GoQuerier interface {
	GetGoCommunity(ctx context.Context, slug string) (dbq.GetGoCommunityRow, error)
	GetGoLink(ctx context.Context, slug string) (dbq.GetGoLinkRow, error)
	ListCustomPlatforms(ctx context.Context) ([]dbq.CustomPlatform, error)
}

// PlatformLookup finds a platform preset by id (*provider.Catalog).
type PlatformLookup interface {
	Get(id string) (provider.Platform, bool)
}

// DBSource implements Source on top of the sqlc queries and the platform
// catalog. Custom platforms (always card "static") are read from the
// database only when the id is not a preset.
type DBSource struct {
	q       GoQuerier
	presets PlatformLookup
}

// NewDBSource returns a Source backed by q and the preset catalog.
func NewDBSource(q GoQuerier, presets PlatformLookup) (*DBSource, error) {
	if q == nil || presets == nil {
		return nil, errors.New("golink: NewDBSource needs queries and a platform catalog")
	}
	return &DBSource{q: q, presets: presets}, nil
}

// Link implements Source. Only links published on the page right now
// resolve (the query checks block visibility); others are ErrNotFound.
func (s *DBSource) Link(ctx context.Context, slug string) (LinkTarget, error) {
	row, err := s.q.GetGoLink(ctx, slug)
	if errors.Is(err, pgx.ErrNoRows) {
		return LinkTarget{}, ErrNotFound
	}
	if err != nil {
		return LinkTarget{}, fmt.Errorf("get link: %w", err)
	}
	return LinkTarget{ID: row.ID.String(), Slug: row.Slug, URL: row.Url}, nil
}

// Community implements Source. Like Link, hidden, scheduled and unplaced
// communities are ErrNotFound, so /go never leaks unpublished content.
func (s *DBSource) Community(ctx context.Context, slug string) (CommunityTarget, error) {
	row, err := s.q.GetGoCommunity(ctx, slug)
	if errors.Is(err, pgx.ErrNoRows) {
		return CommunityTarget{}, ErrNotFound
	}
	if err != nil {
		return CommunityTarget{}, fmt.Errorf("get community: %w", err)
	}
	platform, err := s.platform(ctx, row.Platform)
	if err != nil {
		return CommunityTarget{}, err
	}
	display, err := decodeDisplay(row.Display)
	if err != nil {
		return CommunityTarget{}, fmt.Errorf("community %q: %w", slug, err)
	}
	snap, err := decodeSnapshotData(row.SnapshotData)
	if err != nil {
		return CommunityTarget{}, fmt.Errorf("community %q: %w", slug, err)
	}
	return buildTarget(row, platform, display, snap), nil
}

// platform resolves a preset, then a custom platform. Unknown ids are
// treated as plain static links (no in-app guide) rather than failing.
func (s *DBSource) platform(ctx context.Context, id string) (provider.Platform, error) {
	if p, ok := s.presets.Get(id); ok {
		return p, nil
	}
	rows, err := s.q.ListCustomPlatforms(ctx)
	if err != nil {
		return provider.Platform{}, fmt.Errorf("list custom platforms: %w", err)
	}
	for _, r := range rows {
		if r.ID == id {
			name := map[string]string{}
			if len(r.Name) > 0 {
				if err := json.Unmarshal(r.Name, &name); err != nil {
					return provider.Platform{}, fmt.Errorf("custom platform %q name: %w", id, err)
				}
			}
			return provider.Platform{
				ID: id, Name: name, Card: provider.CardStatic,
				NeedsExternalBrowser: r.NeedsExternalBrowser, Custom: true,
			}, nil
		}
	}
	return provider.Platform{ID: id, Name: map[string]string{}, Card: provider.CardStatic}, nil
}

// snapshotJoinData is the part of provider_snapshots.data used by /go.
// Decoding is lenient so new provider fields never break redirects.
type snapshotJoinData struct {
	InstantInviteURL string     `json:"instantInviteUrl"`
	InviteExpiresAt  *time.Time `json:"inviteExpiresAt"`
	InviteInvalid    bool       `json:"inviteInvalid"`
}

func decodeSnapshotData(data []byte) (snapshotJoinData, error) {
	var out snapshotJoinData
	if len(data) == 0 {
		return out, nil
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return out, fmt.Errorf("snapshot data: %w", err)
	}
	return out, nil
}

// decodeDisplay reads communities.display leniently (the writers decode
// strictly; /go must keep working across schema additions).
func decodeDisplay(data []byte) (site.CommunityDisplay, error) {
	var d site.CommunityDisplay
	if len(data) == 0 {
		return d, nil
	}
	if err := json.Unmarshal(data, &d); err != nil {
		return d, fmt.Errorf("display: %w", err)
	}
	return d, nil
}

func buildTarget(row dbq.GetGoCommunityRow, p provider.Platform, d site.CommunityDisplay, snap snapshotJoinData) CommunityTarget {
	state := provider.State("")
	if row.SnapshotState != nil {
		state = provider.State(*row.SnapshotState)
	}
	card := p.Card
	if card == "" {
		card = provider.CardStatic
	}
	t := CommunityTarget{
		ID: row.ID.String(), Slug: row.Slug, Platform: row.Platform,
		PlatformName:         site.LocalizedText(cloneStrings(p.Name)),
		NeedsExternalBrowser: p.NeedsExternalBrowser,
		Join: provider.JoinInput{
			Provider: row.Provider, Card: card, State: state,
			InviteURL: deref(row.InviteUrl), FallbackURL: deref(row.FallbackUrl),
			InviteInvalid:    snap.InviteInvalid,
			InstantInviteURL: snap.InstantInviteURL,
		},
		HasQR: row.QrID.Valid, QRID: row.QrID.String(),
		Name: cloneText(d.Name), Contact: cloneContact(d.Contact),
		UnavailableText: cloneText(d.UnavailableText),
	}
	if snap.InviteExpiresAt != nil {
		at := *snap.InviteExpiresAt
		t.Join.InstantInviteExpiresAt = &at
	}
	if card == provider.CardQQGroup {
		t.QQGroupNumber = d.QQGroupNumber
	}
	if row.Provider == providerStatic {
		t.Join.InviteInvalid, t.Join.InstantInviteURL, t.Join.InstantInviteExpiresAt = false, "", nil
	}
	return t
}

func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func cloneStrings(m map[string]string) map[string]string {
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func cloneText(t site.LocalizedText) site.LocalizedText {
	return site.LocalizedText(cloneStrings(t))
}

func cloneContact(c *site.ContactView) *site.ContactView {
	if c == nil {
		return nil
	}
	return &site.ContactView{Label: cloneText(c.Label), Value: c.Value}
}
