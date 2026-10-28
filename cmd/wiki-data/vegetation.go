package main

import (
	"sort"

	"github.com/pietjan/valheim-wiki/internal/assets"
	"github.com/pietjan/valheim-wiki/internal/model"
)

// extractVegetation reads the rules that scatter trees, bushes, rocks
// and pickables over a zone, in the order ZoneSystem.SetupLocations
// builds its list: ZoneSystem.m_vegetation on the game's own prefab,
// then every LocationList's m_vegetation by m_sortOrder, then each
// AltBiome's m_addVegetation (rules that only apply inside that
// alternative biome). The grass and small clutter come the same way
// from ClutterSystem.m_clutter and LocationList.m_clutter. Finally every
// Resource learns the biomes its enabled rules place it in; what is
// only placed by a location, or only appears when something else is
// felled or shattered (logs, stubs, rock fragments), keeps none.
func extractVegetation(x *X) error {
	type listRef struct {
		cab   string
		name  string
		order int
		tree  map[string]any
	}
	var lists []listRef
	x.EachMono([]string{"m_locations", "m_vegetation", "m_sortOrder"}, func(cab string, id int64, t map[string]any) {
		root, _ := x.Owner(cab, t)
		lists = append(lists, listRef{cab, root, I(t, "m_sortOrder"), t})
	})
	sort.SliceStable(lists, func(i, j int) bool { return lists[i].order < lists[j].order })

	// ZoneSystem itself: m_locationLists beside m_vegetation. The
	// bundles hold the component twice, on the _ZoneSystem prefab and
	// on its instance under _GameMain, which is the one the game runs;
	// the game has one ZoneSystem, so only that one is read.
	if z, ok := x.systemComponent("m_vegetation", "m_locationLists"); ok {
		x.addVegRules(z.cab, z.root, z.tree["m_vegetation"])
	}
	for _, l := range lists {
		x.addVegRules(l.cab, l.name, l.tree["m_vegetation"])
	}
	x.EachMono([]string{"m_alts"}, func(cab string, id int64, t map[string]any) {
		for _, a := range L(t, "m_alts") {
			am := assets.AsMap(a)
			x.addVegRules(cab, "altbiome:"+S(am, "m_name"), am["m_addVegetation"])
		}
	})

	// ClutterSystem: the one under _GameMain is the game's; the main
	// menu's Backgroundscene has another with its own list.
	if c, ok := x.systemComponent("m_clutter", "m_grassPatchSize"); ok {
		x.addClutter(c.cab, c.root, c.tree["m_clutter"])
	}
	for _, l := range lists {
		x.addClutter(l.cab, l.name, l.tree["m_clutter"])
	}

	// Where each resource grows: the union over its enabled rules.
	for _, v := range x.DB.Vegetation {
		r := x.DB.Resources[v.Prefab]
		if r == nil || !v.Enabled {
			continue
		}
		r.Biomes = unionBiomes(r.Biomes, v.Biomes)
	}
	return nil
}

// systemRef is one MonoBehaviour on a system prefab.
type systemRef struct {
	cab, root string
	tree      map[string]any
}

// systemComponent is the one MonoBehaviour with the fields the game
// runs: the one under _GameMain when the bundles hold several (the
// standalone system prefab, the main menu's background scene), else
// the first loaded.
func (x *X) systemComponent(fields ...string) (systemRef, bool) {
	var found []systemRef
	x.EachMono(fields, func(cab string, id int64, t map[string]any) {
		root, _ := x.Owner(cab, t)
		found = append(found, systemRef{cab, root, t})
	})
	sort.SliceStable(found, func(i, j int) bool { return found[i].root == "_GameMain" && found[j].root != "_GameMain" })
	if len(found) == 0 {
		return systemRef{}, false
	}
	return found[0], true
}

// addVegRules appends a List<ZoneSystem.ZoneVegetation>.
func (x *X) addVegRules(cab, source string, v any) {
	for _, e := range assets.AsList(v) {
		m := assets.AsMap(e)
		x.DB.Vegetation = append(x.DB.Vegetation, &model.VegRule{
			Name:                  S(m, "m_name"),
			Prefab:                x.GameObjectName(cab, m["m_prefab"]),
			Source:                source,
			Enabled:               B(m, "m_enable"),
			Biomes:                Biomes(I(m, "m_biome")),
			BiomeArea:             vegBiomeArea(I(m, "m_biomeArea")),
			Min:                   F(m, "m_min"),
			Max:                   F(m, "m_max"),
			GroupMin:              I(m, "m_groupSizeMin"),
			GroupMax:              I(m, "m_groupSizeMax"),
			GroupRadius:           F(m, "m_groupRadius"),
			MinAltitude:           F(m, "m_minAltitude"),
			MaxAltitude:           F(m, "m_maxAltitude"),
			MinVegetation:         F(m, "m_minVegetation"),
			MaxVegetation:         F(m, "m_maxVegetation"),
			MinOceanDepth:         F(m, "m_minOceanDepth"),
			MaxOceanDepth:         F(m, "m_maxOceanDepth"),
			MinTilt:               F(m, "m_minTilt"),
			MaxTilt:               F(m, "m_maxTilt"),
			InForest:              B(m, "m_inForest"),
			ForestMin:             F(m, "m_forestTresholdMin"),
			ForestMax:             F(m, "m_forestTresholdMax"),
			MinDistanceFromCenter: F(m, "m_minDistanceFromCenter"),
			MaxDistanceFromCenter: F(m, "m_maxDistanceFromCenter"),
			ScaleMin:              F(m, "m_scaleMin"),
			ScaleMax:              F(m, "m_scaleMax"),
			RandTilt:              F(m, "m_randTilt"),
			GroundTiltChance:      F(m, "m_chanceToUseGroundTilt"),
			BlockCheck:            B(m, "m_blockCheck"),
			ForcePlacement:        B(m, "m_forcePlacement"),
			SnapToWater:           B(m, "m_snapToWater"),
			SnapToStaticSolid:     B(m, "m_snapToStaticSolid"),
			GroundOffset:          F(m, "m_groundOffset"),
			TerrainDeltaRadius:    F(m, "m_terrainDeltaRadius"),
			MinTerrainDelta:       F(m, "m_minTerrainDelta"),
			MaxTerrainDelta:       F(m, "m_maxTerrainDelta"),
			SurroundCheck:         B(m, "m_surroundCheckVegetation"),
			SurroundDistance:      F(m, "m_surroundCheckDistance"),
			SurroundLayers:        I(m, "m_surroundCheckLayers"),
			SurroundBetter:        F(m, "m_surroundBetterThanAverage"),
		})
	}
}

// addClutter appends a List<ClutterSystem.Clutter>.
func (x *X) addClutter(cab, source string, v any) {
	for _, e := range assets.AsList(v) {
		m := assets.AsMap(e)
		x.DB.Clutter = append(x.DB.Clutter, &model.Clutter{
			Name:          S(m, "m_name"),
			Prefab:        x.GameObjectName(cab, m["m_prefab"]),
			Source:        source,
			Enabled:       B(m, "m_enabled"),
			Biomes:        Biomes(I(m, "m_biome")),
			Amount:        I(m, "m_amount"),
			Instanced:     B(m, "m_instanced"),
			InForest:      B(m, "m_inForest"),
			ForestMin:     F(m, "m_forestTresholdMin"),
			ForestMax:     F(m, "m_forestTresholdMax"),
			MinAlt:        F(m, "m_minAlt"),
			MaxAlt:        F(m, "m_maxAlt"),
			MinTilt:       F(m, "m_minTilt"),
			MaxTilt:       F(m, "m_maxTilt"),
			MinVegetation: F(m, "m_minVegetation"),
			MaxVegetation: F(m, "m_maxVegetation"),
			MinOceanDepth: F(m, "m_minOceanDepth"),
			MaxOceanDepth: F(m, "m_maxOceanDepth"),
			ScaleMin:      F(m, "m_scaleMin"),
			ScaleMax:      F(m, "m_scaleMax"),
			OnUncleared:   B(m, "m_onUncleared"),
			OnCleared:     B(m, "m_onCleared"),
			SnapToWater:   B(m, "m_snapToWater"),
			TerrainTilt:   B(m, "m_terrainTilt"),
		})
	}
}

// vegBiomeArea names a Heightmap.BiomeArea mask. The inspector's
// "Everything" toggle also sets bits above the two the enum defines
// (Edge 1, Median 2), so the bundles hold 6 and 7 as well as 3; the
// game tests the mask bitwise, so only the low two bits matter. -1 is
// the inspector's other spelling of everything.
func vegBiomeArea(v int) string {
	switch v & 3 {
	case 1:
		return "Edge"
	case 2:
		return "Median"
	case 3:
		return "Everything"
	}
	return ""
}

// unionBiomes merges two biome lists, keeping model.Biomes order.
func unionBiomes(a, b []string) []string {
	has := map[string]bool{}
	for _, s := range a {
		has[s] = true
	}
	for _, s := range b {
		has[s] = true
	}
	var out []string
	for _, s := range model.Biomes {
		if has[s] {
			out = append(out, s)
		}
	}
	return out
}
