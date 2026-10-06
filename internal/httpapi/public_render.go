package httpapi

import (
	"context"
	"crypto/subtle"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/Nanako1900/linksPage/internal/site"
)

// Render endpoint constants (contract section 9).
const (
	ProxyAuthHeader = "X-LP-Proxy-Auth"
	RenderPath      = "/api/v1/public/render"
	CacheRender     = "public, max-age=0, s-maxage=60"
)

// RenderSource renders a public path for the topology C Worker
// (*webui.Renderer satisfies it).
type RenderSource interface {
	RenderDTO(path, lang, acceptLanguage string) (site.RenderDTO, error)
}

// RenderInput are the parameters of GET /api/v1/public/render.
type RenderInput struct {
	Path           string `query:"path" required:"true" minLength:"1" maxLength:"512" doc:"Public path: /, /privacy or /c/{slug}"`
	Lang           string `query:"lang" maxLength:"35" doc:"Requested language (optional)"`
	AcceptLanguage string `header:"Accept-Language" doc:"Visitor Accept-Language, used when lang is empty"`
	ProxyAuth      string `header:"X-LP-Proxy-Auth" doc:"Shared secret (edge.proxy_auth), required when configured"`
}

// RenderOutput is the response of GET /api/v1/public/render.
type RenderOutput struct {
	CacheControl string `header:"Cache-Control"`
	// Vary is always Accept-Language: an unknown ?lang= falls back to it,
	// so even a lang-keyed response can depend on the header.
	Vary string `header:"Vary"`
	Body struct {
		Data site.RenderDTO `json:"data"`
	}
}

func registerRender(api huma.API, p publicAPI) {
	huma.Register(api, huma.Operation{
		OperationID: "getPublicRender",
		Method:      http.MethodGet,
		Path:        RenderPath,
		Summary:     "Server rendering for the edge Worker",
		Description: "Head fragment, fallback markup, data and CSP of a public page (topology C).",
		Tags:        []string{"public"},
		Errors:      []int{http.StatusForbidden, http.StatusServiceUnavailable, http.StatusTooManyRequests},
	}, p.renderPage)
}

func (p publicAPI) renderPage(_ context.Context, in *RenderInput) (*RenderOutput, error) {
	if !proxyAuthOK(p.proxyAuth, in.ProxyAuth) {
		return nil, NewProblem(http.StatusForbidden, CodeForbidden, "missing or invalid "+ProxyAuthHeader)
	}
	if err := p.checkReady(); err != nil {
		return nil, err
	}
	dto, err := p.render.RenderDTO(in.Path, in.Lang, in.AcceptLanguage)
	if err != nil {
		return nil, huma.Error500InternalServerError("render failed", err)
	}
	out := &RenderOutput{CacheControl: CacheRender, Vary: "Accept-Language"}
	if in.Lang == "" {
		// The result depends on Accept-Language, which shared caches ignore.
		out.CacheControl = "no-cache"
	}
	out.Body.Data = dto
	return out, nil
}

// proxyAuthOK compares the header with the configured secret in constant
// time. No secret configured accepts every request.
func proxyAuthOK(secret []byte, got string) bool {
	if len(secret) == 0 {
		return true
	}
	return subtle.ConstantTimeCompare(secret, []byte(got)) == 1
}
