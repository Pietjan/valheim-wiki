package worldgen

import (
	"math"
	"testing"
)

// Classification has to be exact before height can be. Every biome computes
// height differently, so a misclassified point is not slightly wrong — it runs
// the wrong function and can be tens of metres out.
func TestBiomeClassificationMatchesGame(t *testing.T) {
	g := New(npcdev)

	var checked, agree int
	mismatches := map[string]int{}

	forEachGolden(t, func(x, z, _ float64, want string) {
		checked++
		got := g.GetBiome(x, z).String()
		if got == want {
			agree++
			return
		}
		mismatches[want+" -> "+got]++
	})

	if checked < 200 {
		t.Fatalf("only %d samples", checked)
	}
	for m, n := range mismatches {
		t.Errorf("%-30s %d point(s)", m, n)
	}
	if agree != checked {
		t.Errorf("biome agreement %d/%d", agree, checked)
	}
}

// Mathf.PerlinNoise mirrors about each axis. Nothing sampled a negative
// coordinate until GetBiome — every other call site adds 100000 first — so this
// went undetected for a long time, and got the wrong explanation once already.
//
// The values are from the running game.
func TestPerlinIsMirrored(t *testing.T) {
	for _, tc := range []struct {
		x, y, want float32
	}{
		{-0.86721, -9.1213, 0.388583362},
		{-2.49811, 3.39236, 0.352673829},
		{-0.5, -0.5, 0.2966959},
		{-1.25, 2.5, 0.674513},
		{0.75, 1.25, 0.5176242},
	} {
		// 2e-5 matches the port's own accuracy: the gradients were measured by
		// sampling, and TestPerlinMatchesUnity puts the worst case at 1.54e-5.
		// The point here is the mirror, not extra precision.
		got := PerlinNoise(tc.x, tc.y)
		if math.Abs(float64(got-tc.want)) > 2e-5 {
			t.Errorf("PerlinNoise(%v, %v) = %v, want %v", tc.x, tc.y, got, tc.want)
		}
	}

	// The symmetry itself, and the fact that it composes with the period. Both
	// were measured: -255.5 returns what 255.5 does, and -256.5 what 0.5 does.
	for _, x := range []float32{0.5, 1.5, 255.5, 256.5} {
		if PerlinNoise(x, 0.25) != PerlinNoise(-x, 0.25) {
			t.Errorf("noise(%v) != noise(%v); the function is mirrored in x", x, -x)
		}
		if PerlinNoise(0.25, x) != PerlinNoise(0.25, -x) {
			t.Errorf("noise(y=%v) != noise(y=%v); the function is mirrored in y", x, -x)
		}
	}

	// A negative input must stay in range. Reading it as truncation toward zero
	// produced values above 24, which is what made the biome noise nonsense.
	for _, tc := range [][2]float32{{-0.86721, -9.1213}, {-2.49811, 3.39236}, {-1234.5, -6789.25}} {
		if v := PerlinNoise(tc[0], tc[1]); v < 0 || v > 1 {
			t.Errorf("PerlinNoise(%v, %v) = %v, outside [0,1]", tc[0], tc[1], v)
		}
	}
}

// The biome noise is scaled by 0.001f widened to double, not by the double
// 0.001. Using the decimal moves biome edges by metres.
func TestBiomeScaleIsAFloatConstant(t *testing.T) {
	if biomeScale == 0.001 {
		t.Fatal("biomeScale is the exact decimal; it must be 0.001f widened")
	}
	if got, want := biomeScale, 0.0010000000474974513; math.Abs(got-want) > 1e-20 {
		t.Errorf("biomeScale = %.17g, want %.17g", got, want)
	}
}
