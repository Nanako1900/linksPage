package site

import "time"

// LiveDTO is GET /api/v1/public/live (envelope {"data": LiveDTO}).
//
// Polling: the public page polls every 60 s while
// document.visibilityState is "visible" (and once right after becoming
// visible when the last poll is older than 60 s), sending If-None-Match.
// The server answers 304 when unchanged, otherwise 200 with
// "Cache-Control: public, max-age=0, s-maxage=30" and a strong ETag.
// When Revision differs from the page's PublicPage.Revision the client
// reloads GET /api/v1/public/bootstrap. Communities absent from the map
// keep their last state.
type LiveDTO struct {
	Revision    string              `json:"revision"`
	Communities map[string]LiveView `json:"communities"`
	GeneratedAt time.Time           `json:"generatedAt"`
}

// Live extracts the LiveDTO from a page.
func (p *PublicPage) Live() LiveDTO {
	out := LiveDTO{Revision: p.Revision, Communities: make(map[string]LiveView, len(p.Communities)), GeneratedAt: p.GeneratedAt}
	for id, c := range p.Communities {
		out.Communities[id] = c.Live
	}
	return out
}

// NeedsDiscordFrame reports whether any card enables the Discord iframe,
// i.e. whether the public CSP needs "frame-src https://discord.com".
func (p *PublicPage) NeedsDiscordFrame() bool {
	for _, c := range p.Communities {
		if c.Embed != nil {
			return true
		}
	}
	return false
}

// RenderDTO is GET /api/v1/public/render?path=&lang= for the topology C
// Worker (envelope {"data": RenderDTO}). The Worker:
//  1. sets <html lang={Lang} data-appearance={Appearance}>;
//  2. removes <title> and <meta name="description"> from the static shell
//     and appends Head to <head>;
//  3. replaces the children of #root with Fallback;
//  4. when Data is not null, appends
//     <script id="lp-data" type="application/json">{"data":Data}</script>
//     to <body>, escaping "<", ">", "&", U+2028 and U+2029 as \uXXXX;
//  5. responds with Status and the headers Content-Security-Policy: CSP,
//     Content-Security-Policy-Report-Only: CSPReportOnly,
//     Cache-Control: no-cache, no-transform.
//
// path must be "/", "/privacy" or "/c/{slug}"; anything else (or an
// unknown slug) yields Status 404 with the 404 page and Data null.
type RenderDTO struct {
	Status        int         `json:"status"`
	Lang          string      `json:"lang"`
	Appearance    string      `json:"appearance"`
	Head          string      `json:"head"`
	Fallback      string      `json:"fallback"`
	Data          *PublicPage `json:"data"`
	CSP           string      `json:"csp"`
	CSPReportOnly string      `json:"cspReportOnly"`
	// ETag is the origin's validator for this rendering (weak).
	ETag string `json:"etag"`
}
