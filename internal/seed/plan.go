package seed

import (
	"errors"
	"fmt"
	"maps"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Nanako1900/linksPage/internal/content"
	"github.com/Nanako1900/linksPage/internal/media"
	"github.com/Nanako1900/linksPage/internal/site"
	"github.com/Nanako1900/linksPage/internal/store/dbq"
)

// Text limits (runes) for seeded plain text.
const (
	MaxNameRunes      = 100
	MaxPlainTextRunes = site.MaxPlainTextRunes
	MaxContactRunes   = 100
	MaxBlocklistItems = 50
	MaxBlocklistRunes = 32
)

// plan is a fully validated seed, ready to be written. Image fields hold
// paths relative to the seed directory; they are replaced by media keys
// after ingestion.
type plan struct {
	settings    map[string]any // site_settings.data keys provided by the seed
	avatar      string
	ogImage     string
	platforms   []platformPlan
	communities []communityPlan
	links       []linkPlan
	blocks      []blockPlan
}

type platformPlan struct {
	params dbq.InsertCustomPlatformParams
	icon   string
}

type communityPlan struct {
	params  dbq.InsertCommunityParams
	icon    string
	qrImage string
	qrNote  site.LocalizedText
	// fetch is true for provider-backed communities (a pending snapshot row
	// is created so the first fetch happens right away).
	fetch bool
}

type linkPlan struct {
	params dbq.InsertLinkParams
	icon   string
}

type blockPlan struct {
	kind      site.BlockKind
	community string // slug
	link      string // slug
	heading   *site.HeadingBlockData
	text      *site.TextBlockData
	social    []string // link slugs
	visible   bool
	from, to  *time.Time
}

// imageRef is one image to ingest.
type imageRef struct {
	path  string
	kind  media.Kind
	field string // YAML path for error messages
}

func (r imageRef) id() string { return string(r.kind) + "\x00" + r.path }

// images lists every distinct image of the plan.
func (p *plan) images() []imageRef {
	var out []imageRef
	seen := map[string]bool{}
	add := func(path string, kind media.Kind, field string) {
		r := imageRef{path: path, kind: kind, field: field}
		if path == "" || seen[r.id()] {
			return
		}
		seen[r.id()] = true
		out = append(out, r)
	}
	add(p.avatar, media.KindAvatar, "site.avatar")
	add(p.ogImage, media.KindOG, "site.og.image")
	for i, pl := range p.platforms {
		add(pl.icon, media.KindIcon, fmt.Sprintf("platforms[%d].icon", i))
	}
	for i, c := range p.communities {
		add(c.icon, media.KindIcon, fmt.Sprintf("communities[%d].icon", i))
		add(c.qrImage, media.KindQR, fmt.Sprintf("communities[%d].qr.image", i))
	}
	for i, l := range p.links {
		add(l.icon, media.KindIcon, fmt.Sprintf("links[%d].icon", i))
	}
	return out
}

// problems collects validation errors with YAML paths.
type problems struct {
	list []error
}

func (p *problems) add(field, format string, args ...any) {
	p.list = append(p.list, fmt.Errorf("%s: %s", field, fmt.Sprintf(format, args...)))
}

func (p *problems) err() error {
	if len(p.list) == 0 {
		return nil
	}
	return fmt.Errorf("%w: %w", ErrInvalidSeed, errors.Join(p.list...))
}

// text validates locale keys, cleans values (bidi/zero-width/control
// characters) and enforces maxRunes; empty values are dropped.
func (p *problems) text(field string, t Text, maxRunes int, required bool) site.LocalizedText {
	out := site.LocalizedText{}
	for _, l := range slices.Sorted(maps.Keys(t)) {
		if !site.ValidLocale(l) {
			p.add(field, "invalid locale %q", l)
			continue
		}
		v := content.CleanText(t[l], 0)
		if utf8.RuneCountInString(v) > maxRunes {
			p.add(field+"."+l, "longer than %d characters", maxRunes)
			continue
		}
		if v != "" {
			out[l] = v
		}
	}
	if required && len(out) == 0 {
		p.add(field, "is required")
	}
	return out
}

// markdown validates Markdown subset texts (kept verbatim).
func (p *problems) markdown(field string, t Text, md *content.Markdown, required bool) site.LocalizedText {
	out := site.LocalizedText{}
	for _, l := range slices.Sorted(maps.Keys(t)) {
		v := strings.TrimSpace(t[l])
		switch {
		case !site.ValidLocale(l):
			p.add(field, "invalid locale %q", l)
		case v == "":
		default:
			if _, err := md.Render(v); err != nil {
				p.add(field+"."+l, "%v (limit %d bytes)", err, content.MaxMarkdownBytes)
				continue
			}
			out[l] = v
		}
	}
	if required && len(out) == 0 {
		p.add(field, "is required")
	}
	return out
}

// imageExts are the accepted image file extensions (SVG is rejected).
var imageExts = []string{".png", ".jpg", ".jpeg", ".webp"}

// imagePath validates a seed-relative image path.
func (p *problems) imagePath(field, raw string) string {
	if raw == "" {
		return ""
	}
	clean := filepath.FromSlash(raw)
	if !filepath.IsLocal(clean) || strings.Contains(raw, "\\") {
		p.add(field, "image path %q must be relative and stay inside the seed directory", raw)
		return ""
	}
	if !slices.Contains(imageExts, strings.ToLower(path.Ext(raw))) {
		p.add(field, "image %q must be .png, .jpg, .jpeg or .webp", raw)
		return ""
	}
	return clean
}
