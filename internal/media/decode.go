package media

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"net/http"

	"github.com/gen2brain/webp"
)

// Source formats accepted by the ingest pipeline.
const (
	formatPNG  = "png"
	formatJPEG = "jpeg"
	formatWebP = "webp"
)

// sniffLen is how many bytes http.DetectContentType looks at.
const sniffLen = 512

// pngBitDepthOffset is the IHDR bit-depth byte: 8-byte signature, chunk
// length, "IHDR", width, height.
const pngBitDepthOffset = 24

// header is what we know about an image before decoding its pixels.
type header struct {
	format string
	// width and height are the stored dimensions (before orientation).
	width, height int
	// orientation is the EXIF orientation (JPEG only; WebP is rotated by
	// the decoder itself).
	orientation int
}

// displaySize returns the dimensions after applying the orientation.
func (h header) displaySize() (int, int) {
	if h.orientation >= 5 {
		return h.height, h.width
	}
	return h.width, h.height
}

// sniff maps the detected content type to a format; SVG, GIF and anything
// else are ErrUnsupportedType.
func sniff(data []byte) (string, error) {
	switch http.DetectContentType(data[:min(len(data), sniffLen)]) {
	case "image/png":
		return formatPNG, nil
	case "image/jpeg":
		return formatJPEG, nil
	case "image/webp":
		return formatWebP, nil
	}
	return "", ErrUnsupportedType
}

// inspect validates type, dimensions and bit depth without decoding pixels.
func inspect(data []byte) (header, error) {
	format, err := sniff(data)
	if err != nil {
		return header{}, err
	}
	cfg, err := safeDecodeConfig(format, data)
	if err != nil {
		return header{}, fmt.Errorf("%w: %w", ErrInvalidImage, err)
	}
	if cfg.Width < 1 || cfg.Height < 1 || cfg.Width > MaxDimension || cfg.Height > MaxDimension {
		return header{}, fmt.Errorf("%w (got %dx%d)", ErrDimensions, cfg.Width, cfg.Height)
	}
	if format == formatPNG && is16BitPNG(data, cfg.ColorModel) {
		return header{}, Err16BitPNG
	}
	h := header{format: format, width: cfg.Width, height: cfg.Height, orientation: 1}
	if format == formatJPEG {
		h.orientation = jpegOrientation(data)
	}
	return h, nil
}

func is16BitPNG(data []byte, model color.Model) bool {
	if len(data) > pngBitDepthOffset && data[pngBitDepthOffset] == 16 {
		return true
	}
	switch model {
	case color.RGBA64Model, color.NRGBA64Model, color.Gray16Model:
		return true
	}
	return false
}

func safeDecodeConfig(format string, data []byte) (cfg image.Config, err error) {
	defer recoverDecode(&err)
	r := bytes.NewReader(data)
	switch format {
	case formatPNG:
		return png.DecodeConfig(r)
	case formatJPEG:
		return jpeg.DecodeConfig(r)
	default:
		return webp.DecodeConfig(r)
	}
}

// decodePixels decodes data (already checked by inspect) into a zero-origin
// NRGBA image with the EXIF orientation applied.
func decodePixels(h header, data []byte) (*image.NRGBA, error) {
	img, err := safeDecode(h.format, data)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidImage, err)
	}
	b := img.Bounds()
	if b.Dx() < 1 || b.Dy() < 1 || b.Dx() > MaxDimension || b.Dy() > MaxDimension {
		return nil, fmt.Errorf("%w: decoded size %dx%d", ErrInvalidImage, b.Dx(), b.Dy())
	}
	return applyOrientation(toNRGBA(img), h.orientation), nil
}

func safeDecode(format string, data []byte) (img image.Image, err error) {
	defer recoverDecode(&err)
	r := bytes.NewReader(data)
	switch format {
	case formatPNG:
		return png.Decode(r)
	case formatJPEG:
		return jpeg.Decode(r)
	default:
		return webp.Decode(r, webp.Options{AutoRotate: true})
	}
}

// recoverDecode turns a decoder panic (the WebP backend runs translated
// C code) into an error instead of crashing the process.
func recoverDecode(err *error) {
	if r := recover(); r != nil {
		*err = fmt.Errorf("decoder panic: %v", r)
	}
}

// toNRGBA returns img as a zero-origin *image.NRGBA, copying unless it
// already is one.
func toNRGBA(img image.Image) *image.NRGBA {
	if n, ok := img.(*image.NRGBA); ok && n.Rect.Min == (image.Point{}) {
		return n
	}
	b := img.Bounds()
	dst := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(dst, dst.Bounds(), img, b.Min, draw.Src)
	return dst
}
