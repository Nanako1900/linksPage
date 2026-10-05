package imgproxy

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/Nanako1900/linksPage/internal/provider"
	"github.com/Nanako1900/linksPage/internal/store/dbq"
)

// fakeStore is an in-memory media_proxy table.
type fakeStore struct {
	mu        sync.Mutex
	rows      map[string]dbq.MediaProxy
	upserts   int
	gets      int
	upsertErr error
	getErr    error
}

func newFakeStore() *fakeStore { return &fakeStore{rows: map[string]dbq.MediaProxy{}} }

func (f *fakeStore) UpsertMediaProxy(_ context.Context, arg dbq.UpsertMediaProxyParams) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.upserts++
	if f.upsertErr != nil {
		return f.upsertErr
	}
	f.rows[arg.Key] = dbq.MediaProxy{Key: arg.Key, Provider: arg.Provider, Url: arg.Url, Ext: arg.Ext, Kind: arg.Kind}
	return nil
}

func (f *fakeStore) GetMediaProxy(_ context.Context, key string) (dbq.MediaProxy, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.gets++
	if f.getErr != nil {
		return dbq.MediaProxy{}, f.getErr
	}
	row, ok := f.rows[key]
	if !ok {
		return dbq.MediaProxy{}, pgx.ErrNoRows
	}
	return row, nil
}

func (f *fakeStore) TouchMediaProxy(context.Context, []string) error { return nil }

func (f *fakeStore) counts() (upserts, gets int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.upserts, f.gets
}

var testKey = []byte(strings.Repeat("k", 32))

var discordHosts = map[string][]string{"cdn.discordapp.com": {"/icons/", "/banners/", "/splashes/", "/widget-avatars/"}}

func TestKey(t *testing.T) {
	const u = "https://cdn.discordapp.com/icons/1/a.png"
	mac := hmac.New(sha256.New, testKey)
	mac.Write([]byte(u))
	want := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))[:22]
	got := Key(testKey, u)
	if got != want || len(got) != KeyLen || !FileRe.MatchString(got+".png") {
		t.Errorf("Key = %q, want %q", got, want)
	}
	if Key([]byte(strings.Repeat("x", 32)), u) == got || Key(testKey, u+"?size=1") == got {
		t.Error("key must depend on secret and url")
	}
}

func TestNormalize(t *testing.T) {
	long := "https://cdn.discordapp.com/icons/" + strings.Repeat("a", MaxURLLength) + ".png"
	tests := []struct {
		raw, url, ext string
		err           error
	}{
		{raw: "https://cdn.discordapp.com/icons/1/abc.png?size=256", url: "https://cdn.discordapp.com/icons/1/abc.png?size=256", ext: "png"},
		{raw: "https://CDN.discordapp.com/icons/1/a_abc.gif", url: "https://cdn.discordapp.com/icons/1/a_abc.gif", ext: "gif"},
		{raw: "https://cdn.discordapp.com/widget-avatars/x/y", url: "https://cdn.discordapp.com/widget-avatars/x/y", ext: "png"},
		{raw: "https://cdn.discordapp.com/banners/1/b.JPEG", url: "https://cdn.discordapp.com/banners/1/b.JPEG", ext: "jpg"},
		{raw: "https://cdn.discordapp.com/splashes/1/s.webp", url: "https://cdn.discordapp.com/splashes/1/s.webp", ext: "webp"},
		{raw: "https://cdn.discordapp.com/icons//1/./a.jpg", url: "https://cdn.discordapp.com/icons/1/a.jpg", ext: "jpg"},
		{raw: "https://evil.example/icons/1/a.png", err: ErrHostNotAllowed},
		{raw: "https://cdn.discordapp.com/attachments/1/a.png", err: ErrHostNotAllowed},
		{raw: "https://cdn.discordapp.com/icons/", err: ErrHostNotAllowed},
		{raw: "http://cdn.discordapp.com/icons/1/a.png", err: ErrInvalidURL},
		{raw: "https://u:p@cdn.discordapp.com/icons/1/a.png", err: ErrInvalidURL},
		{raw: "https://cdn.discordapp.com:8443/icons/1/a.png", err: ErrInvalidURL},
		{raw: "https:cdn.discordapp.com/icons/1/a.png", err: ErrInvalidURL},
		{raw: "https://cdn.discordapp.com/icons/1/%2e%2e/a.png", err: ErrInvalidURL},
		{raw: `https://cdn.discordapp.com/icons/1\a.png`, err: ErrInvalidURL},
		{raw: "https://cdn.discordapp.com/icons/../attachments/a.png", err: ErrInvalidURL},
		{raw: "https://cdn.discordapp.com/icons/1/a.png#x", err: ErrInvalidURL},
		{raw: "https://cdn.discordapp.com/icons/1/a.png?size=256&x=1", err: ErrInvalidURL},
		{raw: "https://cdn.discordapp.com/icons/1/a.png?size=big", err: ErrInvalidURL},
		{raw: "https://cdn.discordapp.com/icons/1/a.png?size=1;x", err: ErrInvalidURL},
		{raw: "https://cdn.discordapp.com/icons/1/a.svg", err: ErrInvalidURL},
		{raw: "https://cdn.discordapp.com/icons/1/a", err: ErrInvalidURL},
		{raw: "https://cdn.discordapp.com/icons/1/a b.png\x7f", err: ErrInvalidURL},
		{raw: long, err: ErrInvalidURL},
	}
	for _, tt := range tests {
		got, err := Normalize(tt.raw, discordHosts)
		if tt.err != nil {
			if !errors.Is(err, tt.err) {
				t.Errorf("Normalize(%q) err = %v, want %v", tt.raw, err, tt.err)
			}
			continue
		}
		if err != nil || got.URL != tt.url || got.Ext != tt.ext {
			t.Errorf("Normalize(%q) = %+v, %v", tt.raw, got, err)
		}
	}
}

func TestRegistrar(t *testing.T) {
	store := newFakeStore()
	hosts := map[string]map[string][]string{"discord": {"cdn.discordapp.com": discordHosts["cdn.discordapp.com"]}, "kook": {}}
	r, err := NewRegistrar(store, testKey, hosts)
	if err != nil {
		t.Fatal(err)
	}
	hosts["discord"]["evil.example"] = []string{"/"}
	ctx := context.Background()
	const raw = "https://cdn.discordapp.com/widget-avatars/x/y"
	p, err := r.Register(ctx, "discord", provider.ImageAvatar, raw)
	if err != nil {
		t.Fatal(err)
	}
	key := Key(testKey, raw)
	if p != PathPrefix+key+".png" {
		t.Errorf("path = %q", p)
	}
	if row := store.rows[key]; row.Kind != "avatar" || row.Ext != "png" || row.Provider != "discord" || row.Url != raw {
		t.Errorf("row = %+v", row)
	}
	if again, _ := r.Register(ctx, "discord", provider.ImageAvatar, raw); again != p {
		t.Error("registration must be stable")
	}
	if up, _ := store.counts(); up != 1 {
		t.Errorf("upserts = %d, want 1 (cached)", up)
	}
	errTests := []struct {
		provider, kind, url string
		want                error
	}{
		{"discord", "emoji", raw, ErrInvalidKind},
		{"telegram", provider.ImageIcon, raw, ErrInvalidKind},
		{"kook", provider.ImageIcon, raw, ErrHostNotAllowed},
		{"discord", provider.ImageIcon, "https://evil.example/a.png", ErrHostNotAllowed},
	}
	for _, tt := range errTests {
		if _, err := r.Register(ctx, tt.provider, tt.kind, tt.url); !errors.Is(err, tt.want) {
			t.Errorf("Register(%s,%s,%s) = %v, want %v", tt.provider, tt.kind, tt.url, err, tt.want)
		}
	}
	store.upsertErr = errors.New("db down")
	if _, err := r.Register(ctx, "discord", provider.ImageIcon, "https://cdn.discordapp.com/icons/1/z.png"); err == nil {
		t.Error("store error must propagate")
	}
}

func TestNewRegistrarValidation(t *testing.T) {
	if _, err := NewRegistrar(nil, testKey, nil); err == nil {
		t.Error("nil store must fail")
	}
	if _, err := NewRegistrar(newFakeStore(), []byte("short"), nil); err == nil {
		t.Error("short key must fail")
	}
}

func dbqRow(key, rawURL, kind, ext string) dbq.MediaProxy {
	return dbq.MediaProxy{Key: key, Provider: "discord", Url: rawURL, Ext: ext, Kind: kind}
}
