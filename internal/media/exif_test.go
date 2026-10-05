package media

import (
	"encoding/binary"
	"image"
	"image/color"
	"testing"
)

func TestJPEGOrientation(t *testing.T) {
	t.Parallel()
	base := jpegBytes(t, patterned(4, 2))
	badTIFF := exifAPP1("II", 6)
	badTIFF[4+len(exifHeader)+2] = 43 // wrong magic
	wrongType := exifAPP1("MM", 6)
	binary.BigEndian.PutUint16(wrongType[4+len(exifHeader)+12:], 4)
	badOrder := exifAPP1("II", 6)
	copy(badOrder[4+len(exifHeader):], "XX")

	tests := []struct {
		name string
		data []byte
		want int
	}{
		{"no exif", base, 1},
		{"little endian 6", withEXIF(base, exifAPP1("II", 6)), 6},
		{"big endian 8", withEXIF(base, exifAPP1("MM", 8)), 8},
		{"out of range", withEXIF(base, exifAPP1("II", 9)), 1},
		{"bad magic", withEXIF(base, badTIFF), 1},
		{"bad byte order", withEXIF(base, badOrder), 1},
		{"wrong type", withEXIF(base, wrongType), 1},
		{"not jpeg", []byte("\x89PNG\r\n\x1a\n"), 1},
		{"short", []byte{0xFF, 0xD8}, 1},
		{"truncated segment", []byte{0xFF, 0xD8, 0xFF, 0xE1, 0xFF, 0xFF, 0x00}, 1},
		{"garbage after soi", []byte{0xFF, 0xD8, 0x00, 0x00, 0x00, 0x00}, 1},
		{"tiny segment size", []byte{0xFF, 0xD8, 0xFF, 0xE1, 0x00, 0x01, 0x00}, 1},
		{"eoi first", []byte{0xFF, 0xD8, 0xFF, 0xD9, 0x00, 0x00}, 1},
	}
	for _, tt := range tests {
		if got := jpegOrientation(tt.data); got != tt.want {
			t.Errorf("%s: orientation = %d, want %d", tt.name, got, tt.want)
		}
	}
}

func TestTIFFOrientationBounds(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		tiff []byte
	}{
		{"short", []byte("II*")},
		{"ifd past end", []byte{'I', 'I', 42, 0, 0xff, 0, 0, 0}},
		{"ifd before header", []byte{'I', 'I', 42, 0, 2, 0, 0, 0, 0, 0}},
		{"entry past end", []byte{'I', 'I', 42, 0, 8, 0, 0, 0, 5, 0, 1, 2}},
		{"no orientation tag", append([]byte{'I', 'I', 42, 0, 8, 0, 0, 0, 1, 0}, make([]byte, 12)...)},
	}
	for _, tt := range tests {
		if got := tiffOrientation(tt.tiff); got != 1 {
			t.Errorf("%s: got %d", tt.name, got)
		}
	}
}

func TestApplyOrientation(t *testing.T) {
	t.Parallel()
	// 3×2 source; the marker sits at (0,0).
	src := image.NewNRGBA(image.Rect(0, 0, 3, 2))
	marker := color.NRGBA{R: 0xff, A: 0xff}
	src.SetNRGBA(0, 0, marker)

	tests := []struct {
		o      int
		w, h   int
		mx, my int
	}{
		{1, 3, 2, 0, 0},
		{2, 3, 2, 2, 0},
		{3, 3, 2, 2, 1},
		{4, 3, 2, 0, 1},
		{5, 2, 3, 0, 0},
		{6, 2, 3, 1, 0},
		{7, 2, 3, 1, 2},
		{8, 2, 3, 0, 2},
		{0, 3, 2, 0, 0},
		{9, 3, 2, 0, 0},
	}
	for _, tt := range tests {
		got := applyOrientation(src, tt.o)
		if got.Rect.Dx() != tt.w || got.Rect.Dy() != tt.h {
			t.Errorf("o=%d: size %v", tt.o, got.Rect)
			continue
		}
		if got.NRGBAAt(tt.mx, tt.my) != marker {
			t.Errorf("o=%d: marker not at (%d,%d)", tt.o, tt.mx, tt.my)
		}
	}
}
