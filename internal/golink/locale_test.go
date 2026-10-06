package golink

import (
	"reflect"
	"strings"
	"testing"

	"github.com/Nanako1900/linksPage/internal/site"
)

func TestNegotiateLocale(t *testing.T) {
	locales := []string{"zh-CN", "en"}
	tests := []struct {
		name, lang, accept, want string
	}{
		{"default", "", "", "zh-CN"},
		{"lang param", "en", "zh-CN", "en"},
		{"unknown lang param", "fr", "", "zh-CN"},
		{"lang param is exact", "EN", "", "zh-CN"},
		{"accept exact", "", "en-US,en;q=0.9", "en"},
		{"accept primary match", "", "en-GB", "en"},
		{"accept zh-TW matches zh-CN", "", "zh-TW", "zh-CN"},
		{"accept case-insensitive", "", "ZH-cn", "zh-CN"},
		{"accept quality order", "", "zh-CN;q=0.5,en;q=0.8", "en"},
		{"accept skips unknown", "", "fr-FR,de;q=0.9,en;q=0.1", "en"},
		{"accept q=0 ignored", "", "en;q=0", "zh-CN"},
		{"accept wildcard ignored", "", "*", "zh-CN"},
		{"accept malformed", "", "en;q=abc, <script>", "zh-CN"},
		{"accept q out of range", "", "en;q=2", "zh-CN"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := negotiateLocale(locales, "zh-CN", tt.lang, tt.accept); got != tt.want {
				t.Errorf("negotiateLocale() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseAcceptLanguageBounded(t *testing.T) {
	header := strings.Repeat("fr,", 40) + "en"
	if got := parseAcceptLanguage(header); len(got) != maxAcceptLanguageTags {
		t.Fatalf("parsed %d tags, want %d", len(got), maxAcceptLanguageTags)
	}
	if got := parseAcceptLanguage("de;q=0.5, en , ja;q=0.5"); !reflect.DeepEqual(got, []string{"en", "de", "ja"}) {
		t.Errorf("order = %v", got)
	}
	if validTag(strings.Repeat("a", 36)) {
		t.Error("overlong tag accepted")
	}
}

func TestTextForAndCopy(t *testing.T) {
	if textFor("zh-TW").CopyLink != guideTexts["zh"].CopyLink || textFor("ja").CopyLink != guideTexts["en"].CopyLink {
		t.Fatal("textFor picks the wrong language")
	}
	s := site.Default()
	s.Copy = site.CopyOverrides{"en": {site.CopyOpenInBrowser: "Use your browser"}}
	tests := []struct {
		locale, key, want string
	}{
		{"en", site.CopyOpenInBrowser, "Use your browser"},
		{"zh-CN", site.CopyOpenInBrowser, "Use your browser"}, // falls back to copy.en
		{"zh-CN", site.CopyInviteUnavailable, guideTexts["zh"].InviteUnavailable},
		{"zh-CN", site.CopyCommunityUnavailable, guideTexts["zh"].CommunityUnavailable},
		{"zh-CN", "unknown", ""},
	}
	for _, tt := range tests {
		if got := copyText(s, tt.locale, tt.key, textFor(tt.locale)); got != tt.want {
			t.Errorf("copyText(%s,%s) = %q, want %q", tt.locale, tt.key, got, tt.want)
		}
	}
}
