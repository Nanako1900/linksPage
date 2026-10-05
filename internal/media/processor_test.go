package media

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestIngestKinds(t *testing.T) {
	t.Parallel()
	src := patterned(400, 200)

	tests := []struct {
		name      string
		data      []byte
		opts      ProcessOptions
		ct        string
		w, h      int
		variants  []string
		variantCT string
	}{
		{"avatar resized", pngBytes(t, src), ProcessOptions{Kind: KindAvatar, MaxSide: 100}, "image/webp", 100, 50, []string{ExtPNG}, "image/png"},
		{"avatar keeps size", jpegBytes(t, src), ProcessOptions{Kind: KindAvatar}, "image/webp", 400, 200, []string{ExtPNG}, "image/png"},
		{"background", webpBytes(t, src), ProcessOptions{Kind: KindBackground, MaxSide: 800}, "image/webp", 400, 200, []string{ExtPNG}, "image/png"},
		{"icon portrait", pngBytes(t, patterned(50, 300)), ProcessOptions{Kind: KindIcon, MaxSide: 60}, "image/webp", 10, 60, nil, ""},
		{"qr small upscaled", pngBytes(t, patterned(100, 100)), ProcessOptions{Kind: KindQR}, "image/png", 300, 300, nil, ""},
		{"qr large kept", pngBytes(t, patterned(500, 500)), ProcessOptions{Kind: KindQR}, "image/png", 500, 500, nil, ""},
		{"qr never below 240", pngBytes(t, patterned(500, 500)), ProcessOptions{Kind: KindQR, MaxSide: 100}, "image/png", 240, 240, nil, ""},
		{"qr scaled to max", pngBytes(t, patterned(500, 500)), ProcessOptions{Kind: KindQR, MaxSide: 300}, "image/png", 300, 300, nil, ""},
		{"og", pngBytes(t, src), ProcessOptions{Kind: KindOG}, "image/jpeg", OGWidth, OGHeight, nil, ""},
		{"favicon", pngBytes(t, src), ProcessOptions{Kind: KindFavicon}, "image/png", 200, 200, nil, ""},
		{"favicon capped", pngBytes(t, patterned(900, 700)), ProcessOptions{Kind: KindFavicon}, "image/png", 512, 512, nil, ""},
		{"favicon max side", pngBytes(t, src), ProcessOptions{Kind: KindFavicon, MaxSide: 64}, "image/png", 64, 64, nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			store := newTestStore(t)
			p := NewProcessor(store, NewSemaphore(MemoryBudget))
			res, err := p.Ingest(context.Background(), bytes.NewReader(tt.data), tt.opts)
			if err != nil {
				t.Fatal(err)
			}
			checkStored(t, store, res.Primary, tt.ct, tt.w, tt.h)
			if len(res.Variants) != len(tt.variants) {
				t.Fatalf("variants = %v", res.Variants)
			}
			for _, v := range tt.variants {
				checkStored(t, store, res.Variants[v], tt.variantCT, tt.w, tt.h)
			}
			var vj map[string]string
			if err := json.Unmarshal(res.VariantsJSON(), &vj); err != nil || len(vj) != len(tt.variants) {
				t.Fatalf("VariantsJSON = %s, %v", res.VariantsJSON(), err)
			}
		})
	}
}

func checkStored(t *testing.T, store Store, s Stored, ct string, w, h int) {
	t.Helper()
	if s.ContentType != ct || s.Width != w || s.Height != h {
		t.Fatalf("stored = %+v, want %s %dx%d", s, ct, w, h)
	}
	if ContentTypeForKey(s.Key) != ct {
		t.Fatalf("key %s does not match %s", s.Key, ct)
	}
	data := readKey(t, store, s.Key)
	if len(data) != s.Bytes || KeyFor(data, s.Key[strings.LastIndexByte(s.Key, '.')+1:]) != s.Key {
		t.Fatalf("stored bytes do not match key %s", s.Key)
	}
	img := decodeAny(t, data)
	if img.Bounds().Dx() != w || img.Bounds().Dy() != h {
		t.Fatalf("decoded %v, want %dx%d", img.Bounds(), w, h)
	}
}

func TestIngestAppliesOrientationAndStripsEXIF(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	p := NewProcessor(store, NewSemaphore(MemoryBudget))
	data := withEXIF(jpegBytes(t, patterned(40, 20)), exifAPP1("MM", 6))
	res, err := p.Ingest(context.Background(), bytes.NewReader(data), ProcessOptions{Kind: KindAvatar})
	if err != nil {
		t.Fatal(err)
	}
	if res.Primary.Width != 20 || res.Primary.Height != 40 {
		t.Fatalf("rotated size = %dx%d", res.Primary.Width, res.Primary.Height)
	}
	og, err := p.Ingest(context.Background(), bytes.NewReader(data), ProcessOptions{Kind: KindOG})
	if err != nil {
		t.Fatal(err)
	}
	out := readKey(t, store, og.Primary.Key)
	if bytes.Contains(out, exifHeader) {
		t.Fatal("EXIF survived re-encoding")
	}
}

func TestIngestDeterministic(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	p := NewProcessor(store, NewSemaphore(MemoryBudget))
	data := pngBytes(t, patterned(30, 30))
	a, err := p.Ingest(context.Background(), bytes.NewReader(data), ProcessOptions{Kind: KindQR})
	if err != nil {
		t.Fatal(err)
	}
	b, err := p.Ingest(context.Background(), bytes.NewReader(data), ProcessOptions{Kind: KindQR})
	if err != nil {
		t.Fatal(err)
	}
	if a.Primary.Key != b.Primary.Key {
		t.Fatalf("keys differ: %s vs %s", a.Primary.Key, b.Primary.Key)
	}
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errBoom }

func TestIngestErrors(t *testing.T) {
	t.Parallel()
	small := pngBytes(t, patterned(10, 10))
	maxBytesBody := func() io.Reader {
		rec := httptest.NewRecorder()
		return http.MaxBytesReader(rec, io.NopCloser(bytes.NewReader(small)), 10)
	}

	tests := []struct {
		name  string
		r     io.Reader
		opts  ProcessOptions
		store Store
		want  error
	}{
		{"too large", bytes.NewReader(small), ProcessOptions{Kind: KindAvatar, MaxBytes: 10}, nil, ErrTooLarge},
		{"max bytes reader", maxBytesBody(), ProcessOptions{Kind: KindAvatar}, nil, ErrTooLarge},
		{"default limit", bytes.NewReader(make([]byte, DefaultMaxUploadBytes+1)), ProcessOptions{Kind: KindAvatar}, nil, ErrTooLarge},
		{"read error", errReader{}, ProcessOptions{Kind: KindAvatar}, nil, errBoom},
		{"gif", strings.NewReader("GIF89a\x01\x00\x01\x00\x00\x00\x00;"), ProcessOptions{Kind: KindAvatar}, nil, ErrUnsupportedType},
		{"svg", strings.NewReader(`<svg xmlns="http://www.w3.org/2000/svg"><script/></svg>`), ProcessOptions{Kind: KindIcon}, nil, ErrUnsupportedType},
		{"16-bit", bytes.NewReader(png16Bytes(t)), ProcessOptions{Kind: KindAvatar}, nil, Err16BitPNG},
		{"bomb header", bytes.NewReader(pngHeaderOnly(60000, 60000, 8)), ProcessOptions{Kind: KindAvatar}, nil, ErrDimensions},
		{"bomb pixels", bytes.NewReader(zeroPNG(t, 5000, 5000)), ProcessOptions{Kind: KindAvatar}, nil, ErrDimensions},
		{"corrupt", bytes.NewReader(append(append([]byte{}, small[:33]...), pngChunk("IDAT", []byte("zz"))...)), ProcessOptions{Kind: KindAvatar}, nil, ErrInvalidImage},
		{"unknown kind", bytes.NewReader(small), ProcessOptions{Kind: "poster"}, nil, ErrUnknownKind},
		{"store fails", bytes.NewReader(small), ProcessOptions{Kind: KindQR}, &countingStore{Store: newTestStore(t), err: errBoom}, errBoom},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			store := tt.store
			if store == nil {
				store = newTestStore(t)
			}
			p := NewProcessor(store, NewSemaphore(MemoryBudget))
			if _, err := p.Ingest(context.Background(), tt.r, tt.opts); !errors.Is(err, tt.want) {
				t.Fatalf("err = %v, want %v", err, tt.want)
			}
		})
	}
	p := NewProcessor(newTestStore(t), NewSemaphore(MemoryBudget))
	if _, err := p.Ingest(context.Background(), nil, ProcessOptions{Kind: KindAvatar}); err == nil {
		t.Fatal("nil reader: want error")
	}
}

func TestIngestBusy(t *testing.T) {
	t.Parallel()
	sem := NewSemaphore(MemoryBudget)
	rel, err := sem.Acquire(context.Background(), MemoryBudget, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer rel()
	p := NewProcessor(newTestStore(t), sem)
	start := time.Now()
	_, err = p.Ingest(context.Background(), bytes.NewReader(pngBytes(t, patterned(10, 10))), ProcessOptions{Kind: KindAvatar})
	if !errors.Is(err, ErrBusy) {
		t.Fatalf("err = %v, want ErrBusy", err)
	}
	if time.Since(start) < AcquireWait {
		t.Fatal("returned before AcquireWait")
	}
}

// TestIngestLargestImage decodes a 4096² zero PNG (a few KiB compressed,
// 128 MiB weighted) inside the real budget.
func TestIngestLargestImage(t *testing.T) {
	if testing.Short() {
		t.Skip("decodes a 4096×4096 image")
	}
	t.Parallel()
	p := NewProcessor(newTestStore(t), NewSemaphore(MemoryBudget))
	data := zeroPNG(t, MaxDimension, MaxDimension)
	if len(data) > 100<<10 {
		t.Fatalf("fixture unexpectedly large: %d", len(data))
	}
	res, err := p.Ingest(context.Background(), bytes.NewReader(data), ProcessOptions{Kind: KindIcon, MaxSide: 64})
	if err != nil {
		t.Fatal(err)
	}
	if res.Primary.Width != 64 || res.Primary.Height != 64 {
		t.Fatalf("size = %dx%d", res.Primary.Width, res.Primary.Height)
	}
}

func TestTransforms(t *testing.T) {
	t.Parallel()
	fit := []struct{ w, h, max, ww, wh int }{
		{100, 50, 0, 100, 50},
		{100, 50, 200, 100, 50},
		{100, 50, 10, 10, 5},
		{50, 100, 10, 5, 10},
		{1000, 1, 10, 10, 1},
	}
	for _, tt := range fit {
		if w, h := fitWithin(tt.w, tt.h, tt.max); w != tt.ww || h != tt.wh {
			t.Errorf("fitWithin(%d,%d,%d) = %d,%d", tt.w, tt.h, tt.max, w, h)
		}
	}
	cover := []struct {
		w, h, tw, th int
		want         image.Rectangle
	}{
		{400, 200, 1, 1, image.Rect(100, 0, 300, 200)},
		{200, 400, 1, 1, image.Rect(0, 100, 200, 300)},
		{1200, 630, 1200, 630, image.Rect(0, 0, 1200, 630)},
		{100, 100, 1200, 630, image.Rect(0, 24, 100, 76)},
	}
	for _, tt := range cover {
		if got := coverRect(tt.w, tt.h, tt.tw, tt.th); got != tt.want {
			t.Errorf("coverRect(%d,%d,%d,%d) = %v, want %v", tt.w, tt.h, tt.tw, tt.th, got, tt.want)
		}
	}
}
