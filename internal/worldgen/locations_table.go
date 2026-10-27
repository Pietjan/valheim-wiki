package worldgen

import (
	"bufio"
	_ "embed"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"sync"
)

//go:embed tables/locations.tsv
var locationTable string

//go:embed tables/altbiomes.tsv
var altBiomeTable string

// TableTexts is the embedded tables placement reads, as they are: for a
// caller to fingerprint what the sites are placed from.
func TableTexts() []string { return []string{locationTable, altBiomeTable} }

var (
	defaultRules = sync.OnceValue(func() []*LocationRule {
		rules, err := ReadLocationRules(strings.NewReader(locationTable))
		if err != nil {
			panic("worldgen: tables/locations.tsv: " + err.Error())
		}
		return rules
	})
	defaultAlts = sync.OnceValue(func() []*AltBiome {
		alts, err := ReadAltBiomes(strings.NewReader(altBiomeTable))
		if err != nil {
			panic("worldgen: tables/altbiomes.tsv: " + err.Error())
		}
		return alts
	})
)

// DefaultLocationRules is the game's location list, in its order. The rules
// are shared and not to be written to.
func DefaultLocationRules() []*LocationRule { return defaultRules() }

// DefaultAltBiomes is the game's alt-biome list, fresh each call, since the
// roll writes the sectors it lands on into each entry.
func DefaultAltBiomes() []*AltBiome {
	src := defaultAlts()
	out := make([]*AltBiome, len(src))
	for i, a := range src {
		c := *a
		c.Sectors = nil
		out[i] = &c
	}
	return out
}

// Sites places every location of a world from its seed, with the game's
// tables: the biome map, the alt-biome roll and the placement loop in one
// call. existing may be nil for a world created on 1.0; see Existing for a
// world carried over. It takes some twenty seconds.
func Sites(seed int32, existing *Existing) *Locations {
	l, _ := SitesWithMap(seed, existing)
	return l
}

// SitesWithMap is Sites, and the biome map the placement ran on with its
// alt biomes rolled, for what reads the sectors afterwards.
func SitesWithMap(seed int32, existing *Existing) (*Locations, *BiomeMap) {
	g := New(ConfigFromSeed(seed))
	m := g.BuildBiomeMap()
	m.RollAltBiomes(DefaultAltBiomes(), seed)
	return g.PlaceLocations(m, DefaultLocationRules(), existing), m
}

// Instances is every placed site, by zone.
func (l *Locations) Instances() []LocationInstance {
	out := make([]LocationInstance, 0, len(l.ByZone))
	for _, li := range l.ByZone {
		out = append(out, *li)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Zone.Y != out[j].Zone.Y {
			return out[i].Zone.Y < out[j].Zone.Y
		}
		return out[i].Zone.X < out[j].Zone.X
	})
	return out
}

// The rule tables come out of the game: tools/probes/location-gen-dump.cs
// writes ZoneSystem.m_locations and AltBiomeList.m_altBiomes as they are at
// runtime. Both are tab-separated with a commented header naming the columns,
// and are read by column name so a column added to the probe costs nothing
// here. The copies under tables/ are the game's as of 1.0.7 and are what
// DefaultLocationRules and DefaultAltBiomes hand out.

type tsvRow struct {
	cols map[string]int
	f    []string
}

func (r tsvRow) str(name string) string {
	i, ok := r.cols[name]
	if !ok || i >= len(r.f) {
		return ""
	}
	return r.f[i]
}

func (r tsvRow) float(name string) float32 {
	v, _ := strconv.ParseFloat(r.str(name), 32)
	return float32(v)
}

func (r tsvRow) int(name string) int {
	v, _ := strconv.Atoi(r.str(name))
	return v
}

func (r tsvRow) bool(name string) bool { return r.str(name) == "1" }

func (r tsvRow) list(name string) []string {
	s := r.str(name)
	if s == "" {
		return nil
	}
	return strings.Split(s, ",")
}

// readTSV reads a probe table, handing each row to fn with its columns named.
func readTSV(in io.Reader, fn func(tsvRow) error) error {
	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	var cols map[string]int
	line := 0
	for sc.Scan() {
		line++
		text := sc.Text()
		if strings.HasPrefix(text, "#") {
			if cols == nil {
				cols = make(map[string]int)
				for i, name := range strings.Split(strings.TrimSpace(strings.TrimPrefix(text, "#")), "\t") {
					cols[name] = i
				}
			}
			continue
		}
		if text == "" {
			continue
		}
		if cols == nil {
			return fmt.Errorf("line %d: data before the header", line)
		}
		if err := fn(tsvRow{cols: cols, f: strings.Split(text, "\t")}); err != nil {
			return fmt.Errorf("line %d: %w", line, err)
		}
	}
	return sc.Err()
}

// ReadLocationRules reads the probe's rules.tsv, in the game's list order.
func ReadLocationRules(in io.Reader) ([]*LocationRule, error) {
	var rules []*LocationRule
	err := readTSV(in, func(r tsvRow) error {
		rules = append(rules, &LocationRule{
			Name:                      r.str("name"),
			Prefab:                    r.str("prefab"),
			Enable:                    r.bool("enable"),
			Biome:                     Biome(r.int("biome")),
			BiomeArea:                 r.int("biomeArea"),
			Quantity:                  r.int("quantity"),
			Prioritized:               r.bool("prioritized"),
			CenterFirst:               r.bool("centerFirst"),
			Unique:                    r.bool("unique"),
			Group:                     r.str("group"),
			MinDistanceFromSimilar:    r.float("minDistanceFromSimilar"),
			GroupMax:                  r.str("groupMax"),
			MaxDistanceFromSimilar:    r.float("maxDistanceFromSimilar"),
			InteriorRadius:            r.float("interiorRadius"),
			ExteriorRadius:            r.float("exteriorRadius"),
			MinTerrainDelta:           r.float("minTerrainDelta"),
			MaxTerrainDelta:           r.float("maxTerrainDelta"),
			MinimumVegetation:         r.float("minimumVegetation"),
			MaximumVegetation:         r.float("maximumVegetation"),
			SurroundCheckVegetation:   r.bool("surroundCheckVegetation"),
			SurroundCheckDistance:     r.float("surroundCheckDistance"),
			SurroundCheckLayers:       r.int("surroundCheckLayers"),
			SurroundBetterThanAverage: r.float("surroundBetterThanAverage"),
			InForest:                  r.bool("inForest"),
			ForestTresholdMin:         r.float("forestTresholdMin"),
			ForestTresholdMax:         r.float("forestTresholdMax"),
			MinDistanceFromCenter:     r.float("minDistanceFromCenter"),
			MaxDistanceFromCenter:     r.float("maxDistanceFromCenter"),
			MinDistance:               r.float("minDistance"),
			MaxDistance:               r.float("maxDistance"),
			MinAltitude:               r.float("minAltitude"),
			MaxAltitude:               r.float("maxAltitude"),
			AltBiomeParent:            r.str("altBiomeParent"),
			SnapToWater:               r.bool("snapToWater"),
			RandomRotation:            r.bool("randomRotation"),
			SlopeRotation:             r.bool("slopeRotation"),
			ClearArea:                 r.bool("clearArea"),
		})
		return nil
	})
	return rules, err
}

// ReadAltBiomes reads the probe's altbiomes.tsv, in the game's list order.
func ReadAltBiomes(in io.Reader) ([]*AltBiome, error) {
	var alts []*AltBiome
	err := readTSV(in, func(r tsvRow) error {
		alts = append(alts, &AltBiome{
			Name:                  r.str("name"),
			Enabled:               r.bool("enabled"),
			Biome:                 Biome(r.int("biome")),
			MinDistanceFromCenter: r.float("minDistanceFromCenter"),
			MinAmount:             r.int("minAmountSpawned"),
			MaxAmount:             r.int("maxAmountSpawned"),
			Chance:                r.float("chance"),
			RequireNeighbor:       Biome(r.int("requireNeighbor")),
			NotNeighbor:           Biome(r.int("notNeighbor")),
			Incompatible:          r.list("incompatible"),
			MinEdgeSize:           r.int("minEdgeSize"),
			MaxEdgeSize:           r.int("maxEdgeSize"),
			MinAvgHeight:          r.float("minAvgHeight"),
			MaxAvgHeight:          r.float("maxAvgHeight"),
			BelowWorldX:           r.float("belowWorldX"),
			AboveWorldX:           r.float("aboveWorldX"),
			BelowWorldY:           r.float("belowWorldY"),
			AboveWorldY:           r.float("aboveWorldY"),
			BlockLocations:        r.list("blockLocations"),
			AddLocations:          r.list("addLocations"),
		})
		return nil
	})
	return alts, err
}
