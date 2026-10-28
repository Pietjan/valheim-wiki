package render

import (
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
)

// Layer is a render resolved to its output size: the model as
// premultiplied sRGB over transparency, and the sun's shadow on the ground
// beside it.
type Layer struct {
	Size   int
	RGB    []float32 // premultiplied sRGB, 0..1
	A      []float32
	Shadow []float32
}

// Resolve tonemaps the render and box-filters it down by an integer factor:
// the model's edges, the leaves' cutouts and the shadow's edge all come out
// anti-aliased from the supersampled render. Exposure scales the linear
// light before the ACES curve.
func (img *Image) Resolve(factor int, exposure float64) *Layer {
	size := img.W / factor
	l := &Layer{Size: size, RGB: make([]float32, size*size*3), A: make([]float32, size*size), Shadow: make([]float32, size*size)}
	n := float32(factor * factor)
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			var r, g, b, a, sh float32
			for sy := 0; sy < factor; sy++ {
				row := (y*factor + sy) * img.W
				for sx := 0; sx < factor; sx++ {
					k := row + x*factor + sx
					if img.Cover[k] == 0 {
						sh += img.Shadow[k]
						continue
					}
					cr, cg, cb := tonemap(float64(img.RGB[k*3])*exposure, float64(img.RGB[k*3+1])*exposure, float64(img.RGB[k*3+2])*exposure)
					r += float32(linearToSRGB(cr))
					g += float32(linearToSRGB(cg))
					b += float32(linearToSRGB(cb))
					a++
				}
			}
			o := y*size + x
			l.RGB[o*3], l.RGB[o*3+1], l.RGB[o*3+2] = r/n, g/n, b/n
			l.A[o] = a / n
			l.Shadow[o] = sh / n
		}
	}
	return l
}

// Disc composes the icon: a dark warm disc filling the square, its rim
// anti-aliased, with the ground shadow and the model over it and a short
// inner shadow at the rim. There is no ring; the page draws that.
func (l *Layer) Disc() *image.NRGBA {
	const (
		shadowDepth = 0.45 // how dark the ground shadow paints the disc
		rimWidth    = 1.5  // pixels of inner shadow
	)
	centre := [3]float64{28.0 / 255, 22.0 / 255, 19.0 / 255}
	rim := [3]float64{14.0 / 255, 11.0 / 255, 9.0 / 255}
	out := image.NewNRGBA(image.Rect(0, 0, l.Size, l.Size))
	radius := float64(l.Size) / 2
	for y := 0; y < l.Size; y++ {
		for x := 0; x < l.Size; x++ {
			d := math.Hypot(float64(x)+0.5-radius, float64(y)+0.5-radius)
			cover := clamp(radius-d+0.5, 0, 1)
			t := clamp(d/radius, 0, 1)
			t *= t
			alpha := (0.92 + 0.05*t) * cover
			shade := 1 - shadowDepth*float64(l.Shadow[y*l.Size+x])
			shade *= 0.55 + 0.45*clamp((radius-d)/rimWidth, 0, 1)
			var disc [3]float64
			for ch := 0; ch < 3; ch++ {
				disc[ch] = (centre[ch] + (rim[ch]-centre[ch])*t) * shade * alpha
			}
			k := y*l.Size + x
			ma := float64(l.A[k])
			a := ma + alpha*(1-ma)
			var rgb [3]float64
			for ch := 0; ch < 3; ch++ {
				rgb[ch] = float64(l.RGB[k*3+ch]) + disc[ch]*(1-ma)
			}
			out.SetNRGBA(x, y, toNRGBA(rgb, a))
		}
	}
	return out
}

// Flat composes the model alone over transparency, with the ground shadow
// as a soft dark alpha under it: the bestiary portrait.
func (l *Layer) Flat() *image.NRGBA {
	const shadowAlpha = 0.35
	out := image.NewNRGBA(image.Rect(0, 0, l.Size, l.Size))
	for y := 0; y < l.Size; y++ {
		for x := 0; x < l.Size; x++ {
			k := y*l.Size + x
			ma := float64(l.A[k])
			a := ma + shadowAlpha*float64(l.Shadow[k])*(1-ma)
			rgb := [3]float64{float64(l.RGB[k*3]), float64(l.RGB[k*3+1]), float64(l.RGB[k*3+2])}
			out.SetNRGBA(x, y, toNRGBA(rgb, a))
		}
	}
	return out
}

// toNRGBA un-premultiplies and quantises.
func toNRGBA(rgb [3]float64, a float64) color.NRGBA {
	if a <= 0 {
		return color.NRGBA{}
	}
	q := func(v float64) uint8 { return uint8(clamp(v/a, 0, 1)*255 + 0.5) }
	return color.NRGBA{R: q(rgb[0]), G: q(rgb[1]), B: q(rgb[2]), A: uint8(clamp(a, 0, 1)*255 + 0.5)}
}

// WritePNG writes an image to a file, creating it.
func WritePNG(path string, img image.Image) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := png.Encode(f, img); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// tonemap is the ACES filmic curve as three.js applies it (the Stephen
// Hill fit: input matrix, RRT+ODT, output matrix), the viewer's own
// tonemapper, so the icons sit with the scene. three divides by 0.6 so
// that exposure 1 maps middle grey where ACES does.
func tonemap(r, g, b float64) (float64, float64, float64) {
	r, g, b = r/0.6, g/0.6, b/0.6
	x := 0.59719*r + 0.35458*g + 0.04823*b
	y := 0.07600*r + 0.90834*g + 0.01566*b
	z := 0.02840*r + 0.13383*g + 0.83777*b
	x, y, z = rrtODT(x), rrtODT(y), rrtODT(z)
	r = 1.60475*x - 0.53108*y - 0.07367*z
	g = -0.10208*x + 1.10813*y - 0.00605*z
	b = -0.00327*x - 0.07276*y + 1.07602*z
	return clamp(r, 0, 1), clamp(g, 0, 1), clamp(b, 0, 1)
}

func rrtODT(v float64) float64 {
	a := v*(v+0.0245786) - 0.000090537
	b := v*(0.983729*v+0.4329510) + 0.238081
	return a / b
}

func linearToSRGB(c float64) float64 {
	if c <= 0.0031308 {
		return 12.92 * c
	}
	return 1.055*math.Pow(c, 1/2.4) - 0.055
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
