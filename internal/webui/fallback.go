package webui

import (
	"fmt"
	"html/template"

	"github.com/Nanako1900/linksPage/internal/site"
)

// fallbackView is the server-rendered #root markup (template "fallback").
// No-JS visitors and crawlers see real content; React replaces it.
type fallbackView struct {
	Code          string
	Identity      *identityView
	Heading, Lede string
	Note          string
	PinnedLabel   string
	Pinned        *cardView
	Blocks        []blockView
	NoScript      string
	LinkText      string
	LinkHref      string
	Footer        template.HTML
	PoweredBy     string
	PoweredByURL  string
}

type identityView struct {
	Avatar *site.ImageView
	Name   string
	Bio    template.HTML
}

// blockView is one block; exactly one of the kind-specific fields is set.
type blockView struct {
	Heading  string
	Count    string
	Markdown template.HTML
	Link     *linkView
	Social   []linkView
	Card     *cardView
}

type linkView struct{ Label, Href, Rel string }

func (rd *Renderer) fallbackView(pc pageCtx) fallbackView {
	s := pc.snap.Settings
	fv := fallbackView{NoScript: pc.txt.NoScript}
	switch pc.kind {
	case PageNotFound:
		return fallbackView{
			Code: "404", Heading: pc.txt.NotFoundTitle,
			Lede:     firstNonEmpty(pc.text(s.NotFound), pc.txt.NotFoundLede),
			LinkText: pc.txt.BackHome, LinkHref: homeHref(pc),
		}
	case PagePrivacy:
		fv.Heading, fv.Lede = pc.txt.PrivacyTitle, pc.text(s.Description)
		fv.Note, fv.LinkText, fv.LinkHref = pc.txt.Loading, pc.txt.BackHome, homeHref(pc)
	default:
		fv.Identity = rd.identity(pc)
		fv.Blocks = rd.blocks(pc)
		if pc.kind == PageCommunity {
			fv.Pinned, fv.PinnedLabel = cardFor(pc, *pc.community), pc.txt.CommunityPinned
		}
		if len(fv.Blocks) == 0 && fv.Pinned == nil {
			fv.Note = pc.txt.Loading
		}
		fv.Footer = rd.markdown(pc.text(pc.pub.Site.Footer))
		if pc.pub.Site.ShowPoweredBy {
			fv.PoweredBy, fv.PoweredByURL = pc.txt.PoweredBy, poweredByURL
		}
	}
	if rd.opts.Assets.Fallback() {
		fv.Note = pc.txt.MissingBuild
	}
	return fv
}

// homeHref keeps a non-default locale when linking back home.
func homeHref(pc pageCtx) string {
	if pc.locale == pc.defaultLocale() {
		return "/"
	}
	return "/?lang=" + pc.locale
}

func (rd *Renderer) identity(pc pageCtx) *identityView {
	ps := pc.pub.Site
	return &identityView{
		Avatar: ps.Avatar,
		Name:   firstNonEmpty(pc.text(ps.DisplayName), pc.text(ps.Title)),
		Bio:    rd.markdown(pc.text(ps.Bio)),
	}
}

// blocks renders every visible block in order. On a community share page
// the pinned community is rendered first and skipped here.
func (rd *Renderer) blocks(pc pageCtx) []blockView {
	out := make([]blockView, 0, len(pc.pub.Blocks))
	for _, b := range pc.pub.Blocks {
		if v, ok := rd.block(pc, b); ok {
			out = append(out, v)
		}
	}
	return out
}

func (rd *Renderer) block(pc pageCtx, b site.BlockView) (blockView, bool) {
	switch b.Kind {
	case site.BlockHeading:
		v := blockView{Heading: pc.text(b.Text)}
		if b.Count != nil {
			v.Count = fmt.Sprintf("%02d", *b.Count)
		}
		return v, v.Heading != ""
	case site.BlockText:
		v := blockView{Markdown: rd.markdown(pc.text(b.Markdown))}
		return v, v.Markdown != ""
	case site.BlockLink:
		l, ok := pc.pub.Links[b.LinkID]
		if !ok {
			return blockView{}, false
		}
		lv := linkFor(pc, l)
		return blockView{Link: &lv}, true
	case site.BlockSocialRow:
		v := blockView{Social: socialLinks(pc, b.LinkIDs)}
		return v, len(v.Social) > 0
	case site.BlockCommunity:
		c, ok := pc.pub.Communities[b.CommunityID]
		if !ok || (pc.community != nil && c.ID == pc.community.ID) {
			return blockView{}, false
		}
		return blockView{Card: cardFor(pc, c)}, true
	}
	return blockView{}, false
}

func socialLinks(pc pageCtx, ids []string) []linkView {
	out := make([]linkView, 0, len(ids))
	for _, id := range ids {
		if l, ok := pc.pub.Links[id]; ok {
			out = append(out, linkFor(pc, l))
		}
	}
	return out
}

func linkFor(pc pageCtx, l site.LinkView) linkView {
	v := linkView{Label: firstNonEmpty(pc.text(l.Label), l.Slug), Href: l.Href}
	if l.RelMe {
		v.Rel = "me noopener"
	}
	return v
}
