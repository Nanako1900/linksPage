package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humatest"

	"github.com/Nanako1900/linksPage/internal/netx"
	"github.com/Nanako1900/linksPage/internal/site"
	"github.com/Nanako1900/linksPage/internal/webui"
)

const testManifest = `{
  "index.html": {"file": "assets/index-A.js", "isEntry": true, "css": ["assets/index-B.css"]},
  "admin/index.html": {"file": "assets/admin-C.js", "isEntry": true}
}`

type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.String()
}

type testServer struct {
	h      http.Handler
	holder *site.Holder
	ready  *Readiness
	logs   *syncBuffer
	ping   error
}

func newTestServer(t *testing.T, opts ...func(*Deps)) *testServer {
	t.Helper()
	ts := &testServer{logs: &syncBuffer{}}
	logger := slog.New(slog.NewJSONHandler(ts.logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	dist := fstest.MapFS{
		".vite/manifest.json": {Data: []byte(testManifest)},
		"assets/index-A.js":   {Data: []byte("console.info(1)")},
		"assets/index-B.css":  {Data: []byte("body{}")},
		"assets/admin-C.js":   {Data: []byte("x")},
	}
	assets, err := webui.LoadAssets(dist)
	if err != nil {
		t.Fatal(err)
	}
	holder := site.NewHolder(site.DefaultSnapshot())
	ts.holder = holder
	web, err := webui.NewRenderer(webui.Options{
		Assets: assets, BaseURL: "https://links.example.com", AppVersion: "test",
		Logger: logger, Snapshot: holder.Current,
	})
	if err != nil {
		t.Fatal(err)
	}
	trusted, err := netx.ParseTrustedProxies([]string{"172.31.255.2/32"})
	if err != nil {
		t.Fatal(err)
	}
	ts.ready = NewReadiness(func(context.Context) error { return ts.ping })
	deps := Deps{
		BaseURL: "https://links.example.com", Version: "test", Logger: logger, Ready: ts.ready,
		Snapshots: holder, Web: web, Resolver: netx.NewResolver(trusted, "CF-Connecting-IP"),
	}
	for _, o := range opts {
		o(&deps)
	}
	h, err := NewHandler(deps)
	if err != nil {
		t.Fatal(err)
	}
	ts.h = h
	return ts
}

func (ts *testServer) do(method, target string, hdr map[string]string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, target, nil)
	r.RemoteAddr = "172.31.255.2:5000"
	for k, v := range hdr {
		r.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	ts.h.ServeHTTP(w, r)
	return w
}

func decodeProblem(t *testing.T, w *httptest.ResponseRecorder) Problem {
	t.Helper()
	if ct := w.Header().Get("Content-Type"); ct != ProblemContentType {
		t.Fatalf("content-type = %q, body %s", ct, w.Body.String())
	}
	var p Problem
	if err := json.Unmarshal(w.Body.Bytes(), &p); err != nil {
		t.Fatalf("problem json: %v", err)
	}
	if p.RequestID == "" || p.RequestID != w.Header().Get(RequestIDHeader) {
		t.Errorf("requestId %q vs header %q", p.RequestID, w.Header().Get(RequestIDHeader))
	}
	return p
}

func TestHealthAndReadiness(t *testing.T) {
	ts := newTestServer(t)
	if w := ts.do(http.MethodGet, "/healthz", nil); w.Code != 200 || strings.TrimSpace(w.Body.String()) != "ok" {
		t.Fatalf("healthz = %d %q", w.Code, w.Body.String())
	}
	if w := ts.do(http.MethodGet, "/readyz", nil); w.Code != 503 || !strings.Contains(w.Body.String(), "starting") {
		t.Fatalf("readyz before ready = %d", w.Code)
	}
	ts.ready.SetReady()
	if w := ts.do(http.MethodGet, "/readyz", nil); w.Code != 200 {
		t.Fatalf("readyz ready = %d", w.Code)
	}
	ts.ping = errors.New("conn refused to 10.0.0.1")
	if w := ts.do(http.MethodGet, "/readyz", nil); w.Code != 200 {
		t.Fatalf("readyz must reuse the cached ping result: %d", w.Code)
	}
	ts.ready.now = func() time.Time { return time.Now().Add(time.Minute) }
	if w := ts.do(http.MethodGet, "/readyz", nil); w.Code != 503 || strings.Contains(w.Body.String(), "10.0.0.1") {
		t.Fatalf("readyz db down = %d %q", w.Code, w.Body.String())
	}
	ts.ping = nil
	ts.ready.StartDraining()
	if w := ts.do(http.MethodGet, "/readyz", nil); w.Code != 503 || !strings.Contains(w.Body.String(), "draining") {
		t.Fatalf("readyz draining = %d", w.Code)
	}
	if NewReadiness(nil).Check(context.Background()) == nil {
		t.Error("not-ready readiness without ping should fail")
	}
	r := NewReadiness(nil)
	r.SetReady()
	if r.Check(context.Background()) != nil {
		t.Error("ready readiness without ping should pass")
	}
}

func TestReadinessPingIsCachedAndSerialized(t *testing.T) {
	var calls atomic.Int32
	r := NewReadiness(func(context.Context) error {
		calls.Add(1)
		time.Sleep(10 * time.Millisecond)
		return nil
	})
	r.SetReady()
	var wg sync.WaitGroup
	for range 50 {
		wg.Go(func() {
			if err := r.Check(context.Background()); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if n := calls.Load(); n != 1 {
		t.Errorf("ping called %d times, want 1", n)
	}
}

func TestBootstrap(t *testing.T) {
	ts := newTestServer(t)
	w := ts.do(http.MethodGet, "/api/v1/public/bootstrap", nil)
	if w.Code != 503 || decodeProblem(t, w).Code != CodeNotReady {
		t.Fatalf("bootstrap before ready = %d %s", w.Code, w.Body.String())
	}
	ts.ready.SetReady()
	w = ts.do(http.MethodGet, "/api/v1/public/bootstrap", nil)
	if w.Code != 200 {
		t.Fatalf("bootstrap = %d %s", w.Code, w.Body.String())
	}
	var body struct {
		Data site.PublicPage `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Data.Page.Slug != "default" || body.Data.Site.DefaultLocale != "zh-CN" {
		t.Errorf("bootstrap body = %+v", body.Data)
	}
	if strings.Contains(w.Body.String(), "$schema") {
		t.Error("$schema links must be disabled")
	}
	h := w.Header()
	if h.Get("Cache-Control") != "no-cache" || h.Get("Content-Security-Policy") != "default-src 'none'; frame-ancestors 'none'" ||
		h.Get("X-Content-Type-Options") != "nosniff" || h.Get("Cross-Origin-Opener-Policy") != "same-origin" {
		t.Errorf("headers = %v", h)
	}
}

func TestAPINeverFallsBackToHTML(t *testing.T) {
	ts := newTestServer(t)
	for _, target := range []string{"/api", "/api/", "/api/v1/nope", "/api/v1/public/bootstrap/extra"} {
		w := ts.do(http.MethodGet, target, nil)
		if w.Code != 404 {
			t.Errorf("%s: status %d", target, w.Code)
			continue
		}
		if p := decodeProblem(t, w); p.Code != CodeNotFound || p.Status != 404 {
			t.Errorf("%s: problem %+v", target, p)
		}
	}
	w := ts.do(http.MethodDelete, "/api/v1/public/bootstrap", nil)
	if w.Code != 405 || decodeProblem(t, w).Code != CodeMethodNotAllowed {
		t.Errorf("405 problem = %d", w.Code)
	}
	if w := ts.do(http.MethodPost, "/privacy", nil); w.Code != 405 || strings.Contains(w.Body.String(), "<html") {
		t.Errorf("HTML 405 = %d", w.Code)
	}
	if w := ts.do(http.MethodHead, "/api/nope", nil); w.Code != 404 || w.Body.Len() != 0 {
		t.Errorf("HEAD problem = %d len %d", w.Code, w.Body.Len())
	}
}

func TestHTMLRoutes(t *testing.T) {
	ts := newTestServer(t)
	ts.holder.Set(fixtureSnapshot(t))
	tests := []struct {
		target, wantSub, wantCSP string
		status                   int
	}{
		{"/", `<script type="module" src="/assets/index-A.js">`, "script-src 'self' 'sha256-", 200},
		{"/privacy", "lp-data", "script-src 'self' 'sha256-", 200},
		{"/c/discord", "lp-data", "script-src 'self' 'sha256-", 200},
		{"/c/my-community", `<p class="lp-code">404</p>`, "script-src 'self' 'sha256-", 404},
		{"/admin", `src="/assets/admin-C.js"`, webui.AdminCSP, 200},
		{"/admin/settings/theme", `src="/assets/admin-C.js"`, webui.AdminCSP, 200},
		{"/random", `<p class="lp-code">404</p>`, "script-src 'self' 'sha256-", 404},
		{"/c/UPPER", `<p class="lp-code">404</p>`, "script-src 'self' 'sha256-", 404},
		{"/apix", `<p class="lp-code">404</p>`, "script-src 'self' 'sha256-", 404},
	}
	for _, tt := range tests {
		w := ts.do(http.MethodGet, tt.target, nil)
		if w.Code != tt.status || !strings.Contains(w.Body.String(), tt.wantSub) ||
			!strings.Contains(w.Header().Get("Content-Security-Policy"), tt.wantCSP) {
			t.Errorf("%s: status %d csp %q", tt.target, w.Code, w.Header().Get("Content-Security-Policy"))
		}
		if w.Header().Get("Cache-Control") != webui.CacheHTML {
			t.Errorf("%s: cache-control %q", tt.target, w.Header().Get("Cache-Control"))
		}
	}
	first := ts.do(http.MethodGet, "/", nil)
	again := ts.do(http.MethodGet, "/", map[string]string{"If-None-Match": first.Header().Get("ETag")})
	if again.Code != 304 || again.Header().Get("Content-Security-Policy") != first.Header().Get("Content-Security-Policy") {
		t.Errorf("304 = %d, CSP must match", again.Code)
	}
	gz := ts.do(http.MethodGet, "/", map[string]string{"Accept-Encoding": "gzip"})
	if gz.Header().Get("Content-Encoding") != "gzip" {
		t.Error("HTML should be compressed")
	}
	head := ts.do(http.MethodHead, "/", nil)
	if head.Code != 200 || head.Body.Len() != 0 {
		t.Errorf("HEAD / = %d len %d", head.Code, head.Body.Len())
	}
}

func TestAssetsRoutes(t *testing.T) {
	ts := newTestServer(t)
	if w := ts.do(http.MethodGet, "/assets/index-A.js", nil); w.Code != 200 || w.Header().Get("Cache-Control") != webui.CacheImmutable {
		t.Errorf("asset = %d", w.Code)
	}
	if w := ts.do(http.MethodGet, "/assets/missing.js", nil); w.Code != 404 || w.Header().Get("Cache-Control") != webui.CacheNoStore ||
		strings.Contains(w.Body.String(), "<html") {
		t.Errorf("asset miss = %d", w.Code)
	}
}

func TestOpenAPI(t *testing.T) {
	ts := newTestServer(t)
	w := ts.do(http.MethodGet, OpenAPIPath, nil)
	if w.Code != 200 || w.Header().Get("Content-Type") != "application/openapi+json" {
		t.Fatalf("openapi = %d", w.Code)
	}
	var doc map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	if doc["openapi"] != "3.1.0" {
		t.Errorf("openapi version = %v", doc["openapi"])
	}
	spec, err := OpenAPISpec("v1.2.3")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"/api/v1/public/bootstrap"`, `"getPublicBootstrap"`, `"v1.2.3"`, `"requestId"`} {
		if !bytes.Contains(spec, []byte(want)) {
			t.Errorf("spec missing %s", want)
		}
	}
	spec2, _ := OpenAPISpec("v1.2.3")
	if !bytes.Equal(spec, spec2) {
		t.Error("spec must be deterministic")
	}
}

func TestCrossOriginProtection(t *testing.T) {
	ts := newTestServer(t)
	cross := map[string]string{"Sec-Fetch-Site": "cross-site", "Origin": "https://evil.example"}
	w := ts.do(http.MethodPost, "/api/v1/public/bootstrap", cross)
	if w.Code != 403 {
		t.Fatalf("cross-site POST = %d", w.Code)
	}
	if p := decodeProblem(t, w); p.Code != CodeCrossOrigin {
		t.Errorf("code = %s", p.Code)
	}
	if w.Header().Get("X-Content-Type-Options") != "nosniff" || !strings.Contains(w.Header().Get("Content-Security-Policy"), "default-src 'none'") {
		t.Error("cross-origin rejections must carry the common security headers")
	}
	if logs := ts.logs.String(); !strings.Contains(logs, "cross-origin request rejected") || !strings.Contains(logs, `"status":403`) {
		t.Errorf("rejection must be logged and access-logged:\n%s", logs)
	}
	if w := ts.do(http.MethodPost, "/privacy", cross); w.Code != 403 || w.Header().Get("Content-Type") == ProblemContentType {
		t.Errorf("cross-site HTML POST = %d", w.Code)
	}
	trusted := map[string]string{"Sec-Fetch-Site": "cross-site", "Origin": "https://links.example.com"}
	if w := ts.do(http.MethodPost, "/api/v1/public/bootstrap", trusted); w.Code == 403 {
		t.Error("base_url origin must be trusted")
	}
	if w := ts.do(http.MethodGet, "/", cross); w.Code != 200 {
		t.Errorf("cross-site GET = %d", w.Code)
	}
}

func TestAccessLogPrivacy(t *testing.T) {
	ts := newTestServer(t)
	hdr := map[string]string{"CF-Connecting-IP": "203.0.113.77", "User-Agent": "SecretUA/1.0", "Referer": "https://ref.example/x", "CF-IPCity": "Osaka"}
	ts.do(http.MethodGet, "/c/my-community?utm=private-query", hdr)
	ts.do(http.MethodGet, "/api/v1/public/bootstrap?lang=secretlang", hdr)
	public := ts.logs.String()
	for _, leak := range []string{"203.0.113.77", "SecretUA", "ref.example", "private-query", "secretlang", "Osaka"} {
		if strings.Contains(public, leak) {
			t.Errorf("public access log leaks %q:\n%s", leak, public)
		}
	}
	if !strings.Contains(public, `"route":"/c/{slug}"`) {
		t.Errorf("public log should contain route pattern:\n%s", public)
	}
	ts.do(http.MethodGet, "/admin/x?tab=1", hdr)
	ts.do(http.MethodGet, "/api/auth/callback?code=SECRETCODE&state=SECRETSTATE", hdr)
	all := ts.logs.String()
	if strings.Contains(all, "SECRETCODE") || strings.Contains(all, "SECRETSTATE") {
		t.Errorf("OAuth callback parameters leaked:\n%s", all)
	}
	for _, want := range []string{"203.0.113.77", "SecretUA", `"query":"tab=1"`} {
		if !strings.Contains(all, want) {
			t.Errorf("admin log missing %q", want)
		}
	}
}

func TestPanicRecovery(t *testing.T) {
	logs := &syncBuffer{}
	logger := slog.New(slog.NewJSONHandler(logs, nil))
	h := requestContext(logger)(recoverer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("kaboom secret detail")
	})))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/x", nil))
	p := decodeProblem(t, w)
	if w.Code != 500 || p.Code != CodeInternal || strings.Contains(w.Body.String(), "kaboom") {
		t.Errorf("panic problem = %d %s", w.Code, w.Body.String())
	}
	if !strings.Contains(logs.String(), "kaboom secret detail") {
		t.Error("panic must be logged")
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
	if w.Code != 500 || w.Header().Get("Content-Type") == ProblemContentType {
		t.Errorf("HTML panic = %d", w.Code)
	}
	defer func() {
		if rec := recover(); rec != http.ErrAbortHandler { //nolint:errorlint // sentinel identity
			t.Errorf("ErrAbortHandler must be re-panicked, got %v", rec)
		}
	}()
	abort := recoverer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic(http.ErrAbortHandler) }))
	abort.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
}

func TestHumaErrors(t *testing.T) {
	logs := &syncBuffer{}
	logger := slog.New(slog.NewJSONHandler(logs, nil))
	_, api := humatest.New(t, humaConfig("test"))
	huma.Get(api, "/boom", func(context.Context, *struct{}) (*struct{}, error) {
		return nil, errors.New("database password=hunter2 exploded")
	})
	huma.Get(api, "/missing", func(context.Context, *struct{}) (*struct{}, error) {
		return nil, huma.Error404NotFound("thing not found")
	})
	type in struct {
		N int `query:"n" minimum:"1"`
	}
	huma.Get(api, "/validate", func(context.Context, *in) (*struct{}, error) { return nil, nil })

	ctx := context.WithValue(context.WithValue(context.Background(), ctxRequestID, "rid-1"), ctxLogger, logger)
	resp := api.GetCtx(ctx, "/boom")
	var p Problem
	if err := json.Unmarshal(resp.Body.Bytes(), &p); err != nil {
		t.Fatal(err)
	}
	if resp.Code != 500 || p.Code != CodeInternal || p.Detail != "" || len(p.Errors) != 0 || p.RequestID != "rid-1" {
		t.Errorf("5xx problem = %d %+v", resp.Code, p)
	}
	if strings.Contains(resp.Body.String(), "hunter2") || !strings.Contains(logs.String(), "hunter2") {
		t.Error("5xx must hide details from clients but log them")
	}

	resp = api.GetCtx(ctx, "/missing")
	p = Problem{}
	_ = json.Unmarshal(resp.Body.Bytes(), &p)
	if resp.Code != 404 || p.Code != CodeNotFound || p.RequestID != "rid-1" || p.Detail != "thing not found" {
		t.Errorf("404 problem = %+v", p)
	}
	if resp.Header().Get("Content-Type") != ProblemContentType {
		t.Errorf("content-type = %s", resp.Header().Get("Content-Type"))
	}

	resp = api.GetCtx(ctx, "/validate?n=0")
	p = Problem{}
	_ = json.Unmarshal(resp.Body.Bytes(), &p)
	if resp.Code != 422 || p.Code != CodeInvalidRequest || len(p.Errors) == 0 {
		t.Errorf("validation problem = %d %+v", resp.Code, p)
	}
}

func TestCodeMapping(t *testing.T) {
	for status, want := range map[int]string{
		400: CodeInvalidRequest, 404: CodeNotFound, 418: CodeInvalidRequest, 429: CodeRateLimited,
		500: CodeInternal, 502: CodeInternal, 503: CodeNotReady,
	} {
		if got := codeFor(status); got != want {
			t.Errorf("codeFor(%d) = %s, want %s", status, got, want)
		}
	}
	p := NewProblem(500, "custom", "secret detail")
	if p.Code != CodeInternal || p.Detail != "" {
		t.Errorf("NewProblem must sanitize 5xx: %+v", p)
	}
	if newHumaErrorWithContext(nil, 400, "x").(*Problem).RequestID != "" {
		t.Error("nil context should not set request id")
	}
	if loggerFrom(context.Background()) == nil {
		t.Error("loggerFrom must fall back to default")
	}
}

func TestNewHandlerValidation(t *testing.T) {
	if _, err := NewHandler(Deps{}); err == nil {
		t.Error("missing deps should fail")
	}
}

func TestServeGracefulShutdown(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ready := NewReadiness(nil)
	ready.SetReady()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := NewServer(ln.Addr().String(), http.HandlerFunc(healthz), logger)
	if srv.ReadHeaderTimeout != ReadHeaderTimeout || srv.MaxHeaderBytes != MaxHeaderBytes || srv.IdleTimeout != IdleTimeout {
		t.Error("server limits not applied")
	}
	ctx, cancel := context.WithCancel(context.Background())
	var order []string
	done := make(chan error, 1)
	go func() {
		done <- Serve(ctx, srv, ln, ready, logger,
			ShutdownStep{Name: "scheduler", Run: func(context.Context) error {
				if ready.IsReady() {
					order = append(order, "ready-still-true")
				}
				order = append(order, "scheduler")
				return nil
			}},
			ShutdownStep{Name: "pool", Run: func(context.Context) error {
				order = append(order, "pool")
				return errors.New("close failed")
			}},
		)
	}()
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://"+ln.Addr().String()+"/", nil)
	var resp *http.Response
	for range 50 {
		if resp, err = http.DefaultClient.Do(req); err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	cancel()
	err = <-done
	if err == nil || !strings.Contains(err.Error(), "shutdown pool: close failed") {
		t.Errorf("Serve err = %v", err)
	}
	if strings.Join(order, ",") != "scheduler,pool" {
		t.Errorf("shutdown order = %v", order)
	}

	// A listener error is reported.
	closed, _ := net.Listen("tcp", "127.0.0.1:0")
	_ = closed.Close()
	if err := Serve(context.Background(), NewServer("", http.NotFoundHandler(), logger), closed, NewReadiness(nil), logger); err == nil {
		t.Error("closed listener should fail")
	}
}

func TestCompressionHeaders(t *testing.T) {
	ts := newTestServer(t)
	gz := map[string]string{"Accept-Encoding": "gzip"}
	full := ts.do(http.MethodGet, "/", gz)
	if full.Code != 200 || full.Header().Get("Content-Encoding") != "gzip" {
		t.Fatalf("GET / gzip = %d %q", full.Code, full.Header().Get("Content-Encoding"))
	}
	notMod := ts.do(http.MethodGet, "/", map[string]string{"Accept-Encoding": "gzip", "If-None-Match": full.Header().Get("ETag")})
	if notMod.Code != 304 {
		t.Fatalf("conditional GET = %d", notMod.Code)
	}
	for _, h := range []string{"Content-Security-Policy", "Content-Security-Policy-Report-Only", "ETag", "Cache-Control"} {
		if notMod.Header().Get(h) != full.Header().Get(h) {
			t.Errorf("304 %s = %q, 200 has %q", h, notMod.Header().Get(h), full.Header().Get(h))
		}
	}
	for _, w := range []*httptest.ResponseRecorder{full, notMod, ts.do(http.MethodGet, "/", nil)} {
		if !strings.Contains(strings.Join(w.Header().Values("Vary"), ","), "Accept-Encoding") {
			t.Errorf("%d response lacks Vary: Accept-Encoding", w.Code)
		}
	}

	partial := ts.do(http.MethodGet, "/assets/index-A.js", map[string]string{"Accept-Encoding": "gzip", "Range": "bytes=0-3"})
	if partial.Code != http.StatusPartialContent || partial.Header().Get("Content-Encoding") != "" ||
		partial.Header().Get("Content-Range") != "bytes 0-3/15" || partial.Body.String() != "cons" {
		t.Errorf("range = %d enc=%q range=%q body=%q", partial.Code, partial.Header().Get("Content-Encoding"),
			partial.Header().Get("Content-Range"), partial.Body.String())
	}
	if w := ts.do(http.MethodGet, "/assets/index-A.js", gz); w.Header().Get("Content-Encoding") != "gzip" {
		t.Error("non-range asset requests should still be compressed")
	}
}

func TestHSTS(t *testing.T) {
	if w := newTestServer(t).do(http.MethodGet, "/", nil); w.Header().Get("Strict-Transport-Security") != "" {
		t.Error("HSTS must be off by default")
	}
	ts := newTestServer(t, func(d *Deps) { d.HSTS = true })
	if w := ts.do(http.MethodGet, "/healthz", nil); w.Header().Get("Strict-Transport-Security") != HSTSValue {
		t.Errorf("HSTS = %q", w.Header().Get("Strict-Transport-Security"))
	}
	ts = newTestServer(t, func(d *Deps) { d.HSTS, d.BaseURL = true, "http://links.example.com" })
	if w := ts.do(http.MethodGet, "/healthz", nil); w.Header().Get("Strict-Transport-Security") != "" {
		t.Error("HSTS must not be sent for http base_url")
	}
}

func TestRedactQuery(t *testing.T) {
	tests := map[string]string{
		"":                      "",
		"tab=1":                 "tab=1",
		"code=SECRET&state=abc": "code=%5BREDACTED%5D&state=%5BREDACTED%5D",
		"Token=x&next=%2Fadmin": "Token=%5BREDACTED%5D&next=%2Fadmin",
		"bad=%zz&code=SECRET":   "[REDACTED]",
	}
	for in, want := range tests {
		if got := redactQuery(in); got != want {
			t.Errorf("redactQuery(%q) = %q, want %q", in, got, want)
		}
	}
}
