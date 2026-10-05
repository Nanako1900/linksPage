package media

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"maps"
	"sync"

	"golang.org/x/sync/singleflight"
)

// maxStoredRead bounds reads of our own stored files.
const maxStoredRead = 32 << 20

// Twitter card types for the generated head.
const (
	TwitterSummary      = "summary"
	TwitterSummaryLarge = "summary_large_image"
)

// GenerateInput is everything the site file generator needs. It is
// computed by the page builder from the current settings.
type GenerateInput struct {
	// AvatarKey is the site avatar media key ("" when none).
	AvatarKey string
	// OGImageKey is an uploaded OG image key ("" → generate).
	OGImageKey string
	// FaviconKey is an uploaded favicon key ("" → derive from the avatar,
	// else a geometric mark in AccentHex).
	FaviconKey string
	// BackgroundHex and AccentHex are normalized #rrggbb theme colors
	// ("" → Signal Paper defaults).
	BackgroundHex string
	AccentHex     string
	// Name and ShortName go into site.webmanifest.
	Name, ShortName string
	// ThemeColorHex is the manifest theme_color ("" → BackgroundHex).
	ThemeColorHex string
	// Index selects robots.txt "Allow: /" (true) or "Disallow: /".
	Index bool
	// BaseURL is config base_url (robots.txt has no Sitemap in M1).
	BaseURL string
}

// Generated holds the generated assets. Images are stored in the media
// store (content-addressed, served by /media/u); small text files are kept
// in memory and served by SiteFilesHandler.
type Generated struct {
	// OGImage is the 1200×630 JPEG (uploaded or generated, no text). It is
	// the zero value when there is neither an uploaded OG image nor an
	// avatar; the head then uses TwitterCard() == "summary" and may fall
	// back to Favicons[512].
	OGImage Stored
	// Favicons maps size (32, 180, 192, 512) → PNG.
	Favicons map[int]Stored
	Files    SiteFiles
	// Warnings lists referenced media that could not be used (missing
	// file) and were replaced by a fallback; the caller should log them.
	Warnings []string
}

// TwitterCard is "summary_large_image" when there is a wide OG image and
// "summary" otherwise.
func (g Generated) TwitterCard() string {
	if g.OGImage.Key == "" {
		return TwitterSummary
	}
	return TwitterSummaryLarge
}

// StoredImages returns every image in g with its media kind, so the caller
// can insert the media rows (InsertMedia is ON CONFLICT DO NOTHING).
func (g Generated) StoredImages() map[Kind][]Stored {
	out := map[Kind][]Stored{KindFavicon: make([]Stored, 0, len(g.Favicons))}
	for _, size := range FaviconSizes {
		if s, ok := g.Favicons[size]; ok {
			out[KindFavicon] = append(out[KindFavicon], s)
		}
	}
	if g.OGImage.Key != "" {
		out[KindOG] = []Stored{g.OGImage}
	}
	return out
}

// clone returns a deep copy so callers cannot mutate the cache.
func (g Generated) clone() Generated {
	return Generated{
		OGImage:  g.OGImage,
		Favicons: maps.Clone(g.Favicons),
		Files: SiteFiles{
			FaviconICO: bytes.Clone(g.Files.FaviconICO),
			Manifest:   bytes.Clone(g.Files.Manifest),
			RobotsTxt:  bytes.Clone(g.Files.RobotsTxt),
			ETag:       g.Files.ETag,
		},
		Warnings: append([]string(nil), g.Warnings...),
	}
}

// Generator produces Generated. The last result is cached by input so
// rebuilding the page does not re-encode images; concurrent calls with the
// same input share one generation.
type Generator struct {
	store Store
	sem   *Semaphore

	group singleflight.Group
	mu    sync.Mutex
	key   string
	last  Generated
}

// NewGenerator returns a generator writing images to store.
func NewGenerator(store Store, sem *Semaphore) *Generator {
	return &Generator{store: store, sem: sem}
}

// Generate builds (or returns cached) assets for in. The caller inserts
// media rows for stored images (kind og / favicon, see StoredImages).
func (g *Generator) Generate(ctx context.Context, in GenerateInput) (Generated, error) {
	if g.store == nil || g.sem == nil {
		return Generated{}, errors.New("media: generator requires a store and a semaphore")
	}
	colors, err := resolvePalette(in)
	if err != nil {
		return Generated{}, err
	}
	key, err := inputKey(in)
	if err != nil {
		return Generated{}, err
	}
	if cached, ok := g.cached(key); ok {
		return cached, nil
	}
	v, err, _ := g.group.Do(key, func() (any, error) {
		out, err := g.generate(ctx, in, colors)
		if err != nil {
			return Generated{}, err
		}
		g.mu.Lock()
		g.key, g.last = key, out
		g.mu.Unlock()
		return out, nil
	})
	if err != nil {
		return Generated{}, err
	}
	out, _ := v.(Generated)
	return out.clone(), nil
}

func (g *Generator) cached(key string) (Generated, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.key != key {
		return Generated{}, false
	}
	return g.last.clone(), true
}

func inputKey(in GenerateInput) (string, error) {
	b, err := json.Marshal(in)
	if err != nil {
		return "", fmt.Errorf("media: hash generate input: %w", err)
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

func (g *Generator) generate(ctx context.Context, in GenerateInput, colors palette) (Generated, error) {
	var out Generated
	favicons, err := g.favicons(ctx, in, colors, &out.Warnings)
	if err != nil {
		return Generated{}, err
	}
	out.Favicons = favicons.stored
	if out.OGImage, err = g.ogImage(ctx, in, colors, &out.Warnings); err != nil {
		return Generated{}, err
	}
	manifest, err := buildManifest(in, colors, out.Favicons)
	if err != nil {
		return Generated{}, err
	}
	out.Files = newSiteFiles(favicons.ico, manifest, buildRobots(in.Index))
	return out, nil
}

type faviconSet struct {
	stored map[int]Stored
	ico    []byte
}

// favicons renders from the uploaded favicon, else the avatar, else the
// geometric mark.
func (g *Generator) favicons(ctx context.Context, in GenerateInput, colors palette, warnings *[]string) (faviconSet, error) {
	var rendered map[int]encoded
	render := func(img *image.NRGBA) error {
		var err error
		rendered, err = renderFavicons(img, colors)
		return err
	}
	done := false
	for _, key := range []string{in.FaviconKey, in.AvatarKey} {
		if key == "" {
			continue
		}
		ok, err := g.withImage(ctx, key, ImageWeight(DefaultFaviconSide, DefaultFaviconSide), render, warnings)
		if err != nil {
			return faviconSet{}, err
		}
		if ok {
			done = true
			break
		}
	}
	if !done {
		if err := g.withWeight(ctx, ImageWeight(DefaultFaviconSide, DefaultFaviconSide), func() error { return render(nil) }); err != nil {
			return faviconSet{}, err
		}
	}
	set := faviconSet{stored: make(map[int]Stored, len(rendered)), ico: rendered[FaviconICOSize].data}
	for size, e := range rendered {
		s, err := put(ctx, g.store, e)
		if err != nil {
			return faviconSet{}, err
		}
		set.stored[size] = s
	}
	return set, nil
}

// ogImage returns the uploaded OG image, else the avatar composition, else
// the zero Stored (summary card).
func (g *Generator) ogImage(ctx context.Context, in GenerateInput, colors palette, warnings *[]string) (Stored, error) {
	if in.OGImageKey != "" {
		s, err := g.describe(ctx, in.OGImageKey)
		if err == nil {
			return s, nil
		}
		if !errors.Is(err, ErrNotFound) {
			return Stored{}, err
		}
		*warnings = append(*warnings, "og image "+in.OGImageKey+" not found; using the generated image")
	}
	if in.AvatarKey == "" {
		return Stored{}, nil
	}
	var og encoded
	ok, err := g.withImage(ctx, in.AvatarKey, ImageWeight(OGWidth, OGHeight), func(img *image.NRGBA) error {
		var err error
		og, err = encodeJPEG(composeOG(img, colors), JPEGQuality)
		return err
	}, warnings)
	if err != nil || !ok {
		return Stored{}, err
	}
	return put(ctx, g.store, og)
}

// describe reads a stored file's header without decoding its pixels.
func (g *Generator) describe(ctx context.Context, key string) (Stored, error) {
	data, err := readStored(ctx, g.store, key, maxStoredRead)
	if err != nil {
		return Stored{}, err
	}
	h, err := inspect(data)
	if err != nil {
		return Stored{}, fmt.Errorf("media: stored %s: %w", key, err)
	}
	w, ht := h.displaySize()
	return Stored{Key: key, ContentType: ContentTypeForKey(key), Bytes: len(data), Width: w, Height: ht}, nil
}

// withImage decodes the stored image key under the semaphore (plus extra
// bytes for the output) and calls fn. A missing file is reported as a
// warning with ok=false so the caller can fall back.
func (g *Generator) withImage(ctx context.Context, key string, extra int64, fn func(*image.NRGBA) error, warnings *[]string) (bool, error) {
	data, err := readStored(ctx, g.store, key, maxStoredRead)
	if errors.Is(err, ErrNotFound) {
		*warnings = append(*warnings, "image "+key+" not found; using a fallback")
		return false, nil
	}
	if err != nil {
		return false, err
	}
	h, err := inspect(data)
	if err != nil {
		return false, fmt.Errorf("media: stored %s: %w", key, err)
	}
	err = g.withWeight(ctx, ImageWeight(h.width, h.height)+extra, func() error {
		img, err := decodePixels(h, data)
		if err != nil {
			return fmt.Errorf("media: stored %s: %w", key, err)
		}
		return fn(img)
	})
	return err == nil, err
}

func (g *Generator) withWeight(ctx context.Context, weight int64, fn func() error) error {
	release, err := g.sem.Acquire(ctx, weight, AcquireWait)
	if err != nil {
		return err
	}
	defer release()
	return fn()
}
