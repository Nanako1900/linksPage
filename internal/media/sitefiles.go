package media

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// Site file limits and defaults.
const (
	CacheSiteFiles = "public, max-age=3600"
	// SiteFilesRetryAfter is sent with 503 before the first page build.
	SiteFilesRetryAfter = "5"
	DefaultAppName      = "LinksPage"
	maxNameRunes        = 100
	maxShortNameRunes   = 12
)

// SiteFiles are served from memory with Cache-Control: public,
// max-age=3600 and an ETag.
type SiteFiles struct {
	FaviconICO []byte // the 32px PNG served at /favicon.ico (image/png)
	Manifest   []byte // /site.webmanifest (application/manifest+json)
	RobotsTxt  []byte // /robots.txt (text/plain)
	// ETag is the unquoted hex digest of all three files; the handler
	// sends `"<ETag>-<file>"`.
	ETag string
}

// SiteFile selects a file for SiteFilesHandler.
type SiteFile int

// Site files.
const (
	FileFavicon SiteFile = iota
	FileManifest
	FileRobots
)

type siteFileSpec struct {
	suffix      string
	contentType string
	body        func(*SiteFiles) []byte
}

var siteFileSpecs = map[SiteFile]siteFileSpec{
	FileFavicon:  {"ico", "image/png", func(f *SiteFiles) []byte { return f.FaviconICO }},
	FileManifest: {"manifest", "application/manifest+json", func(f *SiteFiles) []byte { return f.Manifest }},
	FileRobots:   {"robots", "text/plain; charset=utf-8", func(f *SiteFiles) []byte { return f.RobotsTxt }},
}

// SiteFilesHandler serves one of /favicon.ico, /site.webmanifest,
// /robots.txt from current() (never nil once the page was built; before
// that it answers 503 with Retry-After). It panics on an unknown file or
// a nil current, which are wiring bugs.
func SiteFilesHandler(file SiteFile, current func() *SiteFiles) http.Handler {
	spec, ok := siteFileSpecs[file]
	if !ok || current == nil {
		panic("media: SiteFilesHandler: unknown file or nil source " + strconv.Itoa(int(file)))
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		files := current()
		if files == nil || len(spec.body(files)) == 0 {
			w.Header().Set("Retry-After", SiteFilesRetryAfter)
			writeText(w, http.StatusServiceUnavailable, CacheNoStore, "starting up")
			return
		}
		h := w.Header()
		h.Set("Content-Type", spec.contentType)
		h.Set("Cache-Control", CacheSiteFiles)
		h.Set("ETag", `"`+files.ETag+"-"+spec.suffix+`"`)
		setSafetyHeaders(h)
		http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(spec.body(files)))
	})
}

// newSiteFiles assembles the in-memory files and their ETag.
func newSiteFiles(favicon32, manifest, robots []byte) SiteFiles {
	sum := sha256.New()
	for _, b := range [][]byte{favicon32, manifest, robots} {
		_, _ = fmt.Fprintf(sum, "%d:", len(b))
		_, _ = sum.Write(b)
	}
	return SiteFiles{
		FaviconICO: favicon32,
		Manifest:   manifest,
		RobotsTxt:  robots,
		ETag:       hex.EncodeToString(sum.Sum(nil))[:32],
	}
}

type manifestIcon struct {
	Src   string `json:"src"`
	Sizes string `json:"sizes"`
	Type  string `json:"type"`
}

type webManifest struct {
	ID              string         `json:"id"`
	Name            string         `json:"name"`
	ShortName       string         `json:"short_name"`
	StartURL        string         `json:"start_url"`
	Scope           string         `json:"scope"`
	Display         string         `json:"display"`
	BackgroundColor string         `json:"background_color"`
	ThemeColor      string         `json:"theme_color"`
	Icons           []manifestIcon `json:"icons"`
}

// manifestIconSizes are the favicon sizes listed in site.webmanifest.
var manifestIconSizes = []int{192, 512}

func buildManifest(in GenerateInput, colors palette, favicons map[int]Stored) ([]byte, error) {
	name := truncateRunes(strings.TrimSpace(in.Name), maxNameRunes)
	if name == "" {
		name = DefaultAppName
	}
	short := truncateRunes(strings.TrimSpace(in.ShortName), maxShortNameRunes)
	if short == "" {
		short = truncateRunes(name, maxShortNameRunes)
	}
	m := webManifest{
		ID: "/", Name: name, ShortName: short, StartURL: "/", Scope: "/",
		Display:         "minimal-ui",
		BackgroundColor: colors.backgroundHex,
		ThemeColor:      colors.themeHex,
		Icons:           make([]manifestIcon, 0, len(manifestIconSizes)),
	}
	for _, size := range manifestIconSizes {
		s, ok := favicons[size]
		if !ok {
			return nil, fmt.Errorf("media: manifest: missing %dpx favicon", size)
		}
		dim := strconv.Itoa(size)
		m.Icons = append(m.Icons, manifestIcon{Src: s.URL(), Sizes: dim + "x" + dim, Type: s.ContentType})
	}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("media: manifest: %w", err)
	}
	return append(b, '\n'), nil
}

// buildRobots honours searchIndexing: "index" allows the public pages but
// keeps crawlers off the API and redirect endpoints; "noindex" disallows
// everything. M1 has no sitemap.
func buildRobots(index bool) []byte {
	if !index {
		return []byte("User-agent: *\nDisallow: /\n")
	}
	return []byte("User-agent: *\nAllow: /\nDisallow: /api/\nDisallow: /go/\nDisallow: /admin\n")
}

func truncateRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return strings.TrimSpace(string(r[:n]))
}
