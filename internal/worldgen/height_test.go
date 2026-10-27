package worldgen

import (
	"bufio"
	"math"
	"os"
	"strconv"
	"strings"
	"testing"
)

// npcdev is the dev world, derived from its seed rather than transcribed from a
// dump. TestConfigFromSeedMatchesTheDump pins it against the measured values.
const npcdevSeed = -1365230248

var npcdev = ConfigFromSeed(npcdevSeed)

// The constants used to be copied out of a server dump by hand. Now they are
// computed, so this guards the computation against the recorded originals.
func TestConfigFromSeedMatchesTheDump(t *testing.T) {
	want := Config{
		Seed:    npcdevSeed,
		Offset0: 3178, Offset1: -8412, Offset2: 3297, Offset3: 8915, Offset4: -8503,
		MinMountainDistance: 1000,
		WorldGenVersion:     2,
		RiverSeed:           328821593,
		StreamSeed:          -1078268828,
	}

	if got := ConfigFromSeed(npcdevSeed); got != want {
		t.Errorf("ConfigFromSeed(%d) =\n  %+v\nwant\n  %+v", npcdevSeed, got, want)
	}
}

// Heights dumped from the running game. This is the end-to-end check: whatever
// the noise and the layering do internally, the port is only right if these
// match.
//
// The goldens are WorldGenerator.GetHeight, which is the point's own biome
// evaluated at the point, so that is what is compared here rather than Height —
// Height is the ground HeightmapBuilder builds and blends four biomes across a
// zone, which is a different number wherever a zone straddles a border. Every
// layer this file is really testing is shared by the two.
func TestBiomeHeightMatchesGame(t *testing.T) {
	f, err := os.Open("testdata/height.tsv")
	if err != nil {
		t.Fatalf("open goldens: %v", err)
	}
	defer f.Close()

	g := New(npcdev)

	var checked int
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "#") || line == "" {
			continue
		}

		fields := strings.Split(line, "\t")
		x, _ := strconv.ParseFloat(fields[0], 64)
		y, _ := strconv.ParseFloat(fields[1], 64)
		want, _ := strconv.ParseFloat(fields[2], 64)
		got, ok := g.BiomeHeight(g.GetBiome(x, y), x, y)
		if !ok {
			continue
		}
		checked++

		// Centimetres in both directions. This allowed 8m of reading high when
		// rivers were uncarved, and was restricted to Meadows when the other
		// biomes fell back to it.
		if d := got - want; math.Abs(d) > 0.05 {
			t.Errorf("height(%v, %v) = %.4f, want %.4f (off by %+.4f)", x, y, got, want, d)
		}
	}

	if checked < 5 {
		t.Fatalf("only %d meadows samples checked", checked)
	}
	t.Logf("checked %d samples", checked)
}

// A profile across biome borders, pinned against the running game in both of
// its height semantics at once (#96).
//
// This profile is where the port was accused of being up to 7.7m wrong. It was
// not: the instrument was diffing Height — the built ground, four corner biomes
// blended across the zone — against the server's GetHeight, which evaluates only
// the biome under the point. Those are the game's own two functions and they
// part by metres wherever a zone straddles a border, which is exactly where
// every "error" clustered, and why four consecutive Mountain interior samples
// agreed to 5mm.
//
// The goldens are both sides of that split, probed on npcdev 2026-08-17:
// getHeight is WorldGenerator.GetHeight verbatim, and built reproduces
// HeightmapBuilder's blend with the server's own GetBiome, GetBiomeHeight and
// DUtils — corner biomes at the zone corners, SmoothStep weights, bilinear
// lerp. Every sample lies on a heightmap vertex, so neither side interpolates.
//
// Pinning both matters: getHeight catches the height functions and the point's
// classification drifting, built additionally catches the corner classification
// and the blend itself. A future regression in either semantic trips here at
// the exact coordinates where the two are furthest apart.
func TestHeightAtBiomeBordersMatchesTheGame(t *testing.T) {
	g := New(npcdev)

	samples := []struct {
		x, z             float64
		biome            Biome
		getHeight, built float64
	}{
		{-300, -3000, BiomeSwamp, 29.164, 27.865},
		{-250, -3000, BiomeSwamp, 29.909, 37.641},
		{-200, -3000, BiomeBlackForest, 54.071, 54.071},
		{-150, -3000, BiomeBlackForest, 79.121, 79.013},
		{-100, -3000, BiomeMountain, 103.663, 103.535},
		{-50, -3000, BiomeBlackForest, 82.523, 83.104},
		{0, -3000, BiomeMountain, 97.011, 91.243},
		{50, -3000, BiomeMountain, 104.100, 97.436},
		{100, -3000, BiomeMountain, 118.486, 118.486},
		{150, -3000, BiomeMountain, 119.455, 119.455},
		{200, -3000, BiomeMountain, 143.786, 143.786},
		{250, -3000, BiomeMountain, 149.424, 149.424},
		{300, -3000, BiomeMountain, 123.345, 121.983},
		{350, -3000, BiomeMountain, 89.336, 86.161},
		{400, -3000, BiomeBlackForest, 68.027, 67.376},
		{0, -6000, BiomeBlackForest, 12.692, 8.388},
	}

	for _, s := range samples {
		biome := g.GetBiome(s.x, s.z)
		if biome != s.biome {
			t.Errorf("GetBiome(%v, %v) = %v, want %v", s.x, s.z, biome, s.biome)
		}

		point, ok := g.BiomeHeight(biome, s.x, s.z)
		if !ok {
			t.Errorf("BiomeHeight(%v, %v, %v) not trustworthy", biome, s.x, s.z)
		}
		if d := point - s.getHeight; math.Abs(d) > 0.05 {
			t.Errorf("point height(%v, %v) = %.4f, want %.4f (off by %+.4f)",
				s.x, s.z, point, s.getHeight, d)
		}

		built, ok := g.Height(s.x, s.z)
		if !ok {
			t.Errorf("Height(%v, %v) not trustworthy", s.x, s.z)
		}
		if d := built - s.built; math.Abs(d) > 0.05 {
			t.Errorf("built height(%v, %v) = %.4f, want %.4f (off by %+.4f)",
				s.x, s.z, built, s.built, d)
		}
	}
}

// Beyond the world edge the game returns a fixed depth and the NPC should not
// trust it as walkable ground.
func TestHeightBeyondWorldEdge(t *testing.T) {
	g := New(npcdev)

	if _, ok := g.Height(10600, 0); ok {
		t.Error("height beyond the edge reported as reliable")
	}
}
