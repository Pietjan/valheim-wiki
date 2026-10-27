package worldgen

import (
	"math"
	"sort"

	"github.com/pietjan/valheim-wiki/internal/stablehash"
	"github.com/pietjan/valheim-wiki/internal/unityrandom"
)

// Location placement: ZoneSystem.GenerateLocationsTimeSliced, reproduced from
// the seed.
//
// Every location the game builds — the start temple, each troll cave, each
// tar pit — is placed once, when the world is first loaded, and saved with
// it. Where it lands is a seeded sequence per location prefab: the world seed
// plus the prefab name's stable hash seeds UnityEngine.Random, and the loop
// draws candidate zones and points until the wanted number is placed or the
// tries run out. The tries are the answer to the "48 of 50 tar pits" line the
// server logs: 12,000 zone picks (60,000 for a prioritised location), each
// tried at six points, and whatever is placed by then is what the world gets.
//
// The order of everything here is the order of the draws, and one draw more
// or fewer moves every location placed after it. Rules that do not consume
// randomness are still checked in the game's order, because passing them is
// what decides whether the terrain-delta check, which does, runs.

// LocationRule is a ZoneSystem.ZoneLocation: what the game knows about where
// one location prefab may go.
type LocationRule struct {
	Name   string
	Prefab string
	Enable bool

	Biome     Biome
	BiomeArea int
	Quantity  int

	Prioritized bool
	CenterFirst bool
	Unique      bool

	Group                  string
	MinDistanceFromSimilar float32
	GroupMax               string
	MaxDistanceFromSimilar float32

	InteriorRadius float32
	ExteriorRadius float32

	MinTerrainDelta float32
	MaxTerrainDelta float32

	MinimumVegetation         float32
	MaximumVegetation         float32
	SurroundCheckVegetation   bool
	SurroundCheckDistance     float32
	SurroundCheckLayers       int
	SurroundBetterThanAverage float32

	InForest          bool
	ForestTresholdMin float32
	ForestTresholdMax float32

	MinDistanceFromCenter float32
	MaxDistanceFromCenter float32

	MinDistance float32
	MaxDistance float32
	MinAltitude float32
	MaxAltitude float32

	// AltBiomeParent names the alt biome that adds this rule, which a sector
	// must carry for the rule to place there. Empty for the ordinary rules.
	AltBiomeParent string

	// What the zone build does with a placed site, not the placement loop:
	// SnapToWater puts it at the waterline whatever the ground says, and the
	// two rotations say how it is turned — by the slope, or by a draw from
	// the global random state at build time, which nothing reproduces.
	SnapToWater    bool
	RandomRotation bool
	SlopeRotation  bool
	ClearArea      bool
}

// Zone is a ZoneSystem zone id: 64 m squares, the origin zone centred on 0.
type Zone struct{ X, Y int16 }

// Vec3 is a world position as the game holds it.
type Vec3 struct{ X, Y, Z float32 }

// LocationInstance is one placed location.
type LocationInstance struct {
	Rule     *LocationRule
	Zone     Zone
	Position Vec3
}

// Placement is the outcome for one rule: how many it wanted, how many it got.
type Placement struct {
	Rule   *LocationRule
	Placed int
}

// Locations is ZoneSystem.m_locationInstances with the caches beside it.
type Locations struct {
	ByZone map[Zone]*LocationInstance
	Report []Placement

	byPrefab   map[string][]*LocationInstance
	byGroup    map[string][]*LocationInstance
	byGroupMax map[string][]*LocationInstance
}

// ZoneOf is ZoneSystem.GetZone.
func ZoneOf(p Vec3) Zone {
	return Zone{
		int16(floorToInt(float32((float64(p.X) + 32) / 64))),
		int16(floorToInt(float32((float64(p.Z) + 32) / 64))),
	}
}

// floorToInt is Utils.FloorToInt: a float32 add and a truncating cast.
func floorToInt(f float32) int { return int(f+64000) - 64000 }

func zonePos(z Zone) Vec3 { return Vec3{float32(z.X) * 64, 0, float32(z.Y) * 64} }

// magnitude is Vector3.magnitude as Mono runs it: the squares, their sum and
// the root in double, narrowed once. See Vec2.magnitude for the measurement.
func (v Vec3) magnitude() float32 {
	return float32(math.Sqrt(float64(v.X)*float64(v.X) + float64(v.Y)*float64(v.Y) + float64(v.Z)*float64(v.Z)))
}

func (v Vec3) sqrMagnitude() float32 {
	return float32(float64(v.X)*float64(v.X) + float64(v.Y)*float64(v.Y) + float64(v.Z)*float64(v.Z))
}

// lengthXZ is Utils.LengthXZ: Mathf.Sqrt of the sum, and a float32 parameter
// is where Mono does narrow, so the root sees the sum rounded.
func (v Vec3) lengthXZ() float32 {
	return float32(math.Sqrt(float64(float32(float64(v.X)*float64(v.X) + float64(v.Z)*float64(v.Z)))))
}

func (v Vec3) sub(o Vec3) Vec3 { return Vec3{v.X - o.X, v.Y - o.Y, v.Z - o.Z} }

// Existing is what a world already holds when the placement loop runs. A
// new world holds nothing. A world carried over from a version with a
// different location list is where this matters: the game drops every site
// not yet built and runs the loop again around what is left. The built
// sites occupy their zones, count towards their rule's quantity, and make a
// unique rule skip entirely; the zones already built are never offered as
// candidates, and an attempt that draws one is spent without its point
// draws, which moves every draw after it. Replaying such an upgrade means
// passing both, as the save had them at the time.
type Existing struct {
	Built     []LocationInstance
	Generated map[Zone]bool
}

// PlaceLocations runs the placement loop for every enabled rule on a world
// whose biome map has had its alt biomes rolled, and returns where
// everything landed. existing may be nil for a new world.
func (g *Generator) PlaceLocations(m *BiomeMap, rules []*LocationRule, existing *Existing) *Locations {
	g.ensureRivers()

	locs := &Locations{
		ByZone:     make(map[Zone]*LocationInstance),
		byPrefab:   make(map[string][]*LocationInstance),
		byGroup:    make(map[string][]*LocationInstance),
		byGroupMax: make(map[string][]*LocationInstance),
	}
	generated := map[Zone]bool{}
	if existing != nil {
		for _, k := range existing.Built {
			locs.register(k.Rule, k.Position)
		}
		generated = existing.Generated
	}

	// OrderByDescending is stable, so prioritised rules come first in their
	// own order and the rest follow in theirs.
	ordered := make([]*LocationRule, 0, len(rules))
	for _, r := range rules {
		if r.Enable && r.Quantity != 0 {
			ordered = append(ordered, r)
		}
	}
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].Prioritized && !ordered[j].Prioritized })

	areas := make(map[Zone]int)
	for _, r := range ordered {
		locs.Report = append(locs.Report, Placement{Rule: r, Placed: g.placeLocation(m, locs, r, areas, generated)})
	}
	return locs
}

// placeLocation is the per-rule body of GenerateLocationsTimeSliced.
func (g *Generator) placeLocation(m *BiomeMap, locs *Locations, rule *LocationRule, areas map[Zone]int, generated map[Zone]bool) int {
	r := unityrandom.New(g.cfg.Seed + stablehash.Code(rule.Prefab))

	maxRadius := float32(math.Max(float64(rule.ExteriorRadius), float64(rule.InteriorRadius)))
	attempts := 12000
	if rule.Prioritized {
		attempts = 60000
	}

	// CountNrOfLocation: what the world already holds of this prefab.
	placed := len(locs.byPrefab[rule.Prefab])
	maxRange := float32(10000)
	var surround []float32

	if rule.Unique && placed > 0 {
		return placed
	}
	if rule.CenterFirst {
		maxRange = rule.MinDistance
	}

	for i := 0; i < attempts && placed < rule.Quantity; i++ {
		var zone Zone
		switch {
		case rule.CenterFirst:
			zone = randomZone(r, maxRange)
		case rule.MinAltitude >= 0:
			zone = ZoneOf(mapPointToWorld(m.randomPointAboveSea(r, rule.Biome)))
		default:
			zone = ZoneOf(mapPointToWorld(m.randomPoint(r, rule.Biome)))
		}
		if rule.CenterFirst {
			maxRange++
		}

		if _, taken := locs.ByZone[zone]; taken {
			continue
		}
		if generated[zone] {
			continue
		}
		if rule.BiomeArea&g.biomeArea(areas, zone) == 0 {
			continue
		}

		for j := 0; j < 6; j++ {
			p := randomPointInZone(r, zone, maxRadius)
			dist := p.magnitude()
			if rule.MinDistance != 0 && dist < rule.MinDistance {
				continue
			}
			if rule.MaxDistance != 0 && dist > rule.MaxDistance {
				continue
			}

			biome := g.GetBiome(float64(p.X), float64(p.Z))
			if rule.Biome&biome == 0 {
				continue
			}

			h, _ := g.BiomeHeight(biome, float64(p.X), float64(p.Z))
			p.Y = float32(h)
			if alt := float32(float64(p.Y) - 30); alt < rule.MinAltitude || alt > rule.MaxAltitude {
				continue
			}

			if rule.InForest {
				if f := ForestFactor(p); f < rule.ForestTresholdMin || f > rule.ForestTresholdMax {
					continue
				}
			}

			if rule.MinDistanceFromCenter > 0 || rule.MaxDistanceFromCenter > 0 {
				d := p.lengthXZ()
				if (rule.MinDistanceFromCenter > 0 && d < rule.MinDistanceFromCenter) ||
					(rule.MaxDistanceFromCenter > 0 && d > rule.MaxDistanceFromCenter) {
					continue
				}
			}

			if delta := g.terrainDelta(r, p, rule.ExteriorRadius); delta > rule.MaxTerrainDelta || delta < rule.MinTerrainDelta {
				continue
			}

			if rule.MinDistanceFromSimilar > 0 && locs.haveInRange(rule.Prefab, rule.Group, p, rule.MinDistanceFromSimilar, false) {
				continue
			}
			if rule.MaxDistanceFromSimilar > 0 && !locs.haveInRange(rule.Prefab, rule.GroupMax, p, rule.MaxDistanceFromSimilar, true) {
				continue
			}

			veg := g.Vegetation(biome, float64(p.X), float64(p.Z))
			if rule.MinimumVegetation > 0 && veg <= rule.MinimumVegetation {
				continue
			}
			if rule.MaximumVegetation < 1 && veg >= rule.MaximumVegetation {
				continue
			}

			sector := m.SectorAt(p.X, p.Z)
			if rule.AltBiomeParent != "" && !sector.hasAltBiome(rule.AltBiomeParent) {
				continue
			}
			if sector.blocks(rule.Name) {
				continue
			}

			if rule.SurroundCheckVegetation {
				// Float expressions of more than one operation are evaluated
				// in double and narrowed at the store, as Mono runs them.
				var sum float32
				dist := float64(rule.SurroundCheckDistance)
				for layer := 0; layer < rule.SurroundCheckLayers; layer++ {
					radius := float32(float64(float32(layer+1)) / float64(float32(rule.SurroundCheckLayers)) * dist)
					for k := 0; k < 6; k++ {
						f := float32(float64(float32(k)) / 6 * float64(float32(math.Pi)) * 2)
						q := Vec3{
							p.X + float32(math.Sin(float64(f)))*radius,
							p.Y,
							p.Z + float32(math.Cos(float64(f)))*radius,
						}
						a := g.Vegetation(g.GetBiome(float64(q.X), float64(q.Z)), float64(q.X), float64(q.Z))
						weight := float32((dist - float64(radius)) / (dist * 2))
						sum = float32(float64(sum) + float64(a)*float64(weight))
					}
				}
				// The first nine candidates only feed the average; the game
				// rejects them outright.
				surround = append(surround, sum)
				if len(surround) < 10 {
					continue
				}
				// Enumerable.Average sums in double and narrows once.
				max := surround[0]
				var total float64
				for _, v := range surround {
					if v > max {
						max = v
					}
					total += float64(v)
				}
				avg := float32(total / float64(len(surround)))
				cutoff := float32(float64(avg) + (float64(max)-float64(avg))*float64(rule.SurroundBetterThanAverage))
				if sum < cutoff {
					continue
				}
			}

			locs.register(rule, p)
			placed++
			break
		}
	}
	return placed
}

// randomZone is ZoneSystem.GetRandomZone: a zone within range of the centre,
// redrawn until its origin is inside the world.
func randomZone(r *unityrandom.Random, rng float32) Zone {
	n := int32(rng) / 64
	for {
		z := Zone{int16(r.RangeInt(-n, n)), int16(r.RangeInt(-n, n))}
		if zonePos(z).magnitude() < 10000 {
			return z
		}
	}
}

// randomPointInZone is ZoneSystem.GetRandomPointInZone: anywhere in the zone
// the location's own radius fits.
func randomPointInZone(r *unityrandom.Random, z Zone, radius float32) Vec3 {
	o := zonePos(z)
	x := r.RangeFloat(-32+radius, 32-radius)
	zz := r.RangeFloat(-32+radius, 32-radius)
	return Vec3{o.X + x, o.Y, o.Z + zz}
}

func mapPointToWorld(p BiomePoint) Vec3 {
	return Vec3{mapToWorld(float32(p.X)), 0, mapToWorld(float32(p.Y))}
}

// randomBiome is AltBiomeWorldData.RandomBiomeFromBiomes: one biome out of a
// mask. Literal, with its two slips — a pick that lands on Plains returns
// BlackForest, and the Ocean branch tests the Meadows bit — and its range of
// one short, so the last biome in the mask is only ever reached by falling
// through to Mistlands.
func randomBiome(r *unityrandom.Random, mask Biome) Biome {
	if mask&(mask-1) == 0 {
		return mask
	}
	order := []Biome{
		BiomeMeadows, BiomeSwamp, BiomeMountain, BiomeBlackForest, BiomePlains,
		BiomeAshLands, BiomeDeepNorth, BiomeOcean, BiomeMistlands,
	}
	n := int32(0)
	for _, b := range order {
		if mask&b != 0 {
			n++
		}
	}
	pick := r.RangeInt(0, n-1)
	next := func(b Biome) bool {
		if mask&b == 0 {
			return false
		}
		pick--
		return pick == -1
	}
	switch {
	case next(BiomeMeadows):
		return BiomeMeadows
	case next(BiomeSwamp):
		return BiomeSwamp
	case next(BiomeMountain):
		return BiomeMountain
	case next(BiomeBlackForest):
		return BiomeBlackForest
	case next(BiomePlains):
		return BiomeBlackForest
	case next(BiomeAshLands):
		return BiomeAshLands
	case next(BiomeDeepNorth):
		return BiomeDeepNorth
	case next(BiomeMeadows):
		return BiomeOcean
	}
	return BiomeMistlands
}

// randomPoint is AltBiomeWorldData.GetRandomPointByBiomes.
func (m *BiomeMap) randomPoint(r *unityrandom.Random, mask Biome) BiomePoint {
	all, _ := m.Points(randomBiome(r, mask))
	return all[r.RangeInt(0, int32(len(all)))]
}

// randomPointAboveSea is GetRandomPointByBiomesAboveSeaLevel, which falls back
// to the whole list for a biome with nothing above the water.
func (m *BiomeMap) randomPointAboveSea(r *unityrandom.Random, mask Biome) BiomePoint {
	all, above := m.Points(randomBiome(r, mask))
	if len(above) == 0 {
		return all[r.RangeInt(0, int32(len(all)))]
	}
	return above[r.RangeInt(0, int32(len(above)))]
}

// biomeArea is WorldGenerator.GetBiomeArea for a zone's centre: Median (2)
// when the eight zone centres around it share its biome, Edge (1) otherwise.
func (g *Generator) biomeArea(cache map[Zone]int, z Zone) int {
	if a, ok := cache[z]; ok {
		return a
	}
	cx, cy := int16(z.X*64), int16(z.Y*64)
	at := func(x, y int16) Biome { return g.GetBiome(float64(x), float64(y)) }
	centre := at(cx, cy)
	area := 2
	for _, o := range [8][2]int16{{-64, -64}, {64, -64}, {64, 64}, {-64, 64}, {-64, 0}, {64, 0}, {0, -64}, {0, 64}} {
		if at(cx-o[0], cy-o[1]) != centre {
			area = 1
			break
		}
	}
	cache[z] = area
	return area
}

// ForestFactor is WorldGenerator.GetForestFactor: three octaves of noise over
// the position scaled twice in float32, as the game's Vector3 arithmetic does.
//
// Low is wooded. The game's own reading is WorldGenerator.InForest, which is
// this under 1.15, and it is what the location rules and the map both consult.
func ForestFactor(p Vec3) float32 {
	x := p.X * 0.01 * 0.4
	y := p.Z * 0.01 * 0.4
	var sum, amp float32 = 0, 1
	for i := 0; i < 3; i++ {
		sum = float32(float64(sum) + float64(amp)*float64(PerlinNoise(x, y)))
		amp = float32(float64(amp) * 0.7)
		x *= 1.6
		y *= 1.6
	}
	return sum
}

// terrainDelta is WorldGenerator.GetTerrainDelta: the height range over ten
// random samples within the radius. It draws twice per sample, which is why
// every rule before it in the loop has to be judged exactly as the game does.
func (g *Generator) terrainDelta(r *unityrandom.Random, centre Vec3, radius float32) float32 {
	var lo, hi float32 = 999999, -999999
	for i := 0; i < 10; i++ {
		dx, dz := r.InsideUnitCircle()
		x, z := centre.X+dx*radius, centre.Z+dz*radius
		h := float32(g.getHeight(float64(x), float64(z)))
		if h < lo {
			lo = h
		}
		if h > hi {
			hi = h
		}
	}
	return float32(float64(hi) - float64(lo))
}

// getHeight is WorldGenerator.GetHeight: the biome under the point and that
// biome's height, with no blending. Placement reads this and not Height.
func (g *Generator) getHeight(wx, wz float64) float64 {
	h, _ := g.BiomeHeight(g.GetBiome(wx, wz), wx, wz)
	return h
}

func (s *BiomeSector) hasAltBiome(name string) bool {
	for _, a := range s.AltBiomes {
		if a.Name == name {
			return true
		}
	}
	return false
}

func (s *BiomeSector) blocks(rule string) bool {
	for _, a := range s.AltBiomes {
		for _, name := range a.BlockLocations {
			if name == rule {
				return true
			}
		}
	}
	return false
}

// haveInRange is ZoneSystem.HaveLocationInRange: anything of the same prefab
// within the radius, or anything in the group. Distances include height.
func (l *Locations) haveInRange(prefab, group string, p Vec3, radius float32, maxGroup bool) bool {
	within := func(list []*LocationInstance) bool {
		for _, li := range list {
			if li.Position.sub(p).sqrMagnitude() < radius*radius {
				return true
			}
		}
		return false
	}
	if within(l.byPrefab[prefab]) {
		return true
	}
	if group != "" && !maxGroup && within(l.byGroup[group]) {
		return true
	}
	if group != "" && maxGroup && within(l.byGroupMax[group]) {
		return true
	}
	return false
}

// register is ZoneSystem.RegisterLocation. A zone already holding a location
// keeps it; the game logs a warning and still counts the placement.
func (l *Locations) register(rule *LocationRule, p Vec3) {
	zone := ZoneOf(p)
	if _, taken := l.ByZone[zone]; taken {
		return
	}
	li := &LocationInstance{Rule: rule, Zone: zone, Position: p}
	l.ByZone[zone] = li
	l.byPrefab[rule.Prefab] = append(l.byPrefab[rule.Prefab], li)
	l.byGroup[rule.Group] = append(l.byGroup[rule.Group], li)
	l.byGroupMax[rule.GroupMax] = append(l.byGroupMax[rule.GroupMax], li)
}
