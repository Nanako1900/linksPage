package media

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"

	"github.com/gen2brain/webp"
)

// webpMethod trades encoding speed for size (0 fast … 6 small).
const webpMethod = 4

// encoded is an encoded image waiting to be stored.
type encoded struct {
	ext    string
	data   []byte
	width  int
	height int
}

func encodePNG(img image.Image) (encoded, error) {
	var buf bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.BestCompression}
	if err := enc.Encode(&buf, img); err != nil {
		return encoded{}, fmt.Errorf("media: encode png: %w", err)
	}
	return newEncoded(ExtPNG, buf.Bytes(), img), nil
}

// encodeJPEG flattens transparency onto white (JPEG has no alpha).
func encodeJPEG(img image.Image, quality int) (encoded, error) {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, flatten(img, color.White), &jpeg.Options{Quality: quality}); err != nil {
		return encoded{}, fmt.Errorf("media: encode jpeg: %w", err)
	}
	return newEncoded(ExtJPG, buf.Bytes(), img), nil
}

func encodeWebP(img image.Image, opts webp.Options) (enc encoded, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("media: encode webp: panic: %v", r)
		}
	}()
	var buf bytes.Buffer
	if err := webp.Encode(&buf, img, opts); err != nil {
		return encoded{}, fmt.Errorf("media: encode webp: %w", err)
	}
	return newEncoded(ExtWebP, buf.Bytes(), img), nil
}

func newEncoded(ext string, data []byte, img image.Image) encoded {
	b := img.Bounds()
	return encoded{ext: ext, data: data, width: b.Dx(), height: b.Dy()}
}

// put stores e and describes the stored file.
func put(ctx context.Context, store Store, e encoded) (Stored, error) {
	key, err := store.Put(ctx, e.data, e.ext)
	if err != nil {
		return Stored{}, err
	}
	return Stored{
		Key:         key,
		ContentType: ContentTypeForExt(e.ext),
		Bytes:       len(e.data),
		Width:       e.width,
		Height:      e.height,
	}, nil
}
