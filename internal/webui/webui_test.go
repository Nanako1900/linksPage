package webui

import (
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/Nanako1900/linksPage/internal/site"
)

const fakeManifest = `{
  "index.html": {"file": "assets/index-AAA.js", "name": "index", "src": "index.html", "isEntry": true,
    "imports": ["_shared-BBB.js"], "css": ["assets/index-CCC.css"], "dynamicImports": ["src/lazy.tsx"]},
  "admin/index.html": {"file": "assets/admin-DDD.js", "src": "admin/index.html", "isEntry": true,
    "imports": ["_shared-BBB.js", "_admin-dep-EEE.js"], "css": ["assets/admin-FFF.css"]},
  "_shared-BBB.js": {"file": "assets/shared-BBB.js", "imports": ["_admin-dep-EEE.js"], "css": ["assets/shared-GGG.css"]},
  "_admin-dep-EEE.js": {"file": "assets/dep-EEE.js", "imports": ["_shared-BBB.js"], "css": ["assets/shared-GGG.css"]},
  "src/lazy.tsx": {"file": "assets/lazy-HHH.js", "isDynamicEntry": true}
}`

func fakeDist() fstest.MapFS {
	files := fstest.MapFS{".vite/manifest.json": {Data: []byte(fakeManifest)}}
	for _, f := range []string{"index-AAA.js", "index-CCC.css", "admin-DDD.js", "admin-FFF.css", "shared-BBB.js", "dep-EEE.js", "shared-GGG.css", "lazy-HHH.js", "font.woff2"} {
		files["assets/"+f] = &fstest.MapFile{Data: []byte("/* " + f + " */"), ModTime: time.Unix(1, 0)}
	}
	return files
}

type testEnv struct {
	rd   *Renderer
	snap *site.Snapshot
	logs *strings.Builder
}

func newEnv(t *testing.T, dist fs.FS, settings site.Settings) *testEnv {
	t.Helper()
	assets, err := LoadAssets(dist)
	if err != nil {
		t.Fatalf("LoadAssets: %v", err)
	}
	snap, err := site.NewSnapshot(7, site.Page{ID: 1, Slug: "default"}, settings, time.Now())
	if err != nil {
		t.Fatalf("NewSnapshot: %v", err)
	}
	env := &testEnv{snap: snap, logs: &strings.Builder{}}
	rd, err := NewRenderer(Options{
		Assets: assets, BaseURL: "https://links.example.com", AppVersion: "test",
		Logger:   slog.New(slog.NewTextHandler(env.logs, nil)),
		Snapshot: func() *site.Snapshot { return env.snap },
	})
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}
	env.rd = rd
	return env
}

func do(h http.Handler, method, target string, hdr map[string]string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, target, nil)
	for k, v := range hdr {
		r.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

var (
	inlineScriptRe = regexp.MustCompile(`<script>([\s\S]*?)</script>`)
	inlineStyleRe  = regexp.MustCompile(`<style id="([a-z-]+)">([\s\S]*?)</style>`)
	lpDataRe       = regexp.MustCompile(`<script id="lp-data" type="application/json">([\s\S]*?)</script>`)
)

func TestLoadAssetsResolvesEntries(t *testing.T) {
	a, err := LoadAssets(fakeDist())
	if err != nil {
		t.Fatal(err)
	}
	if a.Fallback() || a.ManifestHash() == "" || a.ManifestHash() == "none" {
		t.Fatal("expected a real build")
	}
	if a.public.Script != "/assets/index-AAA.js" {
		t.Errorf("public script = %s", a.public.Script)
	}
	if got := strings.Join(a.public.Styles, ","); got != "/assets/index-CCC.css,/assets/shared-GGG.css" {
		t.Errorf("public styles = %s", got)
	}
	if got := strings.Join(a.public.Preloads, ","); got != "/assets/shared-BBB.js,/assets/dep-EEE.js" {
		t.Errorf("public preloads = %s", got)
	}
	if got := strings.Join(a.admin.Preloads, ","); got != "/assets/shared-BBB.js,/assets/dep-EEE.js" {
		t.Errorf("admin preloads = %s", got)
	}
}

func TestLoadAssetsErrors(t *testing.T) {
	tests := []struct {
		name     string
		manifest string
		mutate   func(fstest.MapFS)
		wantSub  string
	}{
		{"bad json", "{", nil, "parse vite manifest"},
		{"missing public", `{"admin/index.html":{"file":"assets/a.js","isEntry":true}}`, nil, `no entry "index.html"`},
		{"not entry", `{"index.html":{"file":"assets/index-AAA.js"}}`, nil, "is not an entry"},
		{"bad path", `{"index.html":{"file":"../etc/passwd","isEntry":true}}`, nil, "clean path"},
		{"bad css path", `{"index.html":{"file":"assets/index-AAA.js","isEntry":true,"css":["x.css"]}}`, nil, "clean path"},
		{"missing import", `{"index.html":{"file":"assets/index-AAA.js","isEntry":true,"imports":["_x"]}}`, nil, `import "_x" is missing`},
		{"bad import path", `{"index.html":{"file":"assets/index-AAA.js","isEntry":true,"imports":["_x"]},"_x":{"file":"assets/a b.js"}}`, nil, "clean path"},
		{"bad import css", `{"index.html":{"file":"assets/index-AAA.js","isEntry":true,"imports":["_x"]},"_x":{"file":"assets/x.js","css":["/abs.css"]}}`, nil, "clean path"},
		{"deep import error", `{"index.html":{"file":"assets/index-AAA.js","isEntry":true,"imports":["_x"]},"_x":{"file":"assets/x.js","imports":["_y"]}}`, nil, `import "_y" is missing`},
		{"missing admin", `{"index.html":{"file":"assets/index-AAA.js","isEntry":true}}`, nil, `no entry "admin/index.html"`},
		{"missing file", "", func(m fstest.MapFS) { delete(m, "assets/shared-GGG.css") }, "missing file /assets/shared-GGG.css"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dist := fakeDist()
			if tt.manifest != "" {
				dist[ManifestPath] = &fstest.MapFile{Data: []byte(tt.manifest)}
			}
			if tt.mutate != nil {
				tt.mutate(dist)
			}
			_, err := LoadAssets(dist)
			if err == nil || !strings.Contains(err.Error(), tt.wantSub) {
				t.Fatalf("err = %v, want %q", err, tt.wantSub)
			}
		})
	}
	if _, err := LoadAssets(errFS{}); err == nil {
		t.Error("read error should fail")
	}
}

type errFS struct{}

func (errFS) Open(string) (fs.File, error) { return nil, errors.New("disk on fire") }

func TestPublicPageRendering(t *testing.T) {
	env := newEnv(t, fakeDist(), site.Default())
	w := do(env.rd.Public(PageHome), http.MethodGet, "/", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	body := w.Body.String()
	h := w.Header()
	if h.Get("Content-Type") != "text/html; charset=utf-8" || h.Get("Cache-Control") != CacheHTML || h.Get("ETag") == "" {
		t.Errorf("headers = %v", h)
	}
	if h.Get("Content-Security-Policy-Report-Only") != TrustedTypesReportOnly {
		t.Error("missing trusted types report-only policy")
	}
	for _, want := range []string{
		`<html lang="zh-CN" data-appearance="auto">`, "<title>我的社区</title>",
		`<meta name="description" content="加入我们的社区。">`,
		`<link rel="canonical" href="https://links.example.com/">`,
		`<meta property="og:locale" content="zh_CN">`, `<meta name="twitter:card" content="summary">`,
		`<meta name="theme-color" content="#f9f7f1" media="(prefers-color-scheme: light)">`,
		`<link rel="stylesheet" href="/assets/index-CCC.css">`, `<link rel="stylesheet" href="/assets/shared-GGG.css">`,
		`<link rel="modulepreload" href="/assets/shared-BBB.js">`, `<link rel="modulepreload" href="/assets/dep-EEE.js">`,
		`<script type="module" src="/assets/index-AAA.js"></script>`, `<div id="root"><main class="lp-shell">`,
		`<p class="lp-note">正在加载…</p>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q", want)
		}
	}
	if strings.Contains(body, "lazy-HHH") {
		t.Error("dynamic imports must not be preloaded")
	}
	assertCSPMatchesInline(t, body, h.Get("Content-Security-Policy"))
	assertBootstrap(t, body, env.snap)
	if strings.Contains(strings.ToLower(body), "style=") {
		t.Error("rendered HTML contains a style attribute")
	}
}

// assertCSPMatchesInline recomputes hashes of every inline script/style in
// the body and checks the CSP allows exactly those.
func assertCSPMatchesInline(t *testing.T, body, csp string) {
	t.Helper()
	scripts := inlineScriptRe.FindAllStringSubmatch(body, -1)
	if len(scripts) != 1 {
		t.Fatalf("expected 1 inline script, got %d", len(scripts))
	}
	if !strings.Contains(csp, "script-src 'self' "+site.CSPHash(scripts[0][1])+";") {
		t.Errorf("CSP %q does not allow boot script hash", csp)
	}
	styles := inlineStyleRe.FindAllStringSubmatch(body, -1)
	if len(styles) != 2 {
		t.Fatalf("expected 2 inline styles, got %d", len(styles))
	}
	for _, s := range styles {
		if !strings.Contains(csp, site.CSPHash(s[2])) {
			t.Errorf("CSP missing hash for style #%s", s[1])
		}
	}
	for _, part := range []string{"default-src 'self'", "frame-ancestors 'none'", "object-src 'none'", "base-uri 'self'"} {
		if !strings.Contains(csp, part) {
			t.Errorf("CSP missing %q", part)
		}
	}
}

func assertBootstrap(t *testing.T, body string, snap *site.Snapshot) {
	t.Helper()
	m := lpDataRe.FindStringSubmatch(body)
	if m == nil {
		t.Fatal("lp-data script missing")
	}
	var env struct {
		Data site.Bootstrap `json:"data"`
	}
	if err := json.Unmarshal([]byte(m[1]), &env); err != nil {
		t.Fatalf("lp-data is not JSON: %v\n%s", err, m[1])
	}
	if env.Data.Version != snap.Version || env.Data.Page.Slug != "default" || env.Data.Site.Theme.Light.Bg != "#f9f7f1" {
		t.Errorf("bootstrap = %+v", env.Data)
	}
}

func TestPublicPageEscaping(t *testing.T) {
	s := site.Default()
	s.Title = site.LocalizedText{"zh-CN": `</script><script>alert(1)</script>`, "en": "x"}
	s.Description = site.LocalizedText{"zh-CN": `"><img src=x onerror=alert(1)>`}
	env := newEnv(t, fakeDist(), s)
	body := do(env.rd.Public(PageHome), http.MethodGet, "/", nil).Body.String()
	if strings.Contains(body, "<script>alert(1)") || strings.Contains(body, "<img src=x") {
		t.Fatalf("unescaped content in body:\n%s", body)
	}
	assertCSPMatchesInline(t, body, PublicCSP(env.rd.bootHash, env.rd.criticalHash, env.snap.ThemeHash))
	m := lpDataRe.FindStringSubmatch(body)
	var v map[string]any
	if m == nil || json.Unmarshal([]byte(m[1]), &v) != nil {
		t.Fatal("lp-data must stay valid JSON")
	}
}

func TestConditionalRequests(t *testing.T) {
	env := newEnv(t, fakeDist(), site.Default())
	h := env.rd.Public(PageHome)
	first := do(h, http.MethodGet, "/", nil)
	etag := first.Header().Get("ETag")
	if !strings.HasPrefix(etag, `W/"`) {
		t.Fatalf("etag = %q", etag)
	}
	for _, inm := range []string{etag, strings.TrimPrefix(etag, "W/"), `"other", ` + etag, "*"} {
		w := do(h, http.MethodGet, "/", map[string]string{"If-None-Match": inm})
		if w.Code != http.StatusNotModified || w.Body.Len() != 0 {
			t.Errorf("If-None-Match %q: status %d len %d", inm, w.Code, w.Body.Len())
		}
		if w.Header().Get("Content-Security-Policy") != first.Header().Get("Content-Security-Policy") {
			t.Error("304 must carry the same CSP as the 200")
		}
		if w.Header().Get("ETag") != etag || w.Header().Get("Cache-Control") != CacheHTML {
			t.Error("304 must carry ETag and Cache-Control")
		}
	}
	if w := do(h, http.MethodGet, "/", map[string]string{"If-None-Match": `W/"stale"`}); w.Code != http.StatusOK {
		t.Errorf("stale etag status = %d", w.Code)
	}

	// Theme change -> new CSP and new ETag.
	s := site.Default()
	s.Theme.Light.Accent = "#ff0000"
	snap, err := site.NewSnapshot(8, env.snap.Page, s, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	env.snap = snap
	second := do(h, http.MethodGet, "/", map[string]string{"If-None-Match": etag})
	if second.Code != http.StatusOK || second.Header().Get("ETag") == etag ||
		second.Header().Get("Content-Security-Policy") == first.Header().Get("Content-Security-Policy") {
		t.Error("theme change must change ETag and CSP")
	}

	if w := do(h, http.MethodHead, "/", nil); w.Code != http.StatusOK || w.Body.Len() != 0 {
		t.Errorf("HEAD status %d len %d", w.Code, w.Body.Len())
	}
}

func TestOtherPublicPages(t *testing.T) {
	env := newEnv(t, fakeDist(), site.Default())
	mux := http.NewServeMux()
	mux.Handle("GET /c/{slug}", env.rd.Public(PageCommunity))
	mux.Handle("GET /privacy", env.rd.Public(PagePrivacy))
	mux.Handle("GET /", env.rd.NotFound())

	priv := do(mux, http.MethodGet, "/privacy?lang=en", nil)
	if priv.Code != 200 || !strings.Contains(priv.Body.String(), "<title>Privacy · My Communities</title>") ||
		!strings.Contains(priv.Body.String(), `<html lang="en"`) {
		t.Errorf("privacy page wrong: %d", priv.Code)
	}
	comm := do(mux, http.MethodGet, "/c/my-server", nil)
	if comm.Code != 200 || !strings.Contains(comm.Body.String(), `href="https://links.example.com/c/my-server"`) {
		t.Errorf("community page wrong: %d", comm.Code)
	}
	for _, target := range []string{"/c/Bad_Slug", "/nope"} {
		w := do(mux, http.MethodGet, target, map[string]string{"If-None-Match": "*"})
		body := w.Body.String()
		if w.Code != http.StatusNotFound || w.Header().Get("ETag") != "" {
			t.Errorf("%s: status %d", target, w.Code)
		}
		if strings.Contains(body, `type="module"`) || strings.Contains(body, "lp-data") ||
			!strings.Contains(body, `<meta name="robots" content="noindex">`) || !strings.Contains(body, `<p class="lp-code">404</p>`) {
			t.Errorf("%s: unexpected 404 body", target)
		}
		assertCSPMatchesInline(t, body, w.Header().Get("Content-Security-Policy"))
	}
}

func TestAdminShell(t *testing.T) {
	env := newEnv(t, fakeDist(), site.Default())
	w := do(env.rd.Admin(), http.MethodGet, "/admin/settings", nil)
	body := w.Body.String()
	if w.Code != 200 || w.Header().Get("Content-Security-Policy") != AdminCSP || w.Header().Get("X-Robots-Tag") == "" {
		t.Fatalf("admin headers wrong: %d %v", w.Code, w.Header())
	}
	if w.Header().Get("Content-Security-Policy-Report-Only") != "" {
		t.Error("admin must not get the public report-only policy")
	}
	if inlineScriptRe.MatchString(body) || strings.Contains(body, "<style") {
		t.Error("admin shell must not contain inline scripts or styles")
	}
	for _, want := range []string{`<script type="module" src="/assets/admin-DDD.js"></script>`, `<link rel="stylesheet" href="/assets/admin-FFF.css">`, `<link rel="modulepreload" href="/assets/dep-EEE.js">`} {
		if !strings.Contains(body, want) {
			t.Errorf("admin body missing %q", want)
		}
	}
	if w2 := do(env.rd.Admin(), http.MethodGet, "/admin", map[string]string{"If-None-Match": w.Header().Get("ETag")}); w2.Code != http.StatusNotModified {
		t.Errorf("admin 304 status = %d", w2.Code)
	}
}

func TestFallbackMode(t *testing.T) {
	env := newEnv(t, fstest.MapFS{}, site.Default())
	if !env.rd.opts.Assets.Fallback() || !strings.Contains(env.logs.String(), "frontend build not embedded") {
		t.Fatal("expected fallback mode with a WARN")
	}
	pub := do(env.rd.Public(PageHome), http.MethodGet, "/", nil).Body.String()
	if strings.Contains(pub, `type="module"`) || !strings.Contains(pub, "前端资源尚未构建") || !strings.Contains(pub, "lp-data") {
		t.Error("fallback public page wrong")
	}
	adm := do(env.rd.Admin(), http.MethodGet, "/admin", nil).Body.String()
	if strings.Contains(adm, `type="module"`) || !strings.Contains(adm, "前端资源尚未构建") {
		t.Error("fallback admin page wrong")
	}
	if w := do(env.rd.AssetsHandler(), http.MethodGet, "/assets/x.js", nil); w.Code != 404 {
		t.Errorf("asset in fallback mode = %d", w.Code)
	}
	// The real embedded dist must load too (empty or built).
	if _, err := LoadAssets(DistFS()); err != nil {
		t.Errorf("embedded dist: %v", err)
	}
}

func TestAssetsHandler(t *testing.T) {
	env := newEnv(t, fakeDist(), site.Default())
	h := env.rd.AssetsHandler()
	ok := do(h, http.MethodGet, "/assets/index-AAA.js", nil)
	if ok.Code != 200 || ok.Header().Get("Cache-Control") != CacheImmutable ||
		ok.Header().Get("Content-Type") != "text/javascript; charset=utf-8" || ok.Body.String() != "/* index-AAA.js */" {
		t.Errorf("asset hit wrong: %d %v %q", ok.Code, ok.Header(), ok.Body.String())
	}
	if w := do(h, http.MethodGet, "/assets/font.woff2", nil); w.Header().Get("Content-Type") != "font/woff2" {
		t.Errorf("woff2 type = %q", w.Header().Get("Content-Type"))
	}
	for _, target := range []string{"/assets/missing.js", "/assets/", "/assets", "/assets/../.vite/manifest.json", "/.vite/manifest.json", "/assets//x.js"} {
		w := do(h, http.MethodGet, target, nil)
		if w.Code != 404 || w.Header().Get("Cache-Control") != CacheNoStore || strings.Contains(w.Body.String(), "<html") {
			t.Errorf("%s: status %d cache %q", target, w.Code, w.Header().Get("Cache-Control"))
		}
	}
	// A directory under assets must not be listed.
	dist := fakeDist()
	dist["assets/sub/file.js"] = &fstest.MapFile{Data: []byte("x")}
	env2 := newEnv(t, dist, site.Default())
	if w := do(env2.rd.AssetsHandler(), http.MethodGet, "/assets/sub", nil); w.Code != 404 {
		t.Errorf("directory status = %d", w.Code)
	}
}

func TestContentType(t *testing.T) {
	for name, want := range map[string]string{
		"a.css": "text/css; charset=utf-8", "a.MJS": "text/javascript; charset=utf-8",
		"a.svg": "image/svg+xml", "a.unknownext": "application/octet-stream", "a.html": "text/html; charset=utf-8",
	} {
		if got := contentType(name); got != want {
			t.Errorf("contentType(%s) = %q, want %q", name, got, want)
		}
	}
}

func TestWriteRenderError(t *testing.T) {
	env := newEnv(t, fakeDist(), site.Default())
	w := httptest.NewRecorder()
	env.rd.write(w, httptest.NewRequest(http.MethodGet, "/", nil), page{}, errors.New("boom"), 200, true)
	if w.Code != 500 || w.Header().Get("Cache-Control") != CacheNoStore || !strings.Contains(env.logs.String(), "boom") {
		t.Errorf("render error handling wrong: %d", w.Code)
	}
	if _, err := NewRenderer(Options{}); err == nil {
		t.Error("NewRenderer without options should fail")
	}
}

// TestTemplatesHaveNoStyleAttributes is the CI-greppable guarantee that Go
// templates never use style= attributes (CSP forbids inline styles).
func TestTemplatesHaveNoStyleAttributes(t *testing.T) {
	err := fs.WalkDir(templateFS, "templates", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		f, err := templateFS.Open(p)
		if err != nil {
			return err
		}
		defer func() { _ = f.Close() }()
		b, err := io.ReadAll(f)
		if err != nil {
			return err
		}
		if regexp.MustCompile(`(?i)\sstyle\s*=`).Match(b) {
			t.Errorf("%s contains a style= attribute", p)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestHelpers(t *testing.T) {
	if textFor("fr-FR").Loading != uiText["en"].Loading || textFor("zh-TW").Loading != uiText["zh"].Loading {
		t.Error("textFor fallback wrong")
	}
	if etagMatches("", `W/"a"`) {
		t.Error("empty If-None-Match must not match")
	}
}
