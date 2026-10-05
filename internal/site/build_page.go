package site

import (
	"fmt"
	"log/slog"
	"maps"
	"strings"

	"github.com/Nanako1900/linksPage/internal/content"
	"github.com/Nanako1900/linksPage/internal/provider"
	"github.com/Nanako1900/linksPage/internal/store/dbq"
)

// warnFunc logs a problem with stored content once per key.
type warnFunc func(key, msg string, attrs ...any)

// assembler turns a buildInput into a PublicPage. Invalid rows are
// reported through warn and skipped, together with the blocks that
// reference them, so one bad row never takes the page down.
type assembler struct {
	in   *buildInput
	live provider.LiveSource
	warn warnFunc
	page *PublicPage
	// skipped remembers community/link ids that failed to build.
	skipped map[string]bool
}

// assemble builds the public page (Revision included).
func assemble(in *buildInput, live provider.LiveSource, warn warnFunc) (*PublicPage, error) {
	a := &assembler{in: in, live: live, warn: warn, skipped: map[string]bool{}}
	a.page = EmptyPublicPage(in.version, in.page, in.settings, in.baseURL, in.now)
	a.page.Site.Avatar = mediaImage(in.media, in.settings.AvatarKey)
	a.page.NextBoundary = in.nextBoundary
	showCount := a.addBlocks()
	applyHeadingCounts(a.page.Blocks, showCount)
	rev, err := computeRevision(a.page)
	if err != nil {
		return nil, err
	}
	a.page.Revision = rev
	return a.page, nil
}

// addBlocks appends every buildable block and returns, per appended block,
// whether it is a heading with showCount.
func (a *assembler) addBlocks() []bool {
	var showCount []bool
	for _, row := range a.in.blocks {
		b, count, err := a.block(row)
		if err != nil {
			a.warn("block:"+row.ID.String(), "skipping page block", slog.String("id", row.ID.String()), slog.Any("error", err))
			continue
		}
		a.page.Blocks = append(a.page.Blocks, b)
		showCount = append(showCount, count)
	}
	return showCount
}

func (a *assembler) block(row dbq.ListVisibleBlocksRow) (BlockView, bool, error) {
	b := BlockView{ID: row.ID.String(), Kind: BlockKind(row.Kind)}
	switch b.Kind {
	case BlockCommunity:
		b.CommunityID = row.CommunityID.String()
		return b, false, a.addCommunity(b.CommunityID)
	case BlockLink:
		b.LinkID = row.LinkID.String()
		return b, false, a.addLink(b.LinkID)
	case BlockHeading:
		var d HeadingBlockData
		if err := decodeBlockData(row.Data, &d); err != nil || len(d.Text) == 0 {
			return b, false, fmt.Errorf("heading data: %w", orEmpty(err, "text"))
		}
		b.Text = maps.Clone(d.Text)
		return b, d.ShowCount, nil
	case BlockText:
		return a.textBlock(b, row.Data)
	case BlockSocialRow:
		return a.socialRow(b, row.Data)
	}
	return b, false, fmt.Errorf("unknown block kind %q", row.Kind)
}

func (a *assembler) textBlock(b BlockView, data []byte) (BlockView, bool, error) {
	var d TextBlockData
	if err := decodeBlockData(data, &d); err != nil || len(d.Markdown) == 0 {
		return b, false, fmt.Errorf("text data: %w", orEmpty(err, "markdown"))
	}
	for l, v := range d.Markdown {
		if len(v) > content.MaxMarkdownBytes || !localeRe.MatchString(l) {
			return b, false, fmt.Errorf("text data: markdown.%s too long or invalid locale", l)
		}
	}
	b.Markdown = maps.Clone(d.Markdown)
	return b, false, nil
}

func (a *assembler) socialRow(b BlockView, data []byte) (BlockView, bool, error) {
	var d SocialRowBlockData
	if err := decodeBlockData(data, &d); err != nil {
		return b, false, fmt.Errorf("social_row data: %w", err)
	}
	ids := []string{}
	for _, id := range d.LinkIDs {
		if err := a.addLink(id); err == nil {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return b, false, fmt.Errorf("social_row data: %w", errEmpty("linkIds"))
	}
	b.LinkIDs = ids
	return b, false, nil
}

func decodeBlockData(data []byte, v any) error {
	if len(data) == 0 {
		return errEmpty("data")
	}
	return DecodeStrict(data, v)
}

type emptyError string

func (e emptyError) Error() string { return string(e) + " is empty" }

func errEmpty(field string) error { return emptyError(field) }

func orEmpty(err error, field string) error {
	if err != nil {
		return err
	}
	return errEmpty(field)
}

// addCommunity builds and validates a referenced community once.
func (a *assembler) addCommunity(id string) error {
	if _, ok := a.page.Communities[id]; ok {
		return nil
	}
	if a.skipped[id] {
		return fmt.Errorf("community %s is invalid", id)
	}
	v, pv, err := a.community(id)
	if err == nil {
		err = a.validateWithPlatform(id, v, pv)
	}
	if err != nil {
		a.skipped[id] = true
		a.warn("community:"+id, "skipping community", slog.String("id", id), slog.Any("error", err))
		return fmt.Errorf("community %s is invalid", id)
	}
	a.page.Communities[id] = v
	a.page.Platforms[pv.ID] = pv
	return nil
}

func (a *assembler) validateWithPlatform(id string, v CommunityView, pv PlatformView) error {
	_, had := a.page.Platforms[pv.ID]
	a.page.Platforms[pv.ID] = pv
	err := a.page.validateCommunity(id, v)
	if !had {
		delete(a.page.Platforms, pv.ID)
	}
	if err == nil && (len(pv.Name) == 0 || validateIcon(pv.Icon) != nil) {
		err = fmt.Errorf("platform %s: invalid name or icon", pv.ID)
	}
	return err
}

func (a *assembler) community(id string) (CommunityView, PlatformView, error) {
	row, ok := a.in.communities[id]
	if !ok {
		return CommunityView{}, PlatformView{}, fmt.Errorf("community %s not found", id)
	}
	display, err := decodeDisplay(row.Display)
	if err != nil {
		return CommunityView{}, PlatformView{}, err
	}
	platform, ok := a.in.catalog.Get(row.Platform)
	if !ok {
		return CommunityView{}, PlatformView{}, fmt.Errorf("unknown platform %q", row.Platform)
	}
	if want := platformProvider(platform); want != row.Provider {
		return CommunityView{}, PlatformView{}, fmt.Errorf("provider %q does not match platform %q", row.Provider, platform.ID)
	}
	var snapRow *dbq.ProviderSnapshot
	if s, ok := a.in.snapshots[id]; ok {
		snapRow = &s
	}
	snap, err := pickSnapshot(id, snapRow, a.live)
	if err != nil {
		return CommunityView{}, PlatformView{}, err
	}
	in := communityInput{row: row, display: display, platform: platform, snap: snap, now: a.in.now}
	if q, ok := a.in.qrCodes[id]; ok {
		in.qr = &q
	}
	if row.IconKey != nil {
		if m, ok := a.in.media[*row.IconKey]; ok {
			in.icon = &m
		}
	}
	return buildCommunity(in), a.platformView(platform), nil
}

// platformProvider maps a platform to communities.provider.
func platformProvider(p provider.Platform) string {
	if p.Provider == "" {
		return "static"
	}
	return p.Provider
}

func (a *assembler) platformView(p provider.Platform) PlatformView {
	icon := iconFromRef(p.Icon)
	if key, ok := a.in.platformIcons[p.ID]; ok && icon == nil {
		icon = mediaIcon(key)
	}
	return PlatformView{ID: p.ID, Name: LocalizedText(maps.Clone(p.Name)), Icon: icon, NeedsExternalBrowser: p.NeedsExternalBrowser}
}

// addLink builds and validates a referenced link once.
func (a *assembler) addLink(id string) error {
	if _, ok := a.page.Links[id]; ok {
		return nil
	}
	if a.skipped[id] {
		return fmt.Errorf("link %s is invalid", id)
	}
	v, err := a.link(id)
	if err == nil {
		err = validateLink(id, v)
	}
	if err != nil {
		a.skipped[id] = true
		a.warn("link:"+id, "skipping link", slog.String("id", id), slog.Any("error", err))
		return fmt.Errorf("link %s is invalid", id)
	}
	a.page.Links[id] = v
	return nil
}

func (a *assembler) link(id string) (LinkView, error) {
	row, ok := a.in.links[id]
	if !ok {
		return LinkView{}, fmt.Errorf("link %s not found", id)
	}
	var label LocalizedText
	if err := DecodeStrict(row.Label, &label); err != nil {
		return LinkView{}, fmt.Errorf("label: %w", err)
	}
	if _, err := content.SafeURL(row.Url); err != nil {
		return LinkView{}, fmt.Errorf("url: %w", err)
	}
	v := LinkView{ID: id, Slug: row.Slug, Kind: row.Kind, Label: label, URL: row.Url, Href: PathGo + row.Slug, RelMe: row.RelMe}
	if row.RelMe {
		v.Href = row.Url
	}
	switch {
	case row.Icon != nil:
		v.Icon = iconFromRef(*row.Icon)
	case row.IconKey != nil:
		v.Icon = mediaIcon(*row.IconKey)
	}
	return v, nil
}

// iconFromRef converts "si:<slug>" / "builtin:<name>" (nil otherwise).
func iconFromRef(ref string) *IconView {
	kind, name, ok := strings.Cut(ref, ":")
	if !ok || !iconNameRe.MatchString(name) {
		return nil
	}
	switch kind {
	case "si":
		return &IconView{Kind: IconSimple, Name: name}
	case "builtin":
		return &IconView{Kind: IconBuiltin, Name: name}
	}
	return nil
}

func mediaIcon(key string) *IconView {
	if !MediaKeyRe.MatchString(key) {
		return nil
	}
	u := PathUploads + key
	return &IconView{Kind: IconMedia, Name: key, URL: &u}
}

func mediaImage(media map[string]dbq.Medium, key string) *ImageView {
	m, ok := media[key]
	if key == "" || !ok {
		return nil
	}
	return &ImageView{URL: PathUploads + m.Key, Width: int(m.Width), Height: int(m.Height)}
}

// applyHeadingCounts sets Count on showCount headings: the number of
// community blocks up to the next heading.
func applyHeadingCounts(blocks []BlockView, showCount []bool) {
	for i := range blocks {
		if blocks[i].Kind != BlockHeading || !showCount[i] {
			continue
		}
		n := 0
		for _, b := range blocks[i+1:] {
			if b.Kind == BlockHeading {
				break
			}
			if b.Kind == BlockCommunity {
				n++
			}
		}
		blocks[i].Count = &n
	}
}
