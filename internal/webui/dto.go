package webui

import (
	"strings"

	"github.com/Nanako1900/linksPage/internal/site"
)

// RenderDTO renders path for the topology C Worker (GET
// /api/v1/public/render). It shares renderParts with the HTML handlers.
func (rd *Renderer) RenderDTO(path, lang, acceptLanguage string) (site.RenderDTO, error) {
	kind, slug := routeFor(path)
	r, err := rd.renderParts(rd.resolve(kind, slug, lang, acceptLanguage))
	if err != nil {
		return site.RenderDTO{}, err
	}
	return site.RenderDTO{
		Status:        r.status,
		Lang:          r.lang,
		Appearance:    r.appearance,
		Head:          r.head,
		Fallback:      r.fallback,
		Data:          r.public,
		CSP:           r.csp,
		CSPReportOnly: TrustedTypesReportOnly,
		ETag:          r.etag,
	}, nil
}

// routeFor maps a RenderDTO path to a page kind. Only "/", "/privacy" and
// "/c/{slug}" are public pages.
func routeFor(path string) (PageKind, string) {
	switch {
	case path == "/":
		return PageHome, ""
	case path == "/privacy":
		return PagePrivacy, ""
	case strings.HasPrefix(path, site.PathCommunity):
		slug := strings.TrimPrefix(path, site.PathCommunity)
		if slugRe.MatchString(slug) {
			return PageCommunity, slug
		}
	}
	return PageNotFound, ""
}
