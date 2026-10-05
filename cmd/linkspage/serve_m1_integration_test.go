package main

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/Nanako1900/linksPage/internal/config"
	"github.com/Nanako1900/linksPage/internal/site"
)

const m1Seed = `version: 1
site:
  title: { zh-CN: 测试小屋, en: Test Lodge }
communities:
  - slug: discord
    platform: discord
    name: { zh-CN: Discord, en: Discord }
    guild_id: "1114391825336250432"
  - slug: qq
    platform: qq-group
    name: { zh-CN: QQ 群 }
    qq_group: "123456789"
  - slug: wechat
    platform: wechat-group
    name: { zh-CN: 微信群 }
    qr: { image: images/qr.png }
links:
  - slug: blog
    label: { en: Blog }
    url: https://blog.example.com
    icon: builtin:link
`

const m1ProxyAuth = "integration-proxy-auth-secret-0123456789"

// writeM1Seed writes the seed file and a QR image into a temp dir.
func writeM1Seed(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "images"), 0o750); err != nil {
		t.Fatal(err)
	}
	img := image.NewGray(image.Rect(0, 0, 256, 256))
	for i := range img.Pix {
		img.Pix[i] = uint8(i % 7 * 36)
	}
	img.Set(0, 0, color.White)
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "images", "qr.png"), buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "seed.yaml")
	if err := os.WriteFile(path, []byte(m1Seed), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// fakeDiscord serves the recorded widget fixture for every widget lookup.
func fakeDiscord(t *testing.T) *httptest.Server {
	t.Helper()
	widget, err := os.ReadFile(filepath.Join("..", "..", "internal", "provider", "discord", "testdata", "widget_ok.json"))
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/widget.json") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(widget)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func startPostgres(ctx context.Context, t *testing.T) *url.URL {
	t.Helper()
	testcontainers.SkipIfProviderIsNotHealthy(t)
	ctr, err := postgres.Run(ctx, "postgres:18-alpine", postgres.WithDatabase("linkspage"),
		postgres.WithUsername("linkspage"), postgres.WithPassword("pw"), postgres.BasicWaitStrategies())
	testcontainers.CleanupContainer(t, ctr)
	if err != nil {
		t.Fatal(err)
	}
	dsn, err := ctr.ConnectionString(ctx)
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

// noRedirect is a client that reports redirects instead of following them.
var noRedirect = &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

func fetch(ctx context.Context, t *testing.T, u string, hdr map[string]string) *http.Response {
	t.Helper()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := noRedirect.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", u, err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

func decodeData[T any](t *testing.T, resp *http.Response) T {
	t.Helper()
	var env struct {
		Data T `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatal(err)
	}
	return env.Data
}

// TestIntegrationServeM1 boots the server with a seed file and a fake
// Discord API and checks every M1 route end to end.
func TestIntegrationServeM1(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test skipped in -short mode")
	}
	ctx := context.Background()
	u := startPostgres(ctx, t)
	for k, v := range map[string]string{
		"LP_CONFIG_FILE": t.TempDir() + "/none.yaml", "LP_BASE_URL": "http://127.0.0.1",
		"LP_DATA_DIR": t.TempDir(), "LP_SEED_FILE": writeM1Seed(t),
		"LP_PROVIDERS__DISCORD__API_BASE": fakeDiscord(t).URL, "LP_EDGE__PROXY_AUTH": m1ProxyAuth,
		"LP_DB__HOST": u.Hostname(), "LP_DB__PORT": u.Port(), "LP_DB__USER": "linkspage",
		"LP_DB__NAME": "linkspage", "LP_DB__PASSWORD": "pw", "LP_LOG__LEVEL": "error",
	} {
		t.Setenv(k, v)
	}
	loaded, err := config.Load(config.Options{})
	if err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	base := "http://" + ln.Addr().String()
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	var logs syncBuffer
	logger := slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelWarn}))
	done := make(chan error, 1)
	go func() {
		done <- serve(runCtx, loaded.Config, logger, func(context.Context, string) (net.Listener, error) { return ln, nil })
	}()
	waitStatus(ctx, t, base+"/readyz", http.StatusOK, 90*time.Second)

	page := decodeData[site.PublicPage](t, fetch(ctx, t, base+"/api/v1/public/bootstrap", nil))
	if len(page.Communities) != 3 || len(page.Links) != 1 || page.Validate() != nil {
		t.Fatalf("seeded page = %d communities %d links, validate %v", len(page.Communities), len(page.Links), page.Validate())
	}
	checkLive(ctx, t, base)
	checkGoLinks(ctx, t, base)
	checkMediaAndFiles(ctx, t, base, page)
	checkRender(ctx, t, base)

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("serve: %v\n%s", err, logs.String())
		}
	case <-time.After(20 * time.Second):
		t.Fatal("serve did not shut down")
	}
}

// checkLive waits for the Discord refresh and checks ETag/304.
func checkLive(ctx context.Context, t *testing.T, base string) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for {
		live := decodeData[site.LiveDTO](t, fetch(ctx, t, base+"/api/v1/public/live", nil))
		var discord *site.LiveView
		for _, v := range live.Communities {
			if v.Online != nil {
				discord = &v
			}
		}
		if discord != nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("discord card never became live: %+v", live.Communities)
		}
		time.Sleep(300 * time.Millisecond)
	}
	first := fetch(ctx, t, base+"/api/v1/public/live", nil)
	tag := first.Header.Get("ETag")
	if first.Header.Get("Cache-Control") != "public, max-age=0, s-maxage=30" || tag == "" {
		t.Errorf("live headers = %v", first.Header)
	}
	if again := fetch(ctx, t, base+"/api/v1/public/live", map[string]string{"If-None-Match": tag}); again.StatusCode != http.StatusNotModified {
		t.Errorf("conditional live = %d", again.StatusCode)
	}
}

func checkGoLinks(ctx context.Context, t *testing.T, base string) {
	t.Helper()
	wechatUA := "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 Mobile/15E148 MicroMessenger/8.0.50"
	tests := []struct {
		path, ua string
		status   int
		location string
	}{
		{"/go/blog", "", 302, "https://blog.example.com"},
		{"/go/discord", "", 302, "https://discord.com/invite/KwdRuAkT"},
		{"/go/discord", wechatUA, 200, ""},
		{"/go/wechat", "", 200, ""},
		{"/go/nope", "", 404, ""},
	}
	for _, tt := range tests {
		resp := fetch(ctx, t, base+tt.path, map[string]string{"User-Agent": tt.ua})
		if resp.StatusCode != tt.status || resp.Header.Get("Location") != tt.location ||
			resp.Header.Get("Cache-Control") != "no-store" {
			t.Errorf("%s (%q) = %d %q %q", tt.path, tt.ua, resp.StatusCode, resp.Header.Get("Location"), resp.Header.Get("Cache-Control"))
		}
	}
}

func checkMediaAndFiles(ctx context.Context, t *testing.T, base string, page site.PublicPage) {
	t.Helper()
	var qrURL string
	for _, c := range page.Communities {
		if c.QR != nil {
			qrURL = c.QR.URL
		}
	}
	tests := []struct {
		path        string
		status      int
		contentType string
	}{
		{qrURL, 200, "image/png"},
		{"/media/q/01900000-0000-7000-8000-000000000000", 410, ""},
		{"/media/q/not-a-uuid", 404, ""},
		{"/media/u/0123456789abcdef0123456789abcdef.webp", 404, ""},
		{"/media/p/AAAAAAAAAAAAAAAAAAAAAA.png", 404, ""},
		{"/favicon.ico", 200, "image/png"},
		{"/robots.txt", 200, "text/plain; charset=utf-8"},
		{"/site.webmanifest", 200, "application/manifest+json"},
		{"/c/discord", 200, "text/html; charset=utf-8"},
		{"/c/nope", 404, "text/html; charset=utf-8"},
	}
	for _, tt := range tests {
		resp := fetch(ctx, t, base+tt.path, nil)
		if resp.StatusCode != tt.status || (tt.contentType != "" && resp.Header.Get("Content-Type") != tt.contentType) {
			t.Errorf("%s = %d %q", tt.path, resp.StatusCode, resp.Header.Get("Content-Type"))
		}
	}
}

func checkRender(ctx context.Context, t *testing.T, base string) {
	t.Helper()
	target := base + "/api/v1/public/render?path=/c/discord&lang=en"
	if resp := fetch(ctx, t, target, nil); resp.StatusCode != http.StatusForbidden {
		t.Errorf("render without proxy auth = %d", resp.StatusCode)
	}
	resp := fetch(ctx, t, target, map[string]string{"X-LP-Proxy-Auth": m1ProxyAuth})
	dto := decodeData[site.RenderDTO](t, resp)
	if resp.StatusCode != http.StatusOK || dto.Status != http.StatusOK || dto.Data == nil || !strings.Contains(dto.CSP, "script-src") {
		t.Errorf("render = %d status %d", resp.StatusCode, dto.Status)
	}
}
