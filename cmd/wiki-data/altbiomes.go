package main

import (
	"strings"

	"github.com/pietjan/valheim-wiki/internal/assets"
	"github.com/pietjan/valheim-wiki/internal/model"
)

// heightmapExtrasNames is Heightmap.HeightmapExtras: AltBiome.m_customGenerator.
var heightmapExtrasNames = []string{"", "Menu", "Flat"}

// extractAltBiomes reads the AltBiomeList (AltBiomes_Erik): themed
// patches the world generator rolls onto biome sectors of the right size,
// height and neighbours, each adding or blocking weather, spawns,
// vegetation and locations inside it. Their spawn rules go into the DB's
// spawns with Source "altbiome:<name>".
func extractAltBiomes(x *X) error {
	seen := map[string]bool{}
	x.EachMono([]string{"m_alts"}, func(cab string, id int64, t map[string]any) {
		for _, e := range L(t, "m_alts") {
			m := assets.AsMap(e)
			name := S(m, "m_name")
			if name == "" || seen[name] {
				continue
			}
			seen[name] = true
			x.DB.AltBiomes = append(x.DB.AltBiomes, x.altBiome(cab, m))
		}
	})
	return nil
}

// altBiome reads one AltBiome.
func (x *X) altBiome(cab string, m map[string]any) *model.AltBiome {
	name := S(m, "m_name")
	biomes := Biomes(I(m, "m_biome"))
	a := &model.AltBiome{
		Name:                  name,
		Enabled:               B(m, "m_enabled"),
		Biome:                 strings.Join(biomes, ","),
		Label:                 x.altLabel(m, biomes),
		Chance:                F(m, "m_chance"),
		MinAmount:             I(m, "m_minAmountSpawned"),
		MaxAmount:             I(m, "m_maxAmountSpawned"),
		MinDistanceFromCenter: F(m, "m_minDistanceFromCenter"),
		RequireNeighbor:       Biomes(I(m, "m_requireNeighbor")),
		NotNeighbor:           Biomes(I(m, "m_notNeighbor")),
		Incompatible:          Strs(m["m_incompatibleAltBiomes"]),
		MinEdge:               I(m, "m_minEdgeSize"),
		MaxEdge:               I(m, "m_maxEdgeSize"),
		MinAvgHeight:          F(m, "m_minAvgHeight"),
		MaxAvgHeight:          F(m, "m_maxAvgHeight"),
		LevelUpMultiplier:     F(m, "m_levelUpChanceMultiplier"),
		ForceEnv:              S(m, "m_forceEnvironment"),
		ForceMusic:            S(m, "m_forceMusic"),
		BlockEnvs:             Strs(m["m_blockEnvironments"]),
		BlockSpawns:           Strs(m["m_blockSpawnNames"]),
		BlockVegetation:       Strs(m["m_blockVegetationNames"]),
		BlockLocations:        Strs(m["m_blockLocationNames"]),
		TerrainTexture:        strings.Join(Biomes(I(m, "m_terrainTextureOverride")), ","),
		HeightMapBiome:        strings.Join(Biomes(I(m, "m_heightMapBiomeOverride")), ","),
		BaseHeightMultiplier:  F(m, "m_baseHeightMultiplier"),
		BaseHeightOffset:      F(m, "m_baseHeightOffset"),
		HeightMapMultiplier:   F(m, "m_heightMapMultiplier"),
		HeightMapOffset:       F(m, "m_heightMapOffset"),
		BelowWorldX:           F(m, "m_belowWorldX"),
		AboveWorldX:           F(m, "m_aboveWorldX"),
		BelowWorldY:           F(m, "m_belowWorldY"),
		AboveWorldY:           F(m, "m_aboveWorldY"),
	}
	if v := I(m, "m_customGenerator"); v > 0 && v < len(heightmapExtrasNames) {
		a.CustomGenerator = heightmapExtrasNames[v]
	}
	for _, e := range L(m, "m_addEnvironments") {
		em := assets.AsMap(e)
		a.AddEnvs = append(a.AddEnvs, model.EnvEntry{Env: S(em, "m_environment"), Weight: F(em, "m_weight")})
	}
	// Vegetation rules are known by their rule name (what m_blockVegetationNames
	// quotes); locations by their prefab, the key of the locations table.
	for _, e := range L(m, "m_addVegetation") {
		if n := S(assets.AsMap(e), "m_name"); n != "" {
			a.AddVegetation = append(a.AddVegetation, n)
		}
	}
	for _, e := range L(m, "m_addLocations") {
		em := assets.AsMap(e)
		n := S(em, "m_prefabName")
		if n == "" {
			n = S(em, "m_name")
		}
		if n != "" {
			a.AddLocations = append(a.AddLocations, n)
		}
	}
	a.Spawns = x.spawnList(cab, m["m_spawn"], "altbiome:"+name)
	return a
}

// altLabel is the sector name the map shows for the patch, composed the
// way BiomeSector.GetName does: the override replaces the biome's name,
// the prefix and suffix wrap it. Empty when the patch is unnamed.
func (x *X) altLabel(m map[string]any, biomes []string) string {
	prefix, suffix, override := x.Text(S(m, "m_namePrefix")), x.Text(S(m, "m_nameSuffix")), x.Text(S(m, "m_nameOverride"))
	if prefix == "" && suffix == "" && override == "" {
		return ""
	}
	base := override
	if base == "" {
		var names []string
		for _, b := range biomes {
			names = append(names, model.BiomeName[b])
		}
		base = strings.Join(names, "/")
	}
	return strings.TrimSpace(prefix + " " + base + " " + suffix)
}
