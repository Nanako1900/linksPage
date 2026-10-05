package site

import (
	"errors"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"
)

// Patterns for public DTO invariants.
var (
	slugPatternRe  = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)
	uuidRe         = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	proxyPathRe    = regexp.MustCompile(`^/media/p/[A-Za-z0-9_-]{22}\.(png|jpg|webp|gif)$`)
	uploadPathRe   = regexp.MustCompile(`^/media/u/[0-9a-f]{32}\.(webp|png|jpg)$`)
	qrPathRe       = regexp.MustCompile(`^/media/q/[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	qqGroupRe      = regexp.MustCompile(`^\d{5,12}$`)
	iconNameRe     = regexp.MustCompile(`^[a-z0-9-]{1,64}$`)
	guildEmbedSrcR = regexp.MustCompile(`^https://discord\.com/widget\?id=\d{17,20}$`)
	publicURLRe    = regexp.MustCompile(`^(https?://|mailto:)`)
)

// Validate checks every invariant of the public DTO contract
// (docs/m1/contract.md). Builders call it in tests; it is not on the
// request path.
func (p *PublicPage) Validate() error {
	var errs []error
	if !slugPatternRe.MatchString(p.Page.Slug) || p.Version < 0 {
		errs = append(errs, errors.New("page: invalid slug or version"))
	}
	if p.Blocks == nil || p.Communities == nil || p.Links == nil || p.Platforms == nil {
		errs = append(errs, errors.New("blocks, communities, links and platforms must not be null"))
	}
	errs = append(errs, p.Site.validate()...)
	for i, b := range p.Blocks {
		if err := p.validateBlock(b); err != nil {
			errs = append(errs, fmt.Errorf("blocks[%d]: %w", i, err))
		}
	}
	for _, id := range slices.Sorted(maps.Keys(p.Communities)) {
		if err := p.validateCommunity(id, p.Communities[id]); err != nil {
			errs = append(errs, fmt.Errorf("communities[%s]: %w", id, err))
		}
	}
	for _, id := range slices.Sorted(maps.Keys(p.Links)) {
		if err := validateLink(id, p.Links[id]); err != nil {
			errs = append(errs, fmt.Errorf("links[%s]: %w", id, err))
		}
	}
	for _, id := range slices.Sorted(maps.Keys(p.Platforms)) {
		pl := p.Platforms[id]
		if pl.ID != id || len(pl.Name) == 0 || validateIcon(pl.Icon) != nil {
			errs = append(errs, fmt.Errorf("platforms[%s]: invalid id, name or icon", id))
		}
	}
	return errors.Join(errs...)
}

func (s PublicSite) validate() []error {
	settings := Default()
	settings.DefaultLocale, settings.Locales, settings.Appearance, settings.Theme = s.DefaultLocale, s.Locales, s.Appearance, s.Theme
	settings.Copy = s.Copy
	var errs []error
	if err := settings.Validate(); err != nil {
		errs = append(errs, fmt.Errorf("site: %w", err))
	}
	if !strings.HasPrefix(s.BaseURL, "http") || strings.HasSuffix(s.BaseURL, "/") {
		errs = append(errs, errors.New("site.baseUrl: must be an absolute origin without trailing slash"))
	}
	if s.Avatar != nil && validateImage(s.Avatar, uploadPathRe) != nil {
		errs = append(errs, errors.New("site.avatar: invalid image"))
	}
	for name, t := range map[string]LocalizedText{
		"title": s.Title, "description": s.Description,
		"displayName": s.DisplayName, "bio": s.Bio, "footer": s.Footer, "notFound": s.NotFound,
	} {
		if t == nil {
			errs = append(errs, fmt.Errorf("site.%s: must not be null", name))
		}
	}
	return errs
}

func (p *PublicPage) validateBlock(b BlockView) error {
	if !uuidRe.MatchString(b.ID) {
		return errors.New("invalid id")
	}
	has := map[string]bool{
		"communityId": b.CommunityID != "", "linkId": b.LinkID != "",
		"text": b.Text != nil, "count": b.Count != nil, "markdown": b.Markdown != nil, "linkIds": b.LinkIDs != nil,
	}
	allowed := map[BlockKind][]string{
		BlockCommunity: {"communityId"}, BlockLink: {"linkId"}, BlockHeading: {"text", "count"},
		BlockText: {"markdown"}, BlockSocialRow: {"linkIds"},
	}
	fields, ok := allowed[b.Kind]
	if !ok {
		return fmt.Errorf("unknown kind %q", b.Kind)
	}
	for f, set := range has {
		if set && !slices.Contains(fields, f) {
			return fmt.Errorf("field %s not allowed for kind %s", f, b.Kind)
		}
	}
	return p.validateBlockRefs(b)
}

func (p *PublicPage) validateBlockRefs(b BlockView) error {
	switch b.Kind {
	case BlockCommunity:
		if _, ok := p.Communities[b.CommunityID]; !ok {
			return errors.New("communityId not in communities")
		}
	case BlockLink:
		if _, ok := p.Links[b.LinkID]; !ok {
			return errors.New("linkId not in links")
		}
	case BlockHeading:
		if len(b.Text) == 0 || (b.Count != nil && *b.Count < 0) {
			return errors.New("heading needs text and a non-negative count")
		}
	case BlockText:
		if len(b.Markdown) == 0 {
			return errors.New("text needs markdown")
		}
	case BlockSocialRow:
		if len(b.LinkIDs) == 0 {
			return errors.New("social_row needs linkIds")
		}
		for _, id := range b.LinkIDs {
			if _, ok := p.Links[id]; !ok {
				return errors.New("linkIds entry not in links")
			}
		}
	}
	return nil
}

func validateImage(img *ImageView, pathRe *regexp.Regexp) error {
	if img.Width <= 0 || img.Height <= 0 {
		return errors.New("image needs positive width and height")
	}
	if !pathRe.MatchString(img.URL) {
		return fmt.Errorf("image url %q has the wrong form", img.URL)
	}
	return nil
}

func validateIcon(icon *IconView) error {
	if icon == nil {
		return nil
	}
	switch icon.Kind {
	case IconSimple, IconBuiltin:
		if icon.URL != nil || !iconNameRe.MatchString(icon.Name) {
			return errors.New("simple/builtin icon needs a name and a null url")
		}
	case IconMedia:
		if icon.URL == nil || *icon.URL != PathUploads+icon.Name || !uploadPathRe.MatchString(*icon.URL) {
			return errors.New("media icon url must be /media/u/{name}")
		}
	default:
		return fmt.Errorf("unknown icon kind %q", icon.Kind)
	}
	return nil
}

func validateLink(id string, l LinkView) error {
	switch {
	case l.ID != id || !slugPatternRe.MatchString(l.Slug):
		return errors.New("invalid id or slug")
	case l.Kind != "link" && l.Kind != "social":
		return errors.New("kind must be link or social")
	case len(l.Label) == 0 || !publicURLRe.MatchString(l.URL):
		return errors.New("label and an allowed url are required")
	case l.RelMe && l.Href != l.URL:
		return errors.New("relMe links must use url as href")
	case !l.RelMe && l.Href != PathGo+l.Slug:
		return errors.New("href must be /go/{slug}")
	}
	return validateIcon(l.Icon)
}
