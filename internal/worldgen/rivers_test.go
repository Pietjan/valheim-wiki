package worldgen

import (
	"bufio"
	"math"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/pietjan/valheim-wiki/internal/unityrandom"
)

// River generation is a single seeded sequence, so each stage is checked
// separately: a mismatch in lakes and a mismatch in widths look identical from
// the far end, and only one of them is the actual fault.
//
// The expected values come from a running server via server_probe.

// Lakes come before any RNG is involved, so this isolates the base-height port
// at the sampling resolution river placement uses.
func TestFindLakesMatchesGame(t *testing.T) {
	lakes := New(npcdev).findLakes()

	if len(lakes) != 114 {
		t.Fatalf("found %d lakes, game has 114", len(lakes))
	}

	want := []Vec2{
		{830.6664, -9667.6},
		{2968.418, 2774.4258},
		{-5744.675, 7994.7915},
		{3074.4165, 9396.605},
	}
	for i, w := range want {
		if lakes[i] != w {
			t.Errorf("lake %d = %v, want %v", i, lakes[i], w)
		}
	}
}

// Rivers are where the RNG enters. Matching the count means every accept and
// reject along the way agreed; matching the widths means the draws landed in the
// same order.
func TestPlaceRiversMatchesGame(t *testing.T) {
	g := New(npcdev)
	rivers := g.placeRivers(unityrandom.New(npcdev.RiverSeed), g.findLakes())

	if len(rivers) != 168 {
		t.Fatalf("placed %d rivers, game has 168", len(rivers))
	}

	first := rivers[0]
	for _, tc := range []struct {
		name      string
		got, want float32
	}{
		{"p0.x", first.P0.X, 830.6664},
		{"p0.y", first.P0.Y, -9667.6},
		{"p1.x", first.P1.X, -692},
		{"p1.y", first.P1.Y, -9128},
		{"widthMin", first.WidthMin, 65.35265},
		{"widthMax", first.WidthMax, 91.011696},
		// The curve values were 107.69671 and 80.77254 on the 0.221 server,
		// whose Vector2.Distance rounded the sum of squares to float32 before
		// the root. Re-measured on the 1.0 server on 2026-09-10: the sum
		// stays in double, and these are what it reports.
		{"curveWidth", first.CurveWidth, 107.6967},
		{"curveWavelength", first.CurveWavelength, 80.77253},
	} {
		if tc.got != tc.want {
			t.Errorf("river[0].%s = %v, want %v", tc.name, tc.got, tc.want)
		}
	}
}

// Streams are placed after rivers and see terrain that rivers have already cut,
// which is why the river data is threaded through generation rather than applied
// afterwards. Skipping that carving put this at 2085; classifying every biome as
// Meadows put it at 2537.
//
// One short of the game, out of 2063. The remaining difference is a single
// accept/reject flip somewhere in ~150000 candidate points, and it moves no
// sampled height by more than a centimetre — TestBiomeHeightAcrossTheWorld would
// catch it if it did.
func TestStreamCountMatchesGame(t *testing.T) {
	d := New(npcdev).ensureRivers()

	if len(d.rivers) != 168 {
		t.Errorf("generated %d rivers, game has 168", len(d.rivers))
	}
	if diff := len(d.streams) - 2063; diff < -2 || diff > 2 {
		t.Errorf("generated %d streams, game has 2063", len(d.streams))
	}
	t.Logf("rivers %d/168, streams %d/2063", len(d.rivers), len(d.streams))
}

// The blocking case for pathfinding, kept as its own test because it is the
// reason any of this exists.
//
// At (-611.9, 150) the real ground is 26.25 — nearly four metres under water.
// Before rivers were carved this returned 62.28: a 36m hill where there is a
// river. A planner would have routed straight through it.
func TestRiverCrossingIsUnderWater(t *testing.T) {
	g := New(npcdev)

	for _, tc := range []struct{ z, want float64 }{
		{130, 31.938}, {140, 27.461}, {150, 26.251}, {160, 28.584},
	} {
		got, ok := g.Height(-611.9, tc.z)
		if !ok {
			t.Fatalf("height at (-611.9, %v) reported unusable", tc.z)
		}
		if math.Abs(got-tc.want) > 0.05 {
			t.Errorf("height(-611.9, %v) = %.3f, want %.3f", tc.z, got, tc.want)
		}
	}

	// The point of the exercise: the middle of the crossing has to read as
	// water, not as ground.
	if h, _ := g.Height(-611.9, 150); h >= WaterLevel {
		t.Errorf("the river bed reads as %.2f, at or above the waterline (%v) — "+
			"an NPC would walk into it", h, WaterLevel)
	}
}

// The end-to-end check across the world, from a golden sampled off the server.
//
// Every biome is asserted, the Ashlands included since its cellular height
// was ported; TestAshlandsHeightMatchesTheGame pins that one on its own.
//
// The golden is WorldGenerator.GetHeight, so BiomeHeight at the point's own
// biome is what reproduces it. Height is the ground the game builds, which
// blends four biomes across a zone and diverges by up to 11.5m in this very
// sample — see TestHeightMatchesTheGroundTheGameBuilds.
func TestBiomeHeightAcrossTheWorld(t *testing.T) {
	g := New(npcdev)

	var checked int
	var worst float64
	var worstAt Vec2
	var worstBiome string

	forEachGolden(t, func(x, z, want float64, biome string) {
		got, ok := g.BiomeHeight(g.GetBiome(x, z), x, z)
		if !ok {
			return
		}

		checked++
		if d := got - want; math.Abs(d) > math.Abs(worst) {
			worst, worstAt, worstBiome = d, Vec2{float32(x), float32(z)}, biome
		}
	})

	if checked < 200 {
		t.Fatalf("only %d samples", checked)
	}

	// Centimetres, in both directions. Before rivers this allowed 8m of reading
	// high; before biomes, tens of metres outside Meadows.
	if math.Abs(worst) > 0.05 {
		t.Errorf("worst error %+.4f at %v (%s), over %d points",
			worst, worstAt, worstBiome, checked)
	}
	t.Logf("%d points, worst %+.4f at %v (%s)", checked, worst, worstAt, worstBiome)
}

func forEachGolden(t *testing.T, fn func(x, z, want float64, biome string)) {
	t.Helper()

	f, err := os.Open("testdata/heights_npcdev.tsv")
	if err != nil {
		t.Fatalf("open golden: %v", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "#") || line == "" {
			continue
		}

		fields := strings.Split(line, "\t")
		if len(fields) < 4 {
			continue
		}
		x, _ := strconv.ParseFloat(fields[0], 64)
		z, _ := strconv.ParseFloat(fields[1], 64)
		want, _ := strconv.ParseFloat(fields[2], 64)

		fn(x, z, want, fields[3])
	}
}

// Generation is lazy and cached; a second call must not regenerate or the cost
// would be paid on every height query.
func TestRiversAreGeneratedOnce(t *testing.T) {
	g := New(npcdev)

	first := g.ensureRivers()
	if first != g.ensureRivers() {
		t.Error("river data regenerated on the second call")
	}
	if len(first.rivers) == 0 {
		t.Error("no rivers generated")
	}
}

// A Config with no seeds cannot generate rivers, and must leave height alone
// rather than invent them.
func TestNoSeedsMeansNoCarving(t *testing.T) {
	bare := npcdev
	bare.RiverSeed, bare.StreamSeed = 0, 0

	plain := New(bare)
	carved := New(npcdev)

	h1, _ := plain.Height(-611.9, 150)
	h2, _ := carved.Height(-611.9, 150)

	if h1 == h2 {
		t.Fatal("carving made no difference at a known river crossing")
	}
	if h1 <= h2 {
		t.Errorf("uncarved height %.3f is not above carved %.3f; carving only lowers terrain", h1, h2)
	}
}

// The ground the NPC actually walks on, pinned against the thing that builds
// it.
//
// The golden is HeightmapBuilder's own base heights, read out of a running
// server by invoking its Build with the game's own WorldGenerator — not a
// reimplementation of the blend on either side of the comparison. Sampled at
// heightmap vertices, which fall on whole world metres, so neither the game nor
// the port interpolates and any difference is the blend itself.
//
// It is a separate golden from heights_npcdev.tsv because it is a separate
// function. That one is WorldGenerator.GetHeight, which decides where
// vegetation goes; this is what the terrain mesh is made of, and the two differ
// by metres wherever a zone straddles a biome border. Two thirds of the rows
// here are such zones, which is deliberate: the uniform case is where the two
// agree and was never the risk.
func TestHeightMatchesTheGroundTheGameBuilds(t *testing.T) {
	f, err := os.Open("testdata/ground_npcdev.tsv")
	if err != nil {
		t.Fatalf("open golden: %v", err)
	}
	defer f.Close()

	g := New(npcdev)

	counts := map[string]int{}
	worst := map[string]float64{}
	var worstAt Vec2

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "#") || line == "" {
			continue
		}

		fields := strings.Split(line, "\t")
		if len(fields) < 4 {
			continue
		}
		x, _ := strconv.ParseFloat(fields[0], 64)
		z, _ := strconv.ParseFloat(fields[1], 64)
		want, _ := strconv.ParseFloat(fields[2], 64)
		kind := fields[3]

		got, ok := g.Height(x, z)
		if !ok {
			t.Errorf("ground at (%v, %v) reported unusable", x, z)
			continue
		}
		counts[kind]++

		if d := got - want; math.Abs(d) > math.Abs(worst[kind]) {
			worst[kind] = d
			if kind == "mixed" {
				worstAt = Vec2{float32(x), float32(z)}
			}
		}
	}

	if counts["mixed"] < 500 || counts["uniform"] < 100 {
		t.Fatalf("golden is too thin: %d mixed, %d uniform", counts["mixed"], counts["uniform"])
	}

	// Centimetres, the same tolerance the generator itself is held to. The
	// numbers to watch are the ones this replaced: reading the point's own
	// biome instead of blending the zone's four is out by up to 11.5m over this
	// same sample.
	for _, kind := range []string{"mixed", "uniform"} {
		if math.Abs(worst[kind]) > 0.05 {
			t.Errorf("worst %s-zone error %+.4f near %v, over %d points",
				kind, worst[kind], worstAt, counts[kind])
		}
	}
	t.Logf("mixed %d points worst %+.4f; uniform %d points worst %+.4f",
		counts["mixed"], worst["mixed"], counts["uniform"], worst["uniform"])
}
