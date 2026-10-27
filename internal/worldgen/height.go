package worldgen

import (
	"github.com/pietjan/valheim-wiki/internal/scalar"
	"math"
	"sync"

	"github.com/pietjan/valheim-wiki/internal/unityrandom"
)

// Config is the per-world state WorldGenerator derives from the seed.
//
// All of it can now be computed with ConfigFromSeed: internal/unityrandom
// reproduces the PRNG these come from. Supplying a Config directly is still
// supported, and is how a world whose seed is unknown can be described.
type Config struct {
	// Seed is the world seed itself, which location placement seeds from
	// again per prefab. Zero for a world described by its constants alone.
	Seed int32

	Offset0, Offset1, Offset2, Offset3, Offset4 float32
	MinMountainDistance                         float32
	WorldGenVersion                             int32

	// Seeds for river and stream placement. Without them rivers cannot be
	// generated, and terrain reads high over every watercourse.
	RiverSeed, StreamSeed int32
}

// ConfigFromSeed derives a world's constants from its seed alone, the way
// WorldGenerator.Initialize does.
//
// The order of these draws is the specification: they come from one seeded
// sequence, so inserting or reordering a single call changes every value after
// it. Note that offset4 is drawn last, after both river seeds.
func ConfigFromSeed(seed int32) Config {
	r := unityrandom.New(seed)

	cfg := Config{Seed: seed, WorldGenVersion: 2, MinMountainDistance: 1000}
	cfg.Offset0 = float32(r.RangeInt(-10000, 10000))
	cfg.Offset1 = float32(r.RangeInt(-10000, 10000))
	cfg.Offset2 = float32(r.RangeInt(-10000, 10000))
	cfg.Offset3 = float32(r.RangeInt(-10000, 10000))
	cfg.RiverSeed = r.RangeInt(math.MinInt32, math.MaxInt32)
	cfg.StreamSeed = r.RangeInt(math.MinInt32, math.MaxInt32)
	cfg.Offset4 = float32(r.RangeInt(-10000, 10000))

	return cfg
}

// Constants shared with WorldGenerator.
const (
	HeightMultiplier    = 200.0
	WaterLevel          = 30.0
	ashlandsMinDistance = 12000.0
	ashlandsYOffset     = -4000.0
)

// Generator computes terrain height for one world. Safe for concurrent use.
type Generator struct {
	cfg Config

	// River generation is expensive and lazy; see ensureRivers.
	riverOnce sync.Once
	riverData *riverData

	// zones caches each zone's four corner biomes; see zoneBiomes.
	zones sync.Map
}

// New returns a Generator for the given world constants.
func New(cfg Config) *Generator { return &Generator{cfg: cfg} }

// Seed is the world seed the generator was configured from.
func (g *Generator) Seed() int32 { return g.cfg.Seed }

// ---------------------------------------------------------------------------
// DUtils equivalents. These are deliberately literal ports: the originals do
// their arithmetic in double and cast back to float at specific points, and
// reproducing where those casts fall matters more than the code looking tidy.
// ---------------------------------------------------------------------------

func length(x, y float64) float64 { return math.Sqrt(x*x + y*y) }

func smoothStep(min, max, x float64) float64 {
	n := scalar.Clamp01((x - min) / (max - min))
	return n * n * (3 - 2*n)
}

// noise is DUtils.PerlinNoise: the arguments are computed in double and cast to
// float before sampling, which is why the terrain is sensitive to exactly where
// the casts fall.
func noise(x, y float64) float64 {
	return float64(PerlinNoise(float32(x), float32(y)))
}

// ---------------------------------------------------------------------------
// Height
// ---------------------------------------------------------------------------

// BaseHeight is the continent shape everything else is layered onto: a product
// of noise octaves, tapered towards the world edge and flattened where mountains
// would otherwise appear too close to the centre.
//
// This one is bit-exact, not merely close: it is compared against thresholds
// — 0.02 for ocean, 0.4 for mountain, 0.05 and 0.25 for swamp — at every
// point of the biome grid, and a point that lands an ulp on the other side
// of one is a different biome, which lengthens that biome's point list by
// one and moves every location drawn from it. So every float32 narrowing
// the game does is done here too, and the frequency constants are the
// game's float32 literals widened, not the decimal they stand for.
func (g *Generator) BaseHeight(wx, wy float64) float64 {
	dist := dlength(float32(wx), float32(wy))

	x := wx + 100000 + float64(g.cfg.Offset0)
	y := wy + 100000 + float64(g.cfg.Offset1)

	const (
		f002 = float64(float32(0.002))
		f003 = float64(float32(0.003))
		f005 = float64(float32(0.005))
		f01  = float64(float32(0.01))
	)

	var h float32
	h = float32(float64(h) + float64(noise32(x*f002*0.5, y*f002*0.5))*float64(noise32(x*f003*0.5, y*f003*0.5))*1.0)
	h = float32(float64(h) + float64(noise32(x*f002*1.0, y*f002*1.0))*float64(noise32(x*f003*1.0, y*f003*1.0))*float64(h)*float64(float32(0.9)))
	h = float32(float64(h) + float64(noise32(x*f005*1.0, y*f005*1.0))*float64(noise32(x*f01*1.0, y*f01*1.0))*0.5*float64(h))
	h = float32(float64(h) - float64(float32(0.07)))

	// A second, low-frequency pair carves the big rivers and lake basins.
	n1 := noise32(x*f002*0.25+float64(float32(0.123)), y*f002*0.25+float64(float32(0.15123)))
	n2 := noise32(x*f002*0.25+float64(float32(0.321)), y*f002*0.25+float64(float32(0.231)))
	v := float32(math.Abs(float64(n1) - float64(n2)))

	rivers := float32(1.0 - float64(lerpStep32(0.02, 0.12, v)))
	rivers = float32(float64(rivers) * float64(smoothStep32(744, 1000, dist)))
	h = float32(float64(h) * (1.0 - float64(rivers)))

	if dist > 10000 {
		h = lerp32(h, -0.2, lerpStep32(10000, 10500, dist))
		if dist > 10490 {
			h = lerp32(h, -2, lerpStep32(10490, 10500, dist))
		}
		return float64(h)
	}

	// Keep mountains away from the spawn area.
	minMountain := g.cfg.MinMountainDistance
	if dist < minMountain && h > 0.28 {
		t := float32(scalar.Clamp01((float64(h) - float64(float32(0.28))) / float64(float32(0.1))))
		h = lerp32(lerp32(0.28, 0.38, t), h, lerpStep32(float32(float64(minMountain)-400), minMountain, dist))
	}
	return float64(h)
}

// The float32 DUtils: each narrows exactly where the game's does.

// dlength is DUtils.Length: float32 in, the root taken in double, float32 out.
func dlength(x, y float32) float32 {
	return float32(math.Sqrt(float64(x)*float64(x) + float64(y)*float64(y)))
}

// noise32 is DUtils.PerlinNoise(double, double): the arguments narrowed.
func noise32(x, y float64) float32 { return PerlinNoise(float32(x), float32(y)) }

func lerpStep32(l, h, v float32) float32 {
	return float32(scalar.Clamp01((float64(v) - float64(l)) / (float64(h) - float64(l))))
}

func smoothStep32(min, max, x float32) float32 {
	n := float32(scalar.Clamp01((float64(x) - float64(min)) / (float64(max) - float64(min))))
	return float32(float64(n) * float64(n) * (3.0 - 2.0*float64(n)))
}

// lerp32 is DUtils.Lerp(float, float, float), clamped at both ends.
func lerp32(a, b, t float32) float32 {
	if t <= 0 {
		return a
	}
	if t >= 1 {
		return b
	}
	return float32(float64(a) + (float64(b)-float64(a))*float64(t))
}

// Biome identifies the terrain type at a point. See biome.go for the rest of
// the values and for classification.
type Biome int

const (
	BiomeMeadows     Biome = 1
	BiomeSwamp       Biome = 2
	BiomeMountain    Biome = 4
	BiomeBlackForest Biome = 8
	BiomePlains      Biome = 16
	BiomeOcean       Biome = 256
)

// BiomeHeight is one biome's height function evaluated at a point, in world
// units, and whether the value is trustworthy: WorldGenerator.GetBiomeHeight.
//
// It is false beyond the world edge, where the game returns a fixed depth
// rather than a height. It used to be false in the Ashlands too, while that
// biome's cellular noise was unported; since 2026-09-10 every biome is.
//
// Asking for a biome rather than reading it off the position is what Height
// needs, and it is not a hypothetical: the ground is built from the biomes at a
// zone's four corners, and three of those four are usually not the biome under
// the point.
func (g *Generator) BiomeHeight(biome Biome, wx, wy float64) (float64, bool) {
	if length(wx, wy) > 10500 {
		return -2 * HeightMultiplier, false
	}

	h, _ := g.biomeHeight32(biome, float32(wx), float32(wy), g.ensureRivers(), false, true)
	return float64(h), true
}

// MeadowsHeight is the Meadows height function alone, in world units.
func (g *Generator) MeadowsHeight(wx, wy float64) float64 {
	return float64(g.meadows32(float32(wx), float32(wy), g.ensureRivers())) * HeightMultiplier
}

// Vegetation is the alpha of the colour mask GetBiomeHeight hands back beside
// the height: how much grows at a point, which location placement reads as
// its vegetation rules. It is not a separate function in the game; each
// biome's height writes it as a side effect, and most write nothing, leaving
// Color.black — whose alpha is one. The Mistlands write the plateau weight
// inverted, so the flat tops are bare and the slopes between them grow; the
// Deep North writes nought; the Ashlands write where the lava lies.
func (g *Generator) Vegetation(biome Biome, wx, wy float64) float32 {
	_, v := g.biomeHeight32(biome, float32(wx), float32(wy), g.ensureRivers(), false, true)
	return v
}

// zoneSize is ZoneSystem.m_zoneSize, and the span the corner blend below runs
// over. It is a world constant rather than a tuning knob: the ground is built
// one zone at a time and each zone is blended across its own width.
const zoneSize = 64.0

// Height returns the height of the ground at a world position, in world units,
// and whether the value is trustworthy.
//
// **This is not WorldGenerator.GetHeight**, and the difference is the whole
// reason this function exists rather than being one line. GetHeight picks the
// biome under the point and evaluates that biome's height function, which steps
// discontinuously wherever the classification changes. The ground the NPC walks
// on is built by HeightmapBuilder instead: it takes the biome at the *zone's
// four corners*, evaluates all four height functions at every point, and blends
// them bilinearly across the 64m zone with a smoothstep on each axis. Only when
// all four corners agree do the two functions coincide, and then only exactly.
//
// The gap is not academic. Measured live at a Meadows/Black Forest border near
// (112, -629) on npcdev, GetHeight steps 6.2m over 0.9m — read as a 63 degree
// face, against a walk limit of 38, along a border where the built ground never
// exceeds 32.5. The planner refused every route across it. Against the y of 171
// real scenery ZDOs around that border, which sit on the ground the game built,
// GetHeight is out by a mean of 1.193m and this by 0.042m.
//
// The blend is continuous across zone seams by construction rather than by
// luck: neighbouring zones share the corner sample points, so each reads the
// same biome there and the two edges evaluate to the same number.
//
// One deliberate difference from the game remains. HeightmapBuilder evaluates
// this at 1m vertices and the mesh interpolates between them, where this is
// evaluated continuously — so between two vertices the game is a shade flatter
// than this is. That is the same approximation the port already made before the
// blend existed, and it is centimetres.
func (g *Generator) Height(wx, wy float64) (float64, bool) {
	corners, uniform := g.zoneBiomes(wx, wy)
	if uniform {
		return g.BiomeHeight(corners[0], wx, wy)
	}

	var h [4]float64
	for i, biome := range corners {
		v, ok := g.BiomeHeight(biome, wx, wy)
		if !ok {
			// Any corner the port cannot compute poisons the whole blend, since
			// its height is a term in every point of the zone. Refusing is the
			// same answer the point's own biome would have given.
			return v, false
		}
		h[i] = v
	}

	x0, y0 := zoneOrigin(wx, wy)
	tx := smoothStep(0, 1, (wx-x0)/zoneSize)
	ty := smoothStep(0, 1, (wy-y0)/zoneSize)

	lo := h[0] + (h[1]-h[0])*tx
	hi := h[2] + (h[3]-h[2])*tx

	return lo + (hi-lo)*ty, true
}

// zoneOrigin is the south-west corner of the zone containing a point, matching
// ZoneSystem.GetZone and the centre HeightmapBuilder is handed.
func zoneOrigin(wx, wy float64) (x, y float64) {
	zx := math.Floor((wx + zoneSize/2) / zoneSize)
	zy := math.Floor((wy + zoneSize/2) / zoneSize)
	return zx*zoneSize - zoneSize/2, zy*zoneSize - zoneSize/2
}

// zoneBiomes returns the biomes at the four corners of a point's zone, in
// HeightmapBuilder's order — (0,0), (1,0), (0,1), (1,1) — and whether they all
// agree, which is the case that needs no blending at all.
//
// Cached per zone, because the whole point of the corner blend is that it is
// the same four classifications for every one of the 4096 square metres it
// covers, and GetBiome is not cheap: it computes the base height, which is six
// Perlin samples before any biome noise. A planner search evaluates tens of
// thousands of points, five samples each, and without this every one of them
// would classify four corners from scratch.
//
// Never evicted, and it does not need to be: the world is 21km across, so there
// are about 110000 zones in it and four biomes each is a few megabytes if an
// NPC somehow visits every one. Terrain is a pure function of the seed, so
// nothing cached here can go stale.
func (g *Generator) zoneBiomes(wx, wy float64) (corners [4]Biome, uniform bool) {
	x0, y0 := zoneOrigin(wx, wy)

	key := [2]float64{x0, y0}
	if got, ok := g.zones.Load(key); ok {
		c := got.([4]Biome)
		return c, c[0] == c[1] && c[0] == c[2] && c[0] == c[3]
	}

	corners = [4]Biome{
		g.GetBiome(x0, y0),
		g.GetBiome(x0+zoneSize, y0),
		g.GetBiome(x0, y0+zoneSize),
		g.GetBiome(x0+zoneSize, y0+zoneSize),
	}
	g.zones.Store(key, corners)

	return corners, corners[0] == corners[1] && corners[0] == corners[2] && corners[0] == corners[3]
}

// ZoneVertexHeight is the height HeightmapBuilder.Build gives one vertex of
// one zone's heightmap: vertex (l, k) of zone (zx, zz), l and k from 0 to
// 64 eastward and northward of the zone's south-west corner. The game's
// arithmetic throughout, in float as it has it: the corner biomes blended
// by DUtils.Lerp at DUtils.SmoothStep weights of the vertex's index. Height
// is the same blend evaluated continuously in double; this is what the
// heightmap's collider is made of, which a ray the game casts lands on, to
// the last bit a threshold on its slope can turn on.
func (g *Generator) ZoneVertexHeight(zx, zz, l, k int) float32 {
	x0 := float32(float64(zx)*zoneSize - zoneSize/2)
	z0 := float32(float64(zz)*zoneSize - zoneSize/2)
	wx := float32(float64(x0) + float64(l))
	wy := float32(float64(z0) + float64(k))
	corners, uniform := g.zoneBiomes(float64(x0)+zoneSize/2, float64(z0)+zoneSize/2)
	rivers := g.ensureRivers()
	if uniform {
		h, _ := g.biomeHeight32(corners[0], wx, wy, rivers, false, true)
		return h
	}
	t := smoothStep32(0, 1, float32(float64(k)/zoneSize))
	t2 := smoothStep32(0, 1, float32(float64(l)/zoneSize))
	var h [4]float32
	for i, b := range corners {
		h[i], _ = g.biomeHeight32(b, wx, wy, rivers, false, true)
	}
	return lerpD32(lerpD32(h[0], h[1], t2), lerpD32(h[2], h[3], t2), t)
}

// lerpD32 is DUtils.Lerp(float, float, float) as written: a(1-t) + bt in
// double, clamped at both ends.
func lerpD32(a, b, t float32) float32 {
	if t <= 0 {
		return a
	}
	if t >= 1 {
		return b
	}
	return float32(float64(a)*(1-float64(t)) + float64(b)*float64(t))
}
