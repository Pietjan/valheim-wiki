package worldgen

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"testing"
)

// A world carried over from before 1.0: npcdev, seed -1365230248, played
// since August on 0.221 and upgraded on 2026-09-09. The game's location
// list changed with 1.0, so on the upgrade load it dropped every site not
// yet built and ran the placement loop again around the 1,151 that were,
// never offering the 4,627 zones already built as candidates.
//
// testdata/npcdev-upgrade holds the three lists that replay needs and the
// answer: before.tsv is ZoneSystem's location list read straight out of the
// pre-upgrade save (npcdev_backup_20260909-153420.db, uncompressed 0.221
// format), zones.tsv its generated zones from the same file, and after.tsv
// the server's m_locationInstances the day after, from a probe.

func openUpgrade(t *testing.T, name string, fn func(tsvRow)) {
	t.Helper()
	f := openFixture(t, "testdata/npcdev-upgrade/"+name)
	defer f.Close()
	if err := readTSV(f, func(r tsvRow) error { fn(r); return nil }); err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
}

func TestUpgradeReplayMatchesTheGame(t *testing.T) {
	if testing.Short() {
		t.Skip("samples the whole world; skipped in -short")
	}
	const seed = -1365230248

	rules := DefaultLocationRules()
	alts := DefaultAltBiomes()
	byPrefab := make(map[string]*LocationRule)
	for _, r := range rules {
		if r.Prefab != "" {
			byPrefab[r.Prefab] = r
		}
	}

	existing := &Existing{Generated: make(map[Zone]bool)}
	dropped := 0
	openUpgrade(t, "before.tsv", func(r tsvRow) {
		if !r.bool("placed") {
			dropped++
			return
		}
		pos := Vec3{r.float("x"), r.float("y"), r.float("z")}
		existing.Built = append(existing.Built, LocationInstance{Rule: byPrefab[r.str("prefab")], Zone: ZoneOf(pos), Position: pos})
	})
	openUpgrade(t, "zones.tsv", func(r tsvRow) {
		existing.Generated[Zone{int16(r.int("zoneX")), int16(r.int("zoneY"))}] = true
	})
	builtZones := make(map[Zone]bool, len(existing.Built))
	for _, b := range existing.Built {
		builtZones[b.Zone] = true
	}

	type placed struct {
		prefab string
		pos    Vec3
	}
	want := make(map[Zone]placed)
	openUpgrade(t, "after.tsv", func(r tsvRow) {
		z := Zone{int16(r.int("zoneX")), int16(r.int("zoneY"))}
		if builtZones[z] {
			return
		}
		want[z] = placed{r.str("prefab"), Vec3{r.float("x"), r.float("y"), r.float("z")}}
	})
	t.Logf("before the upgrade: %d built sites kept, %d unbuilt dropped, %d zones generated; after: %d sites placed anew",
		len(existing.Built), dropped, len(existing.Generated), len(want))

	g := New(ConfigFromSeed(seed))
	m := g.BuildBiomeMap()
	m.RollAltBiomes(alts, seed)
	locs := g.PlaceLocations(m, rules, existing)

	same, total := 0, 0
	miss := make(map[string]int)
	var station []Zone
	for z, li := range locs.ByZone {
		if builtZones[z] || li.Rule.Biome == BiomeAshLands {
			continue
		}
		if li.Rule.Prefab == "AncientUpgradeStation" {
			station = append(station, z)
		}
		total++
		w, ok := want[z]
		if ok && w.prefab == li.Rule.Prefab && math.Abs(float64(w.pos.X-li.Position.X)) < 0.01 && math.Abs(float64(w.pos.Z-li.Position.Z)) < 0.01 {
			same++
		} else {
			miss[li.Rule.Prefab]++
		}
	}

	// The upgrade station is unique: the game placed ten candidates and
	// removed nine when the player built one. That one, at zone (3,34) with
	// its built flag set, has to be among the port's ten.
	found := false
	for _, z := range station {
		if z == (Zone{3, 34}) {
			found = true
		}
	}
	if !found {
		t.Errorf("AncientUpgradeStation candidates %v do not include the one the world kept at (3,34)", station)
	}
	delete(miss, "AncientUpgradeStation")

	var names []string
	for n := range miss {
		names = append(names, fmt.Sprintf("%s:%d", n, miss[n]))
	}
	sort.Strings(names)
	t.Logf("%d of %d non-Ashlands re-placements at the game's spot; the rest are the station's removed candidates", same, total)
	if len(names) > 0 {
		t.Errorf("re-placements off the game's spot: %s", strings.Join(names, " "))
	}
}
