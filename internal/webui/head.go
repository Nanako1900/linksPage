package webui

import (
	"html/template"
	"maps"
	"net/url"
	"slices"

	"github.com/Nanako1900/linksPage/internal/site"
)

// Twitter card types.
const (
	twitterSummary      = "summary"
	twitterSummaryLarge = "summary_large_image"
)

// headView is the <head> fragment model (template "head"). It is shared by
// the HTML document and RenderDTO.Head.
type headView struct {
	Title, Description, Robots       string
	Canonical, XDefault              string
	Alternates                       []alternateView
	SiteName, OGTitle, OGDescription string
	OGLocale                         string
	OGLocaleAlternates               []string
	OGImage                          *absImage
	TwitterCard                      string
	ThemeColorLight, ThemeColorDark  string
	Icons                            []iconLink
	Manifest                         bool
	CriticalCSS                      template.CSS
	ThemeCSS                         template.CSS
	BootScript                       template.JS
}

type alternateView struct{ Lang, Href string }

type absImage struct {
	URL           string
	Width, Height int
}

type iconLink struct{ Rel, Sizes, Href string }

func (rd *Renderer) headView(pc pageCtx) headView {
	s := pc.snap.Settings
	siteName := pc.text(s.Title)
	hv := headView{
		Title:           siteName,
		Description:     pc.text(s.Description),
		SiteName:        siteName,
		ThemeColorLight: s.Theme.ThemeColor(false),
		ThemeColorDark:  s.Theme.ThemeColor(true),
		Icons:           iconLinks(pc.snap.Head.Icons),
		Manifest:        pc.snap.Files != nil,
		CriticalCSS:     template.CSS(CriticalCSS()),    //nolint:gosec // fixed embedded content, hashed for CSP
		ThemeCSS:        template.CSS(pc.snap.ThemeCSS), //nolint:gosec // generated from validated hex/enum tokens
		BootScript:      template.JS(BootScript()),      //nolint:gosec // fixed embedded content, hashed for CSP
	}
	if pc.snap.Head.Robots == site.SearchNoIndex {
		hv.Robots = site.SearchNoIndex
	}
	switch pc.kind {
	case PageNotFound:
		hv.Title = pc.txt.NotFoundTitle + " · " + siteName
		hv.Robots = site.SearchNoIndex
		return hv
	case PagePrivacy:
		hv.Title = pc.txt.PrivacyTitle + " · " + siteName
	case PageCommunity:
		hv.Title = pc.text(pc.community.Name) + " · " + siteName
		if d := pc.text(pc.community.Description); d != "" {
			hv.Description = d
		}
	}
	rd.addSocial(&hv, pc)
	return hv
}

// addSocial fills canonical, hreflang alternates and OG/Twitter/itemprop
// metadata (not emitted on 404 pages).
func (rd *Renderer) addSocial(hv *headView, pc pageCtx) {
	s := pc.snap.Settings
	hv.Canonical = rd.pageURL(pc.path, pc.locale, s.DefaultLocale)
	if len(s.Locales) > 1 {
		for _, l := range s.Locales {
			hv.Alternates = append(hv.Alternates, alternateView{Lang: l, Href: rd.pageURL(pc.path, l, s.DefaultLocale)})
		}
		hv.XDefault = rd.opts.BaseURL + pc.path
	}
	hv.OGLocale = ogLocale(pc.locale)
	for _, l := range s.Locales {
		if l != pc.locale {
			hv.OGLocaleAlternates = append(hv.OGLocaleAlternates, ogLocale(l))
		}
	}
	hv.OGTitle, hv.OGDescription = hv.Title, hv.Description
	if pc.kind == PageHome {
		hv.OGTitle = firstNonEmpty(pc.text(pc.snap.Head.OGTitle), hv.Title)
		hv.OGDescription = firstNonEmpty(pc.text(pc.snap.Head.OGDescription), hv.Description)
	}
	hv.TwitterCard = twitterSummary
	if pc.kind == PageCommunity && pc.community.Icon != nil {
		hv.OGImage = rd.absImage(pc.community.Icon)
	} else if img := pc.snap.Head.OGImage; img != nil {
		hv.OGImage = rd.absImage(img)
		hv.TwitterCard = twitterSummaryLarge
	}
}

// pageURL is the absolute URL of path in locale (?lang= only for
// non-default locales).
func (rd *Renderer) pageURL(path, locale, defaultLocale string) string {
	u := rd.opts.BaseURL + path
	if locale != defaultLocale {
		u += "?lang=" + url.QueryEscape(locale)
	}
	return u
}

func (rd *Renderer) absImage(img *site.ImageView) *absImage {
	return &absImage{URL: rd.opts.BaseURL + img.URL, Width: img.Width, Height: img.Height}
}

// iconLinks emits the 32px favicon and the 180px apple-touch-icon; the
// 192/512 icons are referenced from the web manifest.
func iconLinks(icons map[int]string) []iconLink {
	var out []iconLink
	for _, size := range slices.Sorted(maps.Keys(icons)) {
		href := icons[size]
		switch size {
		case 32:
			out = append(out, iconLink{Rel: "icon", Sizes: "32x32", Href: href})
		case 180:
			out = append(out, iconLink{Rel: "apple-touch-icon", Sizes: "180x180", Href: href})
		}
	}
	return out
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
