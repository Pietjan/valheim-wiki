package worldgen

// Rivers is every river and stream of the world, as lines: the generator
// places them once, from the seed, and the height field carves them. A
// viewer draws them from here rather than tracing the carved ground, which
// is the same arithmetic in reverse and worse at it. Generated on the first
// call; see ensureRivers.
func (g *Generator) Rivers() (rivers, streams []River) {
	d := g.ensureRivers()
	return d.rivers, d.streams
}

// Fbm is DUtils.Fbm: fractal Brownian motion over the game's Perlin noise,
// the sum of octaves each scaled by lacunarity and weighted by gain. The
// clutter system masks grass by it.
func Fbm(x, y float32, octaves int, lacunarity, gain float32) float32 {
	var sum, amp float32 = 0, 1
	for i := 0; i < octaves; i++ {
		sum = float32(float64(sum) + float64(amp)*float64(PerlinNoise(x, y)))
		amp = float32(float64(amp) * float64(gain))
		x *= lacunarity
		y *= lacunarity
	}
	return sum
}
