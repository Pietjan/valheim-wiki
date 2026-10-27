package worldgen

import (
	"math"

	"github.com/pietjan/valheim-wiki/internal/unityrandom"
)

// Rivers and streams, reproduced from the world seed.
//
// This is the part of terrain generation that is not a noise function. Valheim
// finds every lake, joins pairs of them into rivers, scatters streams across the
// map, walks each one laying down overlapping circular "river points", and then
// lowers terrain wherever those circles cover it.
//
// It is driven throughout by UnityEngine.Random, which is why it was left
// unported for so long — and why the terrain used to read up to 36m high over
// water, reporting a walkable hill where there is a river. For a bot that only
// steered around obstacles that was a bounded inaccuracy. For one that plans
// routes it is the difference between a path and a drowning.
//
// Fidelity here is all-or-nothing. The generation is a single seeded sequence,
// so one extra or missing draw shifts every river placed afterwards. That is
// why internal/unityrandom is verified bit-for-bit rather than approximately.

// Vec2 is a 2D point in world coordinates.
//
// float32 throughout, matching Unity's Vector2. The rest of this package works
// in float64 because it is reproducing noise, where the extra precision only
// helps; here it would hurt, because the values are compared and accumulated
// exactly as the game does it.
type Vec2 struct{ X, Y float32 }

func (v Vec2) sub(o Vec2) Vec2      { return Vec2{v.X - o.X, v.Y - o.Y} }
func (v Vec2) add(o Vec2) Vec2      { return Vec2{v.X + o.X, v.Y + o.Y} }
func (v Vec2) scale(s float32) Vec2 { return Vec2{v.X * s, v.Y * s} }

// magnitude mirrors Unity's Vector2.magnitude as Mono runs it: the squares
// and their sum in double, the root in double, narrowed once at the end.
//
// Mono evaluates float32 arithmetic in double between stores, so an
// expression of several float operations rounds once at the end, not at
// each step. Measured on four watercourses of two 1.0 worlds against every
// other rounding: summing in float32 misses one, narrowing before the root
// misses two, and this misses none.
func (v Vec2) magnitude() float32 {
	return float32(math.Sqrt(float64(v.X)*float64(v.X) + float64(v.Y)*float64(v.Y)))
}

func (v Vec2) sqrMagnitude() float32 {
	return float32(float64(v.X)*float64(v.X) + float64(v.Y)*float64(v.Y))
}

func (v Vec2) distance(o Vec2) float32 { return v.sub(o).magnitude() }

func (v Vec2) normalized() Vec2 {
	m := v.magnitude()
	if m > 1e-05 {
		return Vec2{v.X / m, v.Y / m}
	}
	return Vec2{}
}

// River is one watercourse: a line from p0 to p1, meandering by a fixed
// sine-product curve, with a width drawn per point between min and max.
type River struct {
	P0, P1, Center              Vec2
	WidthMin, WidthMax          float32
	CurveWidth, CurveWavelength float32
}

// riverPoint is one circle of water. Terrain inside it is pulled down towards
// the waterline, most strongly at the centre.
type riverPoint struct {
	p  Vec2
	w  float32
	w2 float32 // w squared, so the distance test needs no square root
}

func newRiverPoint(p Vec2, w float32) riverPoint {
	return riverPoint{p: p, w: w, w2: float32(float64(w) * float64(w))}
}

type gridKey struct{ x, y int32 }

// riverData is the generated result: every river point, bucketed by 64m grid
// square so a height query only tests the handful that could reach it.
type riverData struct {
	rivers  []River
	streams []River
	points  map[gridKey][]riverPoint
}

// ensureRivers generates on first use and caches.
//
// Generation is expensive — finding lakes alone samples base height on a 157x157
// grid, and placing streams makes hundreds of thousands of height queries — so
// it is deliberately lazy. A caller that never asks for height near water never
// pays for it.
func (g *Generator) ensureRivers() *riverData {
	g.riverOnce.Do(func() { g.riverData = g.generateRivers() })
	return g.riverData
}

func (g *Generator) generateRivers() *riverData {
	d := &riverData{points: make(map[gridKey][]riverPoint)}

	lakes := g.findLakes()

	// One generator per seed, shared between placing and rendering.
	//
	// Valheim calls RenderRivers from inside PlaceRivers, before restoring the
	// saved RNG state, so the per-point widths continue the same sequence that
	// chose the routes and the river widths. Seeding a second generator for
	// rendering gives every point a plausible width drawn from the wrong place
	// in the stream — rivers land in exactly the right spots and are carved
	// wrong, which reads as a small height error rather than as an obvious fault.
	rivers := unityrandom.New(g.cfg.RiverSeed)
	d.rivers = g.placeRivers(rivers, lakes)
	d.renderRivers(d.rivers, rivers)

	// Rivers are rendered before streams, so where both cross a grid square the
	// river points come first in its list. The weights are summed over that list
	// in float32, where addition is not associative, so the order is part of the
	// result.
	// Since 1.0 streams are placed twice from the same seed: once over the
	// ordinary world, rendered everywhere but the Deep North, and once more
	// for the Deep North alone, where the placer sees the biome a tenth of a
	// unit higher. The game keeps only the first list as its streams.
	streams := unityrandom.New(g.cfg.StreamSeed)
	d.streams = g.placeStreams(streams, d, false)
	d.renderRiversWhere(d.streams, streams, skipDeepNorth)

	north := unityrandom.New(g.cfg.StreamSeed)
	d.renderRiversWhere(g.placeStreams(north, d, true), north, onlyDeepNorth)

	return d
}

// findLakes samples the world on a 128m grid and keeps everything below the
// waterline, then merges neighbours into single points.
func (g *Generator) findLakes() []Vec2 {
	var found []Vec2

	for y := float32(-10000); y <= 10000; y = float32(float64(y) + 128.0) {
		for x := float32(-10000); x <= 10000; x = float32(float64(x) + 128.0) {
			if (Vec2{x, y}).magnitude() > 10000 {
				continue
			}
			if float32(g.BaseHeight(float64(x), float64(y))) < 0.05 {
				found = append(found, Vec2{x, y})
			}
		}
	}

	return mergePoints(found, 800)
}

// mergePoints collapses clusters into their running midpoint.
//
// Note that the midpoint moves as each neighbour is absorbed, so the result
// depends on the order points are visited — this is a faithful port of that,
// not a clustering algorithm chosen on its merits.
func mergePoints(points []Vec2, rng float32) []Vec2 {
	var merged []Vec2

	for len(points) > 0 {
		current := points[0]
		points = points[1:]

		for len(points) > 0 {
			i := findClosest(points, current, rng)
			if i < 0 {
				break
			}
			current = current.add(points[i]).scale(0.5)

			// Swap-remove, exactly as the game does: it changes which point is
			// considered next and so changes the result.
			points[i] = points[len(points)-1]
			points = points[:len(points)-1]
		}

		merged = append(merged, current)
	}

	return merged
}

func findClosest(points []Vec2, p Vec2, maxDistance float32) int {
	best := -1
	nearest := float32(99999)

	for i, o := range points {
		if o == p {
			continue
		}
		if d := p.distance(o); d < maxDistance && d < nearest {
			best = i
			nearest = d
		}
	}

	return best
}

// placeRivers joins lakes together, one river at a time, until no lake can
// reach another.
func (g *Generator) placeRivers(r *unityrandom.Random, lakes []Vec2) []River {
	var rivers []River
	remaining := append([]Vec2(nil), lakes...)

	for len(remaining) > 1 {
		from := remaining[0]

		end := g.findRandomRiverEnd(r, rivers, lakes, from, 2000, 0.4, 128)
		if end == -1 && !haveRiverAt(rivers, from) {
			// A lake with no river at all gets a second, longer look.
			end = g.findRandomRiverEnd(r, rivers, lakes, from, 5000, 0.4, 128)
		}

		if end == -1 {
			remaining = remaining[1:]
			continue
		}

		river := River{P0: from, P1: lakes[end]}
		river.Center = river.P0.add(river.P1).scale(0.5)
		river.WidthMax = r.RangeFloat(60, 100)
		river.WidthMin = r.RangeFloat(60, river.WidthMax)

		length := river.P0.distance(river.P1)
		river.CurveWidth = float32(float64(length) / 15.0)
		river.CurveWavelength = float32(float64(length) / 20.0)

		rivers = append(rivers, river)
	}

	return rivers
}

// placeStreams scatters short watercourses on hillsides: start low, end higher.
func (g *Generator) placeStreams(r *unityrandom.Random, d *riverData, deepNorth bool) []River {
	var streams []River
	riverPregen := !deepNorth

	// A fixed 3000 attempts, most of which fail. The count is part of the
	// sequence, not a tuning knob: stopping early would leave the generator in
	// a different state.
	for i := 0; i < 3000; i++ {
		start, ok := g.findStreamStartPoint(r, d, 100, 26, 31, riverPregen)
		if !ok {
			continue
		}

		end, ok := g.findStreamEndPoint(r, d, 100, 36, 44, start, 80, 200, riverPregen)
		if !ok {
			continue
		}

		center := start.add(end).scale(0.5)
		if h := g.pregenerationHeight(center.X, center.Y, d, riverPregen); h < 26 || h > 44 {
			continue
		}

		stream := River{
			P0:       start,
			P1:       end,
			Center:   center,
			WidthMax: 20,
			WidthMin: 20,
		}

		length := stream.P0.distance(stream.P1)
		stream.CurveWidth = float32(float64(length) / 15.0)
		stream.CurveWavelength = float32(float64(length) / 20.0)

		streams = append(streams, stream)
	}

	return streams
}

func (g *Generator) findStreamStartPoint(r *unityrandom.Random, d *riverData, iterations int, minHeight, maxHeight float32, riverPregen bool) (Vec2, bool) {
	for i := 0; i < iterations; i++ {
		x := r.RangeFloat(-10000, 10000)
		y := r.RangeFloat(-10000, 10000)

		if h := g.pregenerationHeight(x, y, d, riverPregen); h > minHeight && h < maxHeight {
			return Vec2{x, y}, true
		}
	}
	return Vec2{}, false
}

func (g *Generator) findStreamEndPoint(
	r *unityrandom.Random, d *riverData, iterations int, minHeight, maxHeight float32,
	start Vec2, minLength, maxLength float32, riverPregen bool,
) (Vec2, bool) {
	step := float32((float64(maxLength) - float64(minLength)) / float64(iterations))
	length := maxLength

	// The search spirals inwards: each attempt is a random direction at a
	// slightly shorter range than the last.
	for i := 0; i < iterations; i++ {
		length = float32(float64(length) - float64(step))

		angle := r.RangeFloat(0, float32(math.Pi*2))
		dir := Vec2{sinf(angle), cosf(angle)}
		p := start.add(dir.scale(length))

		if h := g.pregenerationHeight(p.X, p.Y, d, riverPregen); h > minHeight && h < maxHeight {
			return p, true
		}
	}

	return Vec2{}, false
}

func sinf(v float32) float32 { return float32(math.Sin(float64(v))) }
func cosf(v float32) float32 { return float32(math.Cos(float64(v))) }

// pregenerationHeight is height before the gaps and before rivers exist — what
// river placement itself consults, since rivers cannot depend on rivers.
//
// This is why the biome port had to come before streams could match: stream
// candidates are scattered across the whole world and accepted by height, so
// classifying everything as Meadows accepted the wrong ones.
func (g *Generator) pregenerationHeight(wx, wy float32, d *riverData, riverPregen bool) float32 {
	h, _ := g.biomeHeight32(g.GetBiome(float64(wx), float64(wy)), wx, wy, d, true, riverPregen)
	return h
}

func haveRiverAt(rivers []River, p Vec2) bool {
	for _, r := range rivers {
		if r.P0 == p || r.P1 == p {
			return true
		}
	}
	return false
}

func haveRiverBetween(rivers []River, p0, p1 Vec2) bool {
	for _, r := range rivers {
		if (r.P0 == p0 && r.P1 == p1) || (r.P0 == p1 && r.P1 == p0) {
			return true
		}
	}
	return false
}

func (g *Generator) findRandomRiverEnd(
	r *unityrandom.Random, rivers []River, points []Vec2, p Vec2,
	maxDistance, heightLimit, checkStep float32,
) int {
	var candidates []int

	for i, o := range points {
		if o == p {
			continue
		}
		if p.distance(o) >= maxDistance {
			continue
		}
		if haveRiverBetween(rivers, p, o) {
			continue
		}
		if !g.isRiverAllowed(p, o, checkStep, heightLimit) {
			continue
		}
		candidates = append(candidates, i)
	}

	if len(candidates) == 0 {
		// No draw is made when there is nothing to choose from, which keeps the
		// RNG in step with the game.
		return -1
	}

	return candidates[r.RangeInt(0, int32(len(candidates)))]
}

// isRiverAllowed walks the straight line between two lakes and rejects it if the
// ground rises too high anywhere, or if it never leaves water at all.
func (g *Generator) isRiverAllowed(p0, p1 Vec2, step, heightLimit float32) bool {
	length := p0.distance(p1)
	dir := p1.sub(p0).normalized()

	allWater := true
	for d := step; d <= float32(float64(length)-float64(step)); d = float32(float64(d) + float64(step)) {
		p := p0.add(dir.scale(d))

		h := float32(g.BaseHeight(float64(p.X), float64(p.Y)))
		if h > heightLimit {
			return false
		}
		if h > 0.05 {
			allWater = false
		}
	}

	// A river entirely under water is not a river.
	return !allWater
}

// renderRivers walks each river laying down overlapping circles.
// riverFilter is WorldGenerator.RiverAdd: which rivers a rendering pass
// carves, decided by whether each one starts in the Deep North.
type riverFilter int

const (
	allRivers riverFilter = iota
	skipDeepNorth
	onlyDeepNorth
)

func (d *riverData) renderRivers(rivers []River, r *unityrandom.Random) {
	d.renderRiversWhere(rivers, r, allRivers)
}

func (d *riverData) renderRiversWhere(rivers []River, r *unityrandom.Random, filter riverFilter) {
	pending := make(map[gridKey][]riverPoint)

	// Insertion order per grid square has to match the game's, so the keys are
	// tracked separately: Go map iteration is randomised, and the weights are
	// summed in float32 where order changes the result.
	var order []gridKey

	for _, river := range rivers {
		if filter != allRivers {
			north := isDeepNorth32(river.P0.X, river.P0.Y)
			if (north && filter == skipDeepNorth) || (!north && filter == onlyDeepNorth) {
				continue
			}
		}
		step := float32(float64(river.WidthMin) / 8.0)
		dir := river.P1.sub(river.P0).normalized()
		side := Vec2{-dir.Y, dir.X}
		length := river.P0.distance(river.P1)

		for d0 := float32(0); d0 <= length; d0 = float32(float64(d0) + float64(step)) {
			t := float32(float64(d0) / float64(river.CurveWavelength))

			// A product of three sines with incommensurable periods: it never
			// repeats, which is what stops rivers looking like sine waves.
			offset := float32(math.Sin(float64(t)) *
				math.Sin(float64(t)*0.634119987487793) *
				math.Sin(float64(t)*0.3341200053691864) *
				float64(river.CurveWidth))

			width := r.RangeFloat(river.WidthMin, river.WidthMax)
			p := river.P0.add(dir.scale(d0)).add(side.scale(offset))

			addRiverPoint(pending, &order, p, width)
		}
	}

	for _, key := range order {
		d.points[key] = append(d.points[key], pending[key]...)
	}
}

// addRiverPoint files one circle into every grid square it touches.
func addRiverPoint(into map[gridKey][]riverPoint, order *[]gridKey, p Vec2, w float32) {
	grid := riverGrid(p.X, p.Y)
	reach := int32(math.Ceil(float64(w) / 64.0))

	for y := grid.y - reach; y <= grid.y+reach; y++ {
		for x := grid.x - reach; x <= grid.x+reach; x++ {
			key := gridKey{x, y}
			if !insideRiverGrid(key, p, w) {
				continue
			}
			if _, seen := into[key]; !seen {
				*order = append(*order, key)
			}
			into[key] = append(into[key], newRiverPoint(p, w))
		}
	}
}

func insideRiverGrid(grid gridKey, p Vec2, w float32) bool {
	center := Vec2{float32(float64(grid.x) * 64.0), float32(float64(grid.y) * 64.0)}
	d := p.sub(center)

	limit := float32(float64(w) + 32.0)
	return absf(d.X) < limit && absf(d.Y) < limit
}

func absf(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}

func riverGrid(wx, wy float32) gridKey {
	// Mathf.FloorToInt of a value narrowed to float32 first.
	return gridKey{
		x: int32(math.Floor(float64(float32((float64(wx) + 32.0) / 64.0)))),
		y: int32(math.Floor(float64(float32((float64(wy) + 32.0) / 64.0)))),
	}
}

// riverWeight returns how strongly this point is inside water, and the width of
// the watercourse there.
func (d *riverData) riverWeight(wx, wy float32) (weight, width float32) {
	points, ok := d.points[riverGrid(wx, wy)]
	if !ok {
		return 0, 0
	}

	at := Vec2{wx, wy}

	var weightedWidth, totalWeight float32
	for _, rp := range points {
		sqr := rp.p.sub(at).sqrMagnitude()
		if sqr >= rp.w2 {
			continue
		}

		// Linear falloff from the centre of the circle to its edge.
		w := float32(1.0 - float64(float32(math.Sqrt(float64(sqr))))/float64(rp.w))
		if w > weight {
			weight = w
		}

		weightedWidth = float32(float64(weightedWidth) + float64(rp.w)*float64(w))
		totalWeight = float32(float64(totalWeight) + float64(w))
	}

	if totalWeight > 0 {
		width = float32(float64(weightedWidth) / float64(totalWeight))
	}
	return weight, width
}
