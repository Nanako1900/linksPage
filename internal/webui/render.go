package webui

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"log/slog"
	"strconv"
	"strings"

	"github.com/Nanako1900/linksPage/internal/content"
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

// MarkdownRenderer renders the public Markdown subset to sanitized HTML
// (*content.Markdown satisfies it).
type MarkdownRenderer interface {
	Render(src string) (string, error)
}

// Options configures a Renderer.
type Options struct {
	Assets     *Assets
	BaseURL    string
	AppVersion string
	Logger     *slog.Logger
	// Snapshot returns the current public snapshot (never nil).
	Snapshot func() *site.Snapshot
	// Markdown renders bio, footer and text blocks in the fallback markup;
	// nil uses content.NewMarkdown().
	Markdown MarkdownRenderer
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
		return nil, errors.New("webui: Assets, Snapshot and Logger are required")
	}
	if opts.Markdown == nil {
		opts.Markdown = content.NewMarkdown()
	}
	opts.BaseURL = strings.TrimSuffix(opts.BaseURL, "/")
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
	body   []byte
	csp    string
	etag   string
	status int
}

// pageCtx is the resolved request: snapshot, page kind and locale.
type pageCtx struct {
	snap      *site.Snapshot
	pub       *site.PublicPage
	kind      PageKind
	path      string
	locale    string
	txt       uiStrings
	community *site.CommunityView // PageCommunity only
}

func (pc pageCtx) defaultLocale() string { return pc.snap.Settings.DefaultLocale }

// text localizes t for the request locale.
func (pc pageCtx) text(t site.LocalizedText) string { return t.Get(pc.locale, pc.defaultLocale()) }

// resolve negotiates the locale and resolves the community for
// PageCommunity; an unknown slug turns the request into PageNotFound.
func (rd *Renderer) resolve(kind PageKind, slug, langParam, acceptLanguage string) pageCtx {
	snap := rd.opts.Snapshot()
	pub := snap.Public
	if pub == nil {
		pub = site.EmptyPublicPage(snap.Version, snap.Page, snap.Settings, rd.opts.BaseURL, snap.BuiltAt)
	}
	s := snap.Settings
	locale := negotiateLocale(s.Locales, s.DefaultLocale, langParam, acceptLanguage)
	pc := pageCtx{snap: snap, pub: pub, kind: kind, locale: locale, txt: textFor(locale)}
	switch kind {
	case PageHome:
		pc.path = "/"
	case PagePrivacy:
		pc.path = "/privacy"
	case PageCommunity:
		pc.community = findCommunity(pub, slug)
		if pc.community == nil {
			pc.kind = PageNotFound
		}
		pc.path = site.PathCommunity + slug
	}
	return pc
}

func findCommunity(pub *site.PublicPage, slug string) *site.CommunityView {
	if !slugRe.MatchString(slug) {
		return nil
	}
	for _, c := range pub.Communities {
		if c.Slug == slug {
			return &c
		}
	}
	return nil
}

// rendered holds the parts shared by the HTML document and RenderDTO, so
// both always carry the same head, fallback, data, CSP and ETag.
type rendered struct {
	status     int
	lang       string
	appearance string
	head       string
	fallback   string
	data       []byte // {"data": PublicPage}; nil for 404
	public     *site.PublicPage
	csp        string
	etag       string
}

type publicEnvelope struct {
	Data *site.PublicPage `json:"data"`
}

func (rd *Renderer) renderParts(pc pageCtx) (rendered, error) {
	r := rendered{status: 200, lang: pc.locale, appearance: pc.snap.Settings.Appearance}
	head, err := rd.executeString("head", rd.headView(pc))
	if err != nil {
		return rendered{}, err
	}
	fallback, err := rd.executeString("fallback", rd.fallbackView(pc))
	if err != nil {
		return rendered{}, err
	}
	r.head, r.fallback = head, fallback
	if pc.kind == PageNotFound {
		r.status = 404
	} else {
		// json.Marshal escapes <, >, &, U+2028 and U+2029, so the payload
		// is safe inside <script type="application/json">.
		data, err := json.Marshal(publicEnvelope{Data: pc.pub})
		if err != nil {
			return rendered{}, fmt.Errorf("encode public page: %w", err)
		}
		r.data, r.public = data, pc.pub
	}
	frame := r.public != nil && r.public.NeedsDiscordFrame()
	r.csp = PublicCSP(rd.bootHash, rd.criticalHash, pc.snap.ThemeHash, frame)
	r.etag = rd.publicETag(r)
	return r, nil
}

// publicETag derives a weak validator from every input of the public
// response: app version, manifest (entry assets), CSP, locale, status and
// the rendered head, fallback and data.
func (rd *Renderer) publicETag(r rendered) string {
	h := sha256.New()
	for _, part := range []string{
		rd.opts.AppVersion, rd.opts.Assets.ManifestHash(), r.csp, r.lang,
		strconv.Itoa(r.status), r.appearance, r.head, r.fallback,
	} {
		h.Write([]byte(part))
		h.Write([]byte{0})
	}
	h.Write(r.data)
	return `W/"` + hex.EncodeToString(h.Sum(nil)[:16]) + `"`
}

type documentData struct {
	Lang, Appearance string
	Head, Fallback   template.HTML
	Data             template.JS
	Styles, Preloads []string
	Script           string
}

// renderPublic renders the full HTML document for a resolved request.
func (rd *Renderer) renderPublic(pc pageCtx) (page, error) {
	r, err := rd.renderParts(pc)
	if err != nil {
		return page{}, err
	}
	d := documentData{
		Lang:       r.lang,
		Appearance: r.appearance,
		Head:       template.HTML(r.head),     //nolint:gosec // produced by html/template
		Fallback:   template.HTML(r.fallback), //nolint:gosec // produced by html/template
		Data:       template.JS(r.data),       //nolint:gosec // json.Marshal output (HTML-escaped)
	}
	if entry := rd.opts.Assets.public; entry != nil {
		d.Styles = entry.Styles
		if pc.kind != PageNotFound {
			d.Script, d.Preloads = entry.Script, entry.Preloads
		}
	}
	var buf bytes.Buffer
	if err := rd.tmpl.ExecuteTemplate(&buf, "public", d); err != nil {
		return page{}, fmt.Errorf("render public: %w", err)
	}
	return page{body: buf.Bytes(), csp: r.csp, etag: r.etag, status: r.status}, nil
}

func (rd *Renderer) executeString(name string, data any) (string, error) {
	var buf strings.Builder
	if err := rd.tmpl.ExecuteTemplate(&buf, name, data); err != nil {
		return "", fmt.Errorf("render %s: %w", name, err)
	}
	return buf.String(), nil
}

// markdown renders src, falling back to escaped plain text when the
// renderer fails (the page must stay up).
func (rd *Renderer) markdown(src string) template.HTML {
	if src == "" {
		return ""
	}
	out, err := rd.opts.Markdown.Render(src)
	if err != nil {
		rd.opts.Logger.Debug("render markdown fallback", slog.Any("error", err))
		return template.HTML("<p>" + template.HTMLEscapeString(src) + "</p>") //nolint:gosec // escaped
	}
	return template.HTML(out) //nolint:gosec // sanitized by content.Markdown (bluemonday)
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
	var buf bytes.Buffer
	if err := rd.tmpl.ExecuteTemplate(&buf, "admin", d); err != nil {
		return page{}, fmt.Errorf("render admin: %w", err)
	}
	return page{body: buf.Bytes(), csp: AdminCSP, etag: rd.adminETag(buf.Bytes()), status: 200}, nil
}

// adminETag derives a weak validator for the admin shell.
func (rd *Renderer) adminETag(body []byte) string {
	h := sha256.New()
	for _, part := range []string{rd.opts.AppVersion, rd.opts.Assets.ManifestHash(), AdminCSP} {
		h.Write([]byte(part))
		h.Write([]byte{0})
	}
	h.Write(body)
	return `W/"` + hex.EncodeToString(h.Sum(nil)[:16]) + `"`
}
