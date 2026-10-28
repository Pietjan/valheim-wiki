package scene

import (
	"bufio"
	"embed"
	"fmt"
	"io/fs"
	"strconv"
	"strings"
	"sync"

	"github.com/pietjan/valheim-wiki/internal/stablehash"
	"github.com/pietjan/valheim-wiki/internal/worldgen"
)

// The game's vegetation rules: what ZoneSystem scatters over a zone once
// its locations stand — trees, bushes, rocks, pickables, the mist and the
// clouds — as the running game has them merged.
//
// ZoneSystem.m_vegetation is built at start-up from the inspector list,
// then every LocationList in load order, then each alt biome's additions,
// and the merged list is what PlaceVegetation walks. The tables here are
// that list read out of the running server by a probe (valheim-npc's
// cmd/valheim-mcp server_probe), not the bundles, because the order is
// decided at runtime and the bundles hold the pieces unmerged:
//
//	vegetation/rules.tsv       ZoneSystem.m_vegetation in list order, every
//	                           field the placement loop reads
//	vegetation/altbiomes.tsv   AltBiomeList.m_altBiomes: the rule names each
//	                           alt biome blocks
//	vegetation/colliders/      per prefab, every collider the placement ray
//	                           can hit, as triangles in the prefab's frame,
//	                           gzipped; see colliderSoup
//	vegetation/spawndraws.tsv  per prefab, how many draws of the random
//	                           stream spawning it costs: a component's Awake
//	                           — a fade-in delay, a material variation —
//	                           draws from the same stream the placement is
//	                           drawing from, between one placement and the
//	                           next. Measured in the game by seeding the
//	                           stream, spawning a ghost and counting the
//	                           steps; the same for every seed tried.
//
//go:embed vegetation/rules.tsv vegetation/altbiomes.tsv vegetation/spawndraws.tsv vegetation/colliders
var vegetationFS embed.FS

// VegRule is one ZoneSystem.ZoneVegetation.
type VegRule struct {
	Index  int
	Name   string
	Prefab string
	// PrefabHash is the prefab name's stable hash, a term of the rule's
	// random seed in each zone.
	PrefabHash int32
	Enable     bool
	Min, Max   float32
	// ForcePlacement multiplies the attempts by fifty.
	ForcePlacement        bool
	ScaleMin, ScaleMax    float32
	RandTilt              float32
	ChanceToUseGroundTilt float32
	Biome                 worldgen.Biome
	// BiomeArea is a mask of Heightmap.BiomeArea: 1 edge, 2 median.
	BiomeArea                 int
	BlockCheck                bool
	SnapToStaticSolid         bool
	MinAltitude, MaxAltitude  float32
	MinVegetation             float32
	MaxVegetation             float32
	SurroundCheckVegetation   bool
	SurroundCheckDistance     float32
	SurroundCheckLayers       int
	SurroundBetterThanAverage float32
	MinOceanDepth             float32
	MaxOceanDepth             float32
	MinTilt, MaxTilt          float32
	TerrainDeltaRadius        float32
	MaxTerrainDelta           float32
	MinTerrainDelta           float32
	SnapToWater               bool
	GroundOffset              float32
	GroupSizeMin              int32
	GroupSizeMax              int32
	GroupRadius               float32
	MinDistanceFromCenter     float32
	MaxDistanceFromCenter     float32
	InForest                  bool
	ForestTresholdMin         float32
	ForestTresholdMax         float32
	// AltBiomeParent names the alt biome that added the rule; it only
	// places in sectors carrying that alt biome.
	AltBiomeParent string
	// ZNetView is whether the prefab is streamed; the game spawns both
	// kinds the same way for placement.
	ZNetView bool
	// SpawnDraws is how many draws spawning the prefab costs the stream.
	SpawnDraws int
}

// vegTables is the rules and the alt biomes' blocks, read once.
type vegTables struct {
	rules []*VegRule
	// blocks is, per alt biome name, the rule names it blocks.
	blocks map[string]map[string]bool
}

var vegetation = sync.OnceValue(func() *vegTables {
	t := &vegTables{blocks: map[string]map[string]bool{}}
	rules, err := fs.ReadFile(vegetationFS, "vegetation/rules.tsv")
	if err != nil {
		panic("world: vegetation/rules.tsv: " + err.Error())
	}
	if err := readColumns(string(rules), func(r row) error {
		v := &VegRule{
			Index: r.int("index"), Name: r.str("name"), Prefab: r.str("prefab"),
			Enable: r.bool("enable"), Min: r.float("min"), Max: r.float("max"),
			ForcePlacement: r.bool("forcePlacement"),
			ScaleMin:       r.float("scaleMin"), ScaleMax: r.float("scaleMax"),
			RandTilt: r.float("randTilt"), ChanceToUseGroundTilt: r.float("chanceToUseGroundTilt"),
			Biome: worldgen.Biome(r.int("biome")), BiomeArea: r.int("biomeArea"),
			BlockCheck: r.bool("blockCheck"), SnapToStaticSolid: r.bool("snapToStaticSolid"),
			MinAltitude: r.float("minAltitude"), MaxAltitude: r.float("maxAltitude"),
			MinVegetation: r.float("minVegetation"), MaxVegetation: r.float("maxVegetation"),
			SurroundCheckVegetation: r.bool("surroundCheckVegetation"), SurroundCheckDistance: r.float("surroundCheckDistance"),
			SurroundCheckLayers: r.int("surroundCheckLayers"), SurroundBetterThanAverage: r.float("surroundBetterThanAverage"),
			MinOceanDepth: r.float("minOceanDepth"), MaxOceanDepth: r.float("maxOceanDepth"),
			MinTilt: r.float("minTilt"), MaxTilt: r.float("maxTilt"),
			TerrainDeltaRadius: r.float("terrainDeltaRadius"), MaxTerrainDelta: r.float("maxTerrainDelta"), MinTerrainDelta: r.float("minTerrainDelta"),
			SnapToWater: r.bool("snapToWater"), GroundOffset: r.float("groundOffset"),
			GroupSizeMin: int32(r.int("groupSizeMin")), GroupSizeMax: int32(r.int("groupSizeMax")), GroupRadius: r.float("groupRadius"),
			MinDistanceFromCenter: r.float("minDistanceFromCenter"), MaxDistanceFromCenter: r.float("maxDistanceFromCenter"),
			InForest: r.bool("inForest"), ForestTresholdMin: r.float("forestTresholdMin"), ForestTresholdMax: r.float("forestTresholdMax"),
			AltBiomeParent: r.str("altBiomeParent"), ZNetView: r.bool("znetview"),
		}
		v.PrefabHash = stablehash.Code(v.Prefab)
		t.rules = append(t.rules, v)
		return nil
	}); err != nil {
		panic("world: vegetation/rules.tsv: " + err.Error())
	}
	draws := map[string]int{}
	spawn, err := fs.ReadFile(vegetationFS, "vegetation/spawndraws.tsv")
	if err != nil {
		panic("world: vegetation/spawndraws.tsv: " + err.Error())
	}
	if err := readColumns(string(spawn), func(r row) error {
		draws[r.str("prefab")] = r.int("draws")
		return nil
	}); err != nil {
		panic("world: vegetation/spawndraws.tsv: " + err.Error())
	}
	for _, v := range t.rules {
		v.SpawnDraws = draws[v.Prefab]
	}
	alts, err := fs.ReadFile(vegetationFS, "vegetation/altbiomes.tsv")
	if err != nil {
		panic("world: vegetation/altbiomes.tsv: " + err.Error())
	}
	if err := readColumns(string(alts), func(r row) error {
		set := map[string]bool{}
		for _, name := range r.list("blockVegetationNames") {
			set[name] = true
		}
		t.blocks[r.str("name")] = set
		return nil
	}); err != nil {
		panic("world: vegetation/altbiomes.tsv: " + err.Error())
	}
	return t
})

// VegetationRules is the game's vegetation list, in its order. Shared and
// not to be written to.
func VegetationRules() []*VegRule { return vegetation().rules }

// row is one line of a table with a commented header naming the columns.
type row struct {
	cols map[string]int
	f    []string
}

func (r row) str(name string) string {
	i, ok := r.cols[name]
	if !ok || i >= len(r.f) {
		return ""
	}
	return r.f[i]
}

func (r row) float(name string) float32 {
	v, _ := strconv.ParseFloat(r.str(name), 32)
	return float32(v)
}

func (r row) int(name string) int {
	v, _ := strconv.Atoi(r.str(name))
	return v
}

func (r row) bool(name string) bool { return r.str(name) == "1" }

func (r row) list(name string) []string {
	s := r.str(name)
	if s == "" {
		return nil
	}
	return strings.Split(s, ",")
}

// readColumns reads a probe table: the first comment line names the
// columns, every other line is a row.
func readColumns(text string, fn func(row) error) error {
	sc := bufio.NewScanner(strings.NewReader(text))
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	var cols map[string]int
	line := 0
	for sc.Scan() {
		line++
		t := sc.Text()
		if strings.HasPrefix(t, "#") {
			if cols == nil {
				cols = map[string]int{}
				for i, name := range strings.Split(strings.TrimSpace(strings.TrimPrefix(t, "#")), "\t") {
					cols[name] = i
				}
			}
			continue
		}
		if t == "" {
			continue
		}
		if cols == nil {
			return fmt.Errorf("line %d: data before the header", line)
		}
		if err := fn(row{cols: cols, f: strings.Split(t, "\t")}); err != nil {
			return fmt.Errorf("line %d: %w", line, err)
		}
	}
	return sc.Err()
}
