package media

import (
	"image"
	"image/draw"

	xdraw "golang.org/x/image/draw"
)

// Generated OG layout: the avatar as a circle in the middle of the
// background with an accent ring. No text, so no CJK font is needed.
const (
	ogAvatarRadius = 150
	ogRingRadius   = 158
)

// composeOG renders the default OG image from the site avatar.
func composeOG(avatar *image.NRGBA, colors palette) *image.NRGBA {
	canvas := image.NewNRGBA(image.Rect(0, 0, OGWidth, OGHeight))
	draw.Draw(canvas, canvas.Bounds(), image.NewUniform(colors.background), image.Point{}, draw.Src)
	center := image.Pt(OGWidth/2, OGHeight/2)
	drawDisk(canvas, center, ogRingRadius, image.NewUniform(colors.accent), image.Point{})
	side := 2 * ogAvatarRadius
	face := resize(squareCrop(avatar), side, side, xdraw.CatmullRom)
	drawDisk(canvas, center, ogAvatarRadius, flatten(face, colors.background), image.Point{})
	return canvas
}
