package engine

import (
	"math"
	"testing"

	"github.com/pietjan/valheim-wiki/internal/scene"
)

// The zone at the spawn stands the start temple and grows meadows
// vegetation around it, every placement on the ground of its own zone.
func TestZone(t *testing.T) {
	w, _ := world(t)
	places := w.Zone(0, 0)
	if len(places) == 0 {
		t.Fatal("nothing in the spawn zone")
	}
	sites, veg := 0, 0
	seen := map[string]bool{}
	for _, p := range places {
		switch p.Kind {
		case "location":
			sites++
			if p.Name != "StartTemple"+scene.SiteSuffix {
				t.Fatalf("site %q in the spawn zone", p.Name)
			}
		case "vegetation":
			veg++
			seen[p.Name] = true
			// Within the zone and its group radius, standing on the ground.
			if math.Abs(p.At[0]) > 48 || math.Abs(p.At[2]) > 48 {
				t.Fatalf("%s at %v is outside zone 0,0", p.Name, p.At)
			}
			g, ok := w.Scene().GroundAt(p.At[0], p.At[2])
			if ok && math.Abs(g-p.At[1]) > 3 {
				t.Fatalf("%s at %v floats %.1f m off the ground", p.Name, p.At, p.At[1]-g)
			}
			if p.Scale[0] <= 0 || p.Rot == [4]float64{} {
				t.Fatalf("%s has scale %v rot %v", p.Name, p.Scale, p.Rot)
			}
		default:
			t.Fatalf("place of kind %q", p.Kind)
		}
	}
	if sites != 1 || veg < 10 || len(seen) < 3 {
		t.Fatalf("%d sites, %d vegetation of %d kinds", sites, veg, len(seen))
	}
	// Deterministic.
	again := w.Zone(0, 0)
	if len(again) != len(places) || again[len(again)-1] != places[len(places)-1] {
		t.Fatal("a second read differs")
	}
	if w2, _ := New(testSeed, 0); w2.Zone(0, 0) != nil {
		t.Fatal("a zone before placement")
	}
}

// A world handed the sites of a cached run answers the same zone.
func TestAdoptSites(t *testing.T) {
	w, _ := world(t)
	var rules []string
	var xyz [][]float32
	byRule := map[string][]float32{}
	for _, li := range w.locs.Instances() {
		byRule[li.Rule.Name] = append(byRule[li.Rule.Name], li.Position.X, li.Position.Z, li.Position.Y)
	}
	for name, v := range byRule {
		rules = append(rules, name)
		xyz = append(xyz, v)
	}
	w2, _ := New(testSeed, 0)
	w2.AdoptSites(rules, xyz)
	a, b := w.Zone(0, 0), w2.Zone(0, 0)
	if len(a) != len(b) {
		t.Fatalf("%d places placed, %d adopted", len(a), len(b))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("place %d: %+v vs %+v", i, a[i], b[i])
		}
	}
}

// A placed world's tile shows the start temple's flattened floor: the
// ground under it is level where the raw generator is not.
func TestTileModified(t *testing.T) {
	w, _ := world(t)
	raw, _ := New(testSeed, 0)
	const n = 64
	a, b := raw.Tile(0, 0, 64, n), w.Tile(0, 0, 64, n)
	changed := 0
	for i := range a.Heights {
		if a.Heights[i] != b.Heights[i] {
			changed++
		}
	}
	if changed == 0 {
		t.Fatal("the temple's modifiers left the tile as the generator made it")
	}
	// The levelled middle is flat.
	mid := b.Heights[(n/2)*n+n/2]
	for _, d := range []int{-3, 3} {
		if math.Abs(float64(b.Heights[(n/2+d)*n+n/2+d]-mid)) > 0.01 {
			t.Fatalf("the floor is not level: %v vs %v", b.Heights[(n/2+d)*n+n/2+d], mid)
		}
	}
}
