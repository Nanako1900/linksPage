// Package webui serves the embedded Vite build and renders the HTML
// shells with html/template (title, OG tags, runtime theme, inline boot
// script, bootstrap data and server-side fallback markup).
package webui

import (
	"embed"
	"fmt"
	"io/fs"
	"strings"
)

// dist is populated from web/dist at build time (make web-dist or the
// Dockerfile). dist/.gitkeep keeps the directive compiling when empty.
//
//go:embed all:dist
var dist embed.FS

//go:embed templates/*.gohtml
var templateFS embed.FS

//go:embed static/boot.js
var bootJS string

//go:embed static/critical.css
var criticalCSS string

// DistFS returns the embedded dist directory.
func DistFS() fs.FS {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(fmt.Sprintf("embedded dist: %v", err)) // unreachable: path is static
	}
	return sub
}

// BootScript is the fixed inline boot script (resolves data-appearance).
func BootScript() string { return strings.TrimSpace(bootJS) }

// CriticalCSS is the fixed inline critical CSS (no @layer, hex colors).
func CriticalCSS() string { return strings.TrimSpace(criticalCSS) }
