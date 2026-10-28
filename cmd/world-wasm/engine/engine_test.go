package engine

import (
	"context"
	"math"
	"math/rand/v2"
	"regexp"
	"sync"
	"testing"

	"github.com/pietjan/valheim-wiki/internal/model"
	"github.com/pietjan/valheim-wiki/internal/worldgen"
	"github.com/pietjan/valheim-wiki/internal/worldmap"
)

// The measured world: seed CXJeyEhsEp, as the worldgen reader report and
// valheim-world's own README have it.
const (
	testSeed       = "CXJeyEhsEp"
	testSeedNumber = -729470066
	wantSites      = 12436
	wantRules      = 183
	wantSectors    = 904
	wantAltSectors = 73
	wantRivers     = 173
	wantStreams    = 2090
)

// One prepared and placed world for every test here, since the placement
// loop is ten seconds and the tests read it, not race it.
var shared = sync.OnceValues(func() (*World, []RuleReport) {
	w, err := New(testSeed, DefaultMapSize)
	if err != nil {
		panic(err)
	}
	w.Prepare()
	return w, w.Place(nil)
})

func world(t *testing.T) (*World, []RuleReport) {
	t.Helper()
	if testing.Short() {
		t.Skip("builds and places a whole world; skipped in -short")
	}
	return shared()
}

func TestVersionNamesTheTables(t *testing.T) {
	v := Version()
	if !regexp.MustCompile(`^v1-[0-9a-f]{8}$`).MatchString(v) {
		t.Fatalf("version %q", v)
	}
	if Version() != v {
		t.Fatal("version is not stable")
	}
}

// The slugs are model.Biomes', bit for bit.
func TestBiomeSlugsAreTheModels(t *testing.T) {
	if len(biomeSlugs) != len(model.BiomeBit) {
		t.Fatalf("%d slugs, model has %d", len(biomeSlugs), len(model.BiomeBit))
	}
	for slug, bit := range model.BiomeBit {
		if got := BiomeSlug(worldgen.Biome(bit)); got != slug {
			t.Errorf("biome %d: slug %q, model says %q", bit, got, slug)
		}
	}
	if BiomeSlug(worldgen.BiomeNone) != "" {
		t.Error("none has a slug")
	}
}

func TestNewRejectsOddSizes(t *testing.T) {
	for _, size := range []int{-1, 1, 100, 1000, 4096} {
		if _, err := New(testSeed, size); err == nil {
			t.Errorf("size %d accepted", size)
		}
	}
	w, err := New(testSeed, 0)
	if err != nil || w.mapSize != DefaultMapSize {
		t.Fatalf("default size: %v, %d", err, w.mapSize)
	}
	if w.seedNumber != testSeedNumber {
		t.Fatalf("seed hashes to %d, want %d", w.seedNumber, testSeedNumber)
	}
	if _, err := New("12345", 256); err != nil {
		t.Fatal("a numeric seed name is a name like any other:", err)
	}
}

func TestInfoAndCounts(t *testing.T) {
	w, report := world(t)
	info := w.Info()
	if info.Seed != testSeed || info.SeedNumber != testSeedNumber || info.MapSize != DefaultMapSize {
		t.Fatalf("info %+v", info)
	}
	if info.Radius != worldmap.MapRadius || info.WorldRadius != worldmap.WorldRadius || info.Water != worldgen.WaterLevel {
		t.Fatalf("geometry %+v", info)
	}
	if info.SectorCount != wantSectors || info.AltSectorCount != wantAltSectors {
		t.Fatalf("%d sectors, %d with alt biomes; want %d and %d", info.SectorCount, info.AltSectorCount, wantSectors, wantAltSectors)
	}
	for biome, c := range worldmap.Palette {
		i := 3 * int(worldmap.BiomeIndex(biome))
		if info.Palette[i] != c.R || info.Palette[i+1] != c.G || info.Palette[i+2] != c.B {
			t.Errorf("palette for %v is not worldmap's", biome)
		}
	}

	if len(report) != wantRules {
		t.Fatalf("%d rules reported, want %d", len(report), wantRules)
	}
	total := 0
	for _, r := range report {
		total += r.Placed
		if r.Placed > r.Wanted {
			t.Errorf("%s placed %d of %d", r.Rule, r.Placed, r.Wanted)
		}
	}
	if total != wantSites {
		t.Fatalf("%d sites placed, want %d", total, wantSites)
	}
}

// What the observer is handed is the placement, rule by rule: the same
// instances Locations holds, in the loop's order, prioritised rules first.
func TestPlaceStreamsWhatItPlaces(t *testing.T) {
	w, report := world(t)
	var streamed []RuleSites
	total := 0
	again := w.Place(func(rs RuleSites) {
		streamed = append(streamed, rs)
		total += len(rs.XZY) / 3
	})
	if len(again) != len(report) || total != wantSites {
		t.Fatalf("second placement: %d rules, %d sites", len(again), total)
	}
	if len(streamed) != wantRules {
		t.Fatalf("%d rules streamed", len(streamed))
	}
	for i, rs := range streamed {
		if rs.Index != i || rs.Total != wantRules || rs.Rule != report[i].Rule || len(rs.XZY) != 3*report[i].Placed {
			t.Fatalf("rule %d streamed as %+v, report says %+v", i, rs, report[i])
		}
	}
	if streamed[0].Rule != "StartTemple" {
		t.Fatalf("first rule is %s", streamed[0].Rule)
	}
	x, z := streamed[0].XZY[0], streamed[0].XZY[1]
	if d := math.Hypot(float64(x), float64(z)); d > 1000 {
		t.Fatalf("StartTemple is %.0f m from the centre", d)
	}

	// Against the loop's own record, as instances by zone.
	locs := w.gen.PlaceLocations(w.m, worldgen.DefaultLocationRules(), nil)
	byPos := map[[3]float32]string{}
	for _, li := range locs.Instances() {
		byPos[[3]float32{li.Position.X, li.Position.Z, li.Position.Y}] = li.Rule.Name
	}
	for _, rs := range streamed {
		for i := 0; i < len(rs.XZY); i += 3 {
			k := [3]float32{rs.XZY[i], rs.XZY[i+1], rs.XZY[i+2]}
			if byPos[k] != rs.Rule {
				t.Fatalf("%s streamed at %v, the loop has %q there", rs.Rule, k, byPos[k])
			}
		}
	}
}

// The raster is worldmap.Renderer's, texel for texel, at the default size
// (sampled) and at the game's 2048 (read off the biome map).
func TestMapIsTheRenderers(t *testing.T) {
	w, _ := world(t)
	for _, size := range []int{DefaultMapSize, worldgen.BiomeMapSize} {
		ww := *w
		ww.mapSize = size
		m := ww.Map()
		data, err := worldmap.New(w.gen, size).Data(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		ref, err := worldmap.Decode(data)
		if err != nil {
			t.Fatal(err)
		}
		if m.Size != size || len(m.Heights) != size*size || len(m.Biomes) != size*size || len(m.Mask) != 4*size*size {
			t.Fatalf("%d: planes %d %d %d", size, len(m.Heights), len(m.Biomes), len(m.Mask))
		}
		bad := 0
		for y := 0; y < size && bad < 10; y++ {
			for x := 0; x < size && bad < 10; x++ {
				i := y*size + x
				h := float64(m.Heights[i])/worldmap.HeightScale - worldmap.HeightOffset
				r, g, b := ref.Mask(x, y)
				if h != ref.Height(x, y) || m.Biomes[i] != ref.Biome(x, y) ||
					float64(m.Mask[4*i])/255 != r || float64(m.Mask[4*i+1])/255 != g || float64(m.Mask[4*i+2])/255 != b || m.Mask[4*i+3] != 255 {
					t.Errorf("%d: texel %d,%d: height %v biome %d mask %v; renderer has %v %d %v %v %v",
						size, x, y, h, m.Biomes[i], m.Mask[4*i:4*i+4], ref.Height(x, y), ref.Biome(x, y), r, g, b)
					bad++
				}
			}
		}
		// The void outside and the land inside are both there.
		if m.Biomes[0] != 0 || m.Biomes[(size/2)*size+size/2] == 0 {
			t.Errorf("%d: corner biome %d, centre biome %d", size, m.Biomes[0], m.Biomes[(size/2)*size+size/2])
		}
	}
}

// The sector plane is the biome map's roll: every grid point reads back
// its own sector's alt biomes, the list is worldmap's, and a plane adopted
// by a world without a biome map answers the same.
func TestSectorsAreTheRoll(t *testing.T) {
	w, _ := world(t)
	s := w.Sectors()
	if s.Size != worldgen.BiomeMapSize || len(s.List) != wantAltSectors {
		t.Fatalf("plane %d wide, %d sectors", s.Size, len(s.List))
	}
	ref, err := worldmap.New(w.gen, 32).Sectors(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(ref.List) != len(s.List) {
		t.Fatalf("%d sectors, worldmap lists %d", len(s.List), len(ref.List))
	}
	for i, sec := range s.List {
		r := ref.List[i]
		if sec.ID != r.ID || sec.Biome != BiomeSlug(r.Biome) || sec.X != r.X || sec.Z != r.Z || len(sec.Names) != len(r.Names) {
			t.Fatalf("sector %d is %+v, worldmap has %+v", i, sec, r)
		}
		for j := range sec.Names {
			if sec.Names[j] != r.Names[j] {
				t.Fatalf("sector %d name %d is %q, want %q", i, j, sec.Names[j], r.Names[j])
			}
		}
	}

	same := func(a []string, b []*worldgen.AltBiome) bool {
		if len(a) != len(b) {
			return false
		}
		for i := range a {
			if a[i] != b[i].Name {
				return false
			}
		}
		return true
	}
	light, _ := New(testSeed, 0)
	light.Adopt(s)
	rng := rand.New(rand.NewPCG(1, 2))
	for i := 0; i < 20000; i++ {
		x := rng.Float64()*2*worldmap.MapRadius - worldmap.MapRadius
		z := rng.Float64()*2*worldmap.MapRadius - worldmap.MapRadius
		if i < 100 {
			// The seams: on, and a hair either side of, a grid point.
			x = float64(12*rng.IntN(2048)-12282) + float64(rng.IntN(3)-1)*0.001
			z = float64(12*rng.IntN(2048)-12282) + float64(rng.IntN(3)-1)*0.001
		}
		want := w.m.SectorAt(float32(x), float32(z)).AltBiomes
		if got := s.At(x, z); !same(got, want) {
			t.Fatalf("at %.3f,%.3f the plane says %v, the map %v", x, z, got, want)
		}
		if a, b := w.Point(x, z), light.Point(x, z); a.OK != b.OK || a.Biome != b.Biome || a.Height != b.Height || !same(b.Sector, want) {
			t.Fatalf("at %.3f,%.3f the prepared world says %+v, the adopted one %+v", x, z, a, b)
		}
	}
	if ref.IDAt(s.List[0].X, s.List[0].Z) != 1 {
		t.Error("worldmap does not read its own first label point as sector 1")
	}
}

func TestRivers(t *testing.T) {
	w, _ := world(t)
	r := w.Rivers()
	if len(r.Rivers) != wantRivers || len(r.Streams) != wantStreams {
		t.Fatalf("%d rivers, %d streams; want %d and %d", len(r.Rivers), len(r.Streams), wantRivers, wantStreams)
	}
	rivers, _ := w.gen.Rivers()
	if l, f := r.Rivers[0], rivers[0]; l != (Line{f.P0.X, f.P0.Y, f.P1.X, f.P1.Y, f.WidthMin, f.WidthMax}) {
		t.Fatalf("first river %v, generator has %+v", l, f)
	}
}

func TestPoint(t *testing.T) {
	w, _ := world(t)
	p := w.Point(0, 0)
	h, _ := w.gen.Height(0, 0)
	if !p.OK || p.Height != h || p.Biome == "" || p.Depth != math.Max(0, worldgen.WaterLevel-h) {
		t.Fatalf("centre %+v, height %v", p, h)
	}
	if p.Forest != (worldgen.ForestFactor(worldgen.Vec3{}) < 1.15) {
		t.Fatal("forest is not InForest")
	}
	if q := w.Point(0, 10600); q.OK || q.Biome != "" || q.Sector != nil {
		t.Fatalf("past the edge %+v", q)
	}
	// Deep sea somewhere: the depth is the water over the ground.
	for _, x := range []float64{-9000, -6000, 6000, 9000} {
		if q := w.Point(x, 0); q.OK && q.Height < worldgen.WaterLevel && q.Depth != worldgen.WaterLevel-q.Height {
			t.Fatalf("depth at %v: %+v", x, q)
		}
	}
}
