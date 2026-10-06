package provider

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNewHTTPClientProxyValidation(t *testing.T) {
	tests := []struct {
		name    string
		proxy   string
		wantErr bool
	}{
		{"direct", "", false},
		{"http proxy", "http://user:pass@127.0.0.1:7890", false},
		{"socks5", "socks5://127.0.0.1:1080", false},
		{"bad scheme", "ftp://127.0.0.1", true},
		{"no host", "http://", true},
		{"unparseable", "http://[::1", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := NewHTTPClient(ClientOptions{ProxyURL: tt.proxy})
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if err == nil && (c.Timeout != DefaultTimeout || c.Jar != nil) {
				t.Errorf("timeout=%s jar=%v", c.Timeout, c.Jar)
			}
		})
	}
}

func TestNewHTTPClientBehaviour(t *testing.T) {
	var gotUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		http.SetCookie(w, &http.Cookie{Name: "c", Value: "v"})
		http.Redirect(w, r, "/elsewhere", http.StatusFound)
	}))
	defer srv.Close()

	c, err := NewHTTPClient(ClientOptions{UserAgent: UserAgent("1.0.0"), Timeout: 2 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := Get(context.Background(), c, srv.URL+"/badge", "")
	if err != nil {
		t.Fatal(err)
	}
	if resp.Status != http.StatusFound || resp.Header.Get("Location") != "/elsewhere" {
		t.Errorf("redirect must not be followed: %d %v", resp.Status, resp.Header)
	}
	if gotUA != "LinksPage/1.0.0 (+https://github.com/Nanako1900/linksPage)" {
		t.Errorf("ua = %q", gotUA)
	}
	if c.Timeout != 2*time.Second {
		t.Errorf("timeout = %s", c.Timeout)
	}
}

func TestNewHTTPClientUsesProxy(t *testing.T) {
	var proxied string
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxied = r.URL.String()
		_, _ = w.Write([]byte(`{}`))
	}))
	defer proxy.Close()
	c, err := NewHTTPClient(ClientOptions{ProxyURL: proxy.URL})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := Get(context.Background(), c, "http://discord.example/api/x", "application/json")
	if err != nil || resp.Status != http.StatusOK {
		t.Fatalf("resp=%+v err=%v", resp, err)
	}
	if proxied != "http://discord.example/api/x" {
		t.Errorf("proxy saw %q", proxied)
	}
}

func TestGetErrors(t *testing.T) {
	big := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Accept") != "application/json" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		_, _ = w.Write([]byte(strings.Repeat("x", MaxResponseBytes+1)))
	}))
	defer big.Close()
	c, _ := NewHTTPClient(ClientOptions{})
	if _, err := Get(context.Background(), c, big.URL, "application/json"); !errors.Is(err, ErrResponseTooLarge) {
		t.Errorf("err = %v, want too large", err)
	}
	if _, err := Get(context.Background(), c, "://bad", ""); err == nil {
		t.Error("bad url must fail")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Get(ctx, c, big.URL, ""); err == nil {
		t.Error("cancelled request must fail")
	}
}

func TestParseAPIBase(t *testing.T) {
	tests := []struct {
		raw, want string
		wantErr   bool
	}{
		{"", DefaultDiscordAPIBase, false},
		{"http://127.0.0.1:8080/prefix", "http://127.0.0.1:8080/prefix", false},
		{"https://discord.com/", "", true},
		{"ftp://x", "", true},
		{"https://u:p@x", "", true},
		{"https://x?a=1", "", true},
		{"https://x?", "", true},
		{"https://x#f", "", true},
		{"/relative", "", true},
		{"http://[::1", "", true},
	}
	for _, tt := range tests {
		u, err := ParseAPIBase(tt.raw, DefaultDiscordAPIBase)
		if (err != nil) != tt.wantErr {
			t.Errorf("ParseAPIBase(%q) err = %v", tt.raw, err)
			continue
		}
		if err == nil && u.String() != tt.want {
			t.Errorf("ParseAPIBase(%q) = %q", tt.raw, u)
		}
		if err != nil && !errors.Is(err, ErrInvalidAPIBase) {
			t.Errorf("error must wrap ErrInvalidAPIBase: %v", err)
		}
	}
	base, _ := ParseAPIBase("http://h/p", "")
	if got := EndpointURL(base, "/api/x", map[string][]string{"a": {"1"}}); got != "http://h/p/api/x?a=1" {
		t.Errorf("EndpointURL = %q", got)
	}
}
