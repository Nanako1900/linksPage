package media

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"sync"
	"testing"

	"github.com/gen2brain/webp"
)

// patterned returns a w×h image with a gradient and a marked top-left
// pixel so orientation can be checked.
func patterned(w, h int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.SetNRGBA(x, y, color.NRGBA{R: uint8(x * 255 / max(1, w-1)), G: uint8(y * 255 / max(1, h-1)), B: 0x80, A: 0xff})
		}
	}
	img.SetNRGBA(0, 0, color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff})
	return img
}

func pngBytes(t testing.TB, img image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func jpegBytes(t testing.TB, img image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func webpBytes(t testing.TB, img image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := webp.Encode(&buf, img, webp.Options{Quality: 80}); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func png16Bytes(t testing.TB) []byte {
	t.Helper()
	img := image.NewRGBA64(image.Rect(0, 0, 4, 4))
	for i := range img.Pix {
		img.Pix[i] = uint8(i * 7)
	}
	return pngBytes(t, img)
}

// pngChunk encodes one PNG chunk with its CRC.
func pngChunk(typ string, data []byte) []byte {
	var b bytes.Buffer
	_ = binary.Write(&b, binary.BigEndian, uint32(len(data)))
	b.WriteString(typ)
	b.Write(data)
	crc := crc32.NewIEEE()
	crc.Write([]byte(typ))
	crc.Write(data)
	_ = binary.Write(&b, binary.BigEndian, crc.Sum32())
	return b.Bytes()
}

// pngHeaderOnly is a PNG whose IHDR claims w×h but has no pixel data
// (a "huge header" decompression-bomb probe).
func pngHeaderOnly(w, h uint32, depth byte) []byte {
	ihdr := make([]byte, 13)
	binary.BigEndian.PutUint32(ihdr[0:], w)
	binary.BigEndian.PutUint32(ihdr[4:], h)
	ihdr[8] = depth
	ihdr[9] = 6 // RGBA
	out := []byte("\x89PNG\r\n\x1a\n")
	out = append(out, pngChunk("IHDR", ihdr)...)
	return append(out, pngChunk("IEND", nil)...)
}

// jpegHeaderOnly is a JFIF prefix whose SOF0 claims w×h.
func jpegHeaderOnly(w, h uint16) []byte {
	out := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0, 1, 1, 0, 0, 1, 0, 1, 0, 0}
	sof := []byte{
		0xFF, 0xC0, 0x00, 0x11, 8, byte(h >> 8), byte(h), byte(w >> 8), byte(w), 3,
		1, 0x11, 0, 2, 0x11, 0, 3, 0x11, 0,
	}
	return append(out, sof...)
}

// webpVP8XHeader is a WebP extended header whose canvas is w×h.
func webpVP8XHeader(w, h int) []byte {
	b := []byte("RIFF\x00\x00\x00\x00WEBPVP8X\x0a\x00\x00\x00\x00\x00\x00\x00")
	b = append(b, byte(w-1), byte((w-1)>>8), byte((w-1)>>16), byte(h-1), byte((h-1)>>8), byte((h-1)>>16))
	binary.LittleEndian.PutUint32(b[4:], uint32(len(b)-8))
	return b
}

// exifAPP1 builds an APP1 segment carrying only an orientation tag.
func exifAPP1(order string, orientation uint16) []byte {
	var bo binary.ByteOrder = binary.LittleEndian
	if order == "MM" {
		bo = binary.BigEndian
	}
	tiff := make([]byte, 8+2+12+4)
	copy(tiff, order)
	bo.PutUint16(tiff[2:], 42)
	bo.PutUint32(tiff[4:], 8)
	bo.PutUint16(tiff[8:], 1)
	bo.PutUint16(tiff[10:], tagOrientation)
	bo.PutUint16(tiff[12:], typeShort)
	bo.PutUint32(tiff[14:], 1)
	bo.PutUint16(tiff[18:], orientation)
	payload := append(append([]byte{}, exifHeader...), tiff...)
	seg := []byte{0xFF, markerAPP1, 0, 0}
	binary.BigEndian.PutUint16(seg[2:], uint16(len(payload)+2))
	return append(seg, payload...)
}

// withEXIF inserts an APP1 segment right after SOI.
func withEXIF(jpg, app1 []byte) []byte {
	out := append([]byte{}, jpg[:2]...)
	out = append(out, app1...)
	return append(out, jpg[2:]...)
}

func newTestStore(t testing.TB) *LocalStore {
	t.Helper()
	s, err := NewLocalStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func decodeAny(t testing.TB, data []byte) image.Image {
	t.Helper()
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	return img
}

func readKey(t testing.TB, s Store, key string) []byte {
	t.Helper()
	f, err := s.Open(context.Background(), key)
	if err != nil {
		t.Fatalf("open %s: %v", key, err)
	}
	defer func() { _ = f.Close() }()
	b, err := io.ReadAll(f)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// countingStore wraps a Store and counts Put calls.
type countingStore struct {
	Store
	mu   sync.Mutex
	puts int
	err  error
}

func (c *countingStore) Put(ctx context.Context, data []byte, ext string) (string, error) {
	c.mu.Lock()
	c.puts++
	err := c.err
	c.mu.Unlock()
	if err != nil {
		return "", err
	}
	return c.Store.Put(ctx, data, ext)
}

func (c *countingStore) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.puts
}

// failingStore fails Open with err.
type failingStore struct {
	Store
	err error
}

func (f failingStore) Open(context.Context, string) (io.ReadSeekCloser, error) {
	return nil, f.err
}

var errBoom = errors.New("boom")
