package media

import (
	"image"
	"image/color"
	"math"

	xdraw "golang.org/x/image/draw"
)

// AppleTouchSize is the favicon size flattened onto the background
// (iOS renders transparent touch icons on black).
const AppleTouchSize = 180

// FaviconICOSize is the favicon served at /favicon.ico.
const FaviconICOSize = 32

// Geometric fallback mark, in fractions of the icon side: an accent
// rounded square with a background-coloured ring around an accent dot.
const (
	markCornerRadius = 0.22
	markRingOuter    = 0.28
	markDotRadius    = 0.12
	markSupersample  = 4
)

// renderFavicons returns one PNG per FaviconSizes entry, from src's centre
// square or, when src is nil, the geometric mark.
func renderFavicons(src *image.NRGBA, colors palette) (map[int]encoded, error) {
	var sq *image.NRGBA
	if src != nil {
		sq = squareCrop(src)
	}
	out := make(map[int]encoded, len(FaviconSizes))
	for _, size := range FaviconSizes {
		var img *image.NRGBA
		if sq != nil {
			img = resize(sq, size, size, xdraw.CatmullRom)
		} else {
			img = geometricMark(size, colors, size != AppleTouchSize)
		}
		if size == AppleTouchSize {
			img = flatten(img, colors.background)
		}
		e, err := encodePNG(img)
		if err != nil {
			return nil, err
		}
		out[size] = e
	}
	return out, nil
}

// geometricMark draws the fallback favicon with 4×4 supersampling.
func geometricMark(size int, colors palette, rounded bool) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	radius := 0.0
	if rounded {
		radius = markCornerRadius
	}
	const n = markSupersample
	for py := range size {
		for px := range size {
			var r, g, b, a float64
			for sy := range n {
				for sx := range n {
					x := (float64(px) + (float64(sx)+0.5)/n) / float64(size)
					y := (float64(py) + (float64(sy)+0.5)/n) / float64(size)
					if c, ok := markColor(x, y, radius, colors); ok {
						r, g, b, a = r+float64(c.R), g+float64(c.G), b+float64(c.B), a+1
					}
				}
			}
			img.SetNRGBA(px, py, averageColor(r, g, b, a, n*n))
		}
	}
	return img
}

// markColor returns the colour at (x, y) in unit coordinates, or false
// outside the rounded square.
func markColor(x, y, radius float64, colors palette) (color.NRGBA, bool) {
	if !inRoundedSquare(x, y, radius) {
		return color.NRGBA{}, false
	}
	d := math.Hypot(x-0.5, y-0.5)
	if d > markDotRadius && d <= markRingOuter {
		return colors.background, true
	}
	return colors.accent, true
}

func inRoundedSquare(x, y, r float64) bool {
	if r <= 0 {
		return true
	}
	cx := math.Max(r, math.Min(1-r, x))
	cy := math.Max(r, math.Min(1-r, y))
	return math.Hypot(x-cx, y-cy) <= r
}

// averageColor turns summed covered samples into a straight-alpha colour.
func averageColor(r, g, b, covered float64, total int) color.NRGBA {
	if covered == 0 {
		return color.NRGBA{}
	}
	return color.NRGBA{
		R: uint8(math.Round(r / covered)),
		G: uint8(math.Round(g / covered)),
		B: uint8(math.Round(b / covered)),
		A: uint8(math.Round(255 * covered / float64(total))),
	}
}
