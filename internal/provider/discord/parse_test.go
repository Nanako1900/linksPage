package discord

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Nanako1900/linksPage/internal/provider/providertest"
)

type fixture struct {
	status int
	header http.Header
	body   []byte
}

func loadFixture(t *testing.T, name, bodyExt string) fixture {
	t.Helper()
	rec, err := providertest.LoadHeaders(filepath.Join("testdata", name+".headers"))
	if err != nil {
		t.Fatalf("load headers: %v", err)
	}
	body, err := os.ReadFile(filepath.Join("testdata", name+bodyExt))
	if err != nil {
		t.Fatalf("load body: %v", err)
	}
	return fixture{status: rec.Status, header: rec.Header, body: body}
}

func TestParseWidgetOK(t *testing.T) {
	f := loadFixture(t, "widget_ok", ".json")
	w, err := ParseWidget(f.status, f.header, f.body)
	if err != nil {
		t.Fatalf("ParseWidget: %v", err)
	}
	if w.ID != "1114391825336250432" || w.Name != "Nanako‘s Community" {
		t.Errorf("id/name = %q/%q", w.ID, w.Name)
	}
	if w.InstantInvite == nil || *w.InstantInvite != "https://discord.com/invite/KwdRuAkT" {
		t.Errorf("instant_invite = %v", w.InstantInvite)
	}
	if len(w.Channels) != 4 || w.PresenceCount != 16 || len(w.Members) != 16 {
		t.Errorf("channels=%d presence=%d members=%d", len(w.Channels), w.PresenceCount, len(w.Members))
	}
	games := 0
	for _, m := range w.Members {
		if !strings.HasPrefix(m.AvatarURL, "https://"+CDNHost+"/widget-avatars/") {
			t.Errorf("avatar_url %q not on widget-avatars", m.AvatarURL)
		}
		if m.Game != nil {
			games++
		}
	}
	if games != 5 {
		t.Errorf("members with game = %d, want 5", games)
	}
	if got := f.header.Get("Cache-Control"); got != "public, max-age=300, s-maxage=300" {
		t.Errorf("cache-control = %q", got)
	}
	if f.header.Get("Access-Control-Allow-Origin") != "" {
		t.Error("widget without Origin must not carry CORS headers")
	}
}

func TestParseInviteOK(t *testing.T) {
	f := loadFixture(t, "invite_ok", ".json")
	inv, err := ParseInvite(f.status, f.header, f.body)
	if err != nil {
		t.Fatalf("ParseInvite: %v", err)
	}
	if inv.Code != "KwdRuAkT" || inv.Guild.ID != "1114391825336250432" {
		t.Errorf("code/guild = %q/%q", inv.Code, inv.Guild.ID)
	}
	if inv.ApproximateMemberCount == nil || *inv.ApproximateMemberCount != 125 {
		t.Errorf("member count = %v", inv.ApproximateMemberCount)
	}
	if inv.ApproximatePresenceCount == nil || *inv.ApproximatePresenceCount != 16 {
		t.Errorf("presence count = %v", inv.ApproximatePresenceCount)
	}
	if inv.IsPermanent() {
		t.Error("recorded invite is temporary (instant_invite) and must not be permanent")
	}
	wantExp := time.Date(2026, 10, 6, 13, 47, 48, 0, time.UTC)
	if !inv.ExpiresAt.Equal(wantExp) {
		t.Errorf("expires_at = %v, want %v", inv.ExpiresAt, wantExp)
	}
	if inv.Guild.Icon == nil || inv.Guild.Splash == nil || inv.Guild.Banner != nil {
		t.Errorf("icon/splash/banner = %v/%v/%v", inv.Guild.Icon, inv.Guild.Splash, inv.Guild.Banner)
	}
	if inv.Channel == nil || inv.Channel.Name != "speak-here" {
		t.Errorf("channel = %+v", inv.Channel)
	}
	if f.header.Get("Cache-Control") != "" {
		t.Error("invite endpoint recorded without cache-control")
	}
}

func TestPermanentInvite(t *testing.T) {
	body := []byte(`{"code":"abc","expires_at":null,"guild":{"id":"1114391825336250432","name":"x"}}`)
	inv, err := ParseInvite(http.StatusOK, http.Header{}, body)
	if err != nil {
		t.Fatalf("ParseInvite: %v", err)
	}
	if !inv.IsPermanent() || inv.ApproximateMemberCount != nil {
		t.Errorf("permanent=%t counts=%v", inv.IsPermanent(), inv.ApproximateMemberCount)
	}
}

func TestParseErrorsFromFixtures(t *testing.T) {
	tests := []struct {
		fixture    string
		bodyExt    string
		invite     bool
		wantErr    error
		wantStatus int
		wantCode   int
		errCode    string
	}{
		{"widget_unknown_guild", ".json", false, ErrUnknownGuild, 404, CodeUnknownGuild, "discord_404_10004"},
		{"widget_disabled", ".json", false, ErrWidgetDisabled, 403, CodeWidgetDisabled, "discord_403_50004"},
		{"invite_unknown", ".json", true, ErrUnknownInvite, 404, CodeUnknownInvite, "discord_404_10006"},
	}
	for _, tc := range tests {
		t.Run(tc.fixture, func(t *testing.T) {
			f := loadFixture(t, tc.fixture, tc.bodyExt)
			var err error
			if tc.invite {
				_, err = ParseInvite(f.status, f.header, f.body)
			} else {
				_, err = ParseWidget(f.status, f.header, f.body)
			}
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
			var apiErr *APIError
			if !errors.As(err, &apiErr) {
				t.Fatalf("want *APIError, got %T", err)
			}
			if apiErr.HTTPStatus != tc.wantStatus || apiErr.Code != tc.wantCode {
				t.Errorf("status/code = %d/%d", apiErr.HTTPStatus, apiErr.Code)
			}
			if strings.Contains(err.Error(), apiErr.Message) {
				t.Errorf("error string %q leaks upstream message", err.Error())
			}
			if got := ErrorCode(err); got != tc.errCode {
				t.Errorf("ErrorCode = %q, want %q", got, tc.errCode)
			}
		})
	}
}

func TestRateLimitFixtures(t *testing.T) {
	tests := []struct {
		fixture    string
		bodyExt    string
		retryAfter time.Duration
		global     bool
		scope      string
		cloudflare bool
	}{
		{"ratelimited_synthetic", ".json", 64570 * time.Millisecond, false, "shared", false},
		{"ratelimited_global_synthetic", ".json", 1500 * time.Millisecond, true, "global", false},
		{"ratelimited_cloudflare_synthetic", ".html", 0, false, "", true},
	}
	for _, tc := range tests {
		t.Run(tc.fixture, func(t *testing.T) {
			f := loadFixture(t, tc.fixture, tc.bodyExt)
			_, err := ParseWidget(f.status, f.header, f.body)
			if !errors.Is(err, ErrRateLimited) {
				t.Fatalf("err = %v, want ErrRateLimited", err)
			}
			var rl *RateLimitError
			if !errors.As(err, &rl) {
				t.Fatalf("want *RateLimitError, got %T", err)
			}
			if rl.RetryAfter != tc.retryAfter || rl.Global != tc.global || rl.Scope != tc.scope || rl.Cloudflare != tc.cloudflare {
				t.Errorf("got %+v", rl)
			}
			if ErrorCode(err) != "discord_429" {
				t.Errorf("ErrorCode = %q", ErrorCode(err))
			}
			if rl.Error() == "" {
				t.Error("empty error string")
			}
		})
	}
}

func TestRateLimitRetryAfterHeader(t *testing.T) {
	tests := []struct {
		name   string
		header string
		body   string
		want   time.Duration
	}{
		{"header integer", "3", `{"message":"x"}`, 3 * time.Second},
		{"header fractional", "0.25", "", 250 * time.Millisecond},
		{"header invalid", "soon", "", 0},
		{"header http-date ignored", "Wed, 21 Oct 2015 07:28:00 GMT", "", 0},
		{"header negative", "-5", "", 0},
		{"body capped", "", `{"retry_after": 1e9}`, maxRetryAfter},
		{"body zero falls to zero", "", `{"retry_after": 0}`, 0},
		{"no hints", "", "", 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := http.Header{}
			if tc.header != "" {
				h.Set("Retry-After", tc.header)
			}
			_, err := ParseInvite(http.StatusTooManyRequests, h, []byte(tc.body))
			var rl *RateLimitError
			if !errors.As(err, &rl) {
				t.Fatalf("want *RateLimitError, got %v", err)
			}
			if rl.RetryAfter != tc.want {
				t.Errorf("RetryAfter = %v, want %v", rl.RetryAfter, tc.want)
			}
		})
	}
}

func TestParseMalformedAndUnknown(t *testing.T) {
	tests := []struct {
		name     string
		status   int
		body     string
		invite   bool
		wantErr  error
		wantCode string
	}{
		{"widget not json", 200, "<html>", false, ErrMalformed, "discord_malformed"},
		{"widget missing id", 200, `{"name":"x"}`, false, ErrMalformed, "discord_malformed"},
		{"invite not json", 200, "null}", true, ErrMalformed, "discord_malformed"},
		{"invite missing guild", 200, `{"code":"abc"}`, true, ErrMalformed, "discord_malformed"},
		{"5xx html", 502, "<html>bad gateway</html>", false, ErrUpstream, "discord_502_0"},
		{"unknown json code", 400, `{"code": 50035, "message": "Invalid Form Body"}`, true, ErrUpstream, "discord_400_50035"},
		{"json without code", 401, `{"message": "401: Unauthorized"}`, false, ErrUpstream, "discord_401_0"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var err error
			if tc.invite {
				_, err = ParseInvite(tc.status, http.Header{}, []byte(tc.body))
			} else {
				_, err = ParseWidget(tc.status, http.Header{}, []byte(tc.body))
			}
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
			if got := ErrorCode(err); got != tc.wantCode {
				t.Errorf("ErrorCode = %q, want %q", got, tc.wantCode)
			}
		})
	}
}

func TestErrorCodeEdgeCases(t *testing.T) {
	if ErrorCode(nil) != "" {
		t.Error("nil error must map to empty code")
	}
	if ErrorCode(errors.New("boom")) != "discord_error" {
		t.Error("generic error must map to discord_error")
	}
}

func TestCORSReflectedWithOrigin(t *testing.T) {
	for _, name := range []string{"widget_ok_with_origin", "invite_ok_with_origin"} {
		rec, err := providertest.LoadHeaders(filepath.Join("testdata", name+".headers"))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got := rec.Header.Get("Access-Control-Allow-Origin"); got != "https://example.com" {
			t.Errorf("%s: ACAO = %q, want reflected origin", name, got)
		}
	}
}
