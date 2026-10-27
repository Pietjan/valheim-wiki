package worldgen

import (
	"fmt"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Everything under testdata/locgen was written by
// tools/probes/location-gen-dump.cs on a world the 1.0 server generated
// from nothing (name locgen, seed 491810132). Generating it a second time
// reproduced every placement and the whole alt-biome roll.

const locgenSeed = 491810132

func openFixture(t *testing.T, path string) *os.File {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	return f
}

func openLocgen(t *testing.T, name string) *os.File {
	t.Helper()
	return openFixture(t, "testdata/locgen/"+name)
}

func locgenRows(t *testing.T, name string, fn func(tsvRow)) {
	t.Helper()
	f := openLocgen(t, name)
	defer f.Close()
	if err := readTSV(f, func(r tsvRow) error { fn(r); return nil }); err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
}

func pointHash(points []BiomePoint) string {
	// The probe folds each coordinate as one 16-bit value per step, which
	// is not FNV over the bytes, so it is folded by hand the same way.
	var v uint64 = 14695981039346656037
	for _, p := range points {
		v = (v ^ uint64(uint16(p.X))) * 1099511628211
		v = (v ^ uint64(uint16(p.Y))) * 1099511628211
	}
	return fmt.Sprintf("%016x", v)
}

func buildLocgen(t *testing.T) (*Generator, *BiomeMap) {
	t.Helper()
	if testing.Short() {
		t.Skip("samples the whole world; skipped in -short")
	}
	g := New(ConfigFromSeed(locgenSeed))
	start := time.Now()
	m := g.BuildBiomeMap()
	t.Logf("biome map built in %v", time.Since(start))
	return g, m
}

// The per-biome point lists are what a seeded index draws a zone from, so
// they have to match in length and in order: the count, the hash over the
// whole sequence, and where the first divergence is when they do not.
func TestBiomeMapPointsMatchTheGame(t *testing.T) {
	_, m := buildLocgen(t)

	locgenRows(t, "points.tsv", func(r tsvRow) {
		biome := Biome(r.int("biome"))
		all, above := m.Points(biome)
		got := all
		if r.str("list") == "aboveSea" {
			got = above
		}
		name := fmt.Sprintf("%v/%s", biome, r.str("list"))
		if len(got) != r.int("count") {
			t.Errorf("%s: %d points, game has %d", name, len(got), r.int("count"))
		}
		if h := pointHash(got); h != r.str("fnv64") && len(got) > 0 {
			t.Errorf("%s: sequence hash %s, game %s", name, h, r.str("fnv64"))
			for _, s := range strings.Fields(r.str("sample")) {
				idx, coord, _ := strings.Cut(s, ":")
				i, _ := strconv.Atoi(idx)
				if i < len(got) && fmt.Sprintf("%d,%d", got[i].X, got[i].Y) != coord {
					t.Errorf("%s: first sampled divergence at %d: %d,%d, game %s", name, i, got[i].X, got[i].Y, coord)
					break
				}
			}
		}
	})
}

// Sectors carry the statistics the alt-biome roll reads, so each one is
// checked against the game's by index: biome, edge count, centre, height
// range and neighbours.
func TestBiomeMapSectorsMatchTheGame(t *testing.T) {
	_, m := buildLocgen(t)

	n := 0
	bad := 0
	locgenRows(t, "sectors.tsv", func(r tsvRow) {
		n++
		i := r.int("index")
		if i >= len(m.Sectors) {
			bad++
			return
		}
		s := m.Sectors[i]
		var problems []string
		if s.Biome != Biome(r.int("biome")) {
			problems = append(problems, fmt.Sprintf("biome %v, game %v", s.Biome, Biome(r.int("biome"))))
		}
		if s.EdgeCount != r.int("edgeCount") {
			problems = append(problems, fmt.Sprintf("edges %d, game %d", s.EdgeCount, r.int("edgeCount")))
		}
		if d := math.Hypot(float64(s.Center.X-r.float("centerX")), float64(s.Center.Y-r.float("centerY"))); d > 0.05 {
			problems = append(problems, fmt.Sprintf("centre %v, game (%v, %v)", s.Center, r.float("centerX"), r.float("centerY")))
		}
		if math.Abs(float64(s.HeightMin-r.float("heightMin"))) > 0.05 || math.Abs(float64(s.HeightMax-r.float("heightMax"))) > 0.05 {
			problems = append(problems, fmt.Sprintf("heights %v..%v, game %v..%v", s.HeightMin, s.HeightMax, r.float("heightMin"), r.float("heightMax")))
		}
		var neighbours []string
		for _, o := range s.Neighbors {
			neighbours = append(neighbours, strconv.Itoa(o.Index))
		}
		want := r.list("neighbors")
		sort.Strings(neighbours)
		sort.Strings(want)
		if strings.Join(neighbours, ",") != strings.Join(want, ",") {
			problems = append(problems, fmt.Sprintf("neighbours %v, game %v", neighbours, want))
		}
		if len(problems) > 0 {
			bad++
			if bad <= 10 {
				t.Errorf("sector %d: %s", i, strings.Join(problems, "; "))
			}
		}
	})
	if len(m.Sectors) != n {
		t.Errorf("%d sectors, game has %d", len(m.Sectors), n)
	}
	if bad > 0 {
		t.Errorf("%d of %d sectors differ", bad, n)
	}
}

// The answer: every location the game placed, where it placed it, with the
// alt-biome roll drawn here and checked against the game's first.
func TestLocationsMatchTheGame(t *testing.T) {
	g, m := buildLocgen(t)

	rules := DefaultLocationRules()
	alts := DefaultAltBiomes()

	m.RollAltBiomes(alts, locgenSeed)
	rolled := 0
	locgenRows(t, "sectors.tsv", func(r tsvRow) {
		s := m.Sectors[r.int("index")]
		var got []string
		for _, a := range s.AltBiomes {
			got = append(got, a.Name)
		}
		rolled += len(got)
		if strings.Join(got, ",") != r.str("altBiomes") {
			t.Errorf("sector %d rolled %v, game %v", s.Index, got, r.list("altBiomes"))
		}
	})
	t.Logf("%d alt biomes rolled onto sectors, all as the game rolled them", rolled)

	type placed struct {
		prefab string
		pos    Vec3
	}
	want := make(map[Zone]placed)
	wantCount := make(map[string]int)
	locgenRows(t, "instances.tsv", func(r tsvRow) {
		z := Zone{int16(r.int("zoneX")), int16(r.int("zoneY"))}
		want[z] = placed{r.str("prefab"), Vec3{r.float("x"), r.float("y"), r.float("z")}}
		wantCount[r.str("prefab")]++
	})

	start := time.Now()
	locs := g.PlaceLocations(m, rules, nil)
	t.Logf("%d locations placed in %v, game has %d", len(locs.ByZone), time.Since(start), len(want))

	matched := make(map[string]int)
	for z, li := range locs.ByZone {
		w, ok := want[z]
		if !ok || w.prefab != li.Rule.Prefab {
			continue
		}
		d := li.Position.sub(w.pos)
		if math.Abs(float64(d.X)) < 0.01 && math.Abs(float64(d.Y)) < 0.05 && math.Abs(float64(d.Z)) < 0.01 {
			matched[w.prefab]++
		}
	}

	var lines []string
	total, totalWant := 0, 0
	for _, p := range locs.Report {
		name := p.Rule.Prefab
		got, ok, w := p.Placed, matched[name], wantCount[name]
		total += ok
		totalWant += w
		mark := ""
		if ok != w || got != w {
			mark = "  <-"
		}
		lines = append(lines, fmt.Sprintf("%-32s placed %4d  game %4d  same %4d%s", name, got, w, ok, mark))
	}
	t.Logf("per rule (placed by the port, placed by the game, at the same spot):\n%s", strings.Join(lines, "\n"))
	t.Logf("%d of %d locations at the game's spot", total, totalWant)

	if total != totalWant {
		t.Errorf("%d of %d locations match the game", total, totalWant)
	}
}
