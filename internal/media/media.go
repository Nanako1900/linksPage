// Package media stores and serves uploaded images (doc 4.7):
// content-addressed local storage (os.OpenRoot), the ingest pipeline
// (validate → memory-weighted semaphore → decode → resize → re-encode,
// EXIF dropped), the /media/u and /media/q handlers and the generated site
// files (OG image, favicons, site.webmanifest, robots.txt).
package media

import (
	"errors"
	"path"
	"regexp"
	"time"
)

// Ingest errors (mapped to 413/415/422/503 by HTTP callers).
var (
	ErrTooLarge        = errors.New("media: file too large")
	ErrUnsupportedType = errors.New("media: only png, jpeg and webp are accepted")
	ErrDimensions      = errors.New("media: image larger than 4096x4096")
	Err16BitPNG        = errors.New("media: 16-bit PNG is not supported; export an 8-bit PNG")
	ErrBusy            = errors.New("media: image processing is busy; retry later")
	ErrNotFound        = errors.New("media: not found")
	// ErrInvalidImage reports a file whose type was recognised but whose
	// header or pixel data cannot be decoded (422).
	ErrInvalidImage = errors.New("media: image is corrupt or truncated")
	// ErrUnknownKind reports a ProcessOptions.Kind the processor does not
	// handle (a programming error, 500).
	ErrUnknownKind = errors.New("media: unknown media kind")
)

// Kind is media.kind.
type Kind string

// Media kinds.
const (
	KindAvatar     Kind = "avatar"
	KindIcon       Kind = "icon"
	KindQR         Kind = "qr"
	KindOG         Kind = "og"
	KindFavicon    Kind = "favicon"
	KindBackground Kind = "background"
)

// Limits (doc 4.7, 4.11).
const (
	DefaultMaxUploadBytes = 5 << 20
	MaxDimension          = 4096
	WebPQuality           = 82
	OGWidth, OGHeight     = 1200, 630
	// JPEGQuality is used for OG images.
	JPEGQuality = 85
	// MinQRSide is the smallest side a QR code is stored at so WeChat and
	// QQ long-press recognition works.
	MinQRSide = 240
	// MemoryBudget is shared by image processing and argon2 (M2a).
	MemoryBudget = 160 << 20
	// AcquireWait is how long a request waits for the semaphore before
	// failing with ErrBusy (→ 503 + Retry-After).
	AcquireWait = 2 * time.Second
)

// FaviconSizes are the generated PNG favicon sizes.
var FaviconSizes = []int{32, 180, 192, 512}

// KeyRe matches a stored key: hex(sha256(bytes)[:16]) + "." + ext.
var KeyRe = regexp.MustCompile(`^[0-9a-f]{32}\.(webp|png|jpg)$`)

// Stored describes one stored file (one media row).
type Stored struct {
	Key         string
	ContentType string // image/webp | image/png | image/jpeg
	Bytes       int
	Width       int
	Height      int
}

// URL returns "/media/u/{key}".
func (s Stored) URL() string { return "/media/u/" + s.Key }

// Extensions and their content types.
const (
	ExtWebP = "webp"
	ExtPNG  = "png"
	ExtJPG  = "jpg"
)

var contentTypes = map[string]string{
	ExtWebP: "image/webp",
	ExtPNG:  "image/png",
	ExtJPG:  "image/jpeg",
}

// ContentTypeForExt returns the content type for a stored extension, or
// "" when ext is not one of webp, png, jpg.
func ContentTypeForExt(ext string) string { return contentTypes[ext] }

// ContentTypeForKey returns the content type implied by a key's extension
// ("" for an invalid key).
func ContentTypeForKey(key string) string {
	if !KeyRe.MatchString(key) {
		return ""
	}
	return contentTypes[path.Ext(key)[1:]]
}
