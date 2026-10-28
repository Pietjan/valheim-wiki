package worldgen

import (
	"math"
	"runtime"
	"sync"

	"github.com/pietjan/valheim-wiki/internal/stablehash"
	"github.com/pietjan/valheim-wiki/internal/unityrandom"
)

// The biome-point map: AltBiomeWorldData, reproduced from the seed.
//
// Since 1.0 the game no longer scatters locations over random zones and hopes
// for the right biome. It samples the whole world once at 12 m onto a 2048²
// grid, keeps every grid point per biome as a list, and draws a location's
// candidate zone as a random index into the list for its biome. The list order
// is therefore part of the seed: it is the order a flood fill visited the
// points, and a port that walks the same points in a different order draws
// different zones from the same random numbers.
//
// The flood fill also cuts each biome into sectors — connected regions — and
// the alt biomes (the "Dark Meadows", the "Goblin Plains") are rolled onto
// those from the seed: each biome's sectors are shuffled and then offered a
// seeded chance in that order. See RollAltBiomes.

// biomeMapSize is AltBiomeWorldData.c_textureSize.
const biomeMapSize = 2048

// BiomeMapSize is the biome map's width and height in points: 2048 cells of
// twelve metres, AltBiomeWorldData.c_textureSize.
const BiomeMapSize = biomeMapSize

// mapToWorld is AltBiomeWorldData.MapSpaceToWorldSpace: 12 m per point, the
// grid centred on the world, sampled at pixel centres.
func mapToWorld(x float32) float32 { return (x-1024)*12 + 6 }

// worldToMap is AltBiomeWorldData.WorldSpaceToMapSpace, truncating like the
// (int) cast it is.
func worldToMap(x float32) int { return int((x-6)/12 + 1024) }

// BiomePoint is a grid coordinate on the biome map.
type BiomePoint struct{ X, Y int16 }

// World is the point's position: the centre of its twelve-metre cell, x
// east and z north.
func (p BiomePoint) World() (x, z float32) {
	return mapToWorld(float32(p.X)), mapToWorld(float32(p.Y))
}

// BiomeSector is one connected region of a biome, with the statistics the
// alt-biome roll reads off it.
type BiomeSector struct {
	Index     int
	Biome     Biome
	EdgeCount int
	Neighbors []*BiomeSector

	// Center, Min and Max are in world space once the map is built. Min stays
	// at the map origin, since the game initialises it to zero and only ever
	// lowers it; that is faithful, and nothing reads Min.
	Center, Min, Max Vec2

	HeightMin, HeightMax, HeightAvg float32
	DistanceFromCenter              float32

	AltBiomes []*AltBiome

	// center accumulates edge coordinates in map space, in float32 and in
	// the game's order, because the sum of forty thousand of them rounds.
	center Vec2
}

type biomePoints struct {
	sectors  []*BiomeSector
	all      []BiomePoint
	aboveSea []BiomePoint
}

// BiomeMap is the world sampled onto the biome grid.
type BiomeMap struct {
	biomes  []Biome
	heights []float32
	sector  []*BiomeSector

	Sectors []*BiomeSector
	biome   map[Biome]*biomePoints
}

// biomeOrder is the order Enum.GetValues gives Heightmap.Biome, which is the
// order the game's per-biome dictionary iterates. The two combined values are
// in it because they are in the enum.
var biomeOrder = []Biome{
	BiomeNone, BiomeMeadows, BiomeSwamp, BiomeMountain, BiomeBlackForest,
	BiomePlains, BiomeAshLands, BiomeDeepNorth, BiomeOcean, BiomeMistlands,
	639, 895,
}

// BuildBiomeMap samples the world onto the biome grid and flood-fills its
// sectors: AltBiomeWorldData.GenerateBiomePoints and GenerateSectors, up to
// but not including the alt-biome roll.
func (g *Generator) BuildBiomeMap() *BiomeMap {
	const n = biomeMapSize * biomeMapSize
	m := &BiomeMap{
		biomes:  make([]Biome, n),
		heights: make([]float32, n),
		sector:  make([]*BiomeSector, n),
		biome:   make(map[Biome]*biomePoints, len(biomeOrder)),
	}
	for _, b := range biomeOrder {
		m.biome[b] = &biomePoints{}
	}

	g.ensureRivers()

	// Sampling is embarrassingly parallel; everything after it is not.
	var wg sync.WaitGroup
	rows := make(chan int, biomeMapSize)
	for w := 0; w < runtime.NumCPU(); w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for y := range rows {
				g.sampleBiomeRow(m, y)
			}
		}()
	}
	for y := 0; y < biomeMapSize; y++ {
		rows <- y
	}
	close(rows)
	wg.Wait()

	m.generateSectors()
	return m
}

func (g *Generator) sampleBiomeRow(m *BiomeMap, y int) {
	wy := mapToWorld(float32(y))
	for x := 0; x < biomeMapSize; x++ {
		wx := mapToWorld(float32(x))
		i := y*biomeMapSize + x
		if wx*wx+wy*wy > 110250000 {
			m.biomes[i] = BiomeOcean
			m.heights[i] = -1000
			continue
		}
		biome := g.GetBiome(float64(wx), float64(wy))
		h, _ := g.BiomeHeight(biome, float64(wx), float64(wy))
		m.biomes[i] = biome
		m.heights[i] = float32(h)
	}
}

func (m *BiomeMap) at(x, y int) int { return y*biomeMapSize + x }

// BiomeAt is the biome sampled at a grid point.
func (m *BiomeMap) BiomeAt(p BiomePoint) Biome { return m.biomes[m.at(int(p.X), int(p.Y))] }

// SectorAtPoint is the sector a grid point was flood-filled into.
func (m *BiomeMap) SectorAtPoint(p BiomePoint) *BiomeSector {
	return m.sector[m.at(int(p.X), int(p.Y))]
}

// HeightAt is the height sampled at a grid point.
func (m *BiomeMap) HeightAt(p BiomePoint) float32 { return m.heights[m.at(int(p.X), int(p.Y))] }

// Points is a biome's grid points in the order the game lists them, and the
// subset at or above sea level in the same order.
func (m *BiomeMap) Points(b Biome) (all, aboveSea []BiomePoint) {
	bp := m.biome[b]
	if bp == nil {
		return nil, nil
	}
	return bp.all, bp.aboveSea
}

// BiomeSectors is a biome's sectors in creation order.
func (m *BiomeMap) BiomeSectors(b Biome) []*BiomeSector {
	if bp := m.biome[b]; bp != nil {
		return bp.sectors
	}
	return nil
}

// SectorAt is WorldGenerator.GetBiomeSector for a world position: the
// sector under the nearest grid point, clamped to the map.
func (m *BiomeMap) SectorAt(wx, wz float32) *BiomeSector {
	p := MapPoint(wx, wz)
	return m.sector[m.at(int(p.X), int(p.Y))]
}

// MapPoint is the grid point a world position reads as, clamped to the
// map: the truncating WorldSpaceToMapSpace that GetBiomeSector and the
// alt-biome lookups go through, so anything indexed by grid point — the
// sectors here, or a plane of their ids kept elsewhere — answers for a
// position the way the game does.
func MapPoint(wx, wz float32) BiomePoint {
	gx, gy := worldToMap(wx), worldToMap(wz)
	if gx < 0 {
		gx = 0
	}
	if gy < 0 {
		gy = 0
	}
	if gx >= biomeMapSize {
		gx = biomeMapSize - 1
	}
	if gy >= biomeMapSize {
		gy = biomeMapSize - 1
	}
	return BiomePoint{int16(gx), int16(gy)}
}

func (m *BiomeMap) newSector(b Biome) *BiomeSector {
	s := &BiomeSector{Index: len(m.Sectors), Biome: b, HeightMin: 99999, HeightMax: -99999}
	m.Sectors = append(m.Sectors, s)
	bp := m.biome[b]
	bp.sectors = append(bp.sectors, s)
	return s
}

func (m *BiomeMap) addPoint(b Biome, x, y int) {
	bp := m.biome[b]
	p := BiomePoint{int16(x), int16(y)}
	bp.all = append(bp.all, p)
	if m.heights[m.at(x, y)] >= 30 {
		bp.aboveSea = append(bp.aboveSea, p)
	}
}

// generateSectors is AltBiomeWorldData.GenerateSectors, kept in the game's
// order throughout because the order is what the point lists are.
func (m *BiomeMap) generateSectors() {
	visited := make([]bool, len(m.biomes))

	// The three biomes that are one region each, whatever the map says, get
	// their sector up front and every point in row-major order.
	whole := make(map[Biome]*BiomeSector, 3)
	for _, b := range []Biome{BiomeAshLands, BiomeDeepNorth, BiomeOcean} {
		whole[b] = m.newSector(b)
	}
	for y := 0; y < biomeMapSize; y++ {
		for x := 0; x < biomeMapSize; x++ {
			i := m.at(x, y)
			if s := whole[m.biomes[i]]; s != nil {
				m.sector[i] = s
				visited[i] = true
				m.addPoint(s.Biome, x, y)
			}
		}
	}

	// Everything else is flood-filled from the first unvisited point in
	// row-major order, with a stack, pushing +x, -x, +y, -y. The seed point
	// of each region is not added to the point lists — the game marks it
	// visited and pushes it but only the fill adds points — and that is
	// reproduced, since a list one entry longer draws every index off by one.
	var stack []BiomePoint
	for y := 0; y < biomeMapSize; y++ {
		for x := 0; x < biomeMapSize; x++ {
			i := m.at(x, y)
			if visited[i] {
				continue
			}
			visited[i] = true
			s := m.newSector(m.biomes[i])
			m.sector[i] = s
			stack = append(stack[:0], BiomePoint{int16(x), int16(y)})
			for len(stack) > 0 {
				p := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				px, py := int(p.X), int(p.Y)
				b := m.biomes[m.at(px, py)]
				stack = m.tryFill(visited, stack, b, s, px+1, py)
				stack = m.tryFill(visited, stack, b, s, px-1, py)
				stack = m.tryFill(visited, stack, b, s, px, py+1)
				stack = m.tryFill(visited, stack, b, s, px, py-1)
			}
		}
	}

	m.findEdges()
}

func (m *BiomeMap) tryFill(visited []bool, stack []BiomePoint, b Biome, s *BiomeSector, x, y int) []BiomePoint {
	if x < 0 || y < 0 || x >= biomeMapSize || y >= biomeMapSize {
		return stack
	}
	i := m.at(x, y)
	if visited[i] || m.biomes[i] != b {
		return stack
	}
	visited[i] = true
	m.sector[i] = s
	m.addPoint(b, x, y)
	return append(stack, BiomePoint{int16(x), int16(y)})
}

// findEdges walks the interior of the map and gives each sector its edge
// count, its neighbours, its bounds and its height range, all read at the
// points where it borders another sector.
func (m *BiomeMap) findEdges() {
	for y := 1; y < biomeMapSize-1; y++ {
		for x := 1; x < biomeMapSize-1; x++ {
			i := m.at(x, y)
			s := m.sector[i]

			// The first differing neighbour, in the game's order, is the one
			// recorded; the others are met from their own side.
			var other *BiomeSector
			for _, n := range [4]*BiomeSector{
				m.sector[m.at(x-1, y)], m.sector[m.at(x+1, y)],
				m.sector[m.at(x, y-1)], m.sector[m.at(x, y+1)],
			} {
				if n != s {
					other = n
					break
				}
			}
			if other == nil {
				continue
			}

			h := m.heights[i]
			s.EdgeCount++
			s.center = s.center.add(Vec2{float32(x), float32(y)})
			if float32(x) < s.Min.X {
				s.Min.X = float32(x)
			}
			if float32(x) > s.Max.X {
				s.Max.X = float32(x)
			}
			if float32(y) < s.Min.Y {
				s.Min.Y = float32(y)
			}
			if float32(y) > s.Max.Y {
				s.Max.Y = float32(y)
			}
			if !containsSector(s.Neighbors, other) {
				s.Neighbors = append(s.Neighbors, other)
			}
			if h < s.HeightMin {
				s.HeightMin = h
			}
			if h > s.HeightMax {
				s.HeightMax = h
			}
		}
	}

	for _, s := range m.Sectors {
		if s.EdgeCount > 0 {
			s.Center = Vec2{
				mapToWorld(s.center.X / float32(s.EdgeCount)),
				mapToWorld(s.center.Y / float32(s.EdgeCount)),
			}
			s.Min = Vec2{mapToWorld(s.Min.X), mapToWorld(s.Min.Y)}
			s.Max = Vec2{mapToWorld(s.Max.X), mapToWorld(s.Max.Y)}
			s.HeightAvg = (s.HeightMin + s.HeightMax) / 2
		}
		// Vector2.Distance: float32 squares and sum, the root in double.
		s.DistanceFromCenter = float32(math.Sqrt(float64(s.Center.X*s.Center.X + s.Center.Y*s.Center.Y)))
	}
}

func containsSector(list []*BiomeSector, s *BiomeSector) bool {
	for _, o := range list {
		if o == s {
			return true
		}
	}
	return false
}

// AltBiome is one entry of AltBiomeList: a modifier rolled onto whole sectors
// of a biome. Only the fields the roll and location placement read are here.
type AltBiome struct {
	Name    string
	Enabled bool
	Biome   Biome

	MinDistanceFromCenter float32
	MinAmount, MaxAmount  int
	Chance                float32

	RequireNeighbor, NotNeighbor Biome
	Incompatible                 []string

	MinEdgeSize, MaxEdgeSize   int
	MinAvgHeight, MaxAvgHeight float32
	BelowWorldX, AboveWorldX   float32
	BelowWorldY, AboveWorldY   float32

	// BlockLocations names location rules (by rule name, not prefab) that
	// may not be placed in a sector carrying this; AddLocations names the
	// prefabs of rules that may only be placed in one that does.
	BlockLocations []string
	AddLocations   []string

	Sectors []*BiomeSector
}

// hasFlag is Enum.HasFlag: every bit of f set in b, so None is in everything.
func (b Biome) hasFlag(f Biome) bool { return b&f == f }

// canAddModifier is BiomeSector.CanAddModifier.
func (s *BiomeSector) canAddModifier(a *AltBiome) bool {
	if s.DistanceFromCenter < a.MinDistanceFromCenter {
		return false
	}
	if s.EdgeCount < a.MinEdgeSize || s.EdgeCount >= a.MaxEdgeSize {
		return false
	}
	if s.HeightAvg < a.MinAvgHeight || s.HeightAvg >= a.MaxAvgHeight {
		return false
	}
	if (a.AboveWorldX != 0 && s.Center.X < a.AboveWorldX) || (a.BelowWorldX != 0 && s.Center.X > a.BelowWorldX) ||
		(a.AboveWorldY != 0 && s.Center.Y < a.AboveWorldY) || (a.BelowWorldY != 0 && s.Center.Y > a.BelowWorldY) {
		return false
	}
	for _, have := range s.AltBiomes {
		for _, name := range have.Incompatible {
			if name == a.Name {
				return false
			}
		}
	}
	for _, name := range a.Incompatible {
		for _, have := range s.AltBiomes {
			if have.Name == name {
				return false
			}
		}
	}
	if a.RequireNeighbor != BiomeNone {
		for i := 0; i < 10; i++ {
			b := biomeIndexToBiome(i)
			if !a.RequireNeighbor.hasFlag(b) {
				continue
			}
			// Literal, bugs included: the neighbour is compared against the
			// index cast to Biome rather than the biome the index names, and
			// index 0 passes the flag test as None and then finds no
			// neighbour of biome None. So a required neighbour can never be
			// satisfied, and that is what the game does.
			found := false
			for _, n := range s.Neighbors {
				if n.Biome == Biome(i) {
					found = true
					break
				}
			}
			if !found {
				return false
			}
		}
	}
	if a.NotNeighbor != BiomeNone {
		for i := 0; i < 10; i++ {
			if !a.NotNeighbor.hasFlag(biomeIndexToBiome(i)) {
				continue
			}
			for _, n := range s.Neighbors {
				if n.Biome == Biome(i) {
					return false
				}
			}
		}
	}
	return true
}

// biomeIndexToBiome is Heightmap.BiomeIndex.ToBiome.
func biomeIndexToBiome(i int) Biome {
	switch i {
	case 1:
		return BiomeMeadows
	case 2:
		return BiomeSwamp
	case 3:
		return BiomeMountain
	case 4:
		return BiomeBlackForest
	case 5:
		return BiomePlains
	case 6:
		return BiomeAshLands
	case 7:
		return BiomeDeepNorth
	case 8:
		return BiomeOcean
	case 9:
		return BiomeMistlands
	}
	return BiomeNone
}

func (s *BiomeSector) addModifier(a *AltBiome) {
	a.Sectors = append(a.Sectors, s)
	s.AltBiomes = append(s.AltBiomes, a)
}

// RollAltBiomes is AltBiomeWorldData.GenerateAltBiomes: for each biome and
// each alt biome that applies to it, the biome's sectors are shuffled and
// then offered the alt biome in that order, a seeded chance each, until the
// alt biome has as many as it may.
//
// The shuffle is Utils.Shuffle, which draws from UnityEngine.Random — the
// same stream the chances come from, seeded just before. The source reads
// `Sectors.Shuffle()`, and the game has another Shuffle in scope that takes
// an optional flag and falls back to an unseeded System.Random; C# binds the
// call to the overload that needs no optional argument filled in, which is
// the seeded one. Measured by generating the same world twice: the roll and
// every placement came out identical.
func (m *BiomeMap) RollAltBiomes(alts []*AltBiome, seed int32) {
	for _, a := range alts {
		a.Sectors = nil
	}
	for _, s := range m.Sectors {
		s.AltBiomes = nil
	}

	for _, biome := range biomeOrder {
		var valid []*AltBiome
		for _, a := range alts {
			if a.Enabled && a.Biome.hasFlag(biome) {
				valid = append(valid, a)
			}
		}
		for _, a := range valid {
			r := unityrandom.New(int32(biome) + stablehash.Code(a.Name) + seed)
			sectors := m.biome[biome].sectors
			// Utils.Shuffle: Fisher-Yates from the top, the partner drawn
			// from [0, i] inclusive. It shuffles the biome's own list, so
			// the order carries into the next alt biome's shuffle.
			for i := len(sectors) - 1; i > 0; i-- {
				j := int(r.RangeInt(0, int32(i+1)))
				sectors[i], sectors[j] = sectors[j], sectors[i]
			}
			for _, s := range sectors {
				if len(a.Sectors) >= a.MaxAmount {
					continue
				}
				roll := r.RangeFloat(0, 1)
				if (len(a.Sectors) < a.MinAmount || a.Chance >= roll) && s.canAddModifier(a) {
					s.addModifier(a)
				}
			}
		}
	}
}
