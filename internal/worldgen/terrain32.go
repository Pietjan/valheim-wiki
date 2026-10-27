package worldgen

import "math"

// The biome height functions, bit-exact.
//
// These are transcriptions of WorldGenerator's Get*Height methods with every
// narrowing the C# does done here in the same place: the game computes in
// double between float32 variables, and each assignment rounds. A port that
// stays in float64 is right to a few millimetres, and a few millimetres is a
// different world since 1.0 — location placement reads these heights against
// the 30 m waterline at every point of a 2048² grid, and against altitude and
// terrain-delta limits at every candidate, and each point that rounds to the
// other side of a limit moves every location drawn after it.
//
// The frequency constants are the game's float32 literals widened to double,
// which is how they appear in the decompiled source: 0.009999999776482582 is
// 0.01f, and multiplying by 0.01 instead moves a sample by an ulp.

// Widened float32 literals.
const (
	c003 = float64(float32(0.003))
	c01  = float64(float32(0.01))
	c02  = float64(float32(0.02))
	c03  = float64(float32(0.03))
	c04  = float64(float32(0.04))
	c05  = float64(float32(0.05))
	c08  = float64(float32(0.08))
	c1   = float64(float32(0.1))
	c2   = float64(float32(0.2))
	c4   = float64(float32(0.4))
	c7   = float64(float32(0.7))
	c12  = float64(float32(1.2))
	c002 = float64(float32(0.002))
)

// worldAngle32 is WorldGenerator.WorldAngle: a float32 atan2, scaled in double,
// narrowed, and its sine narrowed again.
func worldAngle32(wx, wy float32) float32 {
	a := float32(math.Atan2(float64(wx), float64(wy)))
	return float32(math.Sin(float64(float32(float64(a) * 20.0))))
}

// isAshlands32 is WorldGenerator.IsAshlands.
func isAshlands32(x, y float32) bool {
	num := float64(worldAngle32(x, y)) * 100.0
	return float64(dlength(x, float32(float64(y)+float64(ashlandsYOffset)))) > float64(ashlandsMinDistance)+num
}

// isDeepNorth32 is WorldGenerator.IsDeepnorth, which measures with
// Vector2.magnitude rather than DUtils.Length; under Mono the two agree.
func isDeepNorth32(x, y float32) bool {
	num := float32(float64(worldAngle32(x, y)) * 100.0)
	return Vec2{x, float32(float64(y) + 4000.0)}.magnitude() > float32(12000.0+float64(num))
}

// mathfLikeSmoothStep32 is DUtils.MathfLikeSmoothStep, which returns a double
// that has been through a float.
func mathfLikeSmoothStep32(from, to, t float64) float64 {
	t = clamp01(t)
	t = -2.0*t*t*t + 3.0*t*t
	return float64(float32(to*t + from*(1.0-t)))
}

func clamp01(v float64) float64 {
	if v > 1 {
		return 1
	}
	if v < 0 {
		return 0
	}
	return v
}

// createAshlandsGap and createDeepNorthGap are the ocean bands that separate
// the far biomes: 1 across the ordinary world, falling to 0 inside the gap.
func createAshlandsGap(wx, wy float32) float64 {
	num := float64(worldAngle32(wx, wy)) * 100.0
	value := float64(dlength(wx, wy+ashlandsYOffset)) - (float64(ashlandsMinDistance) + num)
	value = clamp01(math.Abs(value) / 400.0)
	return mathfLikeSmoothStep32(0, 1, float64(float32(value)))
}

func createDeepNorthGap(wx, wy float32) float64 {
	num := float64(worldAngle32(wx, wy)) * 100.0
	value := float64(dlength(wx, wy+4000)) - (12000.0 + num)
	value = clamp01(math.Abs(value) / 400.0)
	return mathfLikeSmoothStep32(0, 1, float64(float32(value)))
}

// shifted is the coordinate every biome samples its detail at: offset by the
// world's third constant and a hundred thousand, narrowed, then widened.
func (g *Generator) shifted(wx, wy float32) (x, y float64) {
	x = float64(float32(float64(wx) + 100000.0 + float64(g.cfg.Offset3)))
	y = float64(float32(float64(wy) + 100000.0 + float64(g.cfg.Offset3)))
	return x, y
}

// detail32 is the two-octave product every biome layers over the base shape.
func detail32(x, y float64) float32 {
	d := float32(float64(noise32(x*c01, y*c01)) * float64(noise32(x*c02, y*c02)))
	return float32(float64(d) + float64(noise32(x*c05, y*c05))*float64(noise32(x*c1, y*c1))*float64(d)*0.5)
}

// fine32 is the pair of high-frequency passes that finish most biomes.
func fine32(x, y float64, h float32) float32 {
	h = float32(float64(h) + float64(noise32(x*c1, y*c1))*c01)
	return float32(float64(h) + float64(noise32(x*c4, y*c4))*c003)
}

// base32 is GetBaseHeight as a float32.
func (g *Generator) base32(wx, wy float32) float32 {
	return float32(g.BaseHeight(float64(wx), float64(wy)))
}

// meadows32 is GetMeadowsHeight.
func (g *Generator) meadows32(wx, wy float32, d *riverData) float32 {
	base := g.base32(wx, wy)
	x, y := g.shifted(wx, wy)
	detail := detail32(x, y)

	h := float32(float64(base) + float64(detail)*c1)
	above := float32(float64(h) - float64(float32(0.15)))
	t := float32(clamp01(float64(base) / c4))
	if above > 0 {
		h = float32(float64(h) - float64(above)*((1.0-float64(t))*0.75))
	}
	h = g.addRivers32(wx, wy, h, d)
	return fine32(x, y, h)
}

// forest32 is GetForestHeight: the meadows without the flattening.
func (g *Generator) forest32(wx, wy float32, d *riverData) float32 {
	base := g.base32(wx, wy)
	x, y := g.shifted(wx, wy)
	detail := detail32(x, y)

	h := float32(float64(base) + float64(detail)*c1)
	h = g.addRivers32(wx, wy, h, d)
	return fine32(x, y, h)
}

// plains32 is GetPlainsHeight: the meadows again, with the flattening's
// product taken left to right instead of bracketed, which rounds differently.
func (g *Generator) plains32(wx, wy float32, d *riverData) float32 {
	base := g.base32(wx, wy)
	x, y := g.shifted(wx, wy)
	detail := detail32(x, y)

	h := float32(float64(base) + float64(detail)*c1)
	above := h - 0.15
	t := float32(clamp01(float64(base) / c4))
	if above > 0 {
		h = float32(float64(h) - float64(above)*(1.0-float64(t))*0.75)
	}
	h = g.addRivers32(wx, wy, h, d)
	return fine32(x, y, h)
}

// marsh32 is GetMarshHeight: nearly flat, and shifted without the offset.
func (g *Generator) marsh32(wx, wy float32, d *riverData) float32 {
	x := float64(float32(float64(wx) + 100000.0))
	y := float64(float32(float64(wy) + 100000.0))

	h := float32(0.137)
	n := float32(float64(noise32(x*c04, y*c04)) * float64(noise32(x*c08, y*c08)))
	h = float32(float64(h) + float64(n)*c03)
	h = g.addRivers32(wx, wy, h, d)
	return fine32(x, y, h)
}

// tilt32 is BaseHeightTilt: local steepness from four neighbours a metre away.
func (g *Generator) tilt32(wx, wy float32) float32 {
	west := g.base32(float32(float64(wx)-1.0), wy)
	east := g.base32(float32(float64(wx)+1.0), wy)
	south := g.base32(wx, float32(float64(wy)-1.0))
	north := g.base32(wx, float32(float64(wy)+1.0))
	return float32(float64(absf(float32(float64(east)-float64(west)))) + float64(absf(float32(float64(south)-float64(north)))))
}

// mountain32 is GetSnowMountainHeight.
func (g *Generator) mountain32(wx, wy float32, d *riverData) float32 {
	base := g.base32(wx, wy)
	tilt := g.tilt32(wx, wy)
	x, y := g.shifted(wx, wy)

	over := float32(float64(base) - c4)
	base = float32(float64(base) + float64(over))
	detail := detail32(x, y)
	base = float32(float64(base) + float64(detail)*c2)
	base = g.addRivers32(wx, wy, base, d)
	base = fine32(x, y, base)
	return float32(float64(base) + float64(noise32(x*c2, y*c2))*2.0*float64(tilt))
}

// deepNorth32 is GetDeepNorthHeight, the biome's shape since 1.0.
func (g *Generator) deepNorth32(wx, wy float32, d *riverData) float32 {
	base := g.base32(wx, wy) + 0.1
	x, y := g.shifted(wx, wy)
	detail := detail32(x, y)

	h := float32(float64(base) + float64(detail)*c1)
	above := float32(float64(h) - float64(float32(0.15)))
	t := float32(clamp01(float64(base) / c4))
	if above > 0 {
		h = float32(float64(h) - float64(above)*((1.0-float64(t))*0.75))
	}
	h = g.addRivers32(wx, wy, h, d)
	return fine32(x, y, h)
}

// deepNorthPregenerate32 is GetDeepNorthHeightPregenerate, the shape the
// stream placer sees: the ordinary pass sees it as it was before 1.0, the
// Deep North's own pass a tenth of a unit higher. Its last two passes sample
// the shifted coordinate scaled in float32, unlike every other biome.
func (g *Generator) deepNorthPregenerate32(wx, wy float32, d *riverData, riverPregen bool) float32 {
	h := g.base32(wx, wy)
	if !riverPregen {
		h += 0.1
	}
	x, y := g.shifted(wx, wy)
	sx, sy := float32(x), float32(y)

	over := float32(math.Max(0, float64(float32(float64(h)-c4))))
	h = float32(float64(h) + float64(over))
	detail := detail32(x, y)
	h = float32(float64(h) + float64(detail)*c2)
	h = float32(float64(h) * c12)
	h = g.addRivers32(wx, wy, h, d)
	h = float32(float64(h) + float64(PerlinNoise(sx*0.1, sy*0.1))*c01)
	return float32(float64(h) + float64(PerlinNoise(sx*0.4, sy*0.4))*c003)
}

// mistlands32 is GetMistlandsHeight, with the vegetation alpha it writes into
// the mask beside the height.
func (g *Generator) mistlands32(wx, wy float32, d *riverData) (h, vegetation float32) {
	base := g.base32(wx, wy)
	x, y := g.shifted(wx, wy)

	n := noise32(x*c02*c7, y*c02*c7) * noise32(x*c04*c7, y*c04*c7)
	n = float32(float64(n) + float64(noise32(x*c03*c7, y*c03*c7))*float64(noise32(x*c05*c7, y*c05*c7))*float64(n)*0.5)
	if n > 0 {
		n = float32(math.Pow(float64(n), 1.5))
	}
	base = float32(float64(base) + float64(n)*c4)
	base = g.addRivers32(wx, wy, base, d)

	w := float32(clamp01(float64(n) * 7.0))
	base = float32(float64(base) + float64(noise32(x*c1, y*c1))*c03*float64(w))
	base = float32(float64(base) + float64(noise32(x*c4, y*c4))*c01*float64(w))

	vegetation = float32(1.0 - float64(w)*c12)
	vegetation = float32(float64(vegetation) - (1.0 - float64(lerpStep32(0.1, 0.3, w))))

	smooth := float32(float64(base) + float64(noise32(x*c4, y*c4))*c002)
	stepped := float32(float64(base) * 400.0)
	stepped = float32(math.Ceil(float64(stepped)))
	stepped = float32(float64(stepped) / 400.0)
	return dlerp32(smooth, stepped, w), vegetation
}

// ashlands32 is GetAshlandsHeight, with the vegetation alpha it writes into
// the mask.
//
// It is the one biome computed in double from start to finish: the game
// widens the position once and never narrows until the return, so the
// narrowings the other biomes are full of are absent here, and the two
// remaining ones — Perlin's float32 products, and the smoothstep that
// returns through a float — are the DUtils helpers' own. The FastNoise it
// layers on the base is double too. And it carves no rivers: unlike its
// pregeneration shape below, the final height never calls AddRivers.
func (g *Generator) ashlands32(wx, wy float32) (h, vegetation float32) {
	return g.ashlandsMasked32(wx, wy, false)
}

// AshlandsMask is the alpha of GetAshlandsHeight's mask with cheap set, which
// is how Minimap.GetMaskColor asks for it: the lava's reach, from two octaves
// of the cellular noise rather than five and three. The height beside it is
// the cheap one too and is thrown away, as the game throws it away.
func (g *Generator) AshlandsMask(wx, wy float64) float32 {
	_, mask := g.ashlandsMasked32(float32(wx), float32(wy), true)
	return mask
}

// AshlandsOceanGradient is WorldGenerator.GetAshlandsOceanGradient: how far
// into the Ashlands' sea a point is, in units of 300m past the biome's line,
// which the minimap's mask keeps for the water's colour.
func AshlandsOceanGradient(wx, wy float64) float64 {
	x, y := float32(wx), float32(wy)
	num := float64(worldAngle32(x, y+ashlandsYOffset)) * 100.0
	return (float64(dlength(x, y+ashlandsYOffset)) - (float64(ashlandsMinDistance) + num)) / 300.0
}

func (g *Generator) ashlandsMasked32(wx, wy float32, cheap bool) (h, vegetation float32) {
	x := float64(wx)
	y := float64(wy)
	base := float64(g.base32(wx, wy))
	angle := float64(worldAngle32(wx, wy)) * 100.0

	// The ring: 1 along the biome's centre line, falling off a kilometre
	// either side, and again towards the east and west edges of the world.
	yOffset := float64(ashlandsYOffset)
	ring := length(x, y+yOffset-yOffset*0.3) - (float64(ashlandsMinDistance) + angle)
	ring = math.Abs(ring) / 1000.0
	ring = 1.0 - clamp01(ring)
	ring = mathfLikeSmoothStep32(0.1, 1.0, ring)
	ring *= 1.0 - clamp01(math.Abs(x)/7500.0)

	// The world's edge, 600m in from 10 150.
	edge := 1.0 - clamp01((length(x, y)-10150.0)/600.0)

	// The shift is a float32 sum widened, not the sum narrowed: the
	// position stays double, so this is not the shifted() the other
	// biomes sample at.
	x += float64(float32(100000.0 + float64(g.cfg.Offset3)))
	y += float64(float32(100000.0 + float64(g.cfg.Offset3)))

	// Five octaves of cellular noise, each through the smoothstep.
	cells := 0.0
	amp := 1.0
	freq := float64(float32(0.33))
	octaves, lavaOctaves := 5, 3
	if cheap {
		octaves, lavaOctaves = 2, 2
	}
	for i := 0; i < octaves; i++ {
		cells += amp * mathfLikeSmoothStep32(0.0, 1.0, ashlandsNoise.Cellular(x*freq, y*freq))
		freq *= 2.0
		amp *= 0.5
	}
	cells = remap(cells, -1.0, 1.0, 0.0, 1.0)
	relief := dlerp(ring, blendOverlay(ring, cells), 0.5)

	// The game computes the detail product every other biome layers on here
	// too, into a variable it then never reads; it is left out.

	h64 := dlerp(base, float64(float32(0.15)), 0.75)
	h64 += relief * 0.5
	h64 = dlerp(-1.0, h64, mathfLikeSmoothStep32(0.0, 1.0, edge))

	// Three octaves of raw cellular distance, sharpened to the fourth power:
	// the lava's cell pattern.
	const floor = 0.15
	lava := 0.0
	amp = 1.0
	freq = 8.0
	for i := 0; i < lavaOctaves; i++ {
		lava += amp * ashlandsNoise.Cellular(x*freq, y*freq)
		freq *= 2.0
		amp *= 0.5
	}
	lava = remap(lava, -1.0, 1.0, 0.0, 1.0)
	lava = clamp01(pow4(lava) * 2.0)

	// Simplex over the whole thing, raised to 1.4.
	simplex := ashlandsNoise.SimplexFractal(x*0.075, y*0.075)
	simplex = remap(simplex, -1.0, 1.0, 0.0, 1.0)
	simplex = math.Pow(simplex, float64(float32(1.4)))
	h64 *= simplex

	// Where the lava lies: fbm over the ring, squared, overlaid on the cell
	// pattern, and only where the land stands above the floor.
	flow := fbm(float32(x*c01), float32(y*c01), 3, 2.0, 0.5)
	flow *= clamp01(remap(ring, 0.0, 0.5, 0.5, 1.0))
	flow = lerpStep(float64(float32(0.7)), 1.0, flow)
	flow = pow2(flow)
	lavaMask := blendOverlay(flow, lava)
	lavaMask *= clamp01((h64 - floor - 0.02) / 0.01)

	// The lava's own depth below the land, clamped to the floor.
	depth := float64(noise32(x*0.05+5124.0, y*0.05+5000.0))
	depth = pow2(depth)
	depth = remap(depth, 0.0, 1.0, c01, float64(float32(0.055)))
	sunk := float64(mathfClamp(float32(h64-depth), float32(floor+c01), 5000))
	h64 = dlerp(h64, sunk, lavaMask)

	return float32(h64), float32(lavaMask)
}

// ashlandsPregenerate32 is GetAshlandsHeightPregenerate, the Perlin-only shape
// the stream placer sees before the cellular height above is layered on.
func (g *Generator) ashlandsPregenerate32(wx, wy float32, d *riverData) float32 {
	base := g.base32(wx, wy)
	x, y := g.shifted(wx, wy)
	detail := detail32(x, y)

	base = float32(float64(base) + float64(detail)*c1)
	base = float32(float64(base) + c1)
	base = fine32(x, y, base)
	return g.addRivers32(wx, wy, base, d)
}

// The double DUtils the Ashlands reads. Each is the double overload, which
// has no narrowing in it; the float32 overloads are above and in height.go.

// dlerp is DUtils.Lerp(double, double, double): clamped at both ends.
func dlerp(a, b, t float64) float64 {
	if t <= 0 {
		return a
	}
	if t >= 1 {
		return b
	}
	return a*(1.0-t) + b*t
}

// lerpStep is DUtils.LerpStep(double, double, double).
func lerpStep(l, h, v float64) float64 {
	return clamp01((v - l) / (h - l))
}

// remap is DUtils.Remap: InverseLerp into Lerp, both clamped.
func remap(value, inLow, inHigh, outLow, outHigh float64) float64 {
	t := 0.0
	if inLow != inHigh {
		t = clamp01((value - inLow) / (inHigh - inLow))
	}
	return dlerp(outLow, outHigh, t)
}

// blendOverlay is DUtils.BlendOverlay, the image-editing overlay blend.
func blendOverlay(a, b float64) float64 {
	if a < 0.5 {
		return 2.0 * a * b
	}
	return 1.0 - 2.0*(1.0-a)*(1.0-b)
}

// fbm is DUtils.Fbm(Vector2, int, double, double): the Vector2 is float32,
// widened once; each octave's Perlin sample narrows its arguments.
func fbm(px, py float32, octaves int, lacunarity, gain float64) float64 {
	sum := 0.0
	amp := 1.0
	x := float64(px)
	y := float64(py)
	for i := 0; i < octaves; i++ {
		sum += amp * float64(noise32(x, y))
		amp *= gain
		x *= lacunarity
		y *= lacunarity
	}
	return sum
}

// mathfClamp is Mathf.Clamp(float, float, float).
func mathfClamp(v, min, max float32) float32 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

// pow2 and pow4 are Math.Pow at 2.0 and 4.0. The game's Math.Pow is the C
// library's, correctly rounded or within a fraction of an ulp of it; Go's
// math.Pow raises to an integer by repeated squaring and rounds at each
// square. A square is a single rounding either way. The fourth power keeps
// the first square's rounding error and folds it back in, so the result is
// rounded once.
func pow2(x float64) float64 { return x * x }

func pow4(x float64) float64 {
	sq := x * x
	sqErr := math.FMA(x, x, -sq)
	p := sq * sq
	pErr := math.FMA(sq, sq, -p)
	return p + (pErr + 2.0*sq*sqErr + sqErr*sqErr)
}

// dlerp32 is DUtils.Lerp(float, float, float): clamped, blended in double.
func dlerp32(a, b, t float32) float32 {
	if t <= 0 {
		return a
	}
	if t >= 1 {
		return b
	}
	return float32(float64(a)*(1.0-float64(t)) + float64(b)*float64(t))
}

// addRivers32 is AddRivers: two passes, one pulling the whole circle towards
// the waterline and one cutting the channel near its centre.
func (g *Generator) addRivers32(wx, wy, h float32, d *riverData) float32 {
	if d == nil {
		return h
	}
	weight, width := d.riverWeight(wx, wy)
	if weight <= 0 {
		return h
	}
	t := lerpStep32(20, 60, width)
	surface := dlerp32(0.14, 0.12, t)
	bed := dlerp32(0.139, 0.128, t)
	if h > surface {
		h = dlerp32(h, surface, weight)
	}
	if h > bed {
		h = dlerp32(h, bed, lerpStep32(0.85, 1, weight))
	}
	return h
}

// biomeHeight32 is GetBiomeHeight: a biome's height at a point in world
// units. Before generation the gaps are not applied and the Mistlands,
// Ashlands and Deep North stand in their pregeneration shapes; riverPreDN
// picks which of the Deep North's two the caller is.
func (g *Generator) biomeHeight32(biome Biome, wx, wy float32, d *riverData, pregen, riverPreDN bool) (h, vegetation float32) {
	scale := float32(HeightMultiplier)
	if !pregen {
		scale = float32(HeightMultiplier * createAshlandsGap(wx, wy) * createDeepNorthGap(wx, wy))
	}
	if dlength(wx, wy) > 10500 {
		return -2 * HeightMultiplier, 1
	}

	vegetation = 1
	var n float32
	switch biome {
	case BiomeSwamp:
		n = g.marsh32(wx, wy, d)
	case BiomeDeepNorth:
		if pregen {
			n = g.deepNorthPregenerate32(wx, wy, d, riverPreDN)
		} else {
			n = g.deepNorth32(wx, wy, d)
			vegetation = 0
		}
	case BiomeMountain:
		n = g.mountain32(wx, wy, d)
	case BiomeBlackForest:
		n = g.forest32(wx, wy, d)
	case BiomeOcean:
		n = g.base32(wx, wy)
	case BiomeAshLands:
		if pregen {
			n = g.ashlandsPregenerate32(wx, wy, d)
		} else {
			n, vegetation = g.ashlands32(wx, wy)
		}
	case BiomePlains:
		n = g.plains32(wx, wy, d)
	case BiomeMeadows:
		n = g.meadows32(wx, wy, d)
	case BiomeMistlands:
		if pregen {
			n = g.forest32(wx, wy, d)
		} else {
			n, vegetation = g.mistlands32(wx, wy, d)
		}
	default:
		return 0, 1
	}
	return float32(float64(n) * float64(scale)), vegetation
}
