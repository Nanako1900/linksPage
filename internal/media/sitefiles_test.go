package media

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSiteFilesHandler(t *testing.T) {
	t.Parallel()
	files := newSiteFiles([]byte("\x89PNG ico"), []byte(`{"name":"x"}`), buildRobots(true))
	current := func() *SiteFiles { return &files }

	tests := []struct {
		name string
		file SiteFile
		ct   string
		body string
	}{
		{"favicon", FileFavicon, "image/png", "\x89PNG ico"},
		{"manifest", FileManifest, "application/manifest+json", `{"name":"x"}`},
		{"robots", FileRobots, "text/plain; charset=utf-8", string(buildRobots(true))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := SiteFilesHandler(tt.file, current)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil))
			checkResponse(t, rec, 200, CacheSiteFiles, tt.ct)
			if rec.Body.String() != tt.body {
				t.Fatalf("body = %q", rec.Body.String())
			}
			etag := rec.Header().Get("ETag")
			if !strings.HasPrefix(etag, `"`+files.ETag+"-") {
				t.Fatalf("ETag = %q", etag)
			}
			req := httptest.NewRequest(http.MethodGet, "/x", nil)
			req.Header.Set("If-None-Match", etag)
			rec = httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != http.StatusNotModified {
				t.Fatalf("conditional status = %d", rec.Code)
			}
		})
	}
}

func TestSiteFilesHandlerBeforeBuild(t *testing.T) {
	t.Parallel()
	for name, current := range map[string]func() *SiteFiles{
		"nil":   func() *SiteFiles { return nil },
		"empty": func() *SiteFiles { return &SiteFiles{} },
	} {
		rec := httptest.NewRecorder()
		SiteFilesHandler(FileManifest, current).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
		if rec.Code != http.StatusServiceUnavailable || rec.Header().Get("Retry-After") != SiteFilesRetryAfter {
			t.Errorf("%s: status %d, Retry-After %q", name, rec.Code, rec.Header().Get("Retry-After"))
		}
		if rec.Header().Get("Cache-Control") != CacheNoStore {
			t.Errorf("%s: cache %q", name, rec.Header().Get("Cache-Control"))
		}
	}
}

func TestNewSiteFilesETag(t *testing.T) {
	t.Parallel()
	a := newSiteFiles([]byte("a"), []byte("bc"), []byte("d"))
	b := newSiteFiles([]byte("ab"), []byte("c"), []byte("d"))
	if a.ETag == b.ETag || len(a.ETag) != 32 {
		t.Fatalf("ETags %q %q", a.ETag, b.ETag)
	}
	if newSiteFiles([]byte("a"), []byte("bc"), []byte("d")).ETag != a.ETag {
		t.Fatal("ETag not deterministic")
	}
}

func TestBuildRobots(t *testing.T) {
	t.Parallel()
	tests := []struct {
		index    bool
		contains []string
		absent   []string
	}{
		{true, []string{"User-agent: *", "Allow: /\n", "Disallow: /api/", "Disallow: /go/"}, []string{"Disallow: /\n"}},
		{false, []string{"User-agent: *", "Disallow: /\n"}, []string{"Allow"}},
	}
	for _, tt := range tests {
		got := string(buildRobots(tt.index))
		for _, s := range tt.contains {
			if !strings.Contains(got, s) {
				t.Errorf("index=%v: missing %q in %q", tt.index, s, got)
			}
		}
		for _, s := range tt.absent {
			if strings.Contains(got, s) {
				t.Errorf("index=%v: unexpected %q in %q", tt.index, s, got)
			}
		}
	}
}

func TestBuildManifest(t *testing.T) {
	t.Parallel()
	colors, err := resolvePalette(GenerateInput{ThemeColorHex: "#112233"})
	if err != nil {
		t.Fatal(err)
	}
	favs := map[int]Stored{
		192: {Key: "a.png", ContentType: "image/png"},
		512: {Key: "b.png", ContentType: "image/png"},
	}
	tests := []struct {
		name, in, short string
		wantName        string
		wantShort       string
	}{
		{"defaults", "", "", DefaultAppName, DefaultAppName},
		{"short from name", "  Nanako's Community Hub  ", "", "Nanako's Community Hub", "Nanako's Com"},
		{"explicit short", "社区导航页", "社区", "社区导航页", "社区"},
		{"long name", strings.Repeat("名", 150), "", strings.Repeat("名", 100), strings.Repeat("名", 12)},
	}
	for _, tt := range tests {
		b, err := buildManifest(GenerateInput{Name: tt.in, ShortName: tt.short}, colors, favs)
		if err != nil {
			t.Fatal(err)
		}
		var m webManifest
		if err := json.Unmarshal(b, &m); err != nil {
			t.Fatal(err)
		}
		if m.Name != tt.wantName || m.ShortName != tt.wantShort {
			t.Errorf("%s: name %q short %q", tt.name, m.Name, m.ShortName)
		}
		if m.ThemeColor != "#112233" || m.BackgroundColor != DefaultBackgroundHex || m.StartURL != "/" {
			t.Errorf("%s: colours/start %+v", tt.name, m)
		}
		if len(m.Icons) != 2 || m.Icons[0].Src != "/media/u/a.png" || m.Icons[1].Sizes != "512x512" {
			t.Errorf("%s: icons %+v", tt.name, m.Icons)
		}
	}
	if _, err := buildManifest(GenerateInput{}, colors, map[int]Stored{}); err == nil {
		t.Error("missing favicons: want error")
	}
}
