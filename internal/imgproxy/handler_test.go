package imgproxy

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"net"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Nanako1900/linksPage/internal/provider"
	"github.com/Nanako1900/linksPage/internal/store/dbq"
)

func pngBytes(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// testUpstream is a TLS image origin with per-path behaviour.
type testUpstream struct {
	srv   *httptest.Server
	hits  sync.Map // path → *atomic.Int32
	delay time.Duration
}

func newTestUpstream(t *testing.T, img []byte) *testUpstream {
	t.Helper()
	u := &testUpstream{}
	u.srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		counter, _ := u.hits.LoadOrStore(r.URL.Path, &atomic.Int32{})
		counter.(*atomic.Int32).Add(1)
		if u.delay > 0 {
			time.Sleep(u.delay)
		}
		w.Header().Set("Set-Cookie", "tracking=1")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		switch r.URL.Path {
		case "/icon.png", testAvatarPath:
			_, _ = w.Write(img)
		case "/missing.png":
			http.NotFound(w, r)
		case "/error.png":
			w.WriteHeader(http.StatusInternalServerError)
		case "/html.png":
			_, _ = w.Write([]byte("<html><script>alert(1)</script></html>"))
		case "/big.png":
			_, _ = w.Write(append(slices.Clone(img), make([]byte, MaxUpstreamBytes)...))
		case "/redirect.png":
			http.Redirect(w, r, "/icon.png", http.StatusFound)
		}
	}))
	t.Cleanup(u.srv.Close)
	return u
}

func (u *testUpstream) count(path string) int32 {
	v, ok := u.hits.Load(path)
	if !ok {
		return 0
	}
	return v.(*atomic.Int32).Load()
}

type handlerEnv struct {
	h     *Handler
	store *fakeStore
	up    *testUpstream
	img   []byte
}

// The handler re-checks rows against the allow-list, so tests register
// canonical Discord CDN URLs and the client dials the TLS test server
// instead (its certificate is valid for example.com).
const (
	testCDN        = "https://cdn.discordapp.com"
	testAvatarPath = "/widget-avatars/1/abc"
)

var testHosts = map[string]map[string][]string{"discord": {"cdn.discordapp.com": {"/"}}}

func testClient(up *testUpstream) *http.Client {
	tr := up.srv.Client().Transport.(*http.Transport).Clone()
	addr := up.srv.Listener.Addr().String()
	tr.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, addr)
	}
	tr.TLSClientConfig.ServerName = "example.com"
	return &http.Client{
		Transport:     tr,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

// register stores a row for upstream path p and returns its public file.
func (e *handlerEnv) register(p, kind, ext string) string {
	raw := testCDN + p
	key := Key(testKey, raw)
	e.store.mu.Lock()
	e.store.rows[key] = dbqRow(key, raw, kind, ext)
	e.store.mu.Unlock()
	return key + "." + ext
}

func newHandlerEnv(t *testing.T) *handlerEnv {
	t.Helper()
	img := pngBytes(t)
	up := newTestUpstream(t, img)
	store := newFakeStore()
	h, err := NewHandler(HandlerOptions{Store: store, Client: testClient(up), Hosts: testHosts})
	if err != nil {
		t.Fatal(err)
	}
	return &handlerEnv{h: h, store: store, up: up, img: img}
}

func (e *handlerEnv) do(method, file string, hdr http.Header) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, PathPrefix+file, nil)
	req.SetPathValue("file", file)
	for k, v := range hdr {
		req.Header[k] = v
	}
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	return rec
}

func TestHandlerServesRegisteredImage(t *testing.T) {
	e := newHandlerEnv(t)
	file := e.register("/icon.png", provider.ImageIcon, "png")
	rec := e.do(http.MethodGet, file, nil)
	if rec.Code != http.StatusOK || !bytes.Equal(rec.Body.Bytes(), e.img) {
		t.Fatalf("status = %d", rec.Code)
	}
	want := map[string]string{
		"Content-Type":            "image/png",
		"Cache-Control":           CacheIconBanner,
		"X-Content-Type-Options":  "nosniff",
		"Content-Security-Policy": "default-src 'none'; sandbox",
	}
	for k, v := range want {
		if got := rec.Header().Get(k); got != v {
			t.Errorf("%s = %q, want %q", k, got, v)
		}
	}
	allowed := map[string]bool{
		"Content-Type": true, "Content-Length": true, "Cache-Control": true, "Etag": true,
		"X-Content-Type-Options": true, "Content-Security-Policy": true,
	}
	for k := range rec.Header() {
		if !allowed[k] {
			t.Errorf("unexpected header %s", k)
		}
	}
	etag := rec.Header().Get("ETag")
	if len(etag) != 34 {
		t.Errorf("etag = %q", etag)
	}
	cached := e.do(http.MethodGet, file, nil)
	if cached.Code != http.StatusOK || e.up.count("/icon.png") != 1 {
		t.Errorf("second request must be cached: %d hits=%d", cached.Code, e.up.count("/icon.png"))
	}
	notModified := e.do(http.MethodGet, file, http.Header{"If-None-Match": {`"other", W/` + etag}})
	if notModified.Code != http.StatusNotModified || notModified.Body.Len() != 0 {
		t.Errorf("If-None-Match = %d", notModified.Code)
	}
	head := e.do(http.MethodHead, file, nil)
	if head.Code != http.StatusOK || head.Body.Len() != 0 || head.Header().Get("Content-Length") != strconv.Itoa(len(e.img)) {
		t.Errorf("HEAD = %d body=%d", head.Code, head.Body.Len())
	}
	wrongExt := strings.TrimSuffix(file, ".png") + ".gif"
	if rec := e.do(http.MethodGet, wrongExt, nil); rec.Code != http.StatusNotFound {
		t.Errorf("extension mismatch = %d", rec.Code)
	}
}

func TestHandlerAvatarAndFallbackPath(t *testing.T) {
	e := newHandlerEnv(t)
	file := e.register(testAvatarPath, provider.ImageAvatar, "png")
	req := httptest.NewRequest(http.MethodGet, PathPrefix+file, nil) // no PathValue: falls back to URL path
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || rec.Header().Get("Cache-Control") != CacheAvatar {
		t.Errorf("avatar = %d %q", rec.Code, rec.Header().Get("Cache-Control"))
	}
}

func TestHandlerErrors(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		status   int
		cached   bool // second request does not reach upstream
		wantHits int32
	}{
		{"upstream 404 negative cache", "/missing.png", http.StatusNotFound, true, 1},
		{"upstream 500 not cached", "/error.png", http.StatusBadGateway, false, 2},
		{"html rejected", "/html.png", http.StatusBadGateway, false, 2},
		{"too large", "/big.png", http.StatusBadGateway, false, 2},
		{"redirect not followed", "/redirect.png", http.StatusBadGateway, false, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newHandlerEnv(t)
			file := e.register(tt.path, provider.ImageIcon, "png")
			for range 2 {
				rec := e.do(http.MethodGet, file, nil)
				if rec.Code != tt.status || rec.Header().Get("X-Content-Type-Options") != "nosniff" {
					t.Fatalf("status = %d, want %d", rec.Code, tt.status)
				}
				if rec.Header().Get("Set-Cookie") != "" {
					t.Error("Set-Cookie must never be forwarded")
				}
			}
			if got := e.up.count(tt.path); got != tt.wantHits {
				t.Errorf("upstream hits = %d, want %d", got, tt.wantHits)
			}
			if e.up.count("/icon.png") != 0 {
				t.Error("redirect target must not be requested")
			}
		})
	}
}

func TestHandlerUnknownAndInvalidKeys(t *testing.T) {
	e := newHandlerEnv(t)
	for _, file := range []string{"short.png", "AAAAAAAAAAAAAAAAAAAAAA.svg", "AAAAAAAAAAAAAAAAAAAAAA", "../etc/passwd"} {
		if rec := e.do(http.MethodGet, file, nil); rec.Code != http.StatusNotFound {
			t.Errorf("%s = %d", file, rec.Code)
		}
	}
	if _, gets := e.store.counts(); gets != 0 {
		t.Errorf("invalid names must not hit the store (gets=%d)", gets)
	}
	unknown := "AAAAAAAAAAAAAAAAAAAAAA.png"
	for range 2 {
		if rec := e.do(http.MethodGet, unknown, nil); rec.Code != http.StatusNotFound {
			t.Errorf("unknown = %d", rec.Code)
		}
	}
	if _, gets := e.store.counts(); gets != 1 {
		t.Errorf("unknown key lookups = %d, want 1", gets)
	}
	later := time.Now().Add(2 * unknownKeyTTL)
	e.h.cache.now = func() time.Time { return later }
	e.do(http.MethodGet, unknown, nil)
	if _, gets := e.store.counts(); gets != 2 {
		t.Errorf("expired negative entry must be looked up again (gets=%d)", gets)
	}
	if rec := e.do(http.MethodPost, unknown, nil); rec.Code != http.StatusMethodNotAllowed || rec.Header().Get("Allow") != "GET, HEAD" {
		t.Errorf("POST = %d", rec.Code)
	}
}

func TestHandlerStoreFailures(t *testing.T) {
	e := newHandlerEnv(t)
	e.store.getErr = errors.New("db down")
	if rec := e.do(http.MethodGet, "AAAAAAAAAAAAAAAAAAAAAA.png", nil); rec.Code != http.StatusBadGateway {
		t.Errorf("db error = %d", rec.Code)
	}
	e.store.getErr = nil
}

// Rows that the Registrar would never have written are not fetched.
func TestHandlerRejectsUnregistrableRows(t *testing.T) {
	tests := []struct {
		name string
		row  func(key string) dbq.MediaProxy
	}{
		{"plain http", func(k string) dbq.MediaProxy { return dbqRow(k, "http://cdn.discordapp.com/icon.png", "icon", "png") }},
		{"host not allowed", func(k string) dbq.MediaProxy { return dbqRow(k, "https://evil.example/icon.png", "icon", "png") }},
		{"internal address", func(k string) dbq.MediaProxy { return dbqRow(k, "https://169.254.169.254/latest.png", "icon", "png") }},
		{"not canonical", func(k string) dbq.MediaProxy { return dbqRow(k, "https://CDN.discordapp.com/icon.png", "icon", "png") }},
		{"extension mismatch", func(k string) dbq.MediaProxy { return dbqRow(k, testCDN+"/icon.png", "icon", "webp") }},
		{"unknown provider", func(k string) dbq.MediaProxy {
			r := dbqRow(k, testCDN+"/icon.png", "icon", "png")
			r.Provider = "kook"
			return r
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newHandlerEnv(t)
			key := "BBBBBBBBBBBBBBBBBBBBBB"
			row := tt.row(key)
			e.store.rows[key] = row
			for range 2 {
				if rec := e.do(http.MethodGet, key+"."+row.Ext, nil); rec.Code != http.StatusNotFound {
					t.Fatalf("status = %d, want 404", rec.Code)
				}
			}
			if hits := e.up.count("/icon.png") + e.up.count("/latest.png"); hits != 0 {
				t.Errorf("upstream contacted %d times", hits)
			}
		})
	}
}

func TestHandlerSingleflight(t *testing.T) {
	e := newHandlerEnv(t)
	e.up.delay = 100 * time.Millisecond
	file := e.register("/icon.png", provider.ImageIcon, "png")
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			if rec := e.do(http.MethodGet, file, nil); rec.Code != http.StatusOK {
				t.Errorf("status = %d", rec.Code)
			}
		})
	}
	wg.Wait()
	if hits := e.up.count("/icon.png"); hits != 1 {
		t.Errorf("upstream hits = %d, want 1", hits)
	}
}

func TestHandlerClientGone(t *testing.T) {
	e := newHandlerEnv(t)
	e.up.delay = 200 * time.Millisecond
	file := e.register("/icon.png", provider.ImageIcon, "png")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	req := httptest.NewRequest(http.MethodGet, PathPrefix+file, nil).WithContext(ctx)
	req.SetPathValue("file", file)
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadGateway {
		t.Errorf("status = %d", rec.Code)
	}
	time.Sleep(300 * time.Millisecond)
	if again := e.do(http.MethodGet, file, nil); again.Code != http.StatusOK || e.up.count("/icon.png") != 1 {
		t.Errorf("detached fetch must complete and be cached: %d hits=%d", again.Code, e.up.count("/icon.png"))
	}
}

func TestHandlerBusy(t *testing.T) {
	e := newHandlerEnv(t)
	file := e.register("/icon.png", provider.ImageIcon, "png")
	key := strings.TrimSuffix(file, ".png")
	if !e.h.sem.TryAcquire(UpstreamConcurrency) {
		t.Fatal("acquire")
	}
	defer e.h.sem.Release(UpstreamConcurrency)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if _, err := e.h.fetch(ctx, key); !errors.Is(err, errBusy) {
		t.Errorf("err = %v, want busy", err)
	}
	rec := httptest.NewRecorder()
	writeFailure(rec, errBusy)
	if rec.Code != http.StatusServiceUnavailable || rec.Header().Get("Retry-After") != "1" {
		t.Errorf("busy = %d", rec.Code)
	}
}

func TestNewHandlerValidation(t *testing.T) {
	if _, err := NewHandler(HandlerOptions{}); err == nil {
		t.Error("missing deps must fail")
	}
	if _, err := NewHandler(HandlerOptions{Store: newFakeStore(), Client: http.DefaultClient}); err == nil {
		t.Error("missing allow-list must fail")
	}
}

func TestImageCache(t *testing.T) {
	now := time.Now()
	c := newImageCache(10, func() time.Time { return now })
	c.add("a", cachedImage{body: make([]byte, 6)})
	c.add("b", cachedImage{body: make([]byte, 6)})
	if _, ok := c.get("a"); ok {
		t.Error("a must be evicted by the byte budget")
	}
	if _, ok := c.get("b"); !ok || c.bytes.Load() != 6 {
		t.Errorf("b missing or bytes = %d", c.bytes.Load())
	}
	c.add("huge", cachedImage{body: make([]byte, 11)})
	if _, ok := c.get("huge"); ok {
		t.Error("oversized entries are not cached")
	}
	c.add("b", cachedImage{body: make([]byte, 2)})
	if c.bytes.Load() != 2 {
		t.Errorf("replacing must adjust bytes: %d", c.bytes.Load())
	}
	c.add("neg", cachedImage{notFound: true, expires: now.Add(time.Second)})
	if _, ok := c.get("neg"); !ok {
		t.Error("neg must be cached")
	}
	now = now.Add(2 * time.Second)
	if _, ok := c.get("neg"); ok {
		t.Error("neg must expire")
	}
}

func TestEtagMatches(t *testing.T) {
	tests := []struct {
		header string
		want   bool
	}{
		{"", false}, {`"a"`, true}, {`W/"a"`, true}, {`"b", "a"`, true}, {"*", true}, {`"b"`, false},
	}
	for _, tt := range tests {
		if got := etagMatches(tt.header, `"a"`); got != tt.want {
			t.Errorf("etagMatches(%q) = %v", tt.header, got)
		}
	}
	if cacheControlFor("banner") != CacheIconBanner || cacheControlFor("avatar") != CacheAvatar {
		t.Error("cache control mapping")
	}
}
