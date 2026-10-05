package media

import (
	"image"
	"image/color"
	"image/draw"
	"math"

	xdraw "golang.org/x/image/draw"
)

// fitWithin returns w×h scaled down (never up) so the longer side is at
// most maxSide; maxSide ≤ 0 keeps the size.
func fitWithin(w, h, maxSide int) (int, int) {
	if maxSide <= 0 || (w <= maxSide && h <= maxSide) {
		return w, h
	}
	if w >= h {
		return maxSide, max(1, int(math.Round(float64(h)*float64(maxSide)/float64(w))))
	}
	return max(1, int(math.Round(float64(w)*float64(maxSide)/float64(h)))), maxSide
}

// resize scales src to w×h with s; it returns src unchanged when the size
// already matches.
func resize(src *image.NRGBA, w, h int, s xdraw.Scaler) *image.NRGBA {
	if src.Rect.Dx() == w && src.Rect.Dy() == h {
		return src
	}
	dst := image.NewNRGBA(image.Rect(0, 0, w, h))
	s.Scale(dst, dst.Bounds(), src, src.Bounds(), xdraw.Src, nil)
	return dst
}

// coverRect returns the centred sub-rectangle of a w×h image with the
// aspect ratio tw:th (CSS object-fit: cover).
func coverRect(w, h, tw, th int) image.Rectangle {
	// Compare w/h with tw/th without floating point.
	if int64(w)*int64(th) > int64(h)*int64(tw) {
		cw := max(1, int(int64(h)*int64(tw)/int64(th)))
		x := (w - cw) / 2
		return image.Rect(x, 0, x+cw, h)
	}
	ch := max(1, int(int64(w)*int64(th)/int64(tw)))
	y := (h - ch) / 2
	return image.Rect(0, y, w, y+ch)
}

// cover crops src to the aspect ratio of tw×th and scales it to exactly
// tw×th with s.
func cover(src *image.NRGBA, tw, th int, s xdraw.Scaler) *image.NRGBA {
	r := coverRect(src.Rect.Dx(), src.Rect.Dy(), tw, th)
	dst := image.NewNRGBA(image.Rect(0, 0, tw, th))
	s.Scale(dst, dst.Bounds(), src, r, xdraw.Src, nil)
	return dst
}

// squareCrop returns the centred square of src (shared pixels).
func squareCrop(src *image.NRGBA) *image.NRGBA {
	r := coverRect(src.Rect.Dx(), src.Rect.Dy(), 1, 1)
	sub, ok := src.SubImage(r).(*image.NRGBA)
	if !ok {
		return src
	}
	return toNRGBA(sub)
}

// flatten composites src over an opaque background colour.
func flatten(src image.Image, bg color.Color) *image.NRGBA {
	b := src.Bounds()
	dst := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(dst, dst.Bounds(), image.NewUniform(bg), image.Point{}, draw.Src)
	draw.Draw(dst, dst.Bounds(), src, b.Min, draw.Over)
	return dst
}

// diskMask returns an anti-aliased circular alpha mask of radius r
// centred in a (2r)×(2r) square.
func diskMask(r int) *image.Alpha {
	side := 2 * r
	m := image.NewAlpha(image.Rect(0, 0, side, side))
	c := float64(r)
	for y := range side {
		for x := range side {
			d := math.Hypot(float64(x)+0.5-c, float64(y)+0.5-c)
			a := math.Max(0, math.Min(1, c-d+0.5))
			m.Pix[y*m.Stride+x] = uint8(math.Round(a * 255))
		}
	}
	return m
}

// drawDisk draws src (from srcPt) clipped to a circle of radius r centred
// at center.
func drawDisk(dst draw.Image, center image.Point, r int, src image.Image, srcPt image.Point) {
	rect := image.Rect(center.X-r, center.Y-r, center.X+r, center.Y+r)
	draw.DrawMask(dst, rect, src, srcPt, diskMask(r), image.Point{}, draw.Over)
}
