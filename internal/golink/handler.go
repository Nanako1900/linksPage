package golink

import (
	"context"
	"errors"
	"html/template"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Nanako1900/linksPage/internal/site"
	"github.com/Nanako1900/linksPage/internal/uaclass"
)

// ClickEvent describes one /go request. M1 does not record it.
type ClickEvent struct {
	Slug        string
	CommunityID string // "" for links
	Action      Action
	UA          uaclass.Class
	At          time.Time
}

// ClickHook is the analytics hook for /go (join_click / outbound). M3
// replaces NoopClickHook with the analytics recorder; it must never block
// or fail the redirect, and rate limiting only skips counting (never 429).
type ClickHook interface {
	OnClick(ctx context.Context, ev ClickEvent)
}

// NoopClickHook records nothing (M1: /go does not count yet).
type NoopClickHook struct{}

// OnClick implements ClickHook.
func (NoopClickHook) OnClick(context.Context, ClickEvent) {}

// HandlerOptions configure the /go handler.
type HandlerOptions struct {
	Resolver *Resolver
	// Snapshot returns the current public snapshot (copy texts, other
	// communities for the unavailable page, locale, theme colors).
	Snapshot func() *site.Snapshot
	// BaseURL is config base_url (absolute URLs on guide pages).
	BaseURL string
	Hook    ClickHook
	Logger  *slog.Logger
}

// handler serves GET /go/{slug}.
type handler struct {
	resolver *Resolver
	snapshot func() *site.Snapshot
	baseURL  string
	hook     ClickHook
	logger   *slog.Logger
	tmpl     *template.Template
	styles   styleCache
}

// NewHandler returns the GET /go/{slug} handler (slug from
// r.PathValue("slug"), validated with the slug regex; invalid → 404).
// Guide pages are self-contained HTML (html/template, hash CSP, no
// external scripts) with Cache-Control: no-store and X-Robots-Tag: noindex.
func NewHandler(opts HandlerOptions) (http.Handler, error) {
	if opts.Resolver == nil || opts.Resolver.src == nil || opts.Snapshot == nil {
		return nil, errors.New("golink: NewHandler needs a Resolver with a Source and a Snapshot func")
	}
	base, err := normalizeBaseURL(opts.BaseURL)
	if err != nil {
		return nil, err
	}
	tmpl, err := parseGuideTemplate()
	if err != nil {
		return nil, err
	}
	h := &handler{
		resolver: opts.Resolver, snapshot: opts.Snapshot, baseURL: base,
		hook: opts.Hook, logger: opts.Logger, tmpl: tmpl,
	}
	if h.hook == nil {
		h.hook = NoopClickHook{}
	}
	if h.logger == nil {
		h.logger = slog.New(slog.DiscardHandler)
	}
	return h, nil
}

// normalizeBaseURL requires an absolute http(s) URL and strips the
// trailing slash.
func normalizeBaseURL(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil {
		return "", errors.New("golink: BaseURL must be an absolute http(s) URL")
	}
	return strings.TrimRight(raw, "/"), nil
}

// ServeHTTP implements http.Handler. It never returns 429.
func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	hdr := w.Header()
	hdr.Set("Cache-Control", "no-store")
	hdr.Set("X-Robots-Tag", "noindex")
	ua := uaclass.Classify(r.UserAgent())
	slug := r.PathValue("slug")
	d, err := h.resolver.Resolve(r.Context(), slug, ua)
	if err != nil {
		h.logger.ErrorContext(r.Context(), "golink: resolve failed", "error", err)
		hdr.Set("Retry-After", "5")
		h.renderPage(w, r, Decision{Action: kindError, Slug: slug})
		return
	}
	h.hook.OnClick(r.Context(), ClickEvent{
		Slug: d.Slug, CommunityID: d.CommunityID, Action: d.Action, UA: ua, At: h.resolver.now(),
	})
	if d.Action == ActionRedirect {
		hdr.Set("Location", d.URL)
		w.WriteHeader(http.StatusFound)
		return
	}
	h.renderPage(w, r, d)
}

// renderPage renders the guide page of d in the negotiated language.
func (h *handler) renderPage(w http.ResponseWriter, r *http.Request, d Decision) {
	snap := h.snapshot()
	if snap == nil || snap.Public == nil {
		snap = site.DefaultSnapshot()
	}
	s := snap.Settings
	locale := negotiateLocale(s.Locales, s.DefaultLocale, r.URL.Query().Get("lang"), r.Header.Get("Accept-Language"))
	pc := pageContext{snap: snap, locale: locale, fallback: s.DefaultLocale, text: textFor(locale), baseURL: h.baseURL}
	var v pageView
	if d.Action == kindError {
		v = pageView{Kind: kindError, T: pc.text, SiteTitle: pc.get(s.Title), Heading: pc.text.ErrorTitle, Lede: pc.text.ErrorLede}
	} else {
		v = buildView(pc, d)
	}
	style := h.styles.get(snap)
	h.writePage(w, decorate(v, snap, locale, style), style)
}
