package media

import (
	"bytes"
	"context"
	"errors"
	"testing"
)

// fuzzErrors are the only errors Ingest may return for arbitrary bytes.
var fuzzErrors = []error{ErrInvalidImage, ErrUnsupportedType, ErrDimensions, Err16BitPNG, ErrTooLarge}

// FuzzIngest feeds arbitrary bytes through the whole decoder path:
// decompression bombs and huge headers must be rejected before pixels
// are allocated, and nothing may panic.
func FuzzIngest(f *testing.F) {
	img := patterned(9, 7)
	for _, seed := range [][]byte{
		pngBytes(f, img),
		jpegBytes(f, img),
		webpBytes(f, img),
		withEXIF(jpegBytes(f, img), exifAPP1("II", 6)),
		png16Bytes(f),
		pngHeaderOnly(100000, 100000, 8),
		pngHeaderOnly(4096, 4096, 8),
		jpegHeaderOnly(65535, 65535),
		webpVP8XHeader(16000, 16000),
		zeroPNG(f, 4097, 1),
		[]byte("GIF89a"),
		[]byte("<svg/>"),
	} {
		f.Add(seed)
	}
	store := newTestStore(f)
	p := NewProcessor(store, NewSemaphore(MemoryBudget))
	f.Fuzz(func(t *testing.T, data []byte) {
		res, err := p.Ingest(context.Background(), bytes.NewReader(data), ProcessOptions{Kind: KindIcon, MaxBytes: 1 << 20, MaxSide: 32})
		if err != nil {
			for _, want := range fuzzErrors {
				if errors.Is(err, want) {
					return
				}
			}
			t.Fatalf("unexpected error class: %v", err)
		}
		if res.Primary.Width < 1 || res.Primary.Width > 32 || res.Primary.Height < 1 || res.Primary.Height > 32 {
			t.Fatalf("accepted output %dx%d", res.Primary.Width, res.Primary.Height)
		}
	})
}

// FuzzJPEGOrientation exercises the hand-written EXIF parser.
func FuzzJPEGOrientation(f *testing.F) {
	f.Add(withEXIF(jpegBytes(f, patterned(2, 2)), exifAPP1("MM", 3)))
	f.Add([]byte{0xFF, 0xD8, 0xFF, 0xE1, 0x00, 0x08, 'E', 'x', 'i', 'f', 0, 0})
	f.Fuzz(func(t *testing.T, data []byte) {
		if o := jpegOrientation(data); o < 1 || o > 8 {
			t.Fatalf("orientation %d", o)
		}
	})
}
