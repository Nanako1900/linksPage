package site

import (
	"context"
	"fmt"
	"maps"

	"github.com/Nanako1900/linksPage/internal/content"
	"github.com/Nanako1900/linksPage/internal/media"
	"github.com/Nanako1900/linksPage/internal/store/dbq"
)

// MaxShortNameRunes bounds site.webmanifest short_name.
const MaxShortNameRunes = 12

// headMeta derives the server-only <head> metadata (assets added later).
func headMeta(s Settings) HeadMeta {
	h := HeadMeta{
		OGTitle:       cloneText(s.OG.Title),
		OGDescription: cloneText(s.OG.Description),
		Robots:        s.SearchIndexing,
		Icons:         map[int]string{},
	}
	if len(h.OGTitle) == 0 {
		h.OGTitle = cloneText(s.Title)
	}
	if len(h.OGDescription) == 0 {
		h.OGDescription = cloneText(s.Description)
	}
	return h
}

// generateInput maps settings onto the asset generator input. Keys whose
// media rows are missing are dropped so the generator falls back.
func generateInput(in *buildInput) media.GenerateInput {
	s := in.settings
	name := s.Title.Get(s.DefaultLocale, s.DefaultLocale)
	short := s.DisplayName.Get(s.DefaultLocale, s.DefaultLocale)
	if short == "" {
		short = name
	}
	// Settings were validated (hex colors only), so normalization cannot
	// fail; an empty value makes the generator use its defaults.
	bg, _ := NormalizeHex(s.Theme.Light.Bg)
	accent, _ := NormalizeHex(s.Theme.Light.Accent)
	out := media.GenerateInput{
		BackgroundHex: bg,
		AccentHex:     accent,
		Name:          content.CleanText(name, 0),
		ShortName:     content.CleanText(short, MaxShortNameRunes),
		ThemeColorHex: s.Theme.ThemeColor(false),
		Index:         s.SearchIndexing != SearchNoIndex,
		BaseURL:       in.baseURL,
	}
	if _, ok := in.media[s.AvatarKey]; ok {
		out.AvatarKey = s.AvatarKey
	}
	if _, ok := in.media[s.OG.ImageKey]; ok {
		out.OGImageKey = s.OG.ImageKey
	}
	return out
}

// generateAssets runs the generator and records the media rows of newly
// stored images. On error the snapshot is published without assets and
// Rebuild keeps the previous ones.
func (b *Builder) generateAssets(ctx context.Context, in *buildInput, head *HeadMeta) (*media.SiteFiles, error) {
	gen, err := b.deps.Assets.Generate(ctx, generateInput(in))
	if err != nil {
		return nil, fmt.Errorf("generate site assets: %w", err)
	}
	if err := b.recordMedia(ctx, in.media, gen.OGImage, media.KindOG); err != nil {
		return nil, err
	}
	icons := make(map[int]string, len(gen.Favicons))
	for size, st := range gen.Favicons {
		if err := b.recordMedia(ctx, in.media, st, media.KindFavicon); err != nil {
			return nil, err
		}
		icons[size] = st.URL()
	}
	head.Icons = icons
	if gen.OGImage.Key != "" {
		head.OGImage = &ImageView{URL: gen.OGImage.URL(), Width: gen.OGImage.Width, Height: gen.OGImage.Height}
	}
	files := gen.Files
	return &files, nil
}

// recordMedia inserts the media row for a generated image once per process
// (the insert itself is idempotent).
func (b *Builder) recordMedia(ctx context.Context, known map[string]dbq.Medium, st media.Stored, kind media.Kind) error {
	if st.Key == "" {
		return nil
	}
	if _, ok := known[st.Key]; ok {
		return nil
	}
	if _, ok := b.recorded.Load(st.Key); ok {
		return nil
	}
	err := b.deps.Queries.InsertMedia(ctx, dbq.InsertMediaParams{
		Key: st.Key, Kind: string(kind), ContentType: st.ContentType,
		Bytes: int32(st.Bytes), Width: int32(st.Width), Height: int32(st.Height), //nolint:gosec // bounded by media limits
		Variants: []byte("{}"),
	})
	if err != nil {
		return fmt.Errorf("record generated media %s: %w", st.Key, err)
	}
	b.recorded.Store(st.Key, true)
	return nil
}

// carryAssets copies the previous snapshot's assets into next when
// generation failed for next.
func carryAssets(next, prev *Snapshot) {
	if next.Files != nil || prev == nil || prev.Files == nil {
		return
	}
	next.Files = prev.Files
	next.Head.Icons = maps.Clone(prev.Head.Icons)
	if next.Head.OGImage == nil && prev.Head.OGImage != nil {
		img := *prev.Head.OGImage
		next.Head.OGImage = &img
	}
}
