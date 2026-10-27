package worldgen

import ()

// Biome classification and the per-biome height functions.
//
// `GetBiomeHeight` branches on biome, and every branch layers different detail
// over the same `BaseHeight`. Porting only the Meadows branch — which is where
// this project started, because that is where the player was standing — left
// every other biome computing meadow terrain. Measured against the game that was
// 71m out on a mountain and 54m in the mistlands, far larger than the missing
// rivers ever were.
//
// It matters for more than height. Stream placement scatters candidate points
// across the whole world and accepts them by height, so without biomes the wrong
// streams get placed even inside Meadows.

// Biome values match Heightmap.Biome, which is a bit set.
const (
	BiomeNone      Biome = 0
	BiomeAshLands  Biome = 32
	BiomeDeepNorth Biome = 64
	BiomeMistlands Biome = 512
)

func (b Biome) String() string {
	switch b {
	case BiomeMeadows:
		return "Meadows"
	case BiomeSwamp:
		return "Swamp"
	case BiomeMountain:
		return "Mountain"
	case BiomeBlackForest:
		return "BlackForest"
	case BiomePlains:
		return "Plains"
	case BiomeAshLands:
		return "AshLands"
	case BiomeDeepNorth:
		return "DeepNorth"
	case BiomeOcean:
		return "Ocean"
	case BiomeMistlands:
		return "Mistlands"
	default:
		return "None"
	}
}

// Version-dependent thresholds. VersionSetup loosens these for old worlds;
// worldGenVersion 2 uses the tighter values.
func (g *Generator) maxMarshDistance() float64 {
	if g.cfg.WorldGenVersion <= 1 {
		return 8000
	}
	return 6000
}

func (g *Generator) minDarklandNoise() float64 {
	if g.cfg.WorldGenVersion <= 1 {
		return 0.5
	}
	return 0.4
}

// biomeScale is 0.001f widened to double, which is what the game multiplies by.
//
// Not the double 0.001. The literal in the decompiled source reads
// 0.0010000000474974513 — a float32 constant promoted — and using the exact
// decimal instead moves biome boundaries by metres. That was worth 57 of 300
// sample points misclassified, which then read as tens of metres of height
// error because the wrong branch computed the height.
const biomeScale = float64(float32(0.001))

// biomeNoise samples the classification noise.
//
// The cast placement is load-bearing too: the offset is added to the coordinate
// and narrowed to float32 *before* scaling.
func biomeNoise(offset, wx, wy float64) float64 {
	return noise(float64(float32(offset+wx))*biomeScale, float64(float32(offset+wy))*biomeScale)
}

// GetBiome classifies a world position.
//
// Order matters as much as the tests: ashlands wins over ocean, ocean over
// everything inland, and mountain is checked before the noise-driven biomes so
// high ground is never classified as swamp.
func (g *Generator) GetBiome(wx, wy float64) Biome {
	const oceanLevel = 0.02

	// float32 throughout, because the game's are floats and these are all
	// threshold comparisons. Keeping the extra precision flips a classification
	// whenever a sample lands within a float32 ulp of a boundary — rare per
	// point, but stream placement tests hundreds of thousands of them, and one
	// flip desynchronises every stream drawn afterwards.
	x, y := float32(wx), float32(wy)
	dist := dlength(x, y)
	base := g.base32(x, y)
	angle := float32(float64(worldAngle32(x, y)) * 100.0)

	if isAshlands32(x, y) {
		return BiomeAshLands
	}
	if base <= oceanLevel {
		return BiomeOcean
	}
	// Since 1.0 the Deep North is decided before the mountain test, so it no
	// longer has mountains inside it. Measured on the biome grid of a fresh
	// 1.0 world: 16,424 points the old order called Mountain.
	if isDeepNorth32(x, y) {
		return BiomeDeepNorth
	}
	if base > 0.4 {
		return BiomeMountain
	}

	if float32(biomeNoise(float64(g.cfg.Offset0), wx, wy)) > 0.6 &&
		dist > 2000 && dist < float32(g.maxMarshDistance()) && base > 0.05 && base < 0.25 {
		return BiomeSwamp
	}
	if float32(biomeNoise(float64(g.cfg.Offset4), wx, wy)) > float32(g.minDarklandNoise()) &&
		dist > float32(6000+float64(angle)) && dist < 10000 {
		return BiomeMistlands
	}
	if float32(biomeNoise(float64(g.cfg.Offset1), wx, wy)) > 0.4 &&
		dist > float32(3000+float64(angle)) && dist < 8000 {
		return BiomePlains
	}
	if float32(biomeNoise(float64(g.cfg.Offset2), wx, wy)) > 0.4 &&
		dist > float32(600+float64(angle)) && dist < 6000 {
		return BiomeBlackForest
	}
	if dist > float32(5000+float64(angle)) {
		return BiomeBlackForest
	}

	return BiomeMeadows
}
