package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/Nanako1900/linksPage/internal/netx"
	"github.com/Nanako1900/linksPage/internal/site"
)

var fixturePath = filepath.Join("..", "..", "web", "src", "test", "fixtures", "public-page.json")

// fixtureSnapshot returns the default snapshot with the contract fixture
// as its public page.
func fixtureSnapshot(t *testing.T) *site.Snapshot {
	t.Helper()
	b, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatal(err)
	}
	var page site.PublicPage
	if err := json.Unmarshal(b, &page); err != nil {
		t.Fatal(err)
	}
	snap := *site.DefaultSnapshot()
	snap.Public = &page
	return &snap
}

func readyFixtureServer(t *testing.T, opts ...func(*Deps)) *testServer {
	t.Helper()
	ts := newTestServer(t, opts...)
	ts.holder.Set(fixtureSnapshot(t))
	ts.ready.SetReady()
	return ts
}

func TestBootstrapIsPublicPage(t *testing.T) {
	ts := readyFixtureServer(t)
	w := ts.do(http.MethodGet, "/api/v1/public/bootstrap", nil)
	var body struct {
		Data site.PublicPage `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || w.Code != 200 {
		t.Fatalf("bootstrap = %d %v", w.Code, err)
	}
	if body.Data.Revision != "r-5f2c9a1e" || len(body.Data.Communities) != 8 {
		t.Errorf("bootstrap data = %+v", body.Data)
	}
	if err := body.Data.Validate(); err != nil {
		t.Errorf("bootstrap page must validate: %v", err)
	}
}

func TestLive(t *testing.T) {
	ts := newTestServer(t)
	if w := ts.do(http.MethodGet, "/api/v1/public/live", nil); w.Code != 503 {
		t.Fatalf("live before ready = %d", w.Code)
	}
	ts.holder.Set(fixtureSnapshot(t))
	ts.ready.SetReady()
	w := ts.do(http.MethodGet, "/api/v1/public/live", nil)
	tag := w.Header().Get("ETag")
	if w.Code != 200 || w.Header().Get("Cache-Control") != CacheLive || len(tag) != liveETagHexLen+2 {
		t.Fatalf("live = %d cc=%q etag=%q", w.Code, w.Header().Get("Cache-Control"), tag)
	}
	var body struct {
		Data site.LiveDTO `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || body.Data.Revision != "r-5f2c9a1e" || len(body.Data.Communities) != 8 {
		t.Fatalf("live body = %v %+v", err, body.Data)
	}
	for _, inm := range []string{tag, "W/" + tag, `"other", ` + tag, "*"} {
		nm := ts.do(http.MethodGet, "/api/v1/public/live", map[string]string{"If-None-Match": inm})
		if nm.Code != 304 || nm.Body.Len() != 0 || nm.Header().Get("ETag") != tag || nm.Header().Get("Cache-Control") != CacheLive {
			t.Errorf("If-None-Match %q = %d len %d", inm, nm.Code, nm.Body.Len())
		}
	}
	if w := ts.do(http.MethodGet, "/api/v1/public/live", map[string]string{"If-None-Match": `"nope"`}); w.Code != 200 {
		t.Errorf("mismatched If-None-Match = %d", w.Code)
	}
}

func TestLiveETagIgnoresGeneratedAt(t *testing.T) {
	page := fixtureSnapshot(t).Public
	dto := page.Live()
	a, err := LiveETag(dto)
	if err != nil {
		t.Fatal(err)
	}
	dto.GeneratedAt = dto.GeneratedAt.Add(42e9)
	if b, _ := LiveETag(dto); a != b {
		t.Error("generatedAt must not affect the ETag")
	}
	dto.Revision = "r-other"
	if c, _ := LiveETag(dto); a == c {
		t.Error("revision must affect the ETag")
	}
	cache := &etagCache{}
	first, _ := cache.get(page, page.Live())
	if again, _ := cache.get(page, site.LiveDTO{}); again != first {
		t.Error("cache must reuse the tag of the same page")
	}
}

func TestETagMatches(t *testing.T) {
	tests := []struct {
		header, tag string
		want        bool
	}{
		{"", `"a"`, false},
		{`"a"`, `"a"`, true},
		{`W/"a"`, `"a"`, true},
		{`"b" , "a"`, `"a"`, true},
		{`"b"`, `"a"`, false},
		{"*", `"a"`, true},
	}
	for _, tt := range tests {
		if got := etagMatches(tt.header, tt.tag); got != tt.want {
			t.Errorf("etagMatches(%q, %q) = %v", tt.header, tt.tag, got)
		}
	}
}

const testProxySecret = "0123456789abcdef0123456789abcdef"

func TestRender(t *testing.T) {
	tests := []struct {
		name, secret, header, target string
		status, renderStatus         int
		cache                        string
	}{
		{"open home", "", "", "/api/v1/public/render?path=/&lang=en", 200, 200, CacheRender},
		{"open community", "", "", "/api/v1/public/render?path=/c/discord&lang=zh-CN", 200, 200, CacheRender},
		{"unknown slug", "", "", "/api/v1/public/render?path=/c/nope&lang=en", 200, 404, CacheRender},
		{"no lang", "", "", "/api/v1/public/render?path=/", 200, 200, "no-cache"},
		{"auth ok", testProxySecret, testProxySecret, "/api/v1/public/render?path=/&lang=en", 200, 200, CacheRender},
		{"auth missing", testProxySecret, "", "/api/v1/public/render?path=/&lang=en", 403, 0, "no-store"},
		{"auth wrong", testProxySecret, testProxySecret + "x", "/api/v1/public/render?path=/&lang=en", 403, 0, "no-store"},
		{"path required", "", "", "/api/v1/public/render", 422, 0, "no-store"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts := readyFixtureServer(t, func(d *Deps) { d.ProxyAuth = tt.secret })
			w := ts.do(http.MethodGet, tt.target, map[string]string{ProxyAuthHeader: tt.header})
			if w.Code != tt.status || w.Header().Get("Cache-Control") != tt.cache {
				t.Fatalf("status %d cache %q body %s", w.Code, w.Header().Get("Cache-Control"), w.Body.String())
			}
			if tt.status == 403 && decodeProblem(t, w).Code != CodeForbidden {
				t.Error("403 must be problem+json code=forbidden")
			}
			if tt.status != 200 {
				return
			}
			if vary := strings.Join(w.Header().Values("Vary"), ","); !strings.Contains(vary, "Accept-Language") {
				t.Errorf("Vary = %q, want Accept-Language (an unknown lang falls back to it)", vary)
			}
			var body struct {
				Data site.RenderDTO `json:"data"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body.Data.Status != tt.renderStatus || (tt.renderStatus == 404) != (body.Data.Data == nil) || body.Data.CSP == "" {
				t.Errorf("render dto = status %d data nil %v", body.Data.Status, body.Data.Data == nil)
			}
		})
	}
}

type failingRender struct{}

func (failingRender) RenderDTO(string, string, string) (site.RenderDTO, error) {
	return site.RenderDTO{}, errors.New("template exploded")
}

func TestRenderFailure(t *testing.T) {
	ts := readyFixtureServer(t)
	api := publicAPI{snapshots: ts.holder, ready: ts.ready, render: failingRender{}}
	if _, err := api.renderPage(t.Context(), &RenderInput{Path: "/"}); err == nil {
		t.Error("render error must be returned")
	}
	notReady := publicAPI{snapshots: ts.holder, ready: NewReadiness(nil), render: failingRender{}}
	if _, err := notReady.renderPage(t.Context(), &RenderInput{Path: "/"}); err == nil {
		t.Error("render before ready must fail")
	}
}

// marker returns a handler writing name, to check route mounting.
func marker(name string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(name)) })
}

func withHandlers(d *Deps) {
	d.Favicon, d.Robots, d.Manifest = marker("favicon"), marker("robots"), marker("manifest")
	d.GoLink, d.Uploads, d.ImgProxy, d.QR = marker("go"), marker("uploads"), marker("imgproxy"), marker("qr")
}

func TestPublicHandlersMounted(t *testing.T) {
	ts := newTestServer(t, withHandlers)
	for target, want := range map[string]string{
		"/favicon.ico": "favicon", "/robots.txt": "robots", "/site.webmanifest": "manifest",
		"/go/discord": "go", "/media/u/abc.webp": "uploads", "/media/p/abc.png": "imgproxy",
		"/media/q/0190-x": "qr",
	} {
		if w := ts.do(http.MethodGet, target, nil); w.Code != 200 || w.Body.String() != want {
			t.Errorf("%s = %d %q", target, w.Code, w.Body.String())
		}
	}
	bare := newTestServer(t)
	if w := bare.do(http.MethodGet, "/robots.txt", nil); w.Code != 404 {
		t.Errorf("unmounted handler = %d", w.Code)
	}
	ts.do(http.MethodGet, "/go/secret-slug?utm=leak", map[string]string{"CF-Connecting-IP": "198.51.100.9"})
	if logs := ts.logs.String(); strings.Contains(logs, "198.51.100.9") || strings.Contains(logs, "leak") ||
		!strings.Contains(logs, `"route":"/go/{slug}"`) {
		t.Errorf("/go must use the minimal public access log:\n%s", logs)
	}
}

// hammer sends n requests from ip and returns the status counts.
func hammer(ts *testServer, target, ip string, n int, hdr map[string]string) map[int]int {
	counts := map[int]int{}
	for range n {
		h := map[string]string{"CF-Connecting-IP": ip}
		for k, v := range hdr {
			h[k] = v
		}
		counts[ts.do(http.MethodGet, target, h).Code]++
	}
	return counts
}

func TestRateLimits(t *testing.T) {
	ts := readyFixtureServer(t, withHandlers)
	if c := hammer(ts, "/api/v1/public/live", "203.0.113.1", PublicAPILimit+5, nil); c[200] != PublicAPILimit || c[429] != 5 {
		t.Errorf("public API counts = %v", c)
	}
	w := ts.do(http.MethodGet, "/api/v1/public/bootstrap", map[string]string{"CF-Connecting-IP": "203.0.113.1"})
	if w.Code != 429 || w.Header().Get("Retry-After") != "60" || decodeProblem(t, w).Code != CodeRateLimited ||
		w.Header().Get("X-RateLimit-Remaining") != "" {
		t.Errorf("429 = %d headers %v", w.Code, w.Header())
	}
	if c := hammer(ts, "/api/v1/public/live", "203.0.113.2", 1, nil); c[200] != 1 {
		t.Errorf("other IPs keep their own bucket: %v", c)
	}
	if c := hammer(ts, "/media/p/abc.png", "203.0.113.3", MediaProxyLimit+2, nil); c[200] != MediaProxyLimit || c[429] != 2 {
		t.Errorf("media proxy counts = %v", c)
	}
	if c := hammer(ts, "/go/discord", "203.0.113.4", 3*PublicAPILimit, nil); c[200] != 3*PublicAPILimit {
		t.Errorf("/go must never be limited: %v", c)
	}
	if c := hammer(ts, "/", "203.0.113.5", PublicAPILimit+1, nil); c[429] != 0 {
		t.Errorf("HTML is not limited: %v", c)
	}
}

func TestRateLimitEdgeBucket(t *testing.T) {
	ts := readyFixtureServer(t, func(d *Deps) { d.ProxyAuth = testProxySecret })
	auth := map[string]string{ProxyAuthHeader: testProxySecret}
	if c := hammer(ts, "/api/v1/public/render?path=/&lang=en", "203.0.113.9", PublicAPILimit+3, auth); c[200] != PublicAPILimit+3 {
		t.Errorf("authenticated Worker is not held to the per-IP limit: %v", c)
	}
	if c := hammer(ts, "/api/v1/public/render?path=/&lang=en", "203.0.113.10", PublicAPILimit+1, nil); c[429] != 1 {
		t.Errorf("unauthenticated render requests are limited: %v", c)
	}
	if edgeRequest(nil)(httptest.NewRequest(http.MethodGet, RenderPath, nil)) {
		t.Error("nothing is an edge request without a secret")
	}
}

// The edge bucket is shared by all edge addresses and bounded.
func TestRateLimitEdgeBucketIsBounded(t *testing.T) {
	const limit = 5
	secret := []byte(testProxySecret)
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	h := rateLimits(edgeRequest(secret), limit)(ok)
	counts := map[int]int{}
	for i := range limit + 3 {
		r := httptest.NewRequest(http.MethodGet, RenderPath+"?path=/&lang=x"+strconv.Itoa(i), nil)
		r.RemoteAddr = "198.51.100." + strconv.Itoa(i+1) + ":443"
		r.Header.Set(ProxyAuthHeader, testProxySecret)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		counts[w.Code]++
		if w.Code == http.StatusTooManyRequests && w.Header().Get("Retry-After") != "60" {
			t.Errorf("429 without Retry-After: %v", w.Header())
		}
	}
	if counts[200] != limit || counts[429] != 3 {
		t.Errorf("edge bucket counts = %v, want %d ok and 3 limited", counts, limit)
	}
}

func TestClientKey(t *testing.T) {
	tests := []struct {
		name, remote, ip, want string
	}{
		{"ipv4 from context", "10.0.0.1:1", "203.0.113.7", "203.0.113.7"},
		{"ipv6 bucketed by /64", "10.0.0.1:1", "2001:db8:1:2:3:4:5:6", "2001:db8:1:2::"},
		{"mapped ipv4", "10.0.0.1:1", "::ffff:203.0.113.8", "203.0.113.8"},
		{"remote addr fallback", "192.0.2.4:9", "", "192.0.2.4"},
		{"bad remote addr", "garbage", "", "garbage"},
	}
	for _, tt := range tests {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.RemoteAddr = tt.remote
		if tt.ip != "" {
			rv := netx.NewResolver(netx.TrustedSet{}, "")
			info := rv.Resolve(r)
			info.IP = netip.MustParseAddr(tt.ip)
			r = r.WithContext(netx.WithClientInfo(r.Context(), info))
		}
		if got, err := clientKey(r); err != nil || got != tt.want {
			t.Errorf("%s: clientKey = %q %v, want %q", tt.name, got, err, tt.want)
		}
	}
}

func TestOpenAPIHasM1Operations(t *testing.T) {
	spec, err := OpenAPISpec("test")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"getPublicLive"`, `"getPublicRender"`, `"/api/v1/public/live"`, `"/api/v1/public/render"`, `"304"`} {
		if !bytes.Contains(spec, []byte(want)) {
			t.Errorf("spec missing %s", want)
		}
	}
}
