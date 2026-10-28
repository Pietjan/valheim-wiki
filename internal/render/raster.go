package render

import (
	"math"
)

// The game's clear day (EnvSetup "Clear", linear light): the sun's colour
// and intensity and the ambient under it, as the scene viewer lights its
// models and as the icons keep the palette consistent with it.
var (
	sunColor     = [3]float64{1, 0.772, 0.484}
	sunIntensity = 1.7
	ambient      = [3]float64{0.463, 0.574, 0.706}
)

// Options frame and light one render.
type Options struct {
	// Yaw turns the camera around +y: 0 looks from +z toward the origin,
	// 45 from the +x +z diagonal. Pitch is its elevation in degrees.
	Yaw, Pitch float64
	// Size is the square image rendered, in pixels, before any
	// downsampling; Fit is the fraction of its half-size the model's
	// projected circle fills.
	Size int
	Fit  float64
	// ShadowMap is the sun's depth map resolution; 0 draws no shadows.
	ShadowMap int
	// SunAzimuth and SunElevation place the key light relative to the
	// camera, in degrees: azimuth to the camera's left, elevation above
	// the horizon. The game's sun stands 45° up; the icon's is biased to
	// the camera's side so what faces the viewer is what the sun paints.
	SunAzimuth, SunElevation float64
}

// Image is a render before tonemapping: linear light where the model
// covers a pixel, and the sun's shadow on the ground where it does not.
type Image struct {
	W, H int
	// RGB is the lit colour of covered pixels, three floats per pixel.
	RGB []float32
	// Cover is 1 where the model is.
	Cover []uint8
	// Shadow is 0..1, how much of the ground at that pixel the sun misses.
	Shadow []float32
	depth  []float32
}

// camera is an orthographic view: unit axes right, up and toward the
// camera, and the scale and centre that map metres to pixels.
type camera struct {
	r, u, c  [3]float64
	scale    float64 // pixels per metre
	cx, cy   float64 // image centre, pixels
	sx0, sy0 float64 // the projected point drawn at the centre
	w, h     int
}

func (cam *camera) project(p [3]float64) (px, py, depth float64) {
	px = cam.cx + (dot(p, cam.r)-cam.sx0)*cam.scale
	py = cam.cy - (dot(p, cam.u)-cam.sy0)*cam.scale
	return px, py, dot(p, cam.c)
}

// basis is the camera frame looking down -c with +y as up. In Unity's
// left-handed frame right = up × forward, so a camera on the +x +z
// diagonal has -x +z on its right.
func basis(c [3]float64) (r, u [3]float64) {
	f := scale3(c, -1)
	r = norm3(cross([3]float64{0, 1, 0}, f))
	u = cross(f, r)
	return r, u
}

// Render draws the scene.
func Render(s *Scene, o Options) *Image {
	yaw, pitch := o.Yaw*math.Pi/180, o.Pitch*math.Pi/180
	cam := &camera{w: o.Size, h: o.Size, cx: float64(o.Size) / 2, cy: float64(o.Size) / 2}
	cam.c = [3]float64{math.Cos(pitch) * math.Sin(yaw), math.Sin(pitch), math.Cos(pitch) * math.Cos(yaw)}
	cam.r, cam.u = basis(cam.c)
	cam.fit(s, o.Fit)

	// The key light, in the camera's terms.
	az, el := o.SunAzimuth*math.Pi/180, o.SunElevation*math.Pi/180
	h := norm3([3]float64{cam.c[0], 0, cam.c[2]})
	light := norm3(add3(
		scale3(add3(scale3(h, math.Cos(az)), scale3(cam.r, -math.Sin(az))), math.Cos(el)),
		[3]float64{0, math.Sin(el), 0}))

	img := &Image{W: o.Size, H: o.Size}
	n := o.Size * o.Size
	img.RGB = make([]float32, n*3)
	img.Cover = make([]uint8, n)
	img.Shadow = make([]float32, n)
	img.depth = make([]float32, n)
	for i := range img.depth {
		img.depth[i] = float32(math.Inf(-1))
	}

	var sm *shadowMap
	if o.ShadowMap > 0 {
		sm = newShadowMap(s, light, o.ShadowMap)
	}
	shade := shader{s: s, cam: cam, light: light, sm: sm, img: img}
	shade.height = math.Max(1, s.Max[1]-s.Floor)
	for _, r := range s.runs {
		shade.run(r)
	}
	if sm != nil {
		shade.ground()
	}
	return img
}

// fit centres the projected bounds and scales them into a circle of
// radius fit × half-size.
func (cam *camera) fit(s *Scene, fit float64) {
	minX, minY := math.Inf(1), math.Inf(1)
	maxX, maxY := math.Inf(-1), math.Inf(-1)
	s.eachVertex(func(p [3]float64) {
		if p[1] < s.Floor {
			return
		}
		x, y := dot(p, cam.r), dot(p, cam.u)
		minX, maxX = math.Min(minX, x), math.Max(maxX, x)
		minY, maxY = math.Min(minY, y), math.Max(maxY, y)
	})
	if math.IsInf(minX, 1) {
		cam.scale = 1
		return
	}
	cam.sx0, cam.sy0 = (minX+maxX)/2, (minY+maxY)/2
	radius := 0.0
	s.eachVertex(func(p [3]float64) {
		if p[1] < s.Floor {
			return
		}
		radius = math.Max(radius, math.Hypot(dot(p, cam.r)-cam.sx0, dot(p, cam.u)-cam.sy0))
	})
	if radius == 0 {
		radius = 1
	}
	cam.scale = fit * float64(cam.w) / 2 / radius
}

// rasterize calls fn for every pixel centre inside the screen triangle,
// with the barycentric weights of its three corners.
func rasterize(w, h int, x0, y0, x1, y1, x2, y2 float64, fn func(x, y int, w0, w1, w2 float32)) {
	area := (x1-x0)*(y2-y0) - (x2-x0)*(y1-y0)
	if area == 0 {
		return
	}
	bx0 := int(math.Max(0, math.Floor(math.Min(x0, math.Min(x1, x2)))))
	by0 := int(math.Max(0, math.Floor(math.Min(y0, math.Min(y1, y2)))))
	bx1 := int(math.Min(float64(w-1), math.Ceil(math.Max(x0, math.Max(x1, x2)))))
	by1 := int(math.Min(float64(h-1), math.Ceil(math.Max(y0, math.Max(y1, y2)))))
	if bx0 > bx1 || by0 > by1 {
		return
	}
	inv := 1 / area
	for y := by0; y <= by1; y++ {
		py := float64(y) + 0.5
		for x := bx0; x <= bx1; x++ {
			px := float64(x) + 0.5
			w0 := ((x1-px)*(y2-py) - (x2-px)*(y1-py)) * inv
			w1 := ((x2-px)*(y0-py) - (x0-px)*(y2-py)) * inv
			w2 := 1 - w0 - w1
			if w0 < 0 || w1 < 0 || w2 < 0 {
				continue
			}
			fn(x, y, float32(w0), float32(w1), float32(w2))
		}
	}
}

// shadowMap is the scene's depth as the sun sees it: an orthographic
// view down the light, nearest-to-light depth per texel.
type shadowMap struct {
	cam   camera
	light [3]float64
	depth []float32
	bias  float64
}

func newShadowMap(s *Scene, light [3]float64, res int) *shadowMap {
	sm := &shadowMap{light: light}
	sm.cam = camera{c: light, w: res, h: res, cx: float64(res) / 2, cy: float64(res) / 2}
	sm.cam.r, sm.cam.u = basis(light)
	sm.cam.fit(s, 0.98)
	// A texel and a half of slack, in metres, so a face does not shadow
	// itself where the map quantises it.
	sm.bias = 1.5/sm.cam.scale + 0.01
	sm.depth = make([]float32, res*res)
	for i := range sm.depth {
		sm.depth[i] = float32(math.Inf(-1))
	}
	for _, r := range s.runs {
		sm.run(s, r)
	}
	return sm
}

func (sm *shadowMap) run(s *Scene, r run) {
	mat := r.mat
	floor := float32(s.Floor)
	for i := r.first; i < r.first+r.count; i += 3 {
		a, b, c := s.idx[i], s.idx[i+1], s.idx[i+2]
		pa, pb, pc := s.vertex(a), s.vertex(b), s.vertex(c)
		x0, y0, d0 := sm.cam.project(pa)
		x1, y1, d1 := sm.cam.project(pb)
		x2, y2, d2 := sm.cam.project(pc)
		ya, yb, yc := float32(pa[1]), float32(pb[1]), float32(pc[1])
		rasterize(sm.cam.w, sm.cam.h, x0, y0, x1, y1, x2, y2, func(x, y int, w0, w1, w2 float32) {
			if w0*ya+w1*yb+w2*yc < floor {
				return
			}
			d := w0*float32(d0) + w1*float32(d1) + w2*float32(d2)
			k := y*sm.cam.w + x
			if d <= sm.depth[k] {
				return
			}
			if mat.Cutout && mat.Albedo != nil {
				u, v := s.uvAt(a, b, c, w0, w1, w2, mat)
				if _, _, _, alpha := mat.Albedo.Sample(u, v); alpha < mat.Cutoff {
					return
				}
			}
			sm.depth[k] = d
		})
	}
}

// lit is how much sun reaches a point: a 3×3 comparison against the map,
// so the edge of a shadow is soft.
func (sm *shadowMap) lit(p [3]float64) float64 {
	px, py, d := sm.cam.project(p)
	x, y := int(math.Floor(px)), int(math.Floor(py))
	open := 0.0
	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			xx, yy := x+dx, y+dy
			if xx < 0 || yy < 0 || xx >= sm.cam.w || yy >= sm.cam.h {
				open++
				continue
			}
			if d+sm.bias >= float64(sm.depth[yy*sm.cam.w+xx]) {
				open++
			}
		}
	}
	return open / 9
}

// uvAt is the material's tiled uv at a point of a triangle.
func (s *Scene) uvAt(a, b, c uint32, w0, w1, w2 float32, mat *Material) (u, v float32) {
	u = (w0*s.uv[a*2]+w1*s.uv[b*2]+w2*s.uv[c*2])*mat.Scale[0] + mat.Offset[0]
	v = (w0*s.uv[a*2+1]+w1*s.uv[b*2+1]+w2*s.uv[c*2+1])*mat.Scale[1] + mat.Offset[1]
	return u, v
}

// shader draws the colour pass.
type shader struct {
	s      *Scene
	cam    *camera
	light  [3]float64
	sm     *shadowMap
	img    *Image
	height float64
}

func (sh *shader) run(r run) {
	s, cam, img := sh.s, sh.cam, sh.img
	mat := r.mat
	floor := float32(s.Floor)
	for i := r.first; i < r.first+r.count; i += 3 {
		a, b, c := s.idx[i], s.idx[i+1], s.idx[i+2]
		pa, pb, pc := s.vertex(a), s.vertex(b), s.vertex(c)
		if pa[1] < s.Floor && pb[1] < s.Floor && pc[1] < s.Floor {
			continue
		}
		fn := cross(sub3(pb, pa), sub3(pc, pa))
		facing := dot(fn, cam.c)
		if facing == 0 || (facing < 0 && !mat.TwoSided) {
			continue
		}
		x0, y0, d0 := cam.project(pa)
		x1, y1, d1 := cam.project(pb)
		x2, y2, d2 := cam.project(pc)
		flip := float32(1)
		if facing < 0 {
			flip = -1
		}
		rasterize(cam.w, cam.h, x0, y0, x1, y1, x2, y2, func(x, y int, w0, w1, w2 float32) {
			k := y*cam.w + x
			d := w0*float32(d0) + w1*float32(d1) + w2*float32(d2)
			if d <= img.depth[k] {
				return
			}
			p := [3]float64{
				float64(w0)*pa[0] + float64(w1)*pb[0] + float64(w2)*pc[0],
				float64(w0)*pa[1] + float64(w1)*pb[1] + float64(w2)*pc[1],
				float64(w0)*pa[2] + float64(w1)*pb[2] + float64(w2)*pc[2],
			}
			if float32(p[1]) < floor {
				return
			}
			col := mat.Color
			if mat.Albedo != nil {
				u, v := s.uvAt(a, b, c, w0, w1, w2, mat)
				tr, tg, tb, ta := mat.Albedo.Sample(u, v)
				if mat.Cutout && ta < mat.Cutoff {
					return
				}
				col = [3]float32{col[0] * tr, col[1] * tg, col[2] * tb}
			}
			img.depth[k] = d
			img.Cover[k] = 1
			if mat.Unlit {
				copy(img.RGB[k*3:], col[:])
				return
			}
			n := [3]float64{
				float64(flip * (w0*s.nrm[a*3] + w1*s.nrm[b*3] + w2*s.nrm[c*3])),
				float64(flip * (w0*s.nrm[a*3+1] + w1*s.nrm[b*3+1] + w2*s.nrm[c*3+1])),
				float64(flip * (w0*s.nrm[a*3+2] + w1*s.nrm[b*3+2] + w2*s.nrm[c*3+2])),
			}
			if dot(n, n) < 1e-12 {
				n = scale3(fn, float64(flip))
			}
			n = norm3(n)
			sun := math.Max(0, dot(n, sh.light))
			if sun > 0 && sh.sm != nil {
				sun *= sh.sm.lit(p)
			}
			// Sky light reaches an upward face more than a downward one,
			// and the ground steals some of it from the first metres.
			sky := 0.62 + 0.38*n[1]
			sky *= 0.7 + 0.3*math.Min(1, (p[1]-s.Floor)/math.Min(2, 0.5*sh.height))
			for ch := 0; ch < 3; ch++ {
				img.RGB[k*3+ch] = col[ch] * float32(ambient[ch]*sky+sunIntensity*sunColor[ch]*sun)
			}
		})
	}
}

// ground fills Image.Shadow: the sun's shadow on the plane y = Floor,
// seen through every pixel the model leaves open.
func (sh *shader) ground() {
	cam, img := sh.cam, sh.img
	if cam.c[1] <= 0 {
		return
	}
	for y := 0; y < cam.h; y++ {
		for x := 0; x < cam.w; x++ {
			k := y*cam.w + x
			if img.Cover[k] != 0 {
				continue
			}
			sx := (float64(x)+0.5-cam.cx)/cam.scale + cam.sx0
			sy := (cam.cy-float64(y)-0.5)/cam.scale + cam.sy0
			// p = sx r + sy u + t c with p.y = Floor.
			t := (sh.s.Floor - sx*cam.r[1] - sy*cam.u[1]) / cam.c[1]
			p := add3(add3(scale3(cam.r, sx), scale3(cam.u, sy)), scale3(cam.c, t))
			img.Shadow[k] = float32(1 - sh.sm.lit(p))
		}
	}
}
