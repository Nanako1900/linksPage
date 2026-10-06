package golink

import (
	"html/template"
	"net/http"

	"github.com/Nanako1900/linksPage/internal/provider"
	"github.com/Nanako1900/linksPage/internal/site"
)

// Page kinds rendered by guide.gohtml (the "error" kind is the 503 page).
const kindError = "error"

// defaultQRSide is the rendered QR size when the page snapshot does not
// know the image dimensions (≥240px so WeChat can long-press scan it).
const defaultQRSide = 240

// pageView is the template model of one guide page.
type pageView struct {
	Kind       string
	Lang       string
	Appearance string
	ThemeColor string
	Title      string
	ThemeCSS   template.CSS
	CSS        template.CSS
	Script     template.JS
	T          guideText

	SiteTitle     string
	Heading       string
	Platform      string
	Lede          string
	OpenInBrowser string
	CopyURL       string
	QQNumber      string
	QQHint        string
	QR            *qrView
	ShowNoQR      bool
	Contact       *contactView
	Others        []otherView
}

type qrView struct {
	URL    string
	Width  int
	Height int
	Note   string
}

type contactView struct {
	Label  string
	Value  string
	Button string
}

type otherView struct {
	Name     string
	Platform string
	Href     string
}

// pageContext bundles what every page builder needs.
type pageContext struct {
	snap     *site.Snapshot
	locale   string
	fallback string
	text     guideText
	baseURL  string
}

func (pc pageContext) get(t site.LocalizedText) string {
	return t.Get(pc.locale, pc.fallback)
}

// statusFor maps a page kind to its HTTP status.
func statusFor(kind string) int {
	switch kind {
	case string(ActionNotFound):
		return http.StatusNotFound
	case kindError:
		return http.StatusServiceUnavailable
	default:
		return http.StatusOK
	}
}

// buildView fills the action-specific part of the page model.
func buildView(pc pageContext, d Decision) pageView {
	v := pageView{Kind: string(d.Action), T: pc.text, SiteTitle: pc.get(pc.snap.Settings.Title)}
	if d.Community == nil {
		v.Kind = string(ActionNotFound)
		v.Heading, v.Lede = pc.text.NotFoundTitle, notFoundLede(pc)
		return v
	}
	c := *d.Community
	cv, onPage := pc.snap.Public.Communities[c.ID]
	v.Heading = communityName(pc, c, cv, onPage)
	v.Platform = pc.get(c.PlatformName)
	switch d.Action {
	case ActionOpenInBrowser:
		v.OpenInBrowser = copyText(pc.snap.Settings, pc.locale, site.CopyOpenInBrowser, pc.text)
		v.Lede = pc.text.OpenInBrowserLede
		v.CopyURL = pc.baseURL + site.PathGo + d.Slug
	case ActionQQGroup:
		fillQQGroup(pc, &v, c, cv, onPage)
	case ActionQRCode:
		fillQRCode(pc, &v, c, cv, onPage)
	default:
		fillUnavailable(pc, &v, c)
	}
	return v
}

func notFoundLede(pc pageContext) string {
	if s := pc.get(pc.snap.Settings.NotFound); s != "" {
		return s
	}
	return pc.text.NotFoundLede
}

func communityName(pc pageContext, c CommunityTarget, cv site.CommunityView, onPage bool) string {
	if onPage {
		if s := pc.get(cv.Name); s != "" {
			return s
		}
	}
	if s := pc.get(c.Name); s != "" {
		return s
	}
	return c.Slug
}

func fillQQGroup(pc pageContext, v *pageView, c CommunityTarget, cv site.CommunityView, onPage bool) {
	v.QQNumber = c.QQGroupNumber
	if onPage && cv.QQ != nil {
		v.QQNumber = cv.QQ.GroupNumber
	}
	v.QQHint = pc.text.QQSearchHint
	if v.QR = qrFor(pc, c, cv, onPage); v.QR != nil {
		v.QQHint = pc.text.QQInWeChatHint
	}
	v.Contact = contactFor(pc, c, cv, onPage, false)
}

func fillQRCode(pc pageContext, v *pageView, c CommunityTarget, cv site.CommunityView, onPage bool) {
	v.QR = qrFor(pc, c, cv, onPage)
	v.ShowNoQR = v.QR == nil
	v.Contact = contactFor(pc, c, cv, onPage, true)
}

func fillUnavailable(pc pageContext, v *pageView, c CommunityTarget) {
	v.Lede = copyText(pc.snap.Settings, pc.locale, site.CopyInviteUnavailable, pc.text)
	v.Others = otherCommunities(pc, c.ID)
}

// qrFor prefers the page snapshot (real dimensions and note) and falls
// back to the community's QR id.
func qrFor(pc pageContext, c CommunityTarget, cv site.CommunityView, onPage bool) *qrView {
	if onPage && cv.QR != nil {
		return &qrView{URL: cv.QR.URL, Width: cv.QR.Width, Height: cv.QR.Height, Note: pc.get(cv.QR.Note)}
	}
	if !c.HasQR || c.QRID == "" {
		return nil
	}
	return &qrView{URL: site.PathQR + c.QRID, Width: defaultQRSide, Height: defaultQRSide}
}

func contactFor(pc pageContext, c CommunityTarget, cv site.CommunityView, onPage, wechat bool) *contactView {
	src := c.Contact
	if onPage && cv.Contact != nil {
		src = cv.Contact
	}
	if src == nil || src.Value == "" {
		return nil
	}
	out := &contactView{Label: pc.get(src.Label), Value: src.Value, Button: pc.text.Copy}
	if out.Label == "" {
		out.Label = pc.text.ContactLabel
	}
	if wechat {
		out.Button = pc.text.CopyContact
	}
	return out
}

// otherCommunities lists the page's other joinable communities in block
// order (unavailable ones are skipped).
func otherCommunities(pc pageContext, excludeID string) []otherView {
	page := pc.snap.Public
	seen := map[string]bool{excludeID: true}
	var out []otherView
	for _, b := range page.Blocks {
		if b.Kind != site.BlockCommunity || seen[b.CommunityID] {
			continue
		}
		seen[b.CommunityID] = true
		cv, ok := page.Communities[b.CommunityID]
		if !ok || cv.Live.State == provider.StateUnavailable {
			continue
		}
		href := cv.SharePath
		if cv.Live.JoinURL != nil {
			href = *cv.Live.JoinURL
		}
		out = append(out, otherView{
			Name:     pc.get(cv.Name),
			Platform: pc.get(page.Platforms[cv.Platform].Name),
			Href:     href,
		})
	}
	return out
}
