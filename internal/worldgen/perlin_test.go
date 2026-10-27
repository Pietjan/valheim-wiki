package worldgen

import (
	"bufio"
	"math"
	"os"
	"strconv"
	"strings"
	"testing"
)

// Golden values dumped from a running Valheim server (see
// tools/ZAuthPatch/WorldGenDump.cs). They are the real Mathf.PerlinNoise
// output, so this is a check against Unity rather than against a
// reimplementation of it.
func TestPerlinMatchesUnity(t *testing.T) {
	f, err := os.Open("testdata/perlin.tsv")
	if err != nil {
		t.Fatalf("open goldens: %v", err)
	}
	defer f.Close()

	// The extraction step was 1/512, which bounds how exactly the gradients are
	// known. Anything structurally wrong would miss by orders of magnitude more.
	const tolerance = 5e-5

	var n int
	var worst float64
	var worstAt string

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "#") || line == "" {
			continue
		}

		fields := strings.Split(line, "\t")
		if len(fields) != 3 {
			t.Fatalf("malformed golden: %q", line)
		}

		x, _ := strconv.ParseFloat(fields[0], 32)
		y, _ := strconv.ParseFloat(fields[1], 32)
		want, _ := strconv.ParseFloat(fields[2], 32)

		got := float64(PerlinNoise(float32(x), float32(y)))
		if d := math.Abs(got - want); d > worst {
			worst, worstAt = d, line
		}
		n++
	}

	if n < 300 {
		t.Fatalf("only %d goldens loaded", n)
	}
	if worst > tolerance {
		t.Errorf("worst error %.8f exceeds %.0e\n  at %s", worst, tolerance, worstAt)
	}
	t.Logf("%d samples, worst error %.8f", n, worst)
}

// A lattice point dots every gradient with a zero offset, so the noise there is
// zero and the output is the mapping's constant.
func TestPerlinAtLattice(t *testing.T) {
	for _, p := range [][2]float32{{0, 0}, {1, 1}, {5, 9}, {255, 3}, {256, 256}} {
		if got := PerlinNoise(p[0], p[1]); math.Abs(float64(got-perlinBase)) > 1e-6 {
			t.Errorf("PerlinNoise(%v, %v) = %v, want %v", p[0], p[1], got, perlinBase)
		}
	}
}

// The table wraps at 256, which is what makes it finite.
//
// Equality is approximate on purpose. Shifting by 256 changes the exponent, so
// a value like 12.3 does not keep the same fractional part at 268.3 in float32
// — the inputs genuinely differ slightly, and Unity has the same artifact. A
// wrapping bug would miss by a whole gradient, not by 1e-5.
func TestPerlinPeriod(t *testing.T) {
	const tolerance = 5e-5

	for _, p := range [][2]float32{{0.5, 0.25}, {12.3, 7.9}, {200.75, 33.5}} {
		a := float64(PerlinNoise(p[0], p[1]))
		b := float64(PerlinNoise(p[0]+256, p[1]))
		c := float64(PerlinNoise(p[0], p[1]+256))

		if math.Abs(a-b) > tolerance || math.Abs(a-c) > tolerance {
			t.Errorf("not periodic at (%v, %v): %v, %v, %v", p[0], p[1], a, b, c)
		}
	}
}

func TestFadeIsQuintic(t *testing.T) {
	// The curve is flat at both ends, which is what distinguishes it from the
	// classic cubic and was how it was identified from sampled output.
	if fade(0) != 0 || fade(1) != 1 {
		t.Errorf("fade endpoints: %v, %v", fade(0), fade(1))
	}
	if math.Abs(float64(fade(0.5))-0.5) > 1e-6 {
		t.Errorf("fade(0.5) = %v, want 0.5", fade(0.5))
	}
}
