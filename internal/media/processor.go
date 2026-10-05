package media

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"io"
	"net/http"

	"github.com/gen2brain/webp"
	"golang.org/x/image/draw"
)

// DefaultFaviconSide is the side an uploaded favicon is stored at when
// ProcessOptions.MaxSide is 0 (the largest generated favicon).
const DefaultFaviconSide = 512

// Result is the output of Ingest: the primary file plus sibling variants
// (e.g. a PNG fallback), all already stored.
type Result struct {
	Primary  Stored
	Variants map[string]Stored // format ("png", ...) → stored sibling
}

// VariantsJSON returns the media.variants column value for the primary
// row: {"png": "<key>", ...} ({} when there are none).
func (r Result) VariantsJSON() []byte {
	m := make(map[string]string, len(r.Variants))
	for format, s := range r.Variants {
		m[format] = s.Key
	}
	b, err := json.Marshal(m)
	if err != nil {
		return []byte("{}")
	}
	return b
}

// ProcessOptions select the output for a media kind.
type ProcessOptions struct {
	Kind Kind
	// MaxBytes is uploads.max_bytes (DefaultMaxUploadBytes).
	MaxBytes int64
	// MaxSide resizes (CatmullRom) so the longer side is at most MaxSide;
	// 0 keeps the size. QR codes never go below MinQRSide; OG images are
	// always OGWidth×OGHeight; favicons default to DefaultFaviconSide.
	MaxSide int
}

// Processor runs the ingest pipeline: MaxBytesReader → DetectContentType
// (png/jpeg/webp) → DecodeConfig (≤4096², no 16-bit PNG) → Semaphore →
// decode (EXIF orientation applied) → resize → re-encode, which drops all
// metadata. Outputs per kind:
//   - avatar, background: WebP Q82 + PNG variant
//   - icon: WebP Q82
//   - qr: lossless PNG, upscaled (nearest neighbour) to at least MinQRSide
//   - og: JPEG OGWidth×OGHeight (centre crop)
//   - favicon: square PNG (centre crop)
type Processor struct {
	store  Store
	sem    *Semaphore
	scaler draw.Interpolator
	webp   webp.Options
}

// NewProcessor returns a processor writing to store.
func NewProcessor(store Store, sem *Semaphore) *Processor {
	return &Processor{
		store:  store,
		sem:    sem,
		scaler: draw.CatmullRom,
		webp:   webp.Options{Quality: WebPQuality, Method: webpMethod},
	}
}

// Ingest validates, processes and stores an image. The caller inserts the
// media rows (dbq.InsertMedia) for Primary and Variants.
func (p *Processor) Ingest(ctx context.Context, r io.Reader, opts ProcessOptions) (Result, error) {
	if r == nil {
		return Result{}, errors.New("media: nil reader")
	}
	if !knownKind(opts.Kind) {
		return Result{}, fmt.Errorf("%w: %q", ErrUnknownKind, opts.Kind)
	}
	data, err := readLimited(r, opts.MaxBytes)
	if err != nil {
		return Result{}, err
	}
	h, err := inspect(data)
	if err != nil {
		return Result{}, err
	}
	release, err := p.sem.Acquire(ctx, ingestWeight(h), AcquireWait)
	if err != nil {
		return Result{}, err
	}
	defer release()
	img, err := decodePixels(h, data)
	if err != nil {
		return Result{}, err
	}
	outs, err := p.render(img, opts)
	if err != nil {
		return Result{}, err
	}
	return p.storeAll(ctx, outs)
}

// ingestWeight covers the decoded source plus the largest output canvas.
func ingestWeight(h header) int64 {
	return ImageWeight(h.width, h.height) + ImageWeight(OGWidth, OGHeight)
}

func knownKind(k Kind) bool {
	switch k {
	case KindAvatar, KindBackground, KindIcon, KindQR, KindOG, KindFavicon:
		return true
	}
	return false
}

// readLimited reads at most maxBytes (DefaultMaxUploadBytes when ≤ 0). A
// request body wrapped in http.MaxBytesReader reports ErrTooLarge too.
func readLimited(r io.Reader, maxBytes int64) ([]byte, error) {
	if maxBytes <= 0 {
		maxBytes = DefaultMaxUploadBytes
	}
	data, err := io.ReadAll(io.LimitReader(r, maxBytes+1))
	if err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			return nil, ErrTooLarge
		}
		return nil, fmt.Errorf("media: read upload: %w", err)
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("%w (limit %d bytes)", ErrTooLarge, maxBytes)
	}
	return data, nil
}

// render produces the encoded outputs for opts.Kind; the first is primary.
func (p *Processor) render(img *image.NRGBA, opts ProcessOptions) ([]encoded, error) {
	switch opts.Kind {
	case KindQR:
		return encodeAll(qrResize(img, opts.MaxSide, p.scaler), encodePNG)
	case KindOG:
		og := cover(img, OGWidth, OGHeight, p.scaler)
		return encodeAll(og, func(i image.Image) (encoded, error) { return encodeJPEG(i, JPEGQuality) })
	case KindFavicon:
		side := opts.MaxSide
		if side <= 0 {
			side = DefaultFaviconSide
		}
		sq := squareCrop(img)
		n := min(sq.Rect.Dx(), side)
		return encodeAll(resize(sq, n, n, p.scaler), encodePNG)
	}
	w, h := fitWithin(img.Rect.Dx(), img.Rect.Dy(), opts.MaxSide)
	out := resize(img, w, h, p.scaler)
	toWebP := func(i image.Image) (encoded, error) { return encodeWebP(i, p.webp) }
	if opts.Kind == KindIcon {
		return encodeAll(out, toWebP)
	}
	return encodeAll(out, toWebP, encodePNG)
}

func encodeAll(img image.Image, encoders ...func(image.Image) (encoded, error)) ([]encoded, error) {
	outs := make([]encoded, 0, len(encoders))
	for _, enc := range encoders {
		e, err := enc(img)
		if err != nil {
			return nil, err
		}
		outs = append(outs, e)
	}
	return outs, nil
}

// qrResize keeps QR codes crisp: small codes are upscaled by an integer
// factor with nearest neighbour; large ones are only scaled down to
// max(maxSide, MinQRSide).
func qrResize(img *image.NRGBA, maxSide int, s draw.Scaler) *image.NRGBA {
	w, h := img.Rect.Dx(), img.Rect.Dy()
	long := max(w, h)
	if long < MinQRSide {
		f := (MinQRSide + long - 1) / long
		return resize(img, w*f, h*f, draw.NearestNeighbor)
	}
	if maxSide <= 0 {
		return img
	}
	tw, th := fitWithin(w, h, max(maxSide, MinQRSide))
	return resize(img, tw, th, s)
}

func (p *Processor) storeAll(ctx context.Context, outs []encoded) (Result, error) {
	res := Result{Variants: make(map[string]Stored, len(outs)-1)}
	for i, e := range outs {
		s, err := put(ctx, p.store, e)
		if err != nil {
			return Result{}, err
		}
		if i == 0 {
			res.Primary = s
			continue
		}
		res.Variants[e.ext] = s
	}
	return res, nil
}
