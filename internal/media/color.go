package media

import (
	"fmt"
	"image/color"
	"regexp"
	"strconv"
)

// Signal Paper (light) defaults used when GenerateInput leaves a colour
// empty.
const (
	DefaultBackgroundHex = "#f9f7f1"
	DefaultAccentHex     = "#5b4ad8"
)

var hexColorRe = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// palette holds the resolved generator colours.
type palette struct {
	background    color.NRGBA
	accent        color.NRGBA
	backgroundHex string
	accentHex     string
	themeHex      string
}

// resolvePalette validates the input colours; "" selects the default and
// anything else that is not #rrggbb is an error.
func resolvePalette(in GenerateInput) (palette, error) {
	bgHex, bg, err := parseHexOr(in.BackgroundHex, DefaultBackgroundHex)
	if err != nil {
		return palette{}, fmt.Errorf("media: background colour: %w", err)
	}
	acHex, ac, err := parseHexOr(in.AccentHex, DefaultAccentHex)
	if err != nil {
		return palette{}, fmt.Errorf("media: accent colour: %w", err)
	}
	themeHex, _, err := parseHexOr(in.ThemeColorHex, bgHex)
	if err != nil {
		return palette{}, fmt.Errorf("media: theme colour: %w", err)
	}
	return palette{background: bg, accent: ac, backgroundHex: bgHex, accentHex: acHex, themeHex: themeHex}, nil
}

func parseHexOr(s, fallback string) (string, color.NRGBA, error) {
	if s == "" {
		s = fallback
	}
	if !hexColorRe.MatchString(s) {
		return "", color.NRGBA{}, fmt.Errorf("%q is not #rrggbb", s)
	}
	v, err := strconv.ParseUint(s[1:], 16, 32)
	if err != nil {
		return "", color.NRGBA{}, fmt.Errorf("%q: %w", s, err)
	}
	c := color.NRGBA{R: uint8(v >> 16), G: uint8(v >> 8), B: uint8(v), A: 0xff} //nolint:gosec // G115: masked 8-bit channels of a 24-bit value
	return s, c, nil
}
