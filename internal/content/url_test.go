package content

import (
	"errors"
	"strings"
	"testing"
)

func TestSafeURL(t *testing.T) {
	tests := []struct {
		name, in, want string
		ok             bool
	}{
		{"https", "https://example.com/a?b=1#c", "https://example.com/a?b=1#c", true},
		{"http", "http://example.com", "http://example.com", true},
		{"lowercase scheme and host", "HTTPS://Example.COM/Path", "https://example.com/Path", true},
		{"port", "https://example.com:8443/", "https://example.com:8443/", true},
		{"mailto", "mailto:hi@example.com", "mailto:hi@example.com", true},
		{"mailto upper", "MAILTO:hi@example.com", "mailto:hi@example.com", true},
		{"empty", "", "", false},
		{"javascript", "javascript:alert(1)", "", false},
		{"data", "data:text/html,x", "", false},
		{"ftp", "ftp://example.com", "", false},
		{"relative", "/path", "", false},
		{"protocol relative", "//example.com", "", false},
		{"credentials", "https://user:pw@example.com", "", false},
		{"user only", "https://user@example.com", "", false},
		{"no host", "https:///path", "", false},
		{"opaque https", "https:example.com", "", false},
		{"space", "https://exa mple.com", "", false},
		{"leading space", " https://example.com", "", false},
		{"control", "https://example.com/\x00", "", false},
		{"newline", "https://example.com/\n", "", false},
		{"bidi", "https://example.com/\u202e", "", false},
		{"zero width", "https://exa\u200bmple.com", "", false},
		{"backslash", "https://example.com\\@evil.com", "", false},
		{"bad escape", "https://example.com/%zz", "", false},
		{"mailto without at", "mailto:nobody", "", false},
		{"mailto empty", "mailto:", "", false},
		{"mailto host form", "mailto://a@b.c", "", false},
		{"too long", "https://example.com/" + strings.Repeat("a", MaxURLLength), "", false},
		{"invalid utf8", "https://example.com/\xff", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SafeURL(tt.in)
			if tt.ok {
				if err != nil || got != tt.want {
					t.Fatalf("SafeURL(%q) = %q, %v; want %q", tt.in, got, err, tt.want)
				}
				return
			}
			if !errors.Is(err, ErrUnsafeURL) {
				t.Fatalf("SafeURL(%q) = %q, %v; want ErrUnsafeURL", tt.in, got, err)
			}
		})
	}
}

func TestHTTPSURL(t *testing.T) {
	tests := []struct {
		name  string
		in    string
		hosts []string
		ok    bool
	}{
		{"any host", "https://kook.top/abc", nil, true},
		{"allowed host", "https://qm.qq.com/q/abc", []string{"qm.qq.com", "qun.qq.com"}, true},
		{"host case", "https://QM.qq.com/q/abc", []string{"qm.qq.com"}, true},
		{"other host", "https://evil.example/q", []string{"qm.qq.com"}, false},
		{"suffix trick", "https://qm.qq.com.evil.example/", []string{"qm.qq.com"}, false},
		{"http rejected", "http://qm.qq.com/q/abc", []string{"qm.qq.com"}, false},
		{"mailto rejected", "mailto:a@b.c", nil, false},
		{"unsafe", "javascript:x", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := HTTPSURL(tt.in, tt.hosts...)
			if (err == nil) != tt.ok {
				t.Fatalf("HTTPSURL(%q, %v) err = %v, want ok=%v", tt.in, tt.hosts, err, tt.ok)
			}
			if err != nil && !errors.Is(err, ErrUnsafeURL) {
				t.Fatalf("error does not wrap ErrUnsafeURL: %v", err)
			}
		})
	}
}

func FuzzSafeURL(f *testing.F) {
	for _, s := range []string{"https://a.b", "mailto:a@b", "javascript:x", "HTTP://X", "https://a@b", "//x"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, in string) {
		out, err := SafeURL(in)
		if err != nil {
			return
		}
		if !strings.HasPrefix(out, "https://") && !strings.HasPrefix(out, "http://") && !strings.HasPrefix(out, "mailto:") {
			t.Fatalf("SafeURL(%q) = %q has a disallowed scheme", in, out)
		}
		if len(out) > MaxURLLength || hasUnsafeRune(out) {
			t.Fatalf("SafeURL(%q) = %q is unsafe", in, out)
		}
		again, err := SafeURL(out)
		if err != nil || again != out {
			t.Fatalf("SafeURL not idempotent: %q → %q → %q (%v)", in, out, again, err)
		}
	})
}
