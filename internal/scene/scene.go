// Package scene is what stands on a seed's ground: the vegetation the
// zone build scatters and the sites' footprints on the terrain, computed
// from the placed locations the way the game computes them, for the 3D
// view to draw. Taken from valheim-world's internal/world, which also
// drives a viewer from it; the ground tiles, the site modifiers, the
// facing roll and the vegetation loop are that code, and the dungeons,
// the clutter, the pickables and the routes are not here.
package scene

import (
	"math"
	"math/rand/v2"
	"sort"
	"sync"

	"github.com/pietjan/valheim-wiki/internal/worldgen"
)

// World is one placed seed, answering for its zones.
type World struct {
	Seed int32
	Gen  *worldgen.Generator

	list   []Site
	biomes *worldgen.BiomeMap

	tileCache
	vegCache
}

// New is the scene of a placed world: the generator, the seed's number,
// the placed locations and the biome map they were placed on.
func New(gen *worldgen.Generator, seed int32, locs *worldgen.Locations, biomes *worldgen.BiomeMap) *World {
	w := &World{
		Seed: seed, Gen: gen, biomes: biomes,
		tileCache: tileCache{tiles: map[[2]int]*TerrainTile{}, facings: map[[2]float32]float64{}},
		vegCache:  vegCache{veg: map[[2]int][]Place{}},
	}
	w.list = w.listOf(locs)
	return w
}

// Site is one placed location, as the scene needs it.
type Site struct {
	Name    string
	Prefab  string
	X, Y, Z float32
	Zone    worldgen.Zone
	Biome   worldgen.Biome
	// Distance is from the world's centre, where the player starts.
	Distance float64
	// SnapToWater is the rule's: the site sits at the waterline, not on
	// the ground under it.
	SnapToWater bool
	// SlopeRotation is the rule's: the site faces downhill, see Facing.
	// Radius is the rule's exterior radius, the ground the slope is read
	// over.
	SlopeRotation bool
	Radius        float32
	// ClearArea is the rule's: nothing grows within the exterior radius.
	ClearArea bool
}

// SitesList is every placed site, sorted by distance from the centre.
// The slice is shared: callers sort a copy.
func (w *World) SitesList() []Site { return w.list }

// BiomeMap is the biome map the sites were placed on.
func (w *World) BiomeMap() *worldgen.BiomeMap { return w.biomes }

// listOf turns the placement into the scene's list. The biome is the
// world's at the site rather than the rule's, which is a mask of every
// biome the rule may land in.
func (w *World) listOf(locs *worldgen.Locations) []Site {
	if locs == nil {
		return []Site{}
	}
	inst := locs.Instances()
	out := make([]Site, 0, len(inst))
	for _, li := range inst {
		p := li.Position
		out = append(out, Site{
			Name: li.Rule.Name, Prefab: li.Rule.Prefab,
			X: p.X, Y: p.Y, Z: p.Z, Zone: li.Zone,
			Biome:         w.Gen.GetBiome(float64(p.X), float64(p.Z)),
			Distance:      math.Hypot(float64(p.X), float64(p.Z)),
			SnapToWater:   li.Rule.SnapToWater,
			SlopeRotation: li.Rule.SlopeRotation,
			Radius:        li.Rule.ExteriorRadius,
			ClearArea:     li.Rule.ClearArea,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Distance < out[j].Distance })
	return out
}

// SitesIn is the sites standing in one zone.
func (w *World) SitesIn(zx, zz int) []Site {
	var out []Site
	for _, s := range w.list {
		if s.Zone.X == int16(zx) && s.Zone.Y == int16(zz) {
			out = append(out, s)
		}
	}
	return out
}

// --- the ground ---------------------------------------------------------

// TileSize is the zone's side, in metres: the grid is Valheim's own zones,
// sixty-four metres, centred on multiples of sixty-four.
const TileSize = 64.0

// TileAt is the tile a position falls in, the way ZoneSystem.GetZone finds
// a zone.
func TileAt(x, z float64) (int, int) {
	return int(math.Floor((x + TileSize/2) / TileSize)),
		int(math.Floor((z + TileSize/2) / TileSize))
}

func zoneOf(v float64) int { return int(math.Floor((v + 32) / 64)) }

// TerrainTile is one tile's ground: n by n heights a step apart from the
// south-west corner, row by row northward, and the paint a site's modifier
// left on each, see Paint.
type TerrainTile struct {
	TX, TZ  int
	Origin  [2]float64
	Step    float64
	N       int
	Heights []float32
	Paint   []int
}

// Terrain samples one tile at a metre, 65 samples a side so neighbours
// share their edge, then applies the modifiers of every site within reach
// the way the game's heightmap does. Cached: a tile is several thousand
// height queries.
func (w *World) Terrain(tx, tz int) *TerrainTile {
	k := [2]int{tx, tz}
	w.tilesMu.Lock()
	if t, ok := w.tiles[k]; ok {
		w.tilesMu.Unlock()
		return t
	}
	w.tilesMu.Unlock()
	t := w.buildTerrain(tx, tz)
	w.tilesMu.Lock()
	defer w.tilesMu.Unlock()
	if len(w.tiles) >= maxTiles {
		w.tiles = map[[2]int]*TerrainTile{}
	}
	w.tiles[k] = t
	return t
}

// maxTiles is how many tiles a world keeps, at some forty kilobytes each.
const maxTiles = 2048

func (w *World) buildTerrain(tx, tz int) *TerrainTile {
	const step = 1.0
	n := int(TileSize/step) + 1
	minX := float64(tx)*TileSize - TileSize/2
	minZ := float64(tz)*TileSize - TileSize/2
	t := &TerrainTile{TX: tx, TZ: tz, Origin: [2]float64{minX, minZ}, Step: step, N: n, Heights: make([]float32, 0, n*n)}
	for iz := 0; iz < n; iz++ {
		for ix := 0; ix < n; ix++ {
			x, z := minX+float64(ix)*step, minZ+float64(iz)*step
			h, ok := w.Gen.Height(x, z)
			if !ok {
				h = worldgen.WaterLevel
			}
			t.Heights = append(t.Heights, float32(math.Round(h*100)/100))
		}
	}
	w.modify(t, w.list)
	return t
}

// modify applies the modifiers of every site within reach of the tile, in
// the game's order — all of them at once, sorted as sortOps says — each
// op's level and then its smooth, then its paint. Heightmap.LevelTerrain
// sets every vertex within the radius — a square one axis-aligned in the
// grid, as the game's is — to the op's target; SmoothTerrain2 pulls each
// vertex toward the target by one minus the distance ratio to the power.
func (w *World) modify(t *TerrainTile, sites []Site) {
	n := t.N
	minX, minZ := t.Origin[0], t.Origin[1]
	span := TileSize/2 + 48
	cx, cz := minX+TileSize/2, minZ+TileSize/2
	var paint []int
	var ops []placedOp
	for _, s := range sites {
		if math.Abs(float64(s.X)-cx) > span || math.Abs(float64(s.Z)-cz) > span {
			continue
		}
		for _, p := range w.opsOf(s) {
			r := p.reach()
			if p.X+r < minX || p.X-r > minX+TileSize || p.Z+r < minZ || p.Z-r > minZ+TileSize {
				continue
			}
			ops = append(ops, p)
		}
	}
	sortOps(ops)
	for _, p := range ops {
		vx := int(math.Floor((p.X-minX)/t.Step + 0.5))
		vz := int(math.Floor((p.Z-minZ)/t.Step + 0.5))
		if p.Op.LevelRadius > 0 {
			rv := float64(p.Op.LevelRadius) / t.Step
			c := int(math.Ceil(rv))
			for iz := vz - c; iz <= vz+c; iz++ {
				for ix := vx - c; ix <= vx+c; ix++ {
					if ix < 0 || iz < 0 || ix >= n || iz >= n {
						continue
					}
					if !p.Op.Square && math.Hypot(float64(ix-vx), float64(iz-vz)) > rv {
						continue
					}
					t.Heights[iz*n+ix] = float32(p.Target)
				}
			}
		}
		if p.Op.SmoothRadius > 0 {
			rv := float64(p.Op.SmoothRadius) / t.Step
			c := int(math.Ceil(rv))
			for iz := vz - c; iz <= vz+c; iz++ {
				for ix := vx - c; ix <= vx+c; ix++ {
					if ix < 0 || iz < 0 || ix >= n || iz >= n {
						continue
					}
					d := math.Hypot(float64(ix-vx), float64(iz-vz))
					if d > rv || rv == 0 {
						continue
					}
					f := d / rv
					if p.Op.SmoothPower == 3 {
						f = f * f * f
					} else {
						f = math.Pow(f, float64(p.Op.SmoothPower))
					}
					h := float64(t.Heights[iz*n+ix])
					t.Heights[iz*n+ix] = float32(h + (p.Target-h)*(1-f))
				}
			}
		}
		if p.Op.PaintRadius > 0 && p.Op.Paint != PaintNone {
			if paint == nil {
				paint = make([]int, n*n)
			}
			px := int(math.Floor((p.X-0.5-minX)/t.Step + 0.5))
			pz := int(math.Floor((p.Z-0.5-minZ)/t.Step + 0.5))
			rv := float64(p.Op.PaintRadius) / t.Step
			c := int(math.Ceil(rv))
			for iz := pz - c; iz <= pz+c; iz++ {
				for ix := px - c; ix <= px+c; ix++ {
					if ix < 0 || iz < 0 || ix >= n || iz >= n {
						continue
					}
					if math.Hypot(float64(ix-px), float64(iz-pz)) > rv {
						continue
					}
					if p.Op.Paint == PaintReset {
						paint[iz*n+ix] = int(PaintNone)
					} else {
						paint[iz*n+ix] = int(p.Op.Paint)
					}
				}
			}
		}
	}
	t.Paint = paint
}

// GroundAt is the ground as the game builds it: the tile's heights with
// every site's modifiers applied, read between vertices on the triangles
// the game's mesh has.
func (w *World) GroundAt(x, z float64) (float64, bool) {
	tx, tz := TileAt(x, z)
	t := w.Terrain(tx, tz)
	fx, fz := (x-t.Origin[0])/t.Step, (z-t.Origin[1])/t.Step
	ix, iz := int(math.Floor(fx)), int(math.Floor(fz))
	if ix < 0 || iz < 0 || ix >= t.N-1 || iz >= t.N-1 {
		return w.Gen.Height(x, z)
	}
	n := t.N
	h00, h10 := float64(t.Heights[iz*n+ix]), float64(t.Heights[iz*n+ix+1])
	h01, h11 := float64(t.Heights[(iz+1)*n+ix]), float64(t.Heights[(iz+1)*n+ix+1])
	dx, dz := fx-float64(ix), fz-float64(iz)
	if dx+dz <= 1 {
		return h00 + dx*(h10-h00) + dz*(h01-h00), true
	}
	return h11 + (1-dx)*(h01-h11) + (1-dz)*(h10-h11), true
}

// rawGround is ZoneSystem.GetGroundData while a zone is first built: the
// ray down onto the heightmap's collider, the zone's vertices as
// HeightmapBuilder made them, the cell split on the diagonal the collider
// is. Not the built ground: no site has reshaped it yet. An exact whole
// metre belongs to the cell west or south of it, as the game's ray lands.
func (w *World) rawGround(x, z float64) (float64, [3]float64) {
	ix, iz := math.Floor(x), math.Floor(z)
	if ix == x {
		ix--
	}
	if iz == z {
		iz--
	}
	zx, zz := zoneOf(ix+0.5), zoneOf(iz+0.5)
	l, k := int(ix)-(zx*64-32), int(iz)-(zz*64-32)
	v := func(dl, dk int) float64 { return float64(w.Gen.ZoneVertexHeight(zx, zz, l+dl, k+dk)) }
	h00, h10, h01, h11 := v(0, 0), v(1, 0), v(0, 1), v(1, 1)
	dx, dz := x-ix, z-iz
	if dx+dz <= 1 {
		return h00 + dx*(h10-h00) + dz*(h01-h00), norm3([3]float64{-(h10 - h00), 1, -(h01 - h00)})
	}
	return h11 + (1-dx)*(h01-h11) + (1-dz)*(h10-h11), norm3([3]float64{h01 - h11, 1, -(h11 - h10)})
}

// --- the sites ----------------------------------------------------------

// Place is one thing standing in the world: the model's name, where it
// stands, how it is turned (x, y, z, w) and sized, and what it is.
type Place struct {
	Name  string
	Site  string
	At    [3]float64
	Rot   [4]float64
	Scale [3]float64
	Kind  string
}

// SiteSuffix is what the mesh extractor appends to a location's whole-site
// model, the one with the ZNetView children kept.
const SiteSuffix = "~site"

// SitePlace is where a site's model stands: on the ground, or on the
// waterline when its rule snaps it there, turned as Facing says.
func (w *World) SitePlace(s Site) Place {
	return Place{
		Name: s.Prefab + SiteSuffix, Site: s.Name,
		At:    [3]float64{float64(s.X), w.groundOrWater(s), float64(s.Z)},
		Rot:   yawQuaternion(w.Facing(s)),
		Scale: [3]float64{1, 1, 1},
		Kind:  "location",
	}
}

// Facing is the yaw a site is placed at, in degrees clockwise from north,
// as far as the seed can say.
//
// ZoneSystem.PlaceLocations turns a site one of three ways. A rule with
// slopeRotation faces it downhill: GetTerrainDelta takes ten points at
// random within the exterior radius, the facing is the direction from the
// highest of them to the lowest, and LookRotation of that is rounded to
// 22.5°. A rule with randomRotation draws one of sixteen facings from
// Unity's global random stream at the moment the zone is first built,
// which nothing seeds; those face north here. The rest face north in the
// game too.
//
// The ten points are drawn from that same global stream, so even the
// downhill facing is a roll. What can be computed is the facing the roll
// most often gives: the game's statistic taken a few hundred times with a
// stream seeded by the site's position, and the commonest answer. Cached
// per site.
func (w *World) Facing(s Site) float64 {
	if !s.SlopeRotation || s.Radius <= 0 {
		return 0
	}
	k := [2]float32{s.X, s.Z}
	w.facingMu.Lock()
	yaw, ok := w.facings[k]
	w.facingMu.Unlock()
	if ok {
		return yaw
	}
	rng := rand.New(rand.NewPCG(uint64(math.Float32bits(s.X)), uint64(math.Float32bits(s.Z))))
	count := map[float64]int{}
	for i := 0; i < facingRolls; i++ {
		count[w.downhillRoll(s, rng)]++
	}
	best := -1
	for facing, n := range count {
		if n > best || (n == best && facing < yaw) {
			best, yaw = n, facing
		}
	}
	w.facingMu.Lock()
	w.facings[k] = yaw
	w.facingMu.Unlock()
	return yaw
}

// facingRolls is how many times the game's statistic is rolled for its
// mode: four thousand height reads a site.
const facingRolls = 400

// downhillRoll is one roll of GetTerrainDelta's facing: ten points
// uniformly within the radius, the direction from the highest to the
// lowest, LookRotation's yaw of that rounded to 22.5°.
func (w *World) downhillRoll(s Site, rng *rand.Rand) float64 {
	cx, cz, r := float64(s.X), float64(s.Z), float64(s.Radius)
	lo, hi := math.Inf(1), math.Inf(-1)
	var lowest, highest [2]float64
	for i := 0; i < 10; i++ {
		a := rng.Float64() * 2 * math.Pi
		d := math.Sqrt(rng.Float64())
		x, z := cx+math.Cos(a)*d*r, cz+math.Sin(a)*d*r
		h, ok := w.Gen.Height(x, z)
		if !ok {
			continue
		}
		if h < lo {
			lo, lowest = h, [2]float64{x, z}
		}
		if h > hi {
			hi, highest = h, [2]float64{x, z}
		}
	}
	fx, fz := lowest[0]-highest[0], lowest[1]-highest[1]
	if fx == 0 && fz == 0 {
		return 0
	}
	yaw := math.Atan2(fx, fz) * 180 / math.Pi
	if yaw < 0 {
		yaw += 360
	}
	yaw = math.Round(yaw/22.5) * 22.5
	if yaw >= 360 {
		yaw -= 360
	}
	return yaw
}

// yawQuaternion is Quaternion.Euler(0, yaw, 0), as x, y, z, w.
func yawQuaternion(yaw float64) [4]float64 {
	half := yaw * math.Pi / 360
	return [4]float64{0, math.Sin(half), 0, math.Cos(half)}
}

// groundOrWater is the height a site stands at, as PlaceLocations stands
// it: the ray onto the zone's ground as HeightmapBuilder made it, before
// any site has reshaped it, or the waterline for a rule that snaps there.
func (w *World) groundOrWater(s Site) float64 {
	if s.SnapToWater {
		return worldgen.WaterLevel
	}
	h, _ := w.rawGround(float64(s.X), float64(s.Z))
	return h
}

// tileCache is the terrain tiles a world holds, see Terrain, and the
// facings worked out so far, see Facing.
type tileCache struct {
	tilesMu sync.Mutex
	tiles   map[[2]int]*TerrainTile

	facingMu sync.Mutex
	facings  map[[2]float32]float64
}
