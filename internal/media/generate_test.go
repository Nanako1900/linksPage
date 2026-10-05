package media

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"strings"
	"sync"
	"testing"
	"time"
)

func ingestFixture(t *testing.T, store Store, kind Kind, img image.Image) string {
	t.Helper()
	p := NewProcessor(store, NewSemaphore(MemoryBudget))
	res, err := p.Ingest(context.Background(), bytes.NewReader(pngBytes(t, img)), ProcessOptions{Kind: kind})
	if err != nil {
		t.Fatal(err)
	}
	return res.Primary.Key
}

func solid(w, h int, c color.NRGBA) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = c.R, c.G, c.B, c.A
	}
	return img
}

func TestGenerate(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	avatar := ingestFixture(t, store, KindAvatar, solid(64, 64, color.NRGBA{R: 200, A: 255}))
	favicon := ingestFixture(t, store, KindFavicon, solid(64, 64, color.NRGBA{G: 200, A: 255}))
	og := ingestFixture(t, store, KindOG, patterned(300, 200))
	missing := "00000000000000000000000000000000.webp"

	tests := []struct {
		name         string
		in           GenerateInput
		card         string
		ogKey        string // "" = generated or none
		faviconRGB   *color.NRGBA
		warnings     int
		robotsPrefix string
	}{
		{"nothing", GenerateInput{Index: true}, TwitterSummary, "", nil, 0, "User-agent: *\nAllow"},
		{"avatar", GenerateInput{AvatarKey: avatar}, TwitterSummaryLarge, "", &color.NRGBA{R: 200, A: 255}, 0, "User-agent: *\nDisallow: /"},
		{"uploaded og and favicon", GenerateInput{AvatarKey: avatar, OGImageKey: og, FaviconKey: favicon}, TwitterSummaryLarge, og, &color.NRGBA{G: 200, A: 255}, 0, ""},
		{"missing og falls back", GenerateInput{AvatarKey: avatar, OGImageKey: missing}, TwitterSummaryLarge, "", nil, 1, ""},
		{"missing avatar", GenerateInput{AvatarKey: missing}, TwitterSummary, "", nil, 2, ""},
		{"missing favicon uses avatar", GenerateInput{AvatarKey: avatar, FaviconKey: missing}, TwitterSummaryLarge, "", &color.NRGBA{R: 200, A: 255}, 1, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			g := NewGenerator(store, NewSemaphore(MemoryBudget))
			got, err := g.Generate(context.Background(), tt.in)
			if err != nil {
				t.Fatal(err)
			}
			if got.TwitterCard() != tt.card {
				t.Errorf("card = %s", got.TwitterCard())
			}
			if len(got.Warnings) != tt.warnings {
				t.Errorf("warnings = %v", got.Warnings)
			}
			checkOG(t, store, got.OGImage, tt.card, tt.ogKey)
			checkFavicons(t, store, got, tt.faviconRGB)
			if tt.robotsPrefix != "" && !strings.HasPrefix(string(got.Files.RobotsTxt), tt.robotsPrefix) {
				t.Errorf("robots = %q", got.Files.RobotsTxt)
			}
			checkStoredImages(t, got)
		})
	}
}

func checkOG(t *testing.T, store Store, og Stored, card, wantKey string) {
	t.Helper()
	if card == TwitterSummary {
		if og != (Stored{}) {
			t.Fatalf("OG = %+v, want none", og)
		}
		return
	}
	if wantKey != "" && og.Key != wantKey {
		t.Fatalf("OG key = %s, want uploaded %s", og.Key, wantKey)
	}
	checkStored(t, store, og, "image/jpeg", OGWidth, OGHeight)
}

func checkFavicons(t *testing.T, store Store, got Generated, rgb *color.NRGBA) {
	t.Helper()
	if len(got.Favicons) != len(FaviconSizes) {
		t.Fatalf("favicons = %v", got.Favicons)
	}
	for _, size := range FaviconSizes {
		checkStored(t, store, got.Favicons[size], "image/png", size, size)
	}
	if !bytes.Equal(got.Files.FaviconICO, readKey(t, store, got.Favicons[FaviconICOSize].Key)) {
		t.Fatal("favicon.ico is not the 32px PNG")
	}
	if rgb != nil {
		img := decodeAny(t, got.Files.FaviconICO)
		c, _ := color.NRGBAModel.Convert(img.At(16, 16)).(color.NRGBA)
		if !near(c, *rgb) {
			t.Fatalf("favicon centre = %v, want %v", c, *rgb)
		}
	}
	var m webManifest
	if err := json.Unmarshal(got.Files.Manifest, &m); err != nil || len(m.Icons) != 2 {
		t.Fatalf("manifest %s: %v", got.Files.Manifest, err)
	}
	if m.Icons[0].Src != got.Favicons[192].URL() {
		t.Fatalf("manifest icon %s", m.Icons[0].Src)
	}
}

func checkStoredImages(t *testing.T, got Generated) {
	t.Helper()
	imgs := got.StoredImages()
	if len(imgs[KindFavicon]) != len(FaviconSizes) {
		t.Fatalf("StoredImages favicons = %v", imgs[KindFavicon])
	}
	if (len(imgs[KindOG]) == 1) != (got.OGImage.Key != "") {
		t.Fatalf("StoredImages og = %v", imgs[KindOG])
	}
}

func TestGeometricFavicon(t *testing.T) {
	t.Parallel()
	colors, err := resolvePalette(GenerateInput{BackgroundHex: "#ffffff", AccentHex: "#ff0000"})
	if err != nil {
		t.Fatal(err)
	}
	accent := color.NRGBA{R: 0xff, A: 0xff}
	bg := color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}
	img := geometricMark(64, colors, true)
	tests := []struct {
		name string
		x, y int
		want color.NRGBA
	}{
		{"centre dot", 32, 32, accent},
		{"ring", 32, 32 + 13, bg},
		{"outer field", 4, 32, accent},
		{"rounded corner", 0, 0, color.NRGBA{}},
	}
	for _, tt := range tests {
		if got := img.NRGBAAt(tt.x, tt.y); got != tt.want {
			t.Errorf("%s: %v, want %v", tt.name, got, tt.want)
		}
	}
	if sq := geometricMark(16, colors, false); sq.NRGBAAt(0, 0) != accent {
		t.Errorf("square corner = %v", sq.NRGBAAt(0, 0))
	}
	// The apple-touch icon is flattened: fully opaque.
	set, err := renderFavicons(nil, colors)
	if err != nil {
		t.Fatal(err)
	}
	apple := decodeAny(t, set[AppleTouchSize].data)
	if _, _, _, a := apple.At(0, 0).RGBA(); a != 0xffff {
		t.Errorf("apple-touch corner alpha = %d", a)
	}
}

func TestComposeOG(t *testing.T) {
	t.Parallel()
	colors, err := resolvePalette(GenerateInput{BackgroundHex: "#000000", AccentHex: "#00ff00"})
	if err != nil {
		t.Fatal(err)
	}
	img := composeOG(solid(10, 20, color.NRGBA{B: 0xff, A: 0xff}), colors)
	tests := []struct {
		name string
		x, y int
		want color.NRGBA
	}{
		{"background", 5, 5, color.NRGBA{A: 0xff}},
		{"ring", OGWidth/2 + ogAvatarRadius + 4, OGHeight / 2, color.NRGBA{G: 0xff, A: 0xff}},
		{"avatar", OGWidth / 2, OGHeight / 2, color.NRGBA{B: 0xff, A: 0xff}},
	}
	for _, tt := range tests {
		if got := img.NRGBAAt(tt.x, tt.y); !near(got, tt.want) {
			t.Errorf("%s: %v, want %v", tt.name, got, tt.want)
		}
	}
}

// near compares colours with a small tolerance for lossy codecs and
// resampling.
func near(a, b color.NRGBA) bool {
	const tol = 16
	d := func(x, y uint8) bool { return max(x, y)-min(x, y) <= tol }
	return d(a.R, b.R) && d(a.G, b.G) && d(a.B, b.B) && d(a.A, b.A)
}

func TestGenerateCache(t *testing.T) {
	t.Parallel()
	base := newTestStore(t)
	avatar := ingestFixture(t, base, KindAvatar, patterned(40, 40))
	store := &countingStore{Store: base}
	g := NewGenerator(store, NewSemaphore(MemoryBudget))
	in := GenerateInput{AvatarKey: avatar, Name: "A"}

	var wg sync.WaitGroup
	results := make([]Generated, 4)
	for i := range results {
		wg.Go(func() {
			r, err := g.Generate(context.Background(), in)
			if err != nil {
				t.Error(err)
			}
			results[i] = r
		})
	}
	wg.Wait()
	puts := store.count()
	if puts != len(FaviconSizes)+1 {
		t.Fatalf("puts = %d", puts)
	}
	again, err := g.Generate(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if store.count() != puts {
		t.Fatal("cached input re-generated")
	}
	if again.Files.ETag != results[0].Files.ETag {
		t.Fatal("cached result differs")
	}
	// Mutating a returned value must not leak into the cache.
	again.Favicons[32] = Stored{Key: "mutated"}
	again.Files.Manifest[0] = 'X'
	third, err := g.Generate(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if third.Favicons[32].Key == "mutated" || third.Files.Manifest[0] == 'X' {
		t.Fatal("cache was mutated through a returned value")
	}
	// A different input regenerates.
	if _, err := g.Generate(context.Background(), GenerateInput{AvatarKey: avatar, Name: "B"}); err != nil {
		t.Fatal(err)
	}
	if store.count() == puts {
		t.Fatal("changed input served from cache")
	}
}

func TestGenerateErrors(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	avatar := ingestFixture(t, store, KindAvatar, patterned(20, 20))
	og := ingestFixture(t, store, KindOG, patterned(20, 20))
	corrupt, err := store.Put(context.Background(), []byte("\x89PNG\r\n\x1a\ngarbage-garbage"), ExtPNG)
	if err != nil {
		t.Fatal(err)
	}
	busy := NewSemaphore(MemoryBudget)
	rel, err := busy.Acquire(context.Background(), MemoryBudget, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(rel)

	tests := []struct {
		name string
		g    *Generator
		in   GenerateInput
		want error
	}{
		{"nil store", NewGenerator(nil, NewSemaphore(1)), GenerateInput{}, errAny},
		{"bad background", NewGenerator(store, NewSemaphore(MemoryBudget)), GenerateInput{BackgroundHex: "red"}, errAny},
		{"bad accent", NewGenerator(store, NewSemaphore(MemoryBudget)), GenerateInput{AccentHex: "#12345"}, errAny},
		{"bad theme", NewGenerator(store, NewSemaphore(MemoryBudget)), GenerateInput{ThemeColorHex: "#zzzzzz"}, errAny},
		{"corrupt avatar", NewGenerator(store, NewSemaphore(MemoryBudget)), GenerateInput{AvatarKey: corrupt}, ErrInvalidImage},
		{"corrupt og", NewGenerator(store, NewSemaphore(MemoryBudget)), GenerateInput{OGImageKey: corrupt}, ErrInvalidImage},
		{"store open error", NewGenerator(failingStore{Store: store, err: errBoom}, NewSemaphore(MemoryBudget)), GenerateInput{AvatarKey: avatar}, errBoom},
		{"store open error og", NewGenerator(failingStore{Store: store, err: errBoom}, NewSemaphore(MemoryBudget)), GenerateInput{OGImageKey: og}, errBoom},
		{"store put error", NewGenerator(&countingStore{Store: store, err: errBoom}, NewSemaphore(MemoryBudget)), GenerateInput{}, errBoom},
		{"busy", NewGenerator(store, busy), GenerateInput{AvatarKey: avatar}, ErrBusy},
		{"busy geometric", NewGenerator(store, busy), GenerateInput{}, ErrBusy},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := tt.g.Generate(context.Background(), tt.in)
			if err == nil || (!errors.Is(tt.want, errAny) && !errors.Is(err, tt.want)) {
				t.Fatalf("err = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestResolvePaletteDefaults(t *testing.T) {
	t.Parallel()
	p, err := resolvePalette(GenerateInput{})
	if err != nil {
		t.Fatal(err)
	}
	if p.backgroundHex != DefaultBackgroundHex || p.accentHex != DefaultAccentHex || p.themeHex != DefaultBackgroundHex {
		t.Fatalf("palette = %+v", p)
	}
	if p.accent != (color.NRGBA{R: 0x5b, G: 0x4a, B: 0xd8, A: 0xff}) {
		t.Fatalf("accent = %v", p.accent)
	}
}
