// Package engine is the seed engine behind world.wasm: one world from its
// seed, in the shapes the browser protocol in docs/architecture.md sends —
// the map raster as planes, the alt-biome sectors as an id plane and a
// list, the rivers as lines, the sites rule by rule as they are placed,
// and point queries. It is plain Go so it can be tested natively against
// internal/worldmap and internal/worldgen; the syscall/js marshalling is
// in the main package beside it.
//
// Everything is derived from one Generator and one BiomeMap: the biome map
// is the expensive part (four and a half seconds in the browser), and the
// sectors, the sites and — at the game's own 2048 — the map raster all
// read it rather than sampling the world again.
package engine

import (
	"fmt"
	"hash/fnv"
	"math"

	"github.com/pietjan/valheim-wiki/internal/scene"
	"github.com/pietjan/valheim-wiki/internal/stablehash"
	"github.com/pietjan/valheim-wiki/internal/worldgen"
	"github.com/pietjan/valheim-wiki/internal/worldmap"
)

// DefaultMapSize is the raster's width when the page does not say: half
// the game's 2048, 24 m a texel, six megabytes of planes.
const DefaultMapSize = 1024

// Version names what the engine computes from: the port and the game
// tables it carries. It is the cache key beside the seed in the browser,
// so a seed computed by one version is never shown as another's. The
// prefix changes by hand when the port's arithmetic or this protocol
// changes; the hash follows the embedded location and alt-biome tables.
func Version() string {
	h := fnv.New64a()
	for _, t := range worldgen.TableTexts() {
		h.Write([]byte(t))
		h.Write([]byte{0})
	}
	return fmt.Sprintf("v1-%08x", uint32(h.Sum64()))
}

// biomeSlugs are the biome slugs the wiki uses, model.Biomes, by the
// generator's biome. A table rather than an import of internal/model so
// the wasm does not carry the whole data model; engine_test pins it to
// model.BiomeBit.
var biomeSlugs = map[worldgen.Biome]string{
	worldgen.BiomeMeadows:     "meadows",
	worldgen.BiomeBlackForest: "black-forest",
	worldgen.BiomeSwamp:       "swamp",
	worldgen.BiomeMountain:    "mountain",
	worldgen.BiomePlains:      "plains",
	worldgen.BiomeMistlands:   "mistlands",
	worldgen.BiomeAshLands:    "ashlands",
	worldgen.BiomeDeepNorth:   "deep-north",
	worldgen.BiomeOcean:       "ocean",
}

// BiomeSlug is the wiki's slug for a biome, "" for none.
func BiomeSlug(b worldgen.Biome) string { return biomeSlugs[b] }

// Info is what the page learns about a world first: the seed as typed and
// as the game hashes it, the raster's geometry, and the engine that made
// it.
type Info struct {
	Seed       string
	SeedNumber int32
	Engine     string
	MapSize    int

	// Radius is the raster's half-width in metres, worldmap.MapRadius;
	// WorldRadius is where the world ends inside it; Water is sea level.
	Radius, WorldRadius, Water float64

	// Palette is worldmap.Palette as RGB bytes by worldmap.BiomeIndex,
	// sixteen entries: what the painter colours land with before the
	// game's own palette, when it has one, goes over it.
	Palette [48]uint8

	// SectorCount is how many connected biome regions the biome map has,
	// AltSectorCount how many of them carry an alt biome. Zero until the
	// biome map is built.
	SectorCount, AltSectorCount int
}

// Map is the raster as the painter uploads it: row zero north, Size texels
// a side over [-Radius, Radius]. Heights are worldmap.EncodeHeight of the
// biome height, Biomes worldmap.BiomeIndex (zero past the world's edge),
// Mask the game's GetMaskColor interleaved RGBA with alpha 255.
type Map struct {
	Size    int
	Heights []uint16
	Biomes  []uint8
	Mask    []uint8
}

// Sector is one connected biome region that carries alt biomes.
type Sector struct {
	ID    int
	Biome string
	Names []string
	// X and Z are where to write its name: its own grid point nearest its
	// edge centroid, as worldmap places it.
	X, Z float64
}

// Sectors is the alt-biome sectors as a plane of ids over the biome map's
// grid (row zero north, zero where a sector carries nothing) and the list
// of those sectors by id.
type Sectors struct {
	Size int
	IDs  []uint16
	List []Sector
}

// At is the alt biomes under a world position, by the game's
// GetBiomeSector arithmetic over the plane: nil where there are none.
func (s *Sectors) At(wx, wz float64) []string {
	p := worldgen.MapPoint(float32(wx), float32(wz))
	id := s.IDs[(s.Size-1-int(p.Y))*s.Size+int(p.X)]
	if id == 0 {
		return nil
	}
	return s.List[id-1].Names
}

// Line is one river or stream: its two ends and its width range, as the
// protocol sends it.
type Line [6]float32

// Rivers is the world's watercourses.
type Rivers struct {
	Rivers, Streams []Line
}

// RuleSites is one rule's placements, reported as the loop finishes it.
type RuleSites struct {
	Rule, Prefab string
	Index, Total int
	// XZY is the placed positions, three floats each: x east, z north,
	// then y, the biome height the loop sampled at the point.
	XZY []float32
}

// RuleReport is one line of the placement report: what the rule wanted,
// what it got.
type RuleReport struct {
	Rule, Prefab   string
	Wanted, Placed int
}

// Point answers a query about one position.
type Point struct {
	X, Z float64
	// OK is false past the world's edge, where there is no ground to
	// report; Biome and Height are then empty and zero.
	OK     bool
	Biome  string
	Height float64
	// Depth is how far under the sea the ground is, zero on land.
	Depth float64
	// Forest is the game's InForest: a forest factor under 1.15.
	Forest bool
	// Sector is the alt biomes of the region the point is in, if any, and
	// nil until the world has its sectors.
	Sector []string
}

// World is one seed's world.
type World struct {
	seed       string
	seedNumber int32
	mapSize    int

	gen     *worldgen.Generator
	m       *worldgen.BiomeMap
	sectors *Sectors
	locs    *worldgen.Locations
	scene   *scene.World
}

// New is a world from its seed as the player types it — the game hashes
// every seed name, a numeric one included — at mapSize texels a side,
// DefaultMapSize for zero. Cheap; nothing is sampled until asked.
func New(seed string, mapSize int) (*World, error) {
	if mapSize == 0 {
		mapSize = DefaultMapSize
	}
	if mapSize < 64 || mapSize > worldgen.BiomeMapSize || mapSize&(mapSize-1) != 0 {
		return nil, fmt.Errorf("engine: map size %d: want a power of two from 64 to %d", mapSize, worldgen.BiomeMapSize)
	}
	n := stablehash.Code(seed)
	return &World{
		seed:       seed,
		seedNumber: n,
		mapSize:    mapSize,
		gen:        worldgen.New(worldgen.ConfigFromSeed(n)),
	}, nil
}

// Seed is the seed name the world was opened with.
func (w *World) Seed() string { return w.seed }

// Prepare builds the biome map and rolls the alt biomes onto it — the
// rivers first, since the heights need them — once. Everything but Info,
// Rivers and a sector-less Point waits on it.
func (w *World) Prepare() {
	if w.m != nil {
		return
	}
	w.m = w.gen.BuildBiomeMap()
	w.m.RollAltBiomes(worldgen.DefaultAltBiomes(), w.seedNumber)
}

// Info describes the world; the sector counts are filled in after Prepare.
func (w *World) Info() Info {
	info := Info{
		Seed:        w.seed,
		SeedNumber:  w.seedNumber,
		Engine:      Version(),
		MapSize:     w.mapSize,
		Radius:      worldmap.MapRadius,
		WorldRadius: worldmap.WorldRadius,
		Water:       worldgen.WaterLevel,
	}
	for biome, c := range worldmap.Palette {
		i := 3 * int(worldmap.BiomeIndex(biome))
		info.Palette[i], info.Palette[i+1], info.Palette[i+2] = c.R, c.G, c.B
	}
	if w.m != nil {
		info.SectorCount = len(w.m.Sectors)
		for _, s := range w.m.Sectors {
			if len(s.AltBiomes) > 0 {
				info.AltSectorCount++
			}
		}
	}
	return info
}

// Map samples the raster the way worldmap.Renderer does: at each texel's
// centre, the biome, that biome's height, and the mask from the two.
//
// At the game's own 2048 the texel centres are the biome map's grid points
// exactly — both are twelve metres apart, centred, over the same radius —
// so a prepared world reads its biomes and heights off the map instead of
// sampling four million points again, which is nine seconds in the
// browser. Only the mask is new work there. Any other size samples.
func (w *World) Map() *Map {
	size := w.mapSize
	n := size * size
	out := &Map{Size: size, Heights: make([]uint16, n), Biomes: make([]uint8, n), Mask: make([]uint8, 4*n)}
	fromGrid := w.m != nil && size == worldgen.BiomeMapSize
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			wx, wz := worldmap.TexelCentre(size, x, y)
			i := y*size + x
			inWorld := math.Hypot(wx, wz) <= worldmap.WorldRadius
			var (
				b worldgen.Biome
				h float64
			)
			if fromGrid && inWorld {
				// Grid row zero is south; the raster's is north.
				p := worldgen.BiomePoint{X: int16(x), Y: int16(size - 1 - y)}
				b, h = w.m.BiomeAt(p), float64(w.m.HeightAt(p))
			} else {
				b = w.gen.GetBiome(wx, wz)
				h, _ = w.gen.BiomeHeight(b, wx, wz)
			}
			out.Heights[i] = worldmap.EncodeHeight(h)
			out.Mask[4*i+3] = 255
			if !inWorld {
				// Past the edge the raster says no biome and no mask, and
				// the painter draws the void; the height stays, as the
				// game's own texture keeps it.
				continue
			}
			out.Biomes[i] = worldmap.BiomeIndex(b)
			r, g, bl := worldmap.MaskColor(w.gen, wx, wz, h, b)
			out.Mask[4*i], out.Mask[4*i+1], out.Mask[4*i+2], out.Mask[4*i+3] = r, g, bl, 255
		}
	}
	return out
}

// Sectors derives the alt-biome sectors from the prepared biome map, as
// worldmap.Renderer.Sectors would from a second one, and keeps them for
// Point.
func (w *World) Sectors() *Sectors {
	if w.sectors != nil {
		return w.sectors
	}
	w.Prepare()
	const n = worldgen.BiomeMapSize
	ids := make(map[*worldgen.BiomeSector]uint16)
	s := &Sectors{Size: n, IDs: make([]uint16, n*n)}
	for _, sec := range w.m.Sectors {
		if len(sec.AltBiomes) == 0 {
			continue
		}
		names := make([]string, len(sec.AltBiomes))
		for i, a := range sec.AltBiomes {
			names[i] = a.Name
		}
		id := uint16(len(s.List) + 1)
		ids[sec] = id
		s.List = append(s.List, Sector{ID: int(id), Biome: BiomeSlug(sec.Biome), Names: names})
	}

	best := make([]float64, len(s.List))
	for i := range best {
		best[i] = math.Inf(1)
	}
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			p := worldgen.BiomePoint{X: int16(x), Y: int16(y)}
			sec := w.m.SectorAtPoint(p)
			id := ids[sec]
			if id == 0 {
				continue
			}
			s.IDs[(n-1-y)*n+x] = id
			wx, wz := p.World()
			d := math.Hypot(float64(wx-sec.Center.X), float64(wz-sec.Center.Y))
			if d < best[id-1] {
				best[id-1] = d
				s.List[id-1].X, s.List[id-1].Z = float64(wx), float64(wz)
			}
		}
	}
	w.sectors = s
	return s
}

// Adopt gives a world its sectors from elsewhere — the browser's cache of
// an earlier run — so Point can name them without building the biome map.
func (w *World) Adopt(s *Sectors) { w.sectors = s }

// Rivers is the world's rivers and streams as lines.
func (w *World) Rivers() Rivers {
	rivers, streams := w.gen.Rivers()
	lines := func(in []worldgen.River) []Line {
		out := make([]Line, len(in))
		for i, r := range in {
			out[i] = Line{r.P0.X, r.P0.Y, r.P1.X, r.P1.Y, r.WidthMin, r.WidthMax}
		}
		return out
	}
	return Rivers{Rivers: lines(rivers), Streams: lines(streams)}
}

// Place runs the placement loop with the game's tables, handing each rule's
// sites to observe as the rule is done, and returns the report.
func (w *World) Place(observe func(RuleSites)) []RuleReport {
	w.Prepare()
	locs := w.gen.PlaceLocationsWith(w.m, worldgen.DefaultLocationRules(), nil,
		func(index, total int, outcome worldgen.Placement, placed []worldgen.LocationInstance) {
			if observe == nil {
				return
			}
			rs := RuleSites{Rule: outcome.Rule.Name, Prefab: outcome.Rule.Prefab, Index: index, Total: total, XZY: make([]float32, 0, 3*len(placed))}
			for _, li := range placed {
				rs.XZY = append(rs.XZY, li.Position.X, li.Position.Z, li.Position.Y)
			}
			observe(rs)
		})
	report := make([]RuleReport, len(locs.Report))
	for i, p := range locs.Report {
		report[i] = RuleReport{Rule: p.Rule.Name, Prefab: p.Rule.Prefab, Wanted: p.Rule.Quantity, Placed: p.Placed}
	}
	w.locs = locs
	w.scene = nil
	return report
}

// Placed is whether Place has run, so a zone can be answered.
func (w *World) Placed() bool { return w.locs != nil }

// AdoptSites hands a world the sites a cached run placed — the rule name
// and the x, z, y of each — so its zones can be answered without placing
// again. The biome map is built if it is not yet, which is the slow part.
// A rule name the tables no longer know is skipped.
func (w *World) AdoptSites(rules []string, xyz [][]float32) {
	w.Prepare()
	byName := map[string]*worldgen.LocationRule{}
	for _, r := range worldgen.DefaultLocationRules() {
		byName[r.Name] = r
	}
	locs := &worldgen.Locations{ByZone: map[worldgen.Zone]*worldgen.LocationInstance{}}
	for i, name := range rules {
		rule := byName[name]
		if rule == nil || i >= len(xyz) {
			continue
		}
		v := xyz[i]
		for j := 0; j+2 < len(v); j += 3 {
			p := worldgen.Vec3{X: v[j], Z: v[j+1], Y: v[j+2]}
			locs.ByZone[worldgen.ZoneOf(p)] = &worldgen.LocationInstance{Rule: rule, Zone: worldgen.ZoneOf(p), Position: p}
		}
	}
	w.locs = locs
	w.scene = nil
}

// Scene is what stands on the placed world's ground, nil until Place has
// run: the placed sites are what the vegetation grows around.
func (w *World) Scene() *scene.World {
	if w.scene == nil && w.locs != nil {
		w.scene = scene.New(w.gen, w.seedNumber, w.locs, w.m)
	}
	return w.scene
}

// Zone is what stands in one 64 m zone: its sites' models and the
// vegetation the zone build scatters, in placement order. Nil before
// Place has run.
func (w *World) Zone(zx, zz int) []scene.Place {
	sc := w.Scene()
	if sc == nil {
		return nil
	}
	var out []scene.Place
	for _, s := range sc.SitesIn(zx, zz) {
		out = append(out, sc.SitePlace(s))
	}
	return append(out, sc.Vegetation(zx, zz)...)
}

// Point answers for one position: the biome under it, the ground's height
// as HeightmapBuilder blends it, the depth under the sea, whether the game
// calls it forest, and the alt biomes of its region.
func (w *World) Point(x, z float64) Point {
	p := Point{X: x, Z: z}
	if math.Hypot(x, z) > worldmap.WorldRadius {
		return p
	}
	h, ok := w.gen.Height(x, z)
	if !ok {
		return p
	}
	p.OK = true
	p.Biome = BiomeSlug(w.gen.GetBiome(x, z))
	p.Height = h
	p.Depth = math.Max(0, worldgen.WaterLevel-h)
	p.Forest = worldgen.ForestFactor(worldgen.Vec3{X: float32(x), Z: float32(z)}) < 1.15
	if w.sectors != nil {
		p.Sector = w.sectors.At(x, z)
	}
	return p
}
