// Package texture decodes a Unity Texture2D into an image.
//
// The formats Valheim's icons and map textures come in, and no more: raw
// RGBA32, RGB24 and Alpha8, the DXT1 and DXT5 block formats, BC4, which the
// map's lava mask is, and BC7, which the icon atlas is.
// Only the first, largest mip is read. Unity keeps a texture's rows bottom
// up; the image comes back top down, as an image is.
package texture

import (
	"fmt"
	"image"
	"image/color"
)

// Format is Unity's TextureFormat.
type Format int32

const (
	Alpha8 Format = 1
	RGB24  Format = 3
	RGBA32 Format = 4
	DXT1   Format = 10
	DXT5   Format = 12
	BC7    Format = 25
	BC4    Format = 26
)

// Resolver hands back bytes from a streaming path: the ".resS" node beside
// the serialised file in its archive.
type Resolver func(path string, offset uint64, size uint32) ([]byte, error)

// Decode reads a Texture2D from its type-tree value.
func Decode(tree map[string]any, resolve Resolver) (*image.NRGBA, error) {
	name, _ := tree["m_Name"].(string)
	format := Format(asInt32(tree["m_TextureFormat"]))
	w := int(asInt32(tree["m_Width"]))
	h := int(asInt32(tree["m_Height"]))
	data, _ := tree["image data"].([]byte)
	if sd, ok := tree["m_StreamData"].(map[string]any); ok {
		if path, _ := sd["path"].(string); path != "" {
			if resolve == nil {
				return nil, fmt.Errorf("texture %q: data is in %s and nothing resolves it", name, path)
			}
			b, err := resolve(path, asUint64(sd["offset"]), asUint32(sd["size"]))
			if err != nil {
				return nil, fmt.Errorf("texture %q: %w", name, err)
			}
			data = b
		}
	}
	if w <= 0 || h <= 0 {
		return nil, fmt.Errorf("texture %q: %dx%d", name, w, h)
	}
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	var err error
	switch format {
	case RGBA32:
		err = raw(img, data, 4, func(p []byte) color.NRGBA { return color.NRGBA{p[0], p[1], p[2], p[3]} })
	case RGB24:
		err = raw(img, data, 3, func(p []byte) color.NRGBA { return color.NRGBA{p[0], p[1], p[2], 255} })
	case Alpha8:
		err = raw(img, data, 1, func(p []byte) color.NRGBA { return color.NRGBA{0, 0, 0, p[0]} })
	case DXT1:
		err = blocks(img, data, 8, decodeDXT1)
	case DXT5:
		err = blocks(img, data, 16, decodeDXT5)
	case BC4:
		err = blocks(img, data, 8, decodeBC4)
	case BC7:
		err = blocks(img, data, 16, decodeBC7)
	default:
		return nil, fmt.Errorf("texture %q: format %d is not supported", name, format)
	}
	if err != nil {
		return nil, fmt.Errorf("texture %q: %w", name, err)
	}
	return img, nil
}

// raw fills the image from packed pixels, bottom row first.
func raw(img *image.NRGBA, data []byte, stride int, px func([]byte) color.NRGBA) error {
	w, h := img.Rect.Dx(), img.Rect.Dy()
	if len(data) < w*h*stride {
		return fmt.Errorf("%d bytes for %dx%d at %d per pixel", len(data), w, h, stride)
	}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i := (y*w + x) * stride
			img.SetNRGBA(x, h-1-y, px(data[i:i+stride]))
		}
	}
	return nil
}

// blocks fills the image from 4x4 blocks, bottom row of blocks first.
func blocks(img *image.NRGBA, data []byte, size int, decode func(b []byte, out *[16]color.NRGBA)) error {
	w, h := img.Rect.Dx(), img.Rect.Dy()
	bw, bh := (w+3)/4, (h+3)/4
	if len(data) < bw*bh*size {
		return fmt.Errorf("%d bytes for %dx%d in %d-byte blocks", len(data), w, h, size)
	}
	var out [16]color.NRGBA
	for by := 0; by < bh; by++ {
		for bx := 0; bx < bw; bx++ {
			decode(data[(by*bw+bx)*size:], &out)
			for i := 0; i < 16; i++ {
				x, y := bx*4+i%4, by*4+i/4
				if x < w && y < h {
					img.SetNRGBA(x, h-1-y, out[i])
				}
			}
		}
	}
	return nil
}

// decodeDXT1 is a BC1 block: two 565 colours and 2-bit indices.
func decodeDXT1(b []byte, out *[16]color.NRGBA) {
	c0 := uint16(b[0]) | uint16(b[1])<<8
	c1 := uint16(b[2]) | uint16(b[3])<<8
	var pal [4]color.NRGBA
	pal[0], pal[1] = rgb565(c0), rgb565(c1)
	if c0 > c1 {
		pal[2] = lerp3(pal[0], pal[1], 2, 1)
		pal[3] = lerp3(pal[0], pal[1], 1, 2)
	} else {
		pal[2] = lerp3(pal[0], pal[1], 1, 1)
		pal[3] = color.NRGBA{0, 0, 0, 0}
	}
	idx := uint32(b[4]) | uint32(b[5])<<8 | uint32(b[6])<<16 | uint32(b[7])<<24
	for i := 0; i < 16; i++ {
		out[i] = pal[(idx>>(2*uint(i)))&3]
	}
}

// decodeDXT5 is a BC3 block: an interpolated alpha block, then a colour
// block as BC1 with four colours.
func decodeDXT5(b []byte, out *[16]color.NRGBA) {
	var alpha [16]uint8
	decodeAlphaBlock(b, &alpha)
	c0 := uint16(b[8]) | uint16(b[9])<<8
	c1 := uint16(b[10]) | uint16(b[11])<<8
	var pal [4]color.NRGBA
	pal[0], pal[1] = rgb565(c0), rgb565(c1)
	pal[2] = lerp3(pal[0], pal[1], 2, 1)
	pal[3] = lerp3(pal[0], pal[1], 1, 2)
	idx := uint32(b[12]) | uint32(b[13])<<8 | uint32(b[14])<<16 | uint32(b[15])<<24
	for i := 0; i < 16; i++ {
		c := pal[(idx>>(2*uint(i)))&3]
		c.A = alpha[i]
		out[i] = c
	}
}

// decodeBC4 is one channel in the interpolated form BC3 keeps its alpha in,
// read back as grey.
func decodeBC4(b []byte, out *[16]color.NRGBA) {
	var v [16]uint8
	decodeAlphaBlock(b, &v)
	for i := 0; i < 16; i++ {
		out[i] = color.NRGBA{v[i], v[i], v[i], 255}
	}
}

// decodeAlphaBlock is the eight-byte interpolated block BC3 and BC4 share:
// two end points and 3-bit indices into a ramp between them.
func decodeAlphaBlock(b []byte, out *[16]uint8) {
	a0, a1 := int(b[0]), int(b[1])
	var ramp [8]uint8
	ramp[0], ramp[1] = uint8(a0), uint8(a1)
	if a0 > a1 {
		for i := 1; i <= 6; i++ {
			ramp[i+1] = uint8(((7-i)*a0 + i*a1 + 3) / 7)
		}
	} else {
		for i := 1; i <= 4; i++ {
			ramp[i+1] = uint8(((5-i)*a0 + i*a1 + 2) / 5)
		}
		ramp[6], ramp[7] = 0, 255
	}
	bits := uint64(0)
	for i := 0; i < 6; i++ {
		bits |= uint64(b[2+i]) << (8 * uint(i))
	}
	for i := 0; i < 16; i++ {
		out[i] = ramp[(bits>>(3*uint(i)))&7]
	}
}

func rgb565(c uint16) color.NRGBA {
	r := uint8((c >> 11) & 31)
	g := uint8((c >> 5) & 63)
	b := uint8(c & 31)
	return color.NRGBA{r<<3 | r>>2, g<<2 | g>>4, b<<3 | b>>2, 255}
}

// lerp3 mixes two colours in the ratio wa:wb.
func lerp3(a, b color.NRGBA, wa, wb int) color.NRGBA {
	n := wa + wb
	return color.NRGBA{
		uint8((int(a.R)*wa + int(b.R)*wb + n/2) / n),
		uint8((int(a.G)*wa + int(b.G)*wb + n/2) / n),
		uint8((int(a.B)*wa + int(b.B)*wb + n/2) / n),
		255,
	}
}

func asInt32(v any) int32 {
	switch x := v.(type) {
	case int32:
		return x
	case uint32:
		return int32(x)
	case int64:
		return int32(x)
	}
	return 0
}

func asUint32(v any) uint32 {
	switch x := v.(type) {
	case uint32:
		return x
	case int32:
		return uint32(x)
	}
	return 0
}

func asUint64(v any) uint64 {
	switch x := v.(type) {
	case uint64:
		return x
	case int64:
		return uint64(x)
	case uint32:
		return uint64(x)
	}
	return 0
}

// DecodeArray reads a Texture2DArray from its type-tree value: one image
// per slice. Only the first mip of each slice is taken, and the slices
// lie one after another in the data with each slice's whole mip chain
// before the next, which is how Unity lays an array out.
//
// The format is a GraphicsFormat rather than a TextureFormat: the block
// formats the terrain uses are mapped here and anything else is reported.
func DecodeArray(tree map[string]any, resolve Resolver) ([]*image.NRGBA, error) {
	name, _ := tree["m_Name"].(string)
	w := int(asInt32(tree["m_Width"]))
	h := int(asInt32(tree["m_Height"]))
	depth := int(asInt32(tree["m_Depth"]))
	mips := int(asInt32(tree["m_MipCount"]))
	data, _ := tree["image data"].([]byte)
	if sd, ok := tree["m_StreamData"].(map[string]any); ok {
		if path, _ := sd["path"].(string); path != "" {
			if resolve == nil {
				return nil, fmt.Errorf("texture array %q: data is in %s and nothing resolves it", name, path)
			}
			b, err := resolve(path, asUint64(sd["offset"]), asUint32(sd["size"]))
			if err != nil {
				return nil, fmt.Errorf("texture array %q: %w", name, err)
			}
			data = b
		}
	}
	if w <= 0 || h <= 0 || depth <= 0 {
		return nil, fmt.Errorf("texture array %q: %dx%dx%d", name, w, h, depth)
	}
	var blockSize int
	var decode func(b []byte, out *[16]color.NRGBA)
	switch g := asInt32(tree["m_Format"]); g {
	case 96, 97: // RGBA_DXT1_SRGB, RGBA_DXT1_UNorm
		blockSize, decode = 8, decodeDXT1
	case 100, 101: // RGBA_DXT5_SRGB, RGBA_DXT5_UNorm
		blockSize, decode = 16, decodeDXT5
	case 108, 109: // RGBA_BC7_SRGB, RGBA_BC7_UNorm
		blockSize, decode = 16, decodeBC7
	default:
		return nil, fmt.Errorf("texture array %q: graphics format %d is not supported", name, g)
	}
	// A slice's bytes: its mip chain, the top level first.
	sliceBytes := 0
	for level, mw, mh := 0, w, h; level < max(mips, 1); level++ {
		sliceBytes += ((mw + 3) / 4) * ((mh + 3) / 4) * blockSize
		mw, mh = max(mw/2, 1), max(mh/2, 1)
	}
	if len(data) < sliceBytes*depth {
		// Perhaps no mips are stored after all: one level per slice.
		if top := ((w + 3) / 4) * ((h + 3) / 4) * blockSize; len(data) >= top*depth {
			sliceBytes = top
		} else {
			return nil, fmt.Errorf("texture array %q: %d bytes for %d slices of %d", name, len(data), depth, sliceBytes)
		}
	}
	out := make([]*image.NRGBA, 0, depth)
	for s := 0; s < depth; s++ {
		img := image.NewNRGBA(image.Rect(0, 0, w, h))
		if err := blocks(img, data[s*sliceBytes:], blockSize, decode); err != nil {
			return nil, fmt.Errorf("texture array %q slice %d: %w", name, s, err)
		}
		out = append(out, img)
	}
	return out, nil
}
