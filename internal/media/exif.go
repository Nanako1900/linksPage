package media

import (
	"encoding/binary"
	"image"
)

// JPEG markers used when scanning for the EXIF APP1 segment.
const (
	markerPrefix = 0xFF
	markerSOI    = 0xD8
	markerAPP1   = 0xE1
	markerSOS    = 0xDA
	markerEOI    = 0xD9

	tagOrientation = 0x0112
	typeShort      = 3
)

var exifHeader = []byte("Exif\x00\x00")

// jpegOrientation returns the EXIF orientation (1–8) of a JPEG, or 1 when
// the file has none or it cannot be parsed. Re-encoding drops EXIF, so the
// orientation must be applied to the pixels first.
func jpegOrientation(data []byte) int {
	if len(data) < 4 || data[0] != markerPrefix || data[1] != markerSOI {
		return 1
	}
	for i := 2; i+4 <= len(data); {
		if data[i] != markerPrefix {
			return 1
		}
		marker := data[i+1]
		if marker == markerSOS || marker == markerEOI {
			return 1
		}
		size := int(binary.BigEndian.Uint16(data[i+2:]))
		end := i + 2 + size
		if size < 2 || end > len(data) {
			return 1
		}
		seg := data[i+4 : end]
		if marker == markerAPP1 && len(seg) > len(exifHeader) && string(seg[:len(exifHeader)]) == string(exifHeader) {
			return tiffOrientation(seg[len(exifHeader):])
		}
		i = end
	}
	return 1
}

// tiffOrientation reads tag 0x0112 from IFD0 of a TIFF block.
func tiffOrientation(tiff []byte) int {
	if len(tiff) < 8 {
		return 1
	}
	var bo binary.ByteOrder
	switch string(tiff[:2]) {
	case "II":
		bo = binary.LittleEndian
	case "MM":
		bo = binary.BigEndian
	default:
		return 1
	}
	if bo.Uint16(tiff[2:]) != 42 {
		return 1
	}
	ifd := int64(bo.Uint32(tiff[4:]))
	if ifd < 8 || ifd+2 > int64(len(tiff)) {
		return 1
	}
	count := int64(bo.Uint16(tiff[ifd:]))
	for n := range count {
		off := ifd + 2 + n*12
		if off+12 > int64(len(tiff)) {
			return 1
		}
		entry := tiff[off : off+12]
		if bo.Uint16(entry) != tagOrientation {
			continue
		}
		if bo.Uint16(entry[2:]) != typeShort || bo.Uint32(entry[4:]) != 1 {
			return 1
		}
		o := int(bo.Uint16(entry[8:]))
		if o < 1 || o > 8 {
			return 1
		}
		return o
	}
	return 1
}

// orientTarget maps source pixel (sx, sy) of a sw×sh image to its
// destination for an EXIF orientation.
func orientTarget(o, sx, sy, sw, sh int) (int, int) {
	switch o {
	case 2:
		return sw - 1 - sx, sy
	case 3:
		return sw - 1 - sx, sh - 1 - sy
	case 4:
		return sx, sh - 1 - sy
	case 5:
		return sy, sx
	case 6:
		return sh - 1 - sy, sx
	case 7:
		return sh - 1 - sy, sw - 1 - sx
	case 8:
		return sy, sw - 1 - sx
	}
	return sx, sy
}

// applyOrientation returns src rotated/flipped for orientation o (src
// itself when o is 1 or out of range).
func applyOrientation(src *image.NRGBA, o int) *image.NRGBA {
	if o <= 1 || o > 8 {
		return src
	}
	b := src.Bounds()
	sw, sh := b.Dx(), b.Dy()
	dw, dh := sw, sh
	if o >= 5 {
		dw, dh = sh, sw
	}
	dst := image.NewNRGBA(image.Rect(0, 0, dw, dh))
	for sy := range sh {
		row := src.Pix[sy*src.Stride:]
		for sx := range sw {
			dx, dy := orientTarget(o, sx, sy, sw, sh)
			di := dy*dst.Stride + dx*4
			copy(dst.Pix[di:di+4], row[sx*4:sx*4+4])
		}
	}
	return dst
}
