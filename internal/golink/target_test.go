package golink

import (
	"strings"
	"testing"
	"time"

	"github.com/Nanako1900/linksPage/internal/provider"
)

var testNow = time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)

func TestJoinURL(t *testing.T) {
	future := testNow.Add(time.Hour)
	past := testNow.Add(-time.Second)
	tests := []struct {
		name string
		join provider.JoinInput
		want string
	}{
		{"discord permanent", provider.JoinInput{Provider: "discord", Card: "discord", State: provider.StateLive, InviteURL: "https://discord.gg/abc", InstantInviteURL: "https://discord.com/invite/tmp"}, "https://discord.gg/abc"},
		{"discord 10006 uses instant", provider.JoinInput{Provider: "discord", Card: "discord", State: provider.StateDegraded, InviteURL: "https://discord.gg/abc", InviteInvalid: true, InstantInviteURL: "https://discord.com/invite/tmp", InstantInviteExpiresAt: &future}, "https://discord.com/invite/tmp"},
		{"discord expired instant uses fallback", provider.JoinInput{Provider: "discord", Card: "discord", InviteInvalid: true, InviteURL: "https://discord.gg/abc", InstantInviteURL: "https://discord.com/invite/tmp", InstantInviteExpiresAt: &past, FallbackURL: "https://example.com/join"}, "https://example.com/join"},
		{"discord nothing", provider.JoinInput{Provider: "discord", Card: "discord", InviteInvalid: true, InviteURL: "https://discord.gg/abc"}, ""},
		{"unavailable", provider.JoinInput{Provider: "discord", Card: "discord", State: provider.StateUnavailable, InviteURL: "https://discord.gg/abc"}, ""},
		{"wechat group", provider.JoinInput{Provider: "static", Card: "wechat-group", FallbackURL: "https://example.com"}, ""},
		{"qq allowed host", provider.JoinInput{Provider: "static", Card: "qq-group", InviteURL: "https://qm.qq.com/q/abc"}, "https://qm.qq.com/q/abc"},
		{"qq allowed host case", provider.JoinInput{Provider: "static", Card: "qq-group", InviteURL: "https://QUN.qq.com/x"}, "https://QUN.qq.com/x"},
		{"qq foreign host dropped", provider.JoinInput{Provider: "static", Card: "qq-group", InviteURL: "https://evil.example/q", FallbackURL: "https://example.com/qq"}, "https://example.com/qq"},
		{"qq http dropped", provider.JoinInput{Provider: "static", Card: "qq-group", InviteURL: "http://qm.qq.com/q/abc"}, ""},
		{"qq lookalike dropped", provider.JoinInput{Provider: "static", Card: "qq-group", InviteURL: "https://qm.qq.com.evil.example/q"}, ""},
		{"static invite", provider.JoinInput{Provider: "static", Card: "static", InviteURL: "https://t.me/abc", FallbackURL: "https://example.com"}, "https://t.me/abc"},
		{"static fallback", provider.JoinInput{Provider: "static", Card: "static", FallbackURL: "http://example.com"}, "http://example.com"},
		{"javascript rejected", provider.JoinInput{Provider: "static", Card: "static", InviteURL: "javascript:alert(1)"}, ""},
		{"mailto rejected for community", provider.JoinInput{Provider: "static", Card: "static", InviteURL: "mailto:a@example.com"}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := JoinURL(CommunityTarget{Join: tt.join}, testNow); got != tt.want {
				t.Errorf("JoinURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSafeRedirect(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		ok   bool
	}{
		{"https", "https://example.com/a?b=c#d", true},
		{"http", "http://example.com", true},
		{"mailto", "mailto:me@example.com?subject=hi%20there", true},
		{"matrix fragment", "https://matrix.to/#/#room:example.org", true},
		{"empty", "", false},
		{"relative", "/go/x", false},
		{"protocol relative", "//evil.example", false},
		{"javascript", "javascript:alert(1)", false},
		{"data", "data:text/html,hi", false},
		{"credentials", "https://user:pw@example.com", false},
		{"space", "https://example.com/a b", false},
		{"newline", "https://example.com/\r\nSet-Cookie: x", false},
		{"backslash", "https:\\\\evil.example", false},
		{"empty mailto", "mailto:", false},
		{"too long", "https://example.com/" + strings.Repeat("a", maxRedirectURLLength), false},
		{"bad escape", "https://example.com/%zz", false},
		{"opaque https", "https:example.com", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := safeRedirect(tt.raw, linkSchemes)
			if ok != tt.ok {
				t.Fatalf("safeRedirect(%q) ok = %v, want %v", tt.raw, ok, tt.ok)
			}
			if ok && got != tt.raw {
				t.Errorf("safeRedirect changed the URL: %q", got)
			}
		})
	}
}

func TestQQJoinHostsIsCopy(t *testing.T) {
	h := QQJoinHosts()
	h[0] = "evil.example"
	if QQJoinHosts()[0] != "qm.qq.com" {
		t.Fatal("QQJoinHosts must return a copy")
	}
}
