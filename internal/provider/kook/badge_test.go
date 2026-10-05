package kook

import (
	"errors"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Nanako1900/linksPage/internal/provider/providertest"
)

const shields = "https://img.shields.io/static/v1?"

func loadRecorded(t *testing.T, name string) providertest.Recorded {
	t.Helper()
	rec, err := providertest.LoadHeaders(filepath.Join("testdata", name+".headers"))
	if err != nil {
		t.Fatalf("load %s: %v", name, err)
	}
	return rec
}

func TestParseBadgeResponseFixtures(t *testing.T) {
	tests := []struct {
		fixture string
		style   Style
		want    Badge
		wantErr error
	}{
		{"badge_public_style0", StyleName, Badge{Kind: KindName, Label: "「猎杀对决」中文玩家社区", Name: "「猎杀对决」中文玩家社区"}, nil},
		{"badge_public_style1", StyleOnline, Badge{Kind: KindOnline, Label: "10350 ONLINE", Online: 10350}, nil},
		{"badge_public_style2", StyleOnlineTotal, Badge{Kind: KindOnlineTotal, Label: "10350/107345 ONLINE", Online: 10350, Total: 107345}, nil},
		{"badge_public_style3_with_origin", StyleName, Badge{Kind: KindName, Label: "「猎杀对决」中文玩家社区", Name: "「猎杀对决」中文玩家社区"}, nil},
		{"badge_not_public_style0", StyleName, Badge{}, ErrNotPublic},
		{"badge_not_public_style2", StyleOnlineTotal, Badge{}, ErrNotPublic},
		{"badge_invalid_guild_id", StyleName, Badge{}, ErrNotPublic},
		{"badge_missing_guild_id", StyleName, Badge{}, ErrNotPublic},
	}
	for _, tc := range tests {
		t.Run(tc.fixture, func(t *testing.T) {
			rec := loadRecorded(t, tc.fixture)
			if rec.Status != http.StatusFound {
				t.Fatalf("recorded status = %d, want 302", rec.Status)
			}
			if rec.Header.Get("Cache-Control") != "" || rec.Header.Get("Access-Control-Allow-Origin") != "" {
				t.Error("badge responses were recorded without cache-control and CORS headers")
			}
			got, err := ParseBadgeResponse(rec.Status, rec.Header, tc.style)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("err = %v, want %v", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %+v, want %+v", got, tc.want)
			}
			auto, err := ParseBadgeLocation(rec.Header.Get("Location"))
			if err != nil || auto != tc.want {
				t.Errorf("auto-detect got %+v, %v; want %+v", auto, err, tc.want)
			}
		})
	}
}

func TestParseBadgeLocation(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    Badge
		wantErr error
	}{
		{"name with plus space", shields + "label=My+Server&message=JOIN", Badge{Kind: KindName, Label: "My Server", Name: "My Server"}, nil},
		{"name with literal plus", shields + "label=C%2B%2B+Club&message=JOIN", Badge{Kind: KindName, Label: "C++ Club", Name: "C++ Club"}, nil},
		{"online", shields + "label=0+ONLINE&message=JOIN", Badge{Kind: KindOnline, Label: "0 ONLINE", Online: 0}, nil},
		{"online total", shields + "label=3%2F10+ONLINE", Badge{Kind: KindOnlineTotal, Label: "3/10 ONLINE", Online: 3, Total: 10}, nil},
		{"online greater than total is a name", shields + "label=11%2F10+ONLINE", Badge{Kind: KindName, Label: "11/10 ONLINE", Name: "11/10 ONLINE"}, nil},
		{"lowercase online is a name", shields + "label=5+online", Badge{Kind: KindName, Label: "5 online", Name: "5 online"}, nil},
		{"huge number is a name", shields + "label=12345678901+ONLINE", Badge{Kind: KindName, Label: "12345678901 ONLINE", Name: "12345678901 ONLINE"}, nil},
		{"trimmed", shields + "label=+Spaced+", Badge{Kind: KindName, Label: "Spaced", Name: "Spaced"}, nil},
		{"not public label", shields + "label=" + "%E6%9C%8D%E5%8A%A1%E5%99%A8%E4%B8%8D%E5%AD%98%E5%9C%A8%E6%88%96%E9%9D%9E%E5%85%AC%E5%BC%80" + "&message=404", Badge{}, ErrNotPublic},
		{"message 404 only", shields + "label=Whatever&message=404", Badge{}, ErrNotPublic},
		{"empty", "", Badge{}, ErrMalformed},
		{"unparseable", "https://img.shields.io/static/v1?label=%zz", Badge{}, ErrMalformed},
		{"bad url", "://", Badge{}, ErrMalformed},
		{"http scheme", "http://img.shields.io/static/v1?label=x", Badge{}, ErrMalformed},
		{"other host", "https://evil.example/static/v1?label=x", Badge{}, ErrMalformed},
		{"lookalike host", "https://img.shields.io.evil.example/static/v1?label=x", Badge{}, ErrMalformed},
		{"host with port", "https://img.shields.io:8443/static/v1?label=x", Badge{}, ErrMalformed},
		{"userinfo", "https://a@img.shields.io/static/v1?label=x", Badge{}, ErrMalformed},
		{"other path", "https://img.shields.io/badge/x-y-green", Badge{}, ErrMalformed},
		{"missing label", shields + "message=JOIN", Badge{}, ErrMalformed},
		{"blank label", shields + "label=+++", Badge{}, ErrMalformed},
		{"invalid utf8", shields + "label=%FF%FE", Badge{}, ErrMalformed},
		{"too long", shields + "label=" + strings.Repeat("a", maxLabelBytes+1), Badge{}, ErrMalformed},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseBadgeLocation(tc.in)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("err = %v, want %v", err, tc.wantErr)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("got %+v, %v; want %+v", got, err, tc.want)
			}
		})
	}
}

func TestParseBadgeLocationForStyle(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		style   Style
		want    Badge
		wantErr error
	}{
		{"count-shaped name stays a name", shields + "label=5+ONLINE", StyleName, Badge{Kind: KindName, Label: "5 ONLINE", Name: "5 ONLINE"}, nil},
		{"style1 ok", shields + "label=5+ONLINE", StyleOnline, Badge{Kind: KindOnline, Label: "5 ONLINE", Online: 5}, nil},
		{"style1 mismatch", shields + "label=Server", StyleOnline, Badge{}, ErrMalformed},
		{"style2 mismatch", shields + "label=5+ONLINE", StyleOnlineTotal, Badge{}, ErrMalformed},
		{"style2 online above total", shields + "label=9%2F3+ONLINE", StyleOnlineTotal, Badge{}, ErrMalformed},
		{"unknown style", shields + "label=x", Style(7), Badge{}, ErrInvalidStyle},
		{"not public wins", shields + "label=x&message=404", StyleOnline, Badge{}, ErrNotPublic},
		{"malformed wins", "https://evil.example/static/v1?label=x", StyleName, Badge{}, ErrMalformed},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseBadgeLocationForStyle(tc.in, tc.style)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("err = %v, want %v", err, tc.wantErr)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("got %+v, %v; want %+v", got, err, tc.want)
			}
		})
	}
}

func TestParseBadgeResponseStatus(t *testing.T) {
	loc := http.Header{"Location": {shields + "label=Server"}}
	for _, status := range []int{301, 302, 303, 307, 308} {
		if _, err := ParseBadgeResponse(status, loc, StyleName); err != nil {
			t.Errorf("status %d: %v", status, err)
		}
	}
	for _, status := range []int{200, 304, 403, 429, 500} {
		if _, err := ParseBadgeResponse(status, loc, StyleName); !errors.Is(err, ErrUnexpectedStatus) {
			t.Errorf("status %d: err = %v, want ErrUnexpectedStatus", status, err)
		}
	}
	if _, err := ParseBadgeResponse(302, http.Header{}, StyleName); !errors.Is(err, ErrMalformed) {
		t.Errorf("missing Location: err = %v, want ErrMalformed", err)
	}
}

func TestBadgeURL(t *testing.T) {
	tests := []struct {
		id      string
		style   Style
		want    string
		wantErr error
	}{
		{"5417470909511807", StyleName, "https://www.kookapp.cn/api/v3/badge/guild?guild_id=5417470909511807&style=0", nil},
		{"1", StyleOnlineTotal, "https://www.kookapp.cn/api/v3/badge/guild?guild_id=1&style=2", nil},
		{"", StyleName, "", ErrInvalidGuildID},
		{"123456789012345678901", StyleName, "", ErrInvalidGuildID},
		{"12a", StyleName, "", ErrInvalidGuildID},
		{"1&style=2", StyleName, "", ErrInvalidGuildID},
		{"1", Style(3), "", ErrInvalidStyle},
		{"1", Style(-1), "", ErrInvalidStyle},
	}
	for _, tc := range tests {
		got, err := BadgeURL(tc.id, tc.style)
		if tc.wantErr != nil {
			if !errors.Is(err, tc.wantErr) {
				t.Errorf("BadgeURL(%q,%d) err = %v, want %v", tc.id, tc.style, err, tc.wantErr)
			}
			continue
		}
		if err != nil || got != tc.want {
			t.Errorf("BadgeURL(%q,%d) = %q, %v; want %q", tc.id, tc.style, got, err, tc.want)
		}
	}
}
