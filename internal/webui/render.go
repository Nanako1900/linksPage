package webui

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"html/template"
	"log/slog"
	"strconv"

	"github.com/Nanako1900/linksPage/internal/site"
)

// PageKind selects which public page is rendered.
type PageKind int

// Public page kinds.
const (
	PageHome PageKind = iota
	PagePrivacy
	PageCommunity
	PageNotFound
)

// Options configures a Renderer.
type Options struct {
	Assets     *Assets
	BaseURL    string
	AppVersion string
	Logger     *slog.Logger
	// Snapshot returns the current public snapshot (never nil).
	Snapshot func() *site.Snapshot
}

// Renderer renders the HTML shells. It is safe for concurrent use.
type Renderer struct {
	opts         Options
	tmpl         *template.Template
	bootHash     string
	criticalHash string
}

// NewRenderer parses templates and precomputes hashes of the fixed inline
// script and critical CSS.
func NewRenderer(opts Options) (*Renderer, error) {
	if opts.Assets == nil || opts.Snapshot == nil || opts.Logger == nil {
		return nil, fmt.Errorf("webui: Assets, Snapshot and Logger are required")
	}
	tmpl, err := template.ParseFS(templateFS, "templates/*.gohtml")
	if err != nil {
		return nil, fmt.Errorf("parse templates: %w", err)
	}
	if opts.Assets.Fallback() {
		opts.Logger.Warn("frontend build not embedded (internal/webui/dist is empty); serving minimal fallback pages")
	}
	return &Renderer{
		opts:         opts,
		tmpl:         tmpl,
		bootHash:     site.CSPHash(BootScript()),
		criticalHash: site.CSPHash(CriticalCSS()),
	}, nil
}

// page is a rendered HTML document plus the headers that must accompany
// it (including on 304 responses).
type page struct {
	body []byte
	csp  string
	etag string
}

type fallbackData struct {
	Code, Heading, Lede, Note, NoScript, LinkText string
}

type publicData struct {
	Lang, Appearance, Title, Description, Robots string
	CanonicalURL, SiteName, OGLocale             string
	ThemeColorLight, ThemeColorDark              string
	CriticalCSS                                  template.CSS
	ThemeCSS                                     template.CSS
	BootScript                                   template.JS
	Styles, Preloads                             []string
	Script                                       string
	HasData                                      bool
	Data                                         any
	Fallback                                     fallbackData
}

type bootstrapEnvelope struct {
	Data site.Bootstrap `json:"data"`
}

func (rd *Renderer) renderPublic(kind PageKind, path, langParam string) (page, error) {
	snap := rd.opts.Snapshot()
	s := snap.Settings
	locale := s.ResolveLocale(langParam)
	txt := textFor(locale)
	siteName := s.Title.Get(locale, s.DefaultLocale)
	d := publicData{
		Lang:            locale,
		Appearance:      s.Appearance,
		Title:           siteName,
		Description:     s.Description.Get(locale, s.DefaultLocale),
		CanonicalURL:    rd.opts.BaseURL + path,
		SiteName:        siteName,
		OGLocale:        ogLocale(locale),
		ThemeColorLight: s.Theme.ThemeColor(false),
		ThemeColorDark:  s.Theme.ThemeColor(true),
		CriticalCSS:     template.CSS(CriticalCSS()), //nolint:gosec // fixed embedded content, hashed for CSP
		ThemeCSS:        template.CSS(snap.ThemeCSS), //nolint:gosec // generated from validated hex/enum tokens
		BootScript:      template.JS(BootScript()),   //nolint:gosec // fixed embedded content, hashed for CSP
		Fallback:        fallbackData{Heading: siteName, Lede: s.Description.Get(locale, s.DefaultLocale), Note: txt.Loading, NoScript: txt.NoScript},
	}
	switch kind {
	case PagePrivacy:
		d.Title = txt.PrivacyTitle + " · " + siteName
		d.Fallback.Heading = txt.PrivacyTitle
	case PageNotFound:
		d.Title = txt.NotFoundTitle + " · " + siteName
		d.Robots = "noindex"
		d.Fallback = fallbackData{Code: "404", Heading: txt.NotFoundTitle, Lede: txt.NotFoundLede, LinkText: txt.BackHome}
	}
	if entry := rd.opts.Assets.public; entry != nil {
		d.Styles = entry.Styles
		if kind != PageNotFound {
			d.Script, d.Preloads = entry.Script, entry.Preloads
		}
	} else if kind != PageNotFound {
		d.Fallback.Note = txt.MissingBuild
	}
	if kind != PageNotFound {
		d.HasData = true
		d.Data = bootstrapEnvelope{Data: snap.Bootstrap()}
	}
	csp := PublicCSP(rd.bootHash, rd.criticalHash, snap.ThemeHash)
	return rd.execute("public", d, csp)
}

type adminData struct {
	Lang, Title, Script, NoScript, Note string
	Styles, Preloads                    []string
}

func (rd *Renderer) renderAdmin() (page, error) {
	snap := rd.opts.Snapshot()
	locale := snap.Settings.DefaultLocale
	txt := textFor(locale)
	d := adminData{Lang: locale, Title: txt.AdminTitle, NoScript: txt.AdminNoScript}
	if entry := rd.opts.Assets.admin; entry != nil {
		d.Script, d.Styles, d.Preloads = entry.Script, entry.Styles, entry.Preloads
	} else {
		d.Note = txt.MissingBuild
	}
	return rd.execute("admin", d, AdminCSP)
}

func (rd *Renderer) execute(name string, data any, csp string) (page, error) {
	var buf bytes.Buffer
	if err := rd.tmpl.ExecuteTemplate(&buf, name, data); err != nil {
		return page{}, fmt.Errorf("render %s: %w", name, err)
	}
	return page{body: buf.Bytes(), csp: csp, etag: rd.etag(csp, buf.Bytes())}, nil
}

// etag derives a weak validator from every input that affects the
// response: app version, manifest, CSP (which covers theme and inline
// hashes) and the rendered body (settings version, locale, page kind).
func (rd *Renderer) etag(csp string, body []byte) string {
	h := sha256.New()
	for _, part := range []string{rd.opts.AppVersion, rd.opts.Assets.ManifestHash(), csp, strconv.Itoa(len(body))} {
		h.Write([]byte(part))
		h.Write([]byte{0})
	}
	h.Write(body)
	return `W/"` + hex.EncodeToString(h.Sum(nil)[:16]) + `"`
}
