package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/pietjan/valheim-wiki/internal/assets"
	"github.com/pietjan/valheim-wiki/internal/model"
)

// extractSpawns reads the world's creature spawn tables: the
// SpawnSystemList on each _SpawnList_* prefab (base, mistlands, ashlands,
// DeepNorth), which every zone's SpawnSystem rolls against. A rule says
// which creature, in which biome and terrain, how many at once, how often
// and with what chance; the same SpawnData shape is used by raids and alt
// biomes, whose rules the events and altbiomes extractors append.
//
// Afterwards every creature's Biomes is the union of the biomes its
// enabled world rules name, counting a rule that spawns a spawner prefab
// (Spawner_TrollFrost) for the creature it spawns; creatures with no
// world rule keep an empty list.
func extractSpawns(x *X) error {
	seen := map[string]bool{}
	x.EachMono([]string{"m_spawners", "m_biomeFolded"}, func(cab string, id int64, t map[string]any) {
		root, _ := x.Owner(cab, t)
		if root == "" || seen[root] {
			return
		}
		seen[root] = true
		for i, e := range L(t, "m_spawners") {
			s := x.spawnData(cab, assets.AsMap(e), fmt.Sprintf("%s/%d", root, i), "world:"+root)
			if s != nil {
				x.DB.Spawns = append(x.DB.Spawns, s)
			}
		}
	})
	biomes := map[string]map[string]bool{}
	for _, s := range x.DB.Spawns {
		if !s.Enabled || !strings.HasPrefix(s.Source, "world:") {
			continue
		}
		for _, prefab := range append([]string{s.Prefab}, s.Creatures...) {
			if biomes[prefab] == nil {
				biomes[prefab] = map[string]bool{}
			}
			for _, b := range s.Biomes {
				biomes[prefab][b] = true
			}
		}
	}
	for prefab, set := range biomes {
		c := x.DB.Creatures[prefab]
		if c == nil {
			continue
		}
		c.Biomes = nil
		for _, b := range model.Biomes {
			if set[b] {
				c.Biomes = append(c.Biomes, b)
			}
		}
	}
	return nil
}

// spawnData reads one SpawnSystem.SpawnData. m_spawnChance is a percent;
// altitude is height above sea level; tilt is the ground slope in
// degrees; m_overrideLevelupChance is -1 where the default 10% per level
// applies. Nil when the rule names no prefab. A prefab that is a
// CreatureSpawner or SpawnArea rather than a creature (Spawner_TrollFrost,
// Spawner_CharredStone_event) has what it spawns in Creatures.
func (x *X) spawnData(cab string, m map[string]any, id, source string) *model.Spawn {
	prefab := x.GameObjectName(cab, m["m_prefab"])
	if prefab == "" {
		return nil
	}
	return &model.Spawn{
		ID:                       id,
		Name:                     S(m, "m_name"),
		Prefab:                   prefab,
		Creatures:                x.spawnedBy(prefab),
		Source:                   source,
		Enabled:                  B(m, "m_enabled"),
		Biomes:                   Biomes(I(m, "m_biome")),
		BiomeArea:                BiomeArea(I(m, "m_biomeArea")),
		MaxSpawned:               I(m, "m_maxSpawned"),
		Interval:                 F(m, "m_spawnInterval"),
		Chance:                   F(m, "m_spawnChance"),
		Distance:                 F(m, "m_spawnDistance"),
		RadiusMin:                F(m, "m_spawnRadiusMin"),
		RadiusMax:                F(m, "m_spawnRadiusMax"),
		RequiredKey:              S(m, "m_requiredGlobalKey"),
		RequiredEvent:            S(m, "m_requiredPersistentEvent"),
		Environments:             Strs(m["m_requiredEnvironments"]),
		GroupMin:                 I(m, "m_groupSizeMin"),
		GroupMax:                 I(m, "m_groupSizeMax"),
		GroupRadius:              F(m, "m_groupRadius"),
		Night:                    B(m, "m_spawnAtNight"),
		Day:                      B(m, "m_spawnAtDay"),
		MinAltitude:              F(m, "m_minAltitude"),
		MaxAltitude:              F(m, "m_maxAltitude"),
		MinTilt:                  F(m, "m_minTilt"),
		MaxTilt:                  F(m, "m_maxTilt"),
		InForest:                 B(m, "m_inForest"),
		OutsideForest:            B(m, "m_outsideForest"),
		InLava:                   B(m, "m_inLava"),
		OutsideLava:              B(m, "m_outsideLava"),
		MinOceanDepth:            F(m, "m_minOceanDepth"),
		MaxOceanDepth:            F(m, "m_maxOceanDepth"),
		HuntPlayer:               B(m, "m_huntPlayer"),
		GroundOffset:             F(m, "m_groundOffset"),
		MinDistanceFromCenter:    F(m, "m_minDistanceFromCenter"),
		MaxDistanceFromCenter:    F(m, "m_maxDistanceFromCenter"),
		MinLevel:                 I(m, "m_minLevel"),
		MaxLevel:                 I(m, "m_maxLevel"),
		LevelUpMinCenterDistance: F(m, "m_levelUpMinCenterDistance"),
		LevelUpChance:            F(m, "m_overrideLevelupChance"),
		CanSpawnCloseToPlayer:    B(m, "m_canSpawnCloseToPlayer"),
		InsidePlayerBase:         B(m, "m_insidePlayerBase"),
	}
}

// spawnedBy is what a spawner prefab spawns: the CreatureSpawner's
// creature or the SpawnArea's roster, in weight order. Nil for a prefab
// that is neither (a creature, a projectile, an effect).
func (x *X) spawnedBy(prefab string) []string {
	g, ok := x.Roots[prefab]
	if !ok {
		return nil
	}
	if sp := SpawnerOf(x, g); sp != nil {
		return []string{sp.Creature}
	}
	if n := NestOf(x, g); n != nil {
		var out []string
		for _, e := range n.Creatures {
			out = append(out, e.Creature)
		}
		return out
	}
	return nil
}

// spawnList reads a List<SpawnSystem.SpawnData> into the DB, giving each
// rule the id "<source>/<index>", and returns the ids.
func (x *X) spawnList(cab string, v any, source string) []string {
	var ids []string
	for i, e := range assets.AsList(v) {
		s := x.spawnData(cab, assets.AsMap(e), fmt.Sprintf("%s/%d", source, i), source)
		if s != nil {
			x.DB.Spawns = append(x.DB.Spawns, s)
			ids = append(ids, s.ID)
		}
	}
	return ids
}

// NestOf reads the SpawnArea on a prefab root: a greydwarf nest, a
// draugr pile, a bone pile. A nest rolls one of its weighted creatures
// every Interval seconds while a player is within trigger distance and
// fewer than MaxNear of its spawn stand close by (MaxTotal in the wider
// area). Nil when the prefab has no SpawnArea. Count is 1: the caller
// counts the nests it finds in a location.
func NestOf(x *X, g assets.Ref) *model.Nest {
	c, ok := x.Find(g, "m_prefabs", "m_spawnIntervalSec")
	if !ok {
		return nil
	}
	t := c.Tree
	n := &model.Nest{
		Prefab: x.Name(g), Count: 1, Interval: F(t, "m_spawnIntervalSec"),
		MaxNear: I(t, "m_maxNear"), MaxTotal: I(t, "m_maxTotal"), LevelUp: F(t, "m_levelupChance"),
	}
	for _, e := range L(t, "m_prefabs") {
		m := assets.AsMap(e)
		creature := x.GameObjectName(c.Ref.Cab, m["m_prefab"])
		if creature == "" {
			continue
		}
		n.Creatures = append(n.Creatures, model.NestEntry{Creature: creature, Weight: F(m, "m_weight"), MinLevel: I(m, "m_minLevel"), MaxLevel: I(m, "m_maxLevel")})
	}
	sort.SliceStable(n.Creatures, func(i, j int) bool { return n.Creatures[i].Weight > n.Creatures[j].Weight })
	return n
}

// SpawnerOf reads a CreatureSpawner on a GameObject: one creature that
// appears when a player comes within trigger distance and respawns
// Respawn minutes after it dies (0: once only). Nil when the GameObject
// has none or it names no creature. Count is 1 and Chance is left for
// the caller, who knows whether the spawner sits under a RandomSpawn.
func SpawnerOf(x *X, g assets.Ref) *model.Spawner {
	c, ok := x.Find(g, "m_creaturePrefab", "m_respawnTimeMinuts")
	if !ok {
		return nil
	}
	t := c.Tree
	creature := x.GameObjectName(c.Ref.Cab, t["m_creaturePrefab"])
	if creature == "" {
		return nil
	}
	return &model.Spawner{
		Creature: creature, Count: 1, MinLevel: I(t, "m_minLevel"), MaxLevel: I(t, "m_maxLevel"),
		LevelUp: F(t, "m_levelupChance"), Respawn: F(t, "m_respawnTimeMinuts"),
		Night: B(t, "m_spawnAtNight"), Day: B(t, "m_spawnAtDay"), RequiredKey: S(t, "m_requiredGlobalKey"),
	}
}
