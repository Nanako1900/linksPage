package webui

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestRenderDTOMatchesHTML(t *testing.T) {
	env := fixtureEnv(t, fakeMarkdown{}, withHeadAssets)
	mux := http.NewServeMux()
	mux.Handle("GET /{$}", env.rd.Public(PageHome))
	mux.Handle("GET /privacy", env.rd.Public(PagePrivacy))
	mux.Handle("GET /c/{slug}", env.rd.Public(PageCommunity))
	mux.Handle("GET /", env.rd.NotFound())

	tests := []struct {
		path, lang, accept string
		status             int
		wantLang           string
		hasData            bool
	}{
		{"/", "", "", 200, "zh-CN", true},
		{"/", "en", "", 200, "en", true},
		{"/", "", "en-US", 200, "en", true},
		{"/privacy", "", "", 200, "zh-CN", true},
		{"/c/discord", "en", "", 200, "en", true},
		{"/c/unknown", "", "", 404, "zh-CN", false},
		{"/c/Bad_Slug", "", "", 404, "zh-CN", false},
		{"/c/", "", "", 404, "zh-CN", false},
		{"/admin", "", "", 404, "zh-CN", false},
		{"/privacy/", "", "", 404, "zh-CN", false},
	}
	for _, tt := range tests {
		t.Run(tt.path+"?"+tt.lang+tt.accept, func(t *testing.T) {
			dto, err := env.rd.RenderDTO(tt.path, tt.lang, tt.accept)
			if err != nil {
				t.Fatal(err)
			}
			if dto.Status != tt.status || dto.Lang != tt.wantLang || (dto.Data != nil) != tt.hasData || dto.Appearance != "auto" {
				t.Fatalf("dto = status %d lang %s data %v", dto.Status, dto.Lang, dto.Data != nil)
			}
			if dto.CSPReportOnly != TrustedTypesReportOnly || dto.ETag == "" {
				t.Error("missing report-only policy or etag")
			}
			for _, banned := range []string{"<meta charset", `name="viewport"`, `type="module"`, `rel="stylesheet"`, "lp-data"} {
				if strings.Contains(dto.Head, banned) {
					t.Errorf("head fragment must not contain %q", banned)
				}
			}
			assertCSPMatchesInline(t, dto.Head, dto.CSP)

			target := tt.path
			if tt.lang != "" {
				target += "?lang=" + tt.lang
			}
			w := do(mux, http.MethodGet, target, map[string]string{"Accept-Language": tt.accept})
			body := w.Body.String()
			if w.Code != dto.Status || w.Header().Get("Content-Security-Policy") != dto.CSP {
				t.Errorf("HTML status %d CSP differs from DTO", w.Code)
			}
			if !strings.Contains(body, "\n"+dto.Head+"\n") || !strings.Contains(body, `<div id="root">`+dto.Fallback+"</div>") {
				t.Error("HTML must embed the DTO head and fallback verbatim")
			}
			if dto.Status == 200 && w.Header().Get("ETag") != dto.ETag {
				t.Errorf("ETag: html %q dto %q", w.Header().Get("ETag"), dto.ETag)
			}
		})
	}
}

func TestRenderDTOJSONShape(t *testing.T) {
	env := fixtureEnv(t, fakeMarkdown{}, nil)
	dto, err := env.rd.RenderDTO("/c/nope", "", "")
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(dto)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"status", "lang", "appearance", "head", "fallback", "data", "csp", "cspReportOnly", "etag"} {
		if _, ok := m[k]; !ok {
			t.Errorf("RenderDTO JSON lacks %q", k)
		}
	}
	if m["data"] != nil {
		t.Error("404 data must be null")
	}
}

func TestRouteFor(t *testing.T) {
	for path, want := range map[string]PageKind{
		"/": PageHome, "/privacy": PagePrivacy, "/c/ok-1": PageCommunity, "/c/-bad": PageNotFound,
		"": PageNotFound, "/c": PageNotFound, "/c/a/b": PageNotFound, "//": PageNotFound,
	} {
		if got, _ := routeFor(path); got != want {
			t.Errorf("routeFor(%q) = %v, want %v", path, got, want)
		}
	}
}
