package site

import (
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Font stacks are selected by id; admin input is never written into CSS.
var fontStacks = map[string]string{
	"system":           `system-ui,-apple-system,"Segoe UI",Roboto,"Helvetica Neue",Arial,"PingFang SC","HarmonyOS Sans SC","MiSans","Microsoft YaHei","Noto Sans CJK SC",sans-serif`,
	"instrument-serif": `"Instrument Serif","Songti SC","STSong","Noto Serif CJK SC","Source Han Serif SC",Georgia,serif`,
	"mono":             `ui-monospace,SFMono-Regular,Menlo,Consolas,"Noto Sans Mono CJK SC",monospace`,
}

// Palette is one set of color tokens (hex only).
type Palette struct {
	Bg       string `json:"bg"`
	Fg       string `json:"fg"`
	Muted    string `json:"muted"`
	Card     string `json:"card"`
	Border   string `json:"border"`
	Accent   string `json:"accent"`
	AccentFg string `json:"accentFg"`
}

// Theme holds the runtime theme tokens.
type Theme struct {
	Preset      string  `json:"preset"`
	Light       Palette `json:"light"`
	Dark        Palette `json:"dark"`
	Radius      string  `json:"radius"`
	FontSans    string  `json:"fontSans"`
	FontDisplay string  `json:"fontDisplay"`
}

// DefaultTheme is the "Signal Paper" preset (sRGB hex baseline).
func DefaultTheme() Theme {
	return Theme{
		Preset: "signal-paper",
		Light: Palette{
			Bg: "#f9f7f1", Fg: "#1a1c22", Muted: "#5f6270", Card: "#ffffff",
			Border: "#e4e1d8", Accent: "#5b4ad8", AccentFg: "#ffffff",
		},
		Dark: Palette{
			Bg: "#111216", Fg: "#ecebe6", Muted: "#9a9cab", Card: "#1a1b21",
			Border: "#2c2e36", Accent: "#8b7ff0", AccentFg: "#111216",
		},
		Radius:      "0.75rem",
		FontSans:    "system",
		FontDisplay: "instrument-serif",
	}
}

var (
	hexColorRe = regexp.MustCompile(`^#([0-9a-fA-F]{3}|[0-9a-fA-F]{6})$`)
	radiusRe   = regexp.MustCompile(`^(\d{1,2}(?:\.\d{1,3})?)(rem|px)$`)
	presetRe   = regexp.MustCompile(`^[a-z][a-z0-9-]{0,31}$`)
)

// maxRadiusRem is the upper bound for --lp-radius.
const maxRadiusRem = 2.0

// NormalizeHex validates a hex color and returns it as lowercase #rrggbb.
func NormalizeHex(c string) (string, error) {
	if !hexColorRe.MatchString(c) {
		return "", errors.New("must be a hex color (#rgb or #rrggbb)")
	}
	c = strings.ToLower(c)
	if len(c) == 4 {
		c = "#" + strings.Repeat(c[1:2], 2) + strings.Repeat(c[2:3], 2) + strings.Repeat(c[3:4], 2)
	}
	return c, nil
}

func (p Palette) fields() []struct{ name, css, value string } {
	return []struct{ name, css, value string }{
		{"bg", "--lp-bg", p.Bg},
		{"fg", "--lp-fg", p.Fg},
		{"muted", "--lp-muted", p.Muted},
		{"card", "--lp-card", p.Card},
		{"border", "--lp-border", p.Border},
		{"accent", "--lp-accent", p.Accent},
		{"accentFg", "--lp-accent-fg", p.AccentFg},
	}
}

func validateRadius(r string) error {
	m := radiusRe.FindStringSubmatch(r)
	if m == nil {
		return errors.New("theme.radius: must be a length in rem or px")
	}
	v, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return errors.New("theme.radius: invalid number")
	}
	if m[2] == "px" {
		v /= 16
	}
	if v < 0 || v > maxRadiusRem {
		return errors.New("theme.radius: must be between 0 and 2rem")
	}
	return nil
}

// Validate checks all tokens: hex colors only, bounded radius and font
// ids from the allow-list.
func (t Theme) Validate() error {
	var errs []error
	if !presetRe.MatchString(t.Preset) {
		errs = append(errs, errors.New("theme.preset: invalid id"))
	}
	for mode, p := range map[string]Palette{"light": t.Light, "dark": t.Dark} {
		for _, f := range p.fields() {
			if _, err := NormalizeHex(f.value); err != nil {
				errs = append(errs, fmt.Errorf("theme.%s.%s: %w", mode, f.name, err))
			}
		}
	}
	if err := validateRadius(t.Radius); err != nil {
		errs = append(errs, err)
	}
	if _, ok := fontStacks[t.FontSans]; !ok {
		errs = append(errs, errors.New("theme.fontSans: unknown font id"))
	}
	if _, ok := fontStacks[t.FontDisplay]; !ok {
		errs = append(errs, errors.New("theme.fontDisplay: unknown font id"))
	}
	return errors.Join(errs...)
}

// CSS renders the theme as the content of <style id="lp-theme">. The
// theme must be valid; invalid tokens yield an error.
func (t Theme) CSS() (string, error) {
	if err := t.Validate(); err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString(":root{")
	writePalette(&b, t.Light)
	b.WriteString("--lp-radius:" + t.Radius + ";")
	b.WriteString("--lp-font-sans:" + fontStacks[t.FontSans] + ";")
	b.WriteString("--lp-font-display:" + fontStacks[t.FontDisplay] + "}")
	b.WriteString(`[data-appearance="dark"]{`)
	writePalette(&b, t.Dark)
	b.WriteString("}")
	return b.String(), nil
}

func writePalette(b *strings.Builder, p Palette) {
	for _, f := range p.fields() {
		v, _ := NormalizeHex(f.value) // validated by caller
		b.WriteString(f.css + ":" + v + ";")
	}
}

// ThemeColor returns the normalized background color for a mode.
func (t Theme) ThemeColor(dark bool) string {
	src := t.Light.Bg
	if dark {
		src = t.Dark.Bg
	}
	v, err := NormalizeHex(src)
	if err != nil {
		return ""
	}
	return v
}

// CSPHash returns the CSP source expression for inline content, e.g.
// 'sha256-…' (including the single quotes).
func CSPHash(content string) string {
	sum := sha256.Sum256([]byte(content))
	return "'sha256-" + base64.StdEncoding.EncodeToString(sum[:]) + "'"
}
