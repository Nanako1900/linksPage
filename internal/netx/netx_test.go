package netx

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
)

func mustTrust(t *testing.T, specs ...string) TrustedSet {
	t.Helper()
	ts, err := ParseTrustedProxies(specs)
	if err != nil {
		t.Fatalf("ParseTrustedProxies(%v): %v", specs, err)
	}
	return ts
}

func TestParseTrustedProxies(t *testing.T) {
	tests := []struct {
		name    string
		specs   []string
		wantErr bool
		wantLen int
	}{
		{"default", []string{"172.31.255.2/32"}, false, 1},
		{"bare ipv4", []string{"10.0.0.1"}, false, 1},
		{"bare ipv6", []string{"::1"}, false, 1},
		{"none", []string{"none"}, false, 0},
		{"none upper", []string{" NONE "}, false, 0},
		{"cloudflare", []string{"cloudflare"}, false, len(cloudflarePrefixes)},
		{"mixed", []string{"172.31.255.1/32", "cloudflare"}, false, len(cloudflarePrefixes) + 1},
		{"4in6", []string{"::ffff:10.0.0.0/104"}, false, 1},
		{"empty", nil, true, 0},
		{"none mixed", []string{"none", "10.0.0.1"}, true, 0},
		{"garbage", []string{"dns:proxy"}, true, 0},
		{"bad cidr", []string{"10.0.0.0/33"}, true, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts, err := ParseTrustedProxies(tt.specs)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if err == nil && ts.Len() != tt.wantLen {
				t.Errorf("len = %d, want %d", ts.Len(), tt.wantLen)
			}
		})
	}
}

func TestTrustedSetContains(t *testing.T) {
	ts := mustTrust(t, "::ffff:10.0.0.0/104", "cloudflare")
	for addr, want := range map[string]bool{
		"10.1.2.3":        true,
		"::ffff:10.1.2.3": true,
		"104.16.0.1":      true,
		"2606:4700::1":    true,
		"8.8.8.8":         false,
		"172.31.255.2":    false,
		"2001:db8::1":     false,
	} {
		if got := ts.Contains(netip.MustParseAddr(addr)); got != want {
			t.Errorf("Contains(%s) = %v, want %v", addr, got, want)
		}
	}
	if len(CloudflarePrefixes()) != 22 {
		t.Errorf("unexpected CF prefix count %d", len(CloudflarePrefixes()))
	}
}

func req(remote string, headers map[string][]string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = remote
	for k, vs := range headers {
		for _, v := range vs {
			r.Header.Add(k, v)
		}
	}
	return r
}

func TestResolveCFHeader(t *testing.T) {
	rv := NewResolver(mustTrust(t, "172.31.255.2/32"), "cf-connecting-ip")
	tests := []struct {
		name        string
		remote      string
		headers     map[string][]string
		wantIP      string
		wantCountry string
		wantTrusted bool
		wantMissing bool
	}{
		{"trusted", "172.31.255.2:4000", map[string][]string{"CF-Connecting-IP": {"203.0.113.9"}, "CF-IPCountry": {"cn"}}, "203.0.113.9", "CN", true, false},
		{"trusted ipv6", "172.31.255.2:4000", map[string][]string{"CF-Connecting-IP": {"2001:db8::5"}}, "2001:db8::5", "", true, false},
		{"untrusted spoof", "198.51.100.7:1234", map[string][]string{"CF-Connecting-IP": {"1.1.1.1"}, "CF-IPCountry": {"US"}}, "198.51.100.7", "", false, false},
		{"trusted missing", "172.31.255.2:4000", nil, "172.31.255.2", "", true, true},
		{"trusted garbage", "172.31.255.2:4000", map[string][]string{"CF-Connecting-IP": {"nope"}}, "172.31.255.2", "", true, true},
		{"trusted duplicate header", "172.31.255.2:4000", map[string][]string{"CF-Connecting-IP": {"1.1.1.1", "2.2.2.2"}}, "172.31.255.2", "", true, true},
		{"bad country", "172.31.255.2:4000", map[string][]string{"CF-Connecting-IP": {"1.1.1.1"}, "CF-IPCountry": {"XX1"}}, "1.1.1.1", "", true, false},
		{"mapped remote", "[::ffff:172.31.255.2]:4000", map[string][]string{"CF-Connecting-IP": {"1.1.1.1"}}, "1.1.1.1", "", true, false},
		{"bad remote", "garbage", map[string][]string{"CF-Connecting-IP": {"1.1.1.1"}}, "invalid IP", "", false, false},
		{"remote without port", "172.31.255.2", map[string][]string{"CF-Connecting-IP": {"1.1.1.1"}}, "1.1.1.1", "", true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := rv.Resolve(req(tt.remote, tt.headers))
			if info.IP.String() != tt.wantIP || info.Country != tt.wantCountry ||
				info.ViaTrustedProxy != tt.wantTrusted || info.MissingHeader != tt.wantMissing {
				t.Errorf("got %+v (ip %s)", info, info.IP)
			}
		})
	}
}

func TestResolveXFF(t *testing.T) {
	rv := NewResolver(mustTrust(t, "172.31.255.1/32", "cloudflare"), "X-Forwarded-For")
	tests := []struct {
		name    string
		remote  string
		xff     []string
		wantIP  string
		missing bool
	}{
		{"cf then proxy", "172.31.255.1:1", []string{"203.0.113.9, 104.16.0.5"}, "203.0.113.9", false},
		{"spoofed left", "172.31.255.1:1", []string{"6.6.6.6, 203.0.113.9, 104.16.0.5"}, "203.0.113.9", false},
		{"multiple headers", "172.31.255.1:1", []string{"6.6.6.6", "203.0.113.9,104.16.0.5"}, "203.0.113.9", false},
		{"bypass cf", "172.31.255.1:1", []string{"6.6.6.6, 198.51.100.1"}, "198.51.100.1", false},
		{"all trusted", "172.31.255.1:1", []string{"104.16.0.9, 104.16.0.5"}, "104.16.0.9", false},
		{"with port", "172.31.255.1:1", []string{"203.0.113.9:5555"}, "203.0.113.9", false},
		{"ipv6 with port", "172.31.255.1:1", []string{"[2001:db8::1]:443"}, "2001:db8::1", false},
		{"garbage hop", "172.31.255.1:1", []string{"203.0.113.9, unknown"}, "172.31.255.1", true},
		{"empty", "172.31.255.1:1", nil, "172.31.255.1", true},
		{"untrusted peer", "8.8.8.8:1", []string{"1.2.3.4"}, "8.8.8.8", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := rv.Resolve(req(tt.remote, map[string][]string{"X-Forwarded-For": tt.xff}))
			if info.IP.String() != tt.wantIP || info.MissingHeader != tt.missing {
				t.Errorf("got ip=%s missing=%v, want %s %v", info.IP, info.MissingHeader, tt.wantIP, tt.missing)
			}
		})
	}
}

func TestMiddleware(t *testing.T) {
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, nil))
	rv := NewResolver(mustTrust(t, "172.31.255.2/32"), "CF-Connecting-IP")
	var got ClientInfo
	var gotHeaders http.Header
	h := Middleware(rv, logger)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		got, _ = FromContext(r.Context())
		gotHeaders = r.Header
	}))

	r := req("172.31.255.2:1", map[string][]string{
		"CF-Connecting-IP": {"203.0.113.9"}, "CF-IPCountry": {"JP"}, "CF-IPCity": {"Tokyo"},
		"CF-IPLatitude": {"35.6"}, "CF-Postal-Code": {"100"}, "X-Other": {"keep"},
	})
	h.ServeHTTP(httptest.NewRecorder(), r)
	if got.IP.String() != "203.0.113.9" || got.Country != "JP" {
		t.Errorf("info = %+v", got)
	}
	for _, name := range []string{"CF-IPCity", "CF-IPLatitude", "CF-Postal-Code", "CF-IPCountry"} {
		if gotHeaders.Get(name) != "" {
			t.Errorf("%s not stripped", name)
		}
	}
	if gotHeaders.Get("X-Other") != "keep" {
		t.Error("unrelated header removed")
	}
	if r.Header.Get("CF-IPCity") != "Tokyo" {
		t.Error("original request was mutated")
	}

	// Missing header warns once per interval.
	for range 3 {
		h.ServeHTTP(httptest.NewRecorder(), req("172.31.255.2:1", nil))
	}
	if n := strings.Count(logBuf.String(), "missing a usable client IP header"); n != 1 {
		t.Errorf("warn count = %d, want 1; log: %s", n, logBuf.String())
	}

	if _, ok := FromContext(httptest.NewRequest(http.MethodGet, "/", nil).Context()); ok {
		t.Error("FromContext on bare context should be false")
	}
	if stripLocationHeaders(nil) == nil {
		t.Error("strip(nil) should return empty header")
	}
}
