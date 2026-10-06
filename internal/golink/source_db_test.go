package golink

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/Nanako1900/linksPage/internal/provider"
	"github.com/Nanako1900/linksPage/internal/store/dbq"
)

type fakeQuerier struct {
	communities map[string]dbq.GetGoCommunityRow
	links       map[string]dbq.GetGoLinkRow
	custom      []dbq.CustomPlatform
	err         error
	customErr   error
}

func (f fakeQuerier) GetGoCommunity(_ context.Context, slug string) (dbq.GetGoCommunityRow, error) {
	if f.err != nil {
		return dbq.GetGoCommunityRow{}, f.err
	}
	r, ok := f.communities[slug]
	if !ok {
		return dbq.GetGoCommunityRow{}, pgx.ErrNoRows
	}
	return r, nil
}

// GetGoLink mimics the query: links that are not published on the page
// are simply absent from the map (pgx.ErrNoRows).
func (f fakeQuerier) GetGoLink(_ context.Context, slug string) (dbq.GetGoLinkRow, error) {
	if f.err != nil {
		return dbq.GetGoLinkRow{}, f.err
	}
	l, ok := f.links[slug]
	if !ok {
		return dbq.GetGoLinkRow{}, pgx.ErrNoRows
	}
	return l, nil
}

func (f fakeQuerier) ListCustomPlatforms(context.Context) ([]dbq.CustomPlatform, error) {
	return f.custom, f.customErr
}

type fakeCatalog map[string]provider.Platform

func (c fakeCatalog) Get(id string) (provider.Platform, bool) {
	p, ok := c[id]
	return p, ok
}

var testCatalog = fakeCatalog{
	"discord":  {ID: "discord", Name: map[string]string{"en": "Discord"}, Provider: "discord", Card: provider.CardDiscord, NeedsExternalBrowser: true},
	"qq-group": {ID: "qq-group", Name: map[string]string{"en": "QQ Group", "zh-CN": "QQ 群"}, Card: provider.CardQQGroup},
}

func uuidOf(b byte) pgtype.UUID {
	var u pgtype.UUID
	for i := range u.Bytes {
		u.Bytes[i] = b
	}
	u.Valid = true
	return u
}

func strp(s string) *string { return &s }

func TestNewDBSourceValidation(t *testing.T) {
	if _, err := NewDBSource(nil, testCatalog); err == nil {
		t.Error("nil querier accepted")
	}
	if _, err := NewDBSource(fakeQuerier{}, nil); err == nil {
		t.Error("nil catalog accepted")
	}
}

func TestDBSourceCommunity(t *testing.T) {
	q := fakeQuerier{
		communities: map[string]dbq.GetGoCommunityRow{
			"discord": {
				ID: uuidOf(1), Slug: "discord", Provider: "discord", Platform: "discord",
				Display:       []byte(`{"name":{"en":"Hub"},"contact":{"label":{"en":"Mod"},"value":"mod#1"},"futureKey":1}`),
				InviteUrl:     strp("https://discord.gg/abc"),
				SnapshotState: strp("degraded"),
				SnapshotData:  []byte(`{"inviteInvalid":true,"instantInviteUrl":"https://discord.com/invite/tmp","inviteExpiresAt":"2026-10-05T13:00:00Z","users":null}`),
			},
			"qq": {
				ID: uuidOf(2), Slug: "qq", Provider: "static", Platform: "qq-group",
				Display: []byte(`{"name":{"zh-CN":"群"},"qqGroupNumber":"123456"}`), QrID: uuidOf(3),
				SnapshotData: []byte(`{"inviteInvalid":true,"instantInviteUrl":"https://x.example"}`),
			},
			"custom": {ID: uuidOf(4), Slug: "custom", Provider: "static", Platform: "heybox", Display: []byte(`{"name":{"en":"Box"}}`), FallbackUrl: strp("https://heybox.example")},
			"orphan": {ID: uuidOf(5), Slug: "orphan", Provider: "static", Platform: "gone", Display: []byte(`{"name":{"en":"O"}}`)},
			"bad":    {ID: uuidOf(6), Slug: "bad", Provider: "static", Platform: "qq-group", Display: []byte(`{`)},
			"badsn":  {ID: uuidOf(7), Slug: "badsn", Provider: "discord", Platform: "discord", Display: []byte(`{}`), SnapshotData: []byte(`[`)},
		},
		custom: []dbq.CustomPlatform{{ID: "heybox", Name: []byte(`{"en":"Heybox"}`), NeedsExternalBrowser: true}},
	}
	src, err := NewDBSource(q, testCatalog)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	d, err := src.Community(ctx, "discord")
	if err != nil {
		t.Fatal(err)
	}
	wantExp := time.Date(2026, 10, 5, 13, 0, 0, 0, time.UTC)
	if d.ID != uuidOf(1).String() || !d.NeedsExternalBrowser || d.Join.Card != provider.CardDiscord ||
		d.Join.State != provider.StateDegraded || !d.Join.InviteInvalid || d.Join.InstantInviteURL == "" ||
		d.Join.InstantInviteExpiresAt == nil || !d.Join.InstantInviteExpiresAt.Equal(wantExp) ||
		d.Name["en"] != "Hub" || d.Contact == nil || d.Contact.Value != "mod#1" || d.PlatformName["en"] != "Discord" || d.HasQR {
		t.Errorf("discord target = %+v", d)
	}

	qq, err := src.Community(ctx, "qq")
	if err != nil {
		t.Fatal(err)
	}
	if qq.QQGroupNumber != "123456" || !qq.HasQR || qq.QRID != uuidOf(3).String() || qq.Join.State != "" ||
		qq.Join.InviteInvalid || qq.Join.InstantInviteURL != "" || qq.Join.Provider != "static" {
		t.Errorf("qq target = %+v", qq)
	}

	c, err := src.Community(ctx, "custom")
	if err != nil {
		t.Fatal(err)
	}
	if !c.NeedsExternalBrowser || c.Join.Card != provider.CardStatic || c.PlatformName["en"] != "Heybox" || c.Join.FallbackURL == "" {
		t.Errorf("custom target = %+v", c)
	}

	o, err := src.Community(ctx, "orphan")
	if err != nil {
		t.Fatal(err)
	}
	if o.NeedsExternalBrowser || o.Join.Card != provider.CardStatic {
		t.Errorf("orphan target = %+v", o)
	}
}

func TestDBSourceCommunityErrors(t *testing.T) {
	boom := errors.New("boom")
	base := fakeQuerier{communities: map[string]dbq.GetGoCommunityRow{
		"bad":    {Slug: "bad", Provider: "static", Platform: "qq-group", Display: []byte(`{`)},
		"badsn":  {Slug: "badsn", Provider: "discord", Platform: "discord", Display: []byte(`{}`), SnapshotData: []byte(`[`)},
		"custom": {Slug: "custom", Provider: "static", Platform: "heybox", Display: []byte(`{}`)},
	}}
	badName := base
	badName.custom = []dbq.CustomPlatform{{ID: "heybox", Name: []byte(`nope`)}}
	listErr := base
	listErr.customErr = boom
	tests := []struct {
		name     string
		q        fakeQuerier
		slug     string
		notFound bool
		wrapped  error
	}{
		{"missing", base, "nope", true, nil},
		{"db error", fakeQuerier{err: boom}, "x", false, boom},
		{"bad display", base, "bad", false, nil},
		{"bad snapshot", base, "badsn", false, nil},
		{"custom list error", listErr, "custom", false, boom},
		{"custom bad name", badName, "custom", false, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src, _ := NewDBSource(tt.q, testCatalog)
			_, err := src.Community(context.Background(), tt.slug)
			if err == nil {
				t.Fatal("expected an error")
			}
			if errors.Is(err, ErrNotFound) != tt.notFound {
				t.Errorf("ErrNotFound = %v, want %v (%v)", errors.Is(err, ErrNotFound), tt.notFound, err)
			}
			if tt.wrapped != nil && !errors.Is(err, tt.wrapped) {
				t.Errorf("err = %v, want wrapped %v", err, tt.wrapped)
			}
		})
	}
}

func TestDBSourceLink(t *testing.T) {
	boom := errors.New("boom")
	q := fakeQuerier{links: map[string]dbq.GetGoLinkRow{"blog": {ID: uuidOf(9), Slug: "blog", Url: "https://blog.example"}}}
	src, _ := NewDBSource(q, testCatalog)
	l, err := src.Link(context.Background(), "blog")
	if err != nil || l.URL != "https://blog.example" || l.ID != uuidOf(9).String() || l.Slug != "blog" {
		t.Fatalf("Link = %+v, %v", l, err)
	}
	if _, err := src.Link(context.Background(), "nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing link err = %v", err)
	}
	src, _ = NewDBSource(fakeQuerier{err: boom}, testCatalog)
	if _, err := src.Link(context.Background(), "blog"); !errors.Is(err, boom) {
		t.Errorf("db error = %v", err)
	}
}

func TestDBSourceEmptyJSON(t *testing.T) {
	q := fakeQuerier{communities: map[string]dbq.GetGoCommunityRow{
		"x": {ID: uuidOf(1), Slug: "x", Provider: "discord", Platform: "discord"},
	}}
	src, _ := NewDBSource(q, testCatalog)
	c, err := src.Community(context.Background(), "x")
	if err != nil {
		t.Fatal(err)
	}
	if c.Name == nil || c.UnavailableText == nil || c.Contact != nil {
		t.Errorf("empty display target = %+v", c)
	}
}
