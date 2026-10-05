package golink

import (
	"bytes"
	_ "embed"
	"fmt"
	"html/template"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/Nanako1900/linksPage/internal/site"
)

var (
	//go:embed assets/guide.gohtml
	guideTemplateSrc string
	//go:embed assets/guide.css
	guideCSSSrc string
	//go:embed assets/copy.js
	copyScriptSrc string
)

// Inline assets exactly as served (their hashes go into the CSP).
var (
	guideCSS   = strings.TrimSpace(guideCSSSrc)
	copyScript = strings.TrimSpace(copyScriptSrc)
	scriptHash = site.CSPHash(copyScript)
)

func parseGuideTemplate() (*template.Template, error) {
	t, err := template.New("guide").Parse(guideTemplateSrc)
	if err != nil {
		return nil, fmt.Errorf("golink: parse guide template: %w", err)
	}
	return t, nil
}

// pageStyle is the page CSS for one theme/appearance and its CSP hash.
type pageStyle struct {
	key       string
	themeCSS  string
	themeHash string
	css       string
	cssHash   string
}

// styleCache keeps the style of the most recent theme (themes change
// rarely, so one entry is enough and memory stays bounded).
type styleCache struct {
	p atomic.Pointer[pageStyle]
}

func (c *styleCache) get(snap *site.Snapshot) *pageStyle {
	auto := appearanceAttr(snap.Settings.Appearance) == ""
	key := snap.ThemeCSS + "|" + strconv.FormatBool(auto)
	if s := c.p.Load(); s != nil && s.key == key {
		return s
	}
	css := guideCSS
	if auto {
		css += autoDarkCSS(snap.Settings.Theme.Dark)
	}
	s := &pageStyle{
		key: key, themeCSS: snap.ThemeCSS, themeHash: site.CSPHash(snap.ThemeCSS),
		css: css, cssHash: site.CSPHash(css),
	}
	c.p.Store(s)
	return s
}

// appearanceAttr returns the data-appearance value for fixed modes and
// "" when the page follows the system preference.
func appearanceAttr(mode string) string {
	switch mode {
	case site.AppearanceLight, site.AppearanceDark:
		return mode
	default:
		return ""
	}
}

// autoDarkCSS applies the dark palette under prefers-color-scheme: dark
// (the guide pages run no theme script). Invalid tokens are skipped; the
// palette was validated when the snapshot was built.
func autoDarkCSS(p site.Palette) string {
	vars := []struct{ name, value string }{
		{"--lp-bg", p.Bg},
		{"--lp-fg", p.Fg},
		{"--lp-muted", p.Muted},
		{"--lp-card", p.Card},
		{"--lp-border", p.Border},
		{"--lp-accent", p.Accent},
		{"--lp-accent-fg", p.AccentFg},
	}
	var b strings.Builder
	b.WriteString("@media (prefers-color-scheme:dark){:root{")
	for _, v := range vars {
		if hex, err := site.NormalizeHex(v.value); err == nil {
			b.WriteString(v.name + ":" + hex + ";")
		}
	}
	b.WriteString("}}")
	return b.String()
}

// guideCSP is the Content-Security-Policy of a guide page: nothing but
// the hashed inline blocks and same-origin images (QR codes).
func guideCSP(s *pageStyle) string {
	return strings.Join([]string{
		"default-src 'none'",
		"script-src " + scriptHash,
		"style-src " + s.themeHash + " " + s.cssHash,
		"img-src 'self'",
		"base-uri 'none'",
		"form-action 'none'",
		"frame-ancestors 'none'",
		"object-src 'none'",
	}, "; ")
}

// decorate fills the shared (non action-specific) part of the model.
func decorate(v pageView, snap *site.Snapshot, locale string, s *pageStyle) pageView {
	v.Lang = locale
	v.Appearance = appearanceAttr(snap.Settings.Appearance)
	v.ThemeColor = snap.Settings.Theme.ThemeColor(v.Appearance == site.AppearanceDark)
	v.ThemeCSS = template.CSS(s.themeCSS) //nolint:gosec // G203: generated from validated theme tokens
	v.CSS = template.CSS(s.css)           //nolint:gosec // G203: embedded stylesheet plus validated hex colors
	v.Script = template.JS(copyScript)    //nolint:gosec // G203: embedded constant script
	v.Title = v.Heading
	if v.SiteTitle != "" && v.SiteTitle != v.Heading {
		v.Title = v.Heading + " · " + v.SiteTitle
	}
	return v
}

// writePage renders v and writes it with the page CSP.
func (h *handler) writePage(w http.ResponseWriter, v pageView, s *pageStyle) {
	var buf bytes.Buffer
	if err := h.tmpl.Execute(&buf, v); err != nil {
		h.logger.Error("golink: render guide page", "kind", v.Kind, "error", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	hdr := w.Header()
	hdr.Set("Content-Type", "text/html; charset=utf-8")
	hdr.Set("Content-Security-Policy", guideCSP(s))
	hdr.Set("Content-Length", strconv.Itoa(buf.Len()))
	w.WriteHeader(statusFor(v.Kind))
	if _, err := w.Write(buf.Bytes()); err != nil {
		h.logger.Debug("golink: write guide page", "error", err)
	}
}
