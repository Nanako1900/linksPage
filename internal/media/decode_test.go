package media

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"errors"
	"image"
	"image/color"
	"testing"
)

func TestSniff(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		data []byte
		want string
		err  error
	}{
		{"png", pngBytes(t, patterned(2, 2)), formatPNG, nil},
		{"jpeg", jpegBytes(t, patterned(2, 2)), formatJPEG, nil},
		{"webp", webpBytes(t, patterned(2, 2)), formatWebP, nil},
		{"gif", []byte("GIF89a\x01\x00\x01\x00"), "", ErrUnsupportedType},
		{"svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`), "", ErrUnsupportedType},
		{"html", []byte("<!DOCTYPE html><script>alert(1)</script>"), "", ErrUnsupportedType},
		{"empty", nil, "", ErrUnsupportedType},
	}
	for _, tt := range tests {
		got, err := sniff(tt.data)
		if got != tt.want || !errors.Is(err, tt.err) {
			t.Errorf("%s: sniff = %q, %v; want %q, %v", tt.name, got, err, tt.want, tt.err)
		}
	}
}

// zeroPNG is a valid w×h 8-bit grayscale PNG of zeros: a few KiB on disk
// that decodes to w×h bytes (the classic decompression bomb shape).
func zeroPNG(t testing.TB, w, h int) []byte {
	t.Helper()
	ihdr := make([]byte, 13)
	binary.BigEndian.PutUint32(ihdr[0:], uint32(w))
	binary.BigEndian.PutUint32(ihdr[4:], uint32(h))
	ihdr[8] = 8 // depth
	ihdr[9] = 0 // grayscale
	var raw bytes.Buffer
	zw, err := zlib.NewWriterLevel(&raw, zlib.BestCompression)
	if err != nil {
		t.Fatal(err)
	}
	row := make([]byte, w+1)
	for range h {
		if _, err := zw.Write(row); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	out := []byte("\x89PNG\r\n\x1a\n")
	out = append(out, pngChunk("IHDR", ihdr)...)
	out = append(out, pngChunk("IDAT", raw.Bytes())...)
	return append(out, pngChunk("IEND", nil)...)
}

func TestInspect(t *testing.T) {
	t.Parallel()
	truncated := pngBytes(t, patterned(8, 8))
	truncated = truncated[:20]

	tests := []struct {
		name   string
		data   []byte
		want   error
		w, h   int
		orient int
	}{
		{"png ok", pngBytes(t, patterned(5, 3)), nil, 5, 3, 1},
		{"jpeg ok", jpegBytes(t, patterned(6, 4)), nil, 6, 4, 1},
		{"jpeg rotated", withEXIF(jpegBytes(t, patterned(6, 4)), exifAPP1("II", 6)), nil, 6, 4, 6},
		{"webp ok", webpBytes(t, patterned(7, 5)), nil, 7, 5, 1},
		{"max size header", pngHeaderOnly(4096, 4096, 8), nil, 4096, 4096, 1},
		{"png huge header", pngHeaderOnly(100000, 100000, 8), ErrDimensions, 0, 0, 0},
		{"png wide", pngHeaderOnly(4097, 1, 8), ErrDimensions, 0, 0, 0},
		{"png max uint32", pngHeaderOnly(0x7fffffff, 0x7fffffff, 8), nil, 0, 0, 0},
		{"jpeg huge header", jpegHeaderOnly(65535, 65535), ErrDimensions, 0, 0, 0},
		{"webp huge canvas", webpVP8XHeader(16000, 16000), nil, 0, 0, 0},
		{"16-bit png", png16Bytes(t), Err16BitPNG, 0, 0, 0},
		{"16-bit header", pngHeaderOnly(10, 10, 16), Err16BitPNG, 0, 0, 0},
		{"truncated png", truncated, ErrInvalidImage, 0, 0, 0},
		{"png signature only", []byte("\x89PNG\r\n\x1a\nxxxxxxxxxxxx"), ErrInvalidImage, 0, 0, 0},
		{"svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`), ErrUnsupportedType, 0, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h, err := inspect(tt.data)
			if tt.want == nil && tt.w == 0 {
				// Either rejected as too large or as corrupt; never accepted.
				if err == nil {
					t.Fatalf("accepted %dx%d", h.width, h.height)
				}
				if !errors.Is(err, ErrDimensions) && !errors.Is(err, ErrInvalidImage) {
					t.Fatalf("err = %v", err)
				}
				return
			}
			if !errors.Is(err, tt.want) {
				t.Fatalf("err = %v, want %v", err, tt.want)
			}
			if err == nil && (h.width != tt.w || h.height != tt.h || h.orientation != tt.orient) {
				t.Fatalf("header = %+v", h)
			}
		})
	}
}

func TestHeaderDisplaySize(t *testing.T) {
	t.Parallel()
	if w, h := (header{width: 3, height: 2, orientation: 6}).displaySize(); w != 2 || h != 3 {
		t.Errorf("rotated = %dx%d", w, h)
	}
	if w, h := (header{width: 3, height: 2, orientation: 1}).displaySize(); w != 3 || h != 2 {
		t.Errorf("upright = %dx%d", w, h)
	}
}

func TestIs16BitPNGColorModel(t *testing.T) {
	t.Parallel()
	for _, m := range []color.Model{color.RGBA64Model, color.NRGBA64Model, color.Gray16Model} {
		if !is16BitPNG(nil, m) {
			t.Errorf("%v not detected", m)
		}
	}
	if is16BitPNG(nil, color.NRGBAModel) {
		t.Error("8-bit model flagged")
	}
}

func TestDecodePixels(t *testing.T) {
	t.Parallel()
	good := pngBytes(t, patterned(4, 3))
	h, err := inspect(good)
	if err != nil {
		t.Fatal(err)
	}
	img, err := decodePixels(h, good)
	if err != nil || img.Rect != image.Rect(0, 0, 4, 3) {
		t.Fatalf("decode = %v, %v", img.Rect, err)
	}

	// Header is valid, pixel data is not.
	bad := append(append([]byte{}, good[:33]...), pngChunk("IDAT", []byte("not zlib"))...)
	bh, err := inspect(bad)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decodePixels(bh, bad); !errors.Is(err, ErrInvalidImage) {
		t.Fatalf("corrupt pixels: err = %v", err)
	}
	if _, err := decodePixels(header{format: formatWebP}, []byte("RIFF\x00\x00\x00\x00WEBPVP8 ")); !errors.Is(err, ErrInvalidImage) {
		t.Fatalf("corrupt webp: err = %v", err)
	}
}

func TestToNRGBA(t *testing.T) {
	t.Parallel()
	n := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	if toNRGBA(n) != n {
		t.Error("zero-origin NRGBA should be returned as is")
	}
	offset := image.NewNRGBA(image.Rect(5, 5, 7, 8))
	offset.SetNRGBA(5, 5, color.NRGBA{R: 1, A: 255})
	got := toNRGBA(offset)
	if got.Rect != image.Rect(0, 0, 2, 3) || got.NRGBAAt(0, 0).R != 1 {
		t.Errorf("offset copy = %v", got.Rect)
	}
	if toNRGBA(image.NewGray(image.Rect(0, 0, 3, 1))).Rect.Dx() != 3 {
		t.Error("gray conversion")
	}
}

func TestRecoverDecode(t *testing.T) {
	t.Parallel()
	f := func() (err error) {
		defer recoverDecode(&err)
		panic("boom")
	}
	if err := f(); err == nil {
		t.Fatal("panic not converted")
	}
}
