package seed

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Nanako1900/linksPage/internal/content"
	"github.com/Nanako1900/linksPage/internal/provider"
	"github.com/Nanako1900/linksPage/internal/site"
	"github.com/Nanako1900/linksPage/internal/store/dbq"
)

// Link kinds.
const (
	LinkKindLink   = "link"
	LinkKindSocial = "social"
)

func planLinks(ls []LinkSeed, slugs slugSet, p *problems, out *plan) {
	for i, l := range ls {
		field := fmt.Sprintf("links[%d]", i)
		slugs.claim(p, field+".slug", l.Slug)
		kind := l.Kind
		if kind == "" {
			kind = LinkKindLink
		}
		if kind != LinkKindLink && kind != LinkKindSocial {
			p.add(field+".kind", "must be link or social")
		}
		label, err := json.Marshal(p.text(field+".label", l.Label, MaxNameRunes, true))
		if err != nil {
			p.add(field+".label", "%v", err)
		}
		url, err := content.SafeURL(l.URL)
		if err != nil {
			p.add(field+".url", "must be an http, https or mailto URL")
		}
		if l.RelMe && !strings.HasPrefix(url, "http") {
			p.add(field+".rel_me", "needs an http(s) url")
		}
		lp := linkPlan{params: dbq.InsertLinkParams{PageID: 1, Slug: l.Slug, Kind: kind, Label: label, Url: url, RelMe: l.RelMe}}
		switch {
		case provider.IconRefRe.MatchString(l.Icon):
			icon := l.Icon
			lp.params.Icon = &icon
		case l.Icon != "":
			lp.icon = p.imagePath(field+".icon", l.Icon)
		}
		out.links = append(out.links, lp)
	}
}

// MaxHeadingRunes bounds heading block text.
const MaxHeadingRunes = 100

// planBlocks validates the explicit layout, or builds the default one
// when blocks are omitted.
func planBlocks(f *File, md *content.Markdown, p *problems, out *plan) {
	if len(f.Blocks) == 0 {
		out.blocks = defaultBlocks(f)
		return
	}
	communities, links := map[string]bool{}, map[string]bool{}
	for _, c := range f.Communities {
		communities[c.Slug] = true
	}
	for _, l := range f.Links {
		links[l.Slug] = true
	}
	for i, b := range f.Blocks {
		bc := blockCtx{field: fmt.Sprintf("blocks[%d]", i), seed: b, p: p, md: md, communities: communities, links: links}
		if bp, ok := bc.plan(); ok {
			out.blocks = append(out.blocks, bp)
		}
	}
}

type blockCtx struct {
	field              string
	seed               BlockSeed
	p                  *problems
	md                 *content.Markdown
	communities, links map[string]bool
}

func (bc blockCtx) plan() (blockPlan, bool) {
	b := bc.seed
	set := 0
	for _, has := range []bool{b.Heading != nil, b.Community != "", b.Link != "", b.Text != nil, b.SocialRow != nil} {
		if has {
			set++
		}
	}
	if set != 1 {
		bc.p.add(bc.field, "needs exactly one of heading, community, link, text, social_row")
		return blockPlan{}, false
	}
	if b.ShowCount && b.Heading == nil {
		bc.p.add(bc.field+".show_count", "is only valid for headings")
	}
	bp := blockPlan{visible: b.Visible == nil || *b.Visible, from: b.VisibleFrom, to: b.VisibleTo}
	if b.VisibleFrom != nil && b.VisibleTo != nil && !b.VisibleFrom.Before(*b.VisibleTo) {
		bc.p.add(bc.field+".visible_to", "must be after visible_from")
	}
	bc.content(&bp)
	return bp, true
}

func (bc blockCtx) content(bp *blockPlan) {
	b, f, p := bc.seed, bc.field, bc.p
	switch {
	case b.Heading != nil:
		bp.kind = site.BlockHeading
		bp.heading = &site.HeadingBlockData{Text: p.text(f+".heading", b.Heading, MaxHeadingRunes, true), ShowCount: b.ShowCount}
	case b.Community != "":
		bp.kind, bp.community = site.BlockCommunity, b.Community
		if !bc.communities[b.Community] {
			p.add(f+".community", "unknown community %q", b.Community)
		}
	case b.Link != "":
		bp.kind, bp.link = site.BlockLink, b.Link
		if !bc.links[b.Link] {
			p.add(f+".link", "unknown link %q", b.Link)
		}
	case b.Text != nil:
		bp.kind = site.BlockText
		bp.text = &site.TextBlockData{Markdown: p.markdown(f+".text", b.Text, bc.md, true)}
	default:
		bp.kind, bp.social = site.BlockSocialRow, b.SocialRow
		if len(b.SocialRow) == 0 {
			p.add(f+".social_row", "needs at least one link")
		}
		for j, slug := range b.SocialRow {
			if !bc.links[slug] {
				p.add(fmt.Sprintf("%s.social_row[%d]", f, j), "unknown link %q", slug)
			}
		}
	}
}

// defaultBlocks: a "社区 / Communities" heading with count, every
// community in file order, every kind=link link, then one social_row with
// all kind=social links (omitted when there are none).
func defaultBlocks(f *File) []blockPlan {
	out := []blockPlan{{
		kind:    site.BlockHeading,
		heading: &site.HeadingBlockData{Text: site.LocalizedText{"zh-CN": "社区", "en": "Communities"}, ShowCount: true},
		visible: true,
	}}
	for _, c := range f.Communities {
		out = append(out, blockPlan{kind: site.BlockCommunity, community: c.Slug, visible: true})
	}
	var social []string
	for _, l := range f.Links {
		if l.Kind == LinkKindSocial {
			social = append(social, l.Slug)
			continue
		}
		out = append(out, blockPlan{kind: site.BlockLink, link: l.Slug, visible: true})
	}
	if len(social) > 0 {
		out = append(out, blockPlan{kind: site.BlockSocialRow, social: social, visible: true})
	}
	return out
}
