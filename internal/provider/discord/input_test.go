package discord

import (
	"errors"
	"reflect"
	"testing"
)

func TestValidGuildID(t *testing.T) {
	tests := map[string]bool{
		"1114391825336250432":   true,
		"12345678901234567":     true,
		"12345678901234567890":  true,
		"1234567890123456":      false,
		"123456789012345678901": false,
		"11143918253362504a2":   false,
		"":                      false,
		" 1114391825336250432":  false,
	}
	for in, want := range tests {
		if got := ValidGuildID(in); got != want {
			t.Errorf("ValidGuildID(%q) = %t, want %t", in, got, want)
		}
	}
}

func TestExtractInviteCode(t *testing.T) {
	tests := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{in: "KwdRuAkT", want: "KwdRuAkT"},
		{in: "  my-server  ", want: "my-server"},
		{in: "discord.gg/KwdRuAkT", want: "KwdRuAkT"},
		{in: "https://discord.gg/KwdRuAkT", want: "KwdRuAkT"},
		{in: "https://discord.gg/KwdRuAkT/", want: "KwdRuAkT"},
		{in: "http://www.discord.gg/abc?event=1", want: "abc"},
		{in: "https://discord.com/invite/KwdRuAkT", want: "KwdRuAkT"},
		{in: "https://DISCORD.com/invite/KwdRuAkT", want: "KwdRuAkT"},
		{in: "https://discordapp.com/invite/abc-def", want: "abc-def"},
		{in: "", wantErr: true},
		{in: "a", wantErr: true},
		{in: "https://evil.example/KwdRuAkT", wantErr: true},
		{in: "https://discord.gg.evil.example/KwdRuAkT", wantErr: true},
		{in: "https://discord.com/channels/KwdRuAkT", wantErr: true},
		{in: "https://discord.gg:8443/KwdRuAkT", wantErr: true},
		{in: "https://user@discord.gg/KwdRuAkT", wantErr: true},
		{in: "ftp://discord.gg/KwdRuAkT", wantErr: true},
		{in: "https://discord.gg/abc/def", wantErr: true},
		{in: "https://discord.gg/%2e%2e", wantErr: true},
		{in: "https://discord.gg/ab_cd", wantErr: true},
		{in: "https://[::1", wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			got, err := ExtractInviteCode(tc.in)
			if tc.wantErr {
				if !errors.Is(err, ErrInvalidInput) {
					t.Fatalf("got %q, %v; want ErrInvalidInput", got, err)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("got %q, %v; want %q", got, err, tc.want)
			}
		})
	}
}

func TestEndpointURLs(t *testing.T) {
	w, err := WidgetURL("1114391825336250432")
	if err != nil || w != "https://discord.com/api/guilds/1114391825336250432/widget.json" {
		t.Errorf("WidgetURL = %q, %v", w, err)
	}
	if _, err := WidgetURL("../1"); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("WidgetURL invalid: %v", err)
	}
	i, err := InviteURL("KwdRuAkT")
	if err != nil || i != "https://discord.com/api/v10/invites/KwdRuAkT?with_counts=true" {
		t.Errorf("InviteURL = %q, %v", i, err)
	}
	if _, err := InviteURL("a/b"); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("InviteURL invalid: %v", err)
	}
}

func TestImageURLs(t *testing.T) {
	const gid = "1114391825336250432"
	const hash = "42e0892e057eccdf8f50765cef3fbfd8"
	tests := []struct {
		name    string
		fn      func(string, string, int) (string, error)
		gid     string
		hash    string
		size    int
		want    string
		wantErr bool
	}{
		{"icon png", IconURL, gid, hash, 256, "https://cdn.discordapp.com/icons/" + gid + "/" + hash + ".png?size=256", false},
		{"icon animated", IconURL, gid, "a_" + hash, 128, "https://cdn.discordapp.com/icons/" + gid + "/a_" + hash + ".gif?size=128", false},
		{"banner", BannerURL, gid, hash, 1024, "https://cdn.discordapp.com/banners/" + gid + "/" + hash + ".png?size=1024", false},
		{"splash", SplashURL, gid, "a77ff53d4950ca8eec961cde606ba91f", 512, "https://cdn.discordapp.com/splashes/" + gid + "/a77ff53d4950ca8eec961cde606ba91f.png?size=512", false},
		{"splash animated rejected", SplashURL, gid, "a_" + hash, 512, "", true},
		{"bad hash", IconURL, gid, "../../x", 256, "", true},
		{"uppercase hash", IconURL, gid, "42E0892E057ECCDF8F50765CEF3FBFD8", 256, "", true},
		{"bad guild", IconURL, "123", hash, 256, "", true},
		{"size not power of two", IconURL, gid, hash, 300, "", true},
		{"size too small", IconURL, gid, hash, 8, "", true},
		{"size too large", IconURL, gid, hash, 8192, "", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.fn(tc.gid, tc.hash, tc.size)
			if tc.wantErr {
				if !errors.Is(err, ErrInvalidImage) {
					t.Fatalf("got %q, %v; want ErrInvalidImage", got, err)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("got %q, %v; want %q", got, err, tc.want)
			}
		})
	}
}

func TestImageHostsReturnsFreshCopy(t *testing.T) {
	a := ImageHosts()
	a[CDNHost][0] = "/mutated/"
	a["evil.example"] = []string{"/"}
	b := ImageHosts()
	want := map[string][]string{CDNHost: {"/icons/", "/banners/", "/splashes/", "/widget-avatars/"}}
	if !reflect.DeepEqual(b, want) {
		t.Errorf("ImageHosts = %v, want %v", b, want)
	}
}
