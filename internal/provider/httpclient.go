package provider

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Default upstream API bases. Overridable only via config
// (providers.discord.api_base / providers.kook.api_base) so tests can point
// providers at httptest servers.
const (
	DefaultDiscordAPIBase = "https://discord.com"
	DefaultKOOKAPIBase    = "https://www.kookapp.cn"
)

// DefaultTimeout is the outbound request timeout (doc 5.1).
const DefaultTimeout = 10 * time.Second

// RepoURL is advertised in the User-Agent.
const RepoURL = "https://github.com/Nanako1900/linksPage"

// MaxResponseBytes bounds provider API response bodies.
const MaxResponseBytes = 1 << 20

// ErrInvalidAPIBase is returned by ParseAPIBase.
var ErrInvalidAPIBase = errors.New("provider: invalid api base")

// ClientOptions configure the outbound provider client.
type ClientOptions struct {
	// ProxyURL is providers.http_proxy (raw, may hold credentials); ""
	// means direct. The process environment (HTTP_PROXY) is ignored.
	ProxyURL string
	// UserAgent defaults to UserAgent(version).
	UserAgent string
	// Timeout defaults to DefaultTimeout.
	Timeout time.Duration
}

// UserAgent returns "LinksPage/<version> (+<repo>)".
func UserAgent(version string) string {
	return "LinksPage/" + version + " (+" + RepoURL + ")"
}

// NewHTTPClient returns the client used for Discord/KOOK API calls and the
// media proxy upstream: it never follows redirects
// (http.ErrUseLastResponse), keeps no cookies, sets the User-Agent on
// every request and uses ProxyURL for all requests.
func NewHTTPClient(opts ClientOptions) (*http.Client, error) {
	proxy, err := proxyFunc(opts.ProxyURL)
	if err != nil {
		return nil, err
	}
	ua := opts.UserAgent
	if ua == "" {
		ua = UserAgent("dev")
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	transport := &http.Transport{
		Proxy:                  proxy,
		DialContext:            (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		ForceAttemptHTTP2:      true,
		MaxIdleConns:           16,
		MaxIdleConnsPerHost:    4,
		IdleConnTimeout:        90 * time.Second,
		TLSHandshakeTimeout:    5 * time.Second,
		ResponseHeaderTimeout:  timeout,
		ExpectContinueTimeout:  time.Second,
		MaxResponseHeaderBytes: 64 << 10,
	}
	return &http.Client{
		Transport: &uaTransport{base: transport, userAgent: ua},
		Timeout:   timeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}, nil
}

// proxyFunc validates the proxy URL; "" disables proxying (and ignores
// HTTP_PROXY from the environment).
func proxyFunc(raw string) (func(*http.Request) (*url.URL, error), error) {
	if raw == "" {
		return nil, nil
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, errors.New("provider: invalid http_proxy url")
	}
	switch u.Scheme {
	case "http", "https", "socks5", "socks5h":
	default:
		return nil, fmt.Errorf("provider: unsupported http_proxy scheme %q", u.Scheme)
	}
	if u.Host == "" {
		return nil, errors.New("provider: http_proxy has no host")
	}
	return http.ProxyURL(u), nil
}

// uaTransport sets the User-Agent on a clone of every request.
type uaTransport struct {
	base      http.RoundTripper
	userAgent string
}

func (t *uaTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.Header.Set("User-Agent", t.userAgent)
	return t.base.RoundTrip(clone)
}

// ParseAPIBase validates an api_base value: absolute http(s), no
// credentials, query, fragment or trailing slash; a path prefix is allowed.
// "" returns def.
func ParseAPIBase(raw, def string) (*url.URL, error) {
	if raw == "" {
		raw = def
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("%w: %q", ErrInvalidAPIBase, raw)
	}
	switch {
	case u.Scheme != "https" && u.Scheme != "http",
		u.Host == "",
		u.User != nil,
		u.RawQuery != "" || u.ForceQuery,
		u.Fragment != "",
		strings.HasSuffix(u.Path, "/"):
		return nil, fmt.Errorf("%w: %q", ErrInvalidAPIBase, raw)
	}
	return u, nil
}

// EndpointURL appends path (unescaped; callers validate its segments
// first) and query to base.
func EndpointURL(base *url.URL, path string, query url.Values) string {
	u := *base
	u.RawPath = ""
	u.Path = base.Path + path
	u.RawQuery = query.Encode()
	return u.String()
}
