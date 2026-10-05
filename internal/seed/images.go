package seed

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/Nanako1900/linksPage/internal/media"
)

// Longest side (px) of seeded images per kind; 0 lets the processor
// decide (QR codes keep their size, OG images become 1200×630).
var maxSides = map[media.Kind]int{
	media.KindAvatar: 512,
	media.KindIcon:   256,
	media.KindQR:     0,
	media.KindOG:     0,
}

// ingestAll processes every image before the transaction starts
// (content-addressed files left behind by a failed import are harmless).
// Images are opened through os.Root, so paths cannot escape dir, not even
// through symlinks.
func (im *Importer) ingestAll(ctx context.Context, dir string, refs []imageRef) (map[string]media.Result, error) {
	out := make(map[string]media.Result, len(refs))
	if len(refs) == 0 {
		return out, nil
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, fmt.Errorf("%w: open seed directory: %w", ErrInvalidSeed, err)
	}
	defer closeQuietly(root)
	for _, r := range refs {
		res, err := im.ingest(ctx, root, r)
		if err != nil {
			return nil, err
		}
		out[r.id()] = res
	}
	return out, nil
}

func (im *Importer) ingest(ctx context.Context, root *os.Root, r imageRef) (media.Result, error) {
	f, err := root.Open(r.path)
	if err != nil {
		return media.Result{}, fmt.Errorf("%w: %s: open image: %w", ErrInvalidSeed, r.field, err)
	}
	defer closeQuietly(f)
	maxBytes := im.deps.MaxUploadBytes
	if maxBytes <= 0 {
		maxBytes = media.DefaultMaxUploadBytes
	}
	res, err := im.deps.Media.Ingest(ctx, f, media.ProcessOptions{Kind: r.kind, MaxBytes: maxBytes, MaxSide: maxSides[r.kind]})
	switch {
	case err == nil:
		return res, nil
	case invalidImage(err):
		return media.Result{}, fmt.Errorf("%w: %s: %w", ErrInvalidSeed, r.field, err)
	default:
		// Busy, cancelled or a storage failure (disk full, I/O error):
		// transient, startup retries.
		return media.Result{}, fmt.Errorf("seed: %s: %w", r.field, err)
	}
}

// imageRejections are the media errors caused by the image file itself;
// only these make the seed invalid (fatal at startup).
var imageRejections = []error{
	media.ErrTooLarge, media.ErrUnsupportedType, media.ErrDimensions,
	media.Err16BitPNG, media.ErrInvalidImage, media.ErrUnknownKind,
}

func invalidImage(err error) bool {
	for _, target := range imageRejections {
		if errors.Is(err, target) {
			return true
		}
	}
	return false
}

// closeQuietly closes read-only handles; a close error cannot lose data.
func closeQuietly(c interface{ Close() error }) {
	_ = c.Close()
}
