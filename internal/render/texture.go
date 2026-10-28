package render

import (
	"image"
	"image/draw"
	"image/png"
	"math"
	"os"
)

// Texture is one albedo PNG as the extractor wrote it: top-down rows of
// non-premultiplied sRGB RGBA. Unity's v=0 is the bottom row, so a sample
// reads row (1-v)*H.
type Texture struct {
	W, H int
	Pix  []uint8
}

// srgbToLinear decodes one 8-bit sRGB channel to linear light; the
// extractor's albedo PNGs carry no colour-space chunk but are sRGB (Unity
// m_ColorSpace 1), and lighting happens in linear light.
var srgbToLinear = func() (t [256]float32) {
	for i := range t {
		c := float64(i) / 255
		if c <= 0.04045 {
			t[i] = float32(c / 12.92)
		} else {
			t[i] = float32(math.Pow((c+0.055)/1.055, 2.4))
		}
	}
	return
}()

// LoadTexture decodes one PNG.
func LoadTexture(path string) (*Texture, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		return nil, err
	}
	b := img.Bounds()
	n := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(n, n.Bounds(), img, b.Min, draw.Src)
	return &Texture{W: n.Rect.Dx(), H: n.Rect.Dy(), Pix: n.Pix}, nil
}

// Sample is the bilinear, wrapped sample at a Unity uv (already tiled and
// offset by the material): linear RGB and alpha in 0..1.
func (t *Texture) Sample(u, v float32) (r, g, b, a float32) {
	fu := u - float32(math.Floor(float64(u)))
	fv := v - float32(math.Floor(float64(v)))
	x := fu*float32(t.W) - 0.5
	y := (1-fv)*float32(t.H) - 0.5
	x0 := int(math.Floor(float64(x)))
	y0 := int(math.Floor(float64(y)))
	tx := x - float32(x0)
	ty := y - float32(y0)
	x1 := x0 + 1
	y1 := y0 + 1
	x0 = wrap(x0, t.W)
	x1 = wrap(x1, t.W)
	y0 = wrap(y0, t.H)
	y1 = wrap(y1, t.H)
	p00 := t.Pix[(y0*t.W+x0)*4:]
	p10 := t.Pix[(y0*t.W+x1)*4:]
	p01 := t.Pix[(y1*t.W+x0)*4:]
	p11 := t.Pix[(y1*t.W+x1)*4:]
	w00 := (1 - tx) * (1 - ty)
	w10 := tx * (1 - ty)
	w01 := (1 - tx) * ty
	w11 := tx * ty
	r = w00*srgbToLinear[p00[0]] + w10*srgbToLinear[p10[0]] + w01*srgbToLinear[p01[0]] + w11*srgbToLinear[p11[0]]
	g = w00*srgbToLinear[p00[1]] + w10*srgbToLinear[p10[1]] + w01*srgbToLinear[p01[1]] + w11*srgbToLinear[p11[1]]
	b = w00*srgbToLinear[p00[2]] + w10*srgbToLinear[p10[2]] + w01*srgbToLinear[p01[2]] + w11*srgbToLinear[p11[2]]
	a = (w00*float32(p00[3]) + w10*float32(p10[3]) + w01*float32(p01[3]) + w11*float32(p11[3])) / 255
	return
}

func wrap(i, n int) int {
	i %= n
	if i < 0 {
		i += n
	}
	return i
}
