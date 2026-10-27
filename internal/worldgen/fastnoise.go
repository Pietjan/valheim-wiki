package worldgen

// The slice of FastNoise the game uses.
//
// assembly_utils ships Auburn's FastNoise, the C# one, and WorldGenerator
// holds a single instance of it for the Ashlands: cellular noise returning
// the squared Euclidean distance to the nearest jittered cell point, and
// simplex noise summed over two octaves. Nothing else in the game reads it,
// so nothing else is ported — no 3D, no other distance functions or return
// types, no Perlin or value or cubic variants.
//
// It is a double generator throughout, unlike the float32 Perlin the other
// biomes are built on, so there are no narrowings to place. What there is
// instead is integer hashing: every cell and gradient is picked by an int32
// hash that overflows on purpose, and Go's int32 wraps the same way C#'s
// unchecked int does.

// fastNoise is the configuration WorldGenerator.m_noiseGen carries. The
// noise type, distance function and return type are fixed, since only one
// combination is ever asked for.
type fastNoise struct {
	seed      int32
	frequency float64

	// Fractal settings for GetSimplexFractal: octave count, lacunarity, gain
	// and the normalisation CalculateFractalBounding derives from them.
	octaves    int
	lacunarity float64
	gain       float64
	bounding   float64

	// jitter is m_cellularJitter, a float32 0.45 the cellular pass widens.
	jitter float64
}

// ashlandsNoise is WorldGenerator.m_noiseGen as its constructor leaves it.
// The instance is built with the world seed and then SetSeed(0) on every
// construction, so the seed is 0 for every world; the frequency is the
// library default; octaves are set to 2 and the rest are defaults.
var ashlandsNoise = newFastNoise(0, 2)

func newFastNoise(seed int32, octaves int) *fastNoise {
	n := &fastNoise{
		seed:       seed,
		frequency:  0.01,
		octaves:    octaves,
		lacunarity: 2.0,
		gain:       0.5,
		jitter:     float64(float32(0.45)),
	}
	n.calculateFractalBounding()
	return n
}

// calculateFractalBounding is what SetFractalOctaves and SetFractalGain
// recompute: one over the sum of the octave amplitudes.
func (n *fastNoise) calculateFractalBounding() {
	amp := n.gain
	sum := 1.0
	for i := 1; i < n.octaves; i++ {
		sum += amp
		amp *= n.gain
	}
	n.bounding = 1.0 / sum
}

// fastFloor and fastRound are the library's own: a truncating cast, adjusted.
func fastFloor(f float64) int32 {
	if f >= 0 {
		return int32(f)
	}
	return int32(f) - 1
}

func fastRound(f float64) int32 {
	if f >= 0 {
		return int32(f + 0.5)
	}
	return int32(f - 0.5)
}

// hash2D is FastNoise.Hash2D: two primes, a cube, a fold. It relies on int32
// overflow, which wraps here as it does in C#.
func hash2D(seed, x, y int32) int32 {
	h := seed
	h ^= 1619 * x
	h ^= 31337 * y
	h = h * h * h * 60493
	return (h >> 13) ^ h
}

// grad2D is FastNoise.GRAD_2D, the eight gradients simplex picks from.
var grad2D = [8][2]float64{
	{-1, -1}, {1, -1}, {-1, 1}, {1, 1},
	{0, -1}, {-1, 0}, {0, 1}, {1, 0},
}

// gradCoord2D is FastNoise.GradCoord2D: the hash again, inlined there, then
// the dot of the picked gradient with the offset.
func gradCoord2D(seed, x, y int32, xd, yd float64) float64 {
	g := grad2D[hash2D(seed, x, y)&7]
	return xd*g[0] + yd*g[1]
}

// Simplex skew constants, as the library's literals.
const (
	simplexF2  = 0.3660254037844386
	simplexG2  = 0.21132486540518713
	simplexG22 = 0.42264973081037427
)

// singleSimplex is FastNoise.SingleSimplex(int, double, double): the 2D
// simplex kernel at one seed and frequency.
func singleSimplex(seed int32, x, y float64) float64 {
	t := (x + y) * simplexF2
	i := fastFloor(x + t)
	j := fastFloor(y + t)
	t = float64(i+j) * simplexG2
	x0 := x - (float64(i) - t)
	y0 := y - (float64(j) - t)

	var i1, j1 int32
	if x0 > y0 {
		i1, j1 = 1, 0
	} else {
		i1, j1 = 0, 1
	}
	x1 := x0 - float64(i1) + simplexG2
	y1 := y0 - float64(j1) + simplexG2
	x2 := x0 - 1.0 + simplexG22
	y2 := y0 - 1.0 + simplexG22

	var n0, n1, n2 float64
	t = 0.5 - x0*x0 - y0*y0
	if t >= 0 {
		t *= t
		n0 = t * t * gradCoord2D(seed, i, j, x0, y0)
	}
	t = 0.5 - x1*x1 - y1*y1
	if t >= 0 {
		t *= t
		n1 = t * t * gradCoord2D(seed, i+i1, j+j1, x1, y1)
	}
	t = 0.5 - x2*x2 - y2*y2
	if t >= 0 {
		t *= t
		n2 = t * t * gradCoord2D(seed, i+1, j+1, x2, y2)
	}
	return 50.0 * (n0 + n1 + n2)
}

// SimplexFractal is FastNoise.GetSimplexFractal(double, double) with the
// fractal type at its default, FBM: each octave doubles the frequency, halves
// the amplitude and bumps the seed, and the sum is normalised by bounding.
func (n *fastNoise) SimplexFractal(x, y float64) float64 {
	x *= n.frequency
	y *= n.frequency

	seed := n.seed
	sum := singleSimplex(seed, x, y)
	amp := 1.0
	for i := 1; i < n.octaves; i++ {
		x *= n.lacunarity
		y *= n.lacunarity
		amp *= n.gain
		seed++
		sum += singleSimplex(seed, x, y) * amp
	}
	return sum * n.bounding
}

// Cellular is FastNoise.GetCellular(double, double) for the Euclidean
// distance function and the Distance return type: the squared distance —
// the library never takes the root — from the point to the nearest of the
// nine surrounding cells' jittered feature points.
func (n *fastNoise) Cellular(x, y float64) float64 {
	x *= n.frequency
	y *= n.frequency

	xr := fastRound(x)
	yr := fastRound(y)
	best := 999999.0
	for xi := xr - 1; xi <= xr+1; xi++ {
		for yi := yr - 1; yi <= yr+1; yi++ {
			c := cell2D[hash2D(n.seed, xi, yi)&0xFF]
			vx := float64(xi) - x + c[0]*n.jitter
			vy := float64(yi) - y + c[1]*n.jitter
			d := vx*vx + vy*vy
			if d < best {
				best = d
			}
		}
	}
	return best
}
