package scene

import (
	"math"
	"sort"
	"sync"

	"github.com/pietjan/valheim-wiki/internal/unityrandom"
	"github.com/pietjan/valheim-wiki/internal/worldgen"
)

// ZoneSystem.PlaceVegetation, draw for draw.
//
// A zone's vegetation is a pure function of the seed and of what stands in
// the zone before it: each rule re-seeds Unity's random stream with the
// seed, the zone and the prefab's name hash, so one rule's draws never
// depend on another's — except through the placement ray, which hits what
// earlier rules put down. The ray against things in neighbouring zones
// depends on which zone the player walked into first, and is left out;
// the measure of that is in the tests against the fleet's atlas.

// Vegetation is ZoneSystem.PlaceVegetation for one zone: what it stands
// there, in placement order. Cached per zone.
func (w *World) Vegetation(zx, zz int) []Place {
	w.vegMu.Lock()
	if got, ok := w.veg[[2]int{zx, zz}]; ok {
		w.vegMu.Unlock()
		return got
	}
	w.vegMu.Unlock()
	sites := w.SitesList()
	biomes := w.BiomeMap()
	if biomes == nil {
		return nil
	}
	out := w.placeVegetation(zx, zz, sites, biomes, vegOptions{keep: func(v *VegRule) bool { return true }})
	if out == nil {
		out = []Place{}
	}
	w.vegMu.Lock()
	defer w.vegMu.Unlock()
	if len(w.veg) >= maxVegZones {
		w.veg = map[[2]int][]Place{}
	}
	w.veg[[2]int{zx, zz}] = out
	return out
}

// maxVegZones is how many zones' vegetation a world keeps.
const maxVegZones = 4096

// zoneGround is one zone's heightmap as the placement reads it: the
// tile, the sector biomes at its corners, its ocean depths and its
// vegetation mask.
type zoneGround struct {
	w *World
	// tileAt is where the ground comes from: the world's cache for the
	// page, a pass's own for a whole-world pass.
	tileAt  func(tx, tz int) *TerrainTile
	tile    *TerrainTile
	cx, cz  float64
	corners [4]worldgen.Biome
	uniform bool
	// depth is Heightmap.m_oceanDepth: NW, NE, SE, SW.
	depth [4]float32
	// mask is the paint mask's alpha a vertex, lazily: NaN until read.
	mask  []float32
	sites []Site
}

func (w *World) zoneGround(zx, zz int, biomes *worldgen.BiomeMap, sites []Site, tileAt func(tx, tz int) *TerrainTile) *zoneGround {
	if tileAt == nil {
		tileAt = w.Terrain
	}
	t := tileAt(zx, zz)
	g := &zoneGround{w: w, tileAt: tileAt, tile: t, cx: float64(zx) * TileSize, cz: float64(zz) * TileSize, sites: sites}
	x0, z0 := g.cx-TileSize/2, g.cz-TileSize/2
	// HeightmapBuilder.Build: the sector at each corner, (0,0), (1,0),
	// (0,1), (1,1).
	g.corners = [4]worldgen.Biome{
		biomes.SectorAt(float32(x0), float32(z0)).Biome,
		biomes.SectorAt(float32(x0+TileSize), float32(z0)).Biome,
		biomes.SectorAt(float32(x0), float32(z0+TileSize)).Biome,
		biomes.SectorAt(float32(x0+TileSize), float32(z0+TileSize)).Biome,
	}
	g.uniform = g.corners[0] == g.corners[1] && g.corners[0] == g.corners[2] && g.corners[0] == g.corners[3]
	// Heightmap.UpdateCornerDepths.
	n := t.N
	for i, v := range [4][2]int{{0, n - 1}, {n - 1, n - 1}, {n - 1, 0}, {0, 0}} {
		g.depth[i] = float32(math.Max(0, float64(float32(worldgen.WaterLevel)-t.Heights[v[1]*n+v[0]])))
	}
	g.mask = make([]float32, n*n)
	for i := range g.mask {
		g.mask[i] = float32(math.NaN())
	}
	return g
}

// haveBiome is Heightmap.HaveBiome.
func (g *zoneGround) haveBiome(b worldgen.Biome) bool {
	for _, c := range g.corners {
		if c&b != 0 {
			return true
		}
	}
	return false
}

// isEdge is Heightmap.IsBiomeEdge; biomeArea its BiomeArea.
func (g *zoneGround) biomeArea() int {
	if g.uniform {
		return 2 // Median
	}
	return 1 // Edge
}

// biomeIndex is Heightmap.Biome.ToIndex.
func biomeIndex(b worldgen.Biome) int {
	switch b {
	case worldgen.BiomeMeadows:
		return 1
	case worldgen.BiomeSwamp:
		return 2
	case worldgen.BiomeMountain:
		return 3
	case worldgen.BiomeBlackForest:
		return 4
	case worldgen.BiomePlains:
		return 5
	case worldgen.BiomeAshLands:
		return 6
	case worldgen.BiomeDeepNorth:
		return 7
	case worldgen.BiomeOcean:
		return 8
	case worldgen.BiomeMistlands:
		return 9
	}
	return 0
}

var biomesByIndex = [10]worldgen.Biome{
	worldgen.BiomeNone, worldgen.BiomeMeadows, worldgen.BiomeSwamp, worldgen.BiomeMountain, worldgen.BiomeBlackForest,
	worldgen.BiomePlains, worldgen.BiomeAshLands, worldgen.BiomeDeepNorth, worldgen.BiomeOcean, worldgen.BiomeMistlands,
}

// biome is Heightmap.GetBiome: the corner biome in a uniform zone; else
// each corner weighted by (1.414 - distance)³ from the point in the
// zone's unit square, and the heaviest biome, the lowest index on a tie.
func (g *zoneGround) biome(x, z float64) worldgen.Biome {
	if g.uniform {
		return g.corners[0]
	}
	nx := float32((x-g.cx)/TileSize + 0.5)
	nz := float32((z-g.cz)/TileSize + 0.5)
	var weights [10]float32
	dist := func(rx, ry float32) float32 {
		dx, dy := nx-rx, nz-ry
		d := float32(math.Sqrt(float64(dx*dx + dy*dy)))
		k := 1.4140000343322754 - d
		return k * k * k
	}
	weights[biomeIndex(g.corners[0])] += dist(0, 0)
	weights[biomeIndex(g.corners[1])] += dist(1, 0)
	weights[biomeIndex(g.corners[2])] += dist(0, 1)
	weights[biomeIndex(g.corners[3])] += dist(1, 1)
	best, bw := 0, float32(-99999)
	for j := 1; j < len(weights); j++ {
		if weights[j] > bw {
			best, bw = j, weights[j]
		}
	}
	return biomesByIndex[best]
}

// vertex is Heightmap.WorldToVertex.
func (g *zoneGround) vertex(x, z float64) (int, int) {
	return int(math.Floor((x-g.cx)/g.tile.Step+0.5)) + g.tile.N/2,
		int(math.Floor((z-g.cz)/g.tile.Step+0.5)) + g.tile.N/2
}

// oceanDepth is Heightmap.GetOceanDepth.
func (g *zoneGround) oceanDepth(x, z float64) float32 {
	vx, vz := g.vertex(x, z)
	width := float32(g.tile.N - 1)
	t := float32(vx) / width
	t2 := float32(vz) / width
	a := g.depth[3] + (g.depth[2]-g.depth[3])*t
	b := g.depth[0] + (g.depth[1]-g.depth[0])*t
	return a + (b-a)*t2
}

// vegetationMask is Heightmap.GetVegetationMask: the paint mask's alpha
// at the vertex under the point, half a metre back. The alpha starts as
// the world's vegetation mask, the lava and the mist, blended across the
// zone's corners as the heights are, and a site's ClearVegetation paint
// pulls it toward nought.
func (g *zoneGround) vegetationMask(x, z float64) float32 {
	vx, vz := g.vertex(x-0.5, z-0.5)
	n := g.tile.N
	if vx < 0 || vz < 0 || vx >= n || vz >= n {
		return 0
	}
	i := vz*n + vx
	if v := g.mask[i]; v == v {
		return v
	}
	wx := g.tile.Origin[0] + float64(vx)*g.tile.Step
	wz := g.tile.Origin[1] + float64(vz)*g.tile.Step
	var v float32
	if g.uniform {
		v = g.w.Gen.Vegetation(g.corners[0], wx, wz)
	} else {
		var c [4]float32
		for k, b := range g.corners {
			c[k] = g.w.Gen.Vegetation(b, wx, wz)
		}
		tx := smoothStep(float32(vx) / float32(n-1))
		tz := smoothStep(float32(vz) / float32(n-1))
		a := c[0] + (c[1]-c[0])*tx
		b := c[2] + (c[3]-c[2])*tx
		v = a + (b-a)*tz
	}
	v = g.clearedAt(vx, vz, v)
	g.mask[i] = v
	return v
}

func smoothStep(t float32) float32 {
	t = min(1, max(0, t))
	return t * t * (3 - 2*t)
}

// clearedAt applies every site's ClearVegetation paint to one vertex's
// alpha: Heightmap.PaintCleared lerps the pixel toward alpha nought by
// (1 - d/r)^0.1 within the radius.
func (g *zoneGround) clearedAt(vx, vz int, alpha float32) float32 {
	span := TileSize/2 + 48
	for _, s := range g.sites {
		if math.Abs(float64(s.X)-g.cx) > span || math.Abs(float64(s.Z)-g.cz) > span {
			continue
		}
		for _, p := range g.w.opsOf(s) {
			if p.Op.PaintRadius <= 0 || p.Op.Paint != PaintCleared {
				continue
			}
			px, pz := g.vertex(p.X-0.5, p.Z-0.5)
			rv := float32(p.Op.PaintRadius) / float32(g.tile.Step)
			d := float32(math.Hypot(float64(vx-px), float64(vz-pz)))
			if d > float32(math.Ceil(float64(rv))) {
				continue
			}
			f := 1 - min(1, max(0, d/rv))
			f = float32(math.Pow(float64(f), 0.1))
			alpha += (0 - alpha) * f
		}
	}
	return alpha
}

// groundData is ZoneSystem.GetGroundData: the ray onto the terrain, the
// height and the face normal where it lands, and the biome there.
func (g *zoneGround) groundData(x, z float64) (h float64, normal [3]float64, biome worldgen.Biome) {
	h, normal = groundHitIn(g.tileAt, x, z)
	return h, normal, g.biome(x, z)
}

// groundHit is the terrain collider under a point: the height on the
// triangle the game's mesh has there, and that triangle's normal. The
// mesh splits each cell on the diagonal from (x, z+1) to (x+1, z).
func (w *World) groundHit(x, z float64) (float64, [3]float64) { return groundHitIn(w.Terrain, x, z) }

func groundHitIn(tileAt func(tx, tz int) *TerrainTile, x, z float64) (float64, [3]float64) {
	tx, tz := TileAt(x, z)
	t := tileAt(tx, tz)
	fx, fz := (x-t.Origin[0])/t.Step, (z-t.Origin[1])/t.Step
	ix, iz := int(math.Floor(fx)), int(math.Floor(fz))
	n := t.N
	if ix < 0 {
		ix = 0
	}
	if iz < 0 {
		iz = 0
	}
	if ix > n-2 {
		ix = n - 2
	}
	if iz > n-2 {
		iz = n - 2
	}
	h00, h10 := float64(t.Heights[iz*n+ix]), float64(t.Heights[iz*n+ix+1])
	h01, h11 := float64(t.Heights[(iz+1)*n+ix]), float64(t.Heights[(iz+1)*n+ix+1])
	dx, dz := fx-float64(ix), fz-float64(iz)
	var h float64
	var nrm [3]float64
	if dx+dz <= 1 {
		h = h00 + dx*(h10-h00) + dz*(h01-h00)
		nrm = [3]float64{-(h10 - h00) / t.Step, 1, -(h01 - h00) / t.Step}
	} else {
		h = h11 + (1-dx)*(h01-h11) + (1-dz)*(h10-h11)
		nrm = [3]float64{(h01 - h11) / t.Step, 1, -(h11 - h10) / t.Step}
	}
	return h, norm3(nrm)
}

// terrainDelta is ZoneSystem.GetTerrainDelta: the height range over ten
// random points within the radius, two draws each.
func (g *zoneGround) terrainDelta(rng *unityrandom.Random, x, z float64, radius float32) float32 {
	hi, lo := float32(-999999), float32(999999)
	for i := 0; i < 10; i++ {
		ox, oz := rng.InsideUnitCircle()
		px := float32(x) + ox*radius
		pz := float32(z) + oz*radius
		h, _ := groundHitIn(g.tileAt, float64(px), float64(pz))
		gh := float32(h)
		if gh < lo {
			lo = gh
		}
		if gh > hi {
			hi = gh
		}
	}
	return hi - lo
}

// vegOptions says what a run of placeVegetation is for. keep chooses the
// placements returned. run, when set, chooses which rules run at all: a
// rule may be skipped only when nothing it places can matter to a rule
// kept - it comes after every kept rule, since each rule draws from its
// own random stream, or it stands nothing a later rule's placement can
// hit. tileAt is where the ground comes from, the world's cache when nil.
type vegOptions struct {
	keep   func(*VegRule) bool
	run    func(index int, rule *VegRule) bool
	tileAt func(tx, tz int) *TerrainTile
}

// placeVegetation runs every rule over one zone.
//
// What one rule stands is in the way of the next, so a rule whose
// placements are not wanted may still have to run; see vegOptions.
func (w *World) placeVegetation(zx, zz int, sites []Site, biomes *worldgen.BiomeMap, opt vegOptions) []Place {
	tables := vegetation()
	keep := opt.keep
	g := w.zoneGround(zx, zz, biomes, sites, opt.tileAt)
	cx, cz := g.cx, g.cz

	// What stands before the vegetation: the zone's sites, each a blocker
	// and, when its rule clears, a clear area. PlaceLocations adds the
	// clear area at the site's position with the exterior radius.
	var standing blockers
	type clearArea struct{ x, z, r float64 }
	var clears []clearArea
	for _, s := range sites {
		if s.Zone.X != int16(zx) || s.Zone.Y != int16(zz) {
			continue
		}
		if s.ClearArea {
			clears = append(clears, clearArea{float64(s.X), float64(s.Z), float64(s.Radius)})
		}
		// The site's solids, from the site table, stand in the way as its
		// pieces would.
		if soup := siteSoup(s.Prefab); soup != nil {
			yaw := w.Facing(s)
			standing.add(soup, [3]float64{float64(s.X), w.groundOrWater(s), float64(s.Z)}, axisAngle([3]float64{0, 1, 0}, yaw), 1)
		}
	}
	insideClear := func(x, z float64) bool {
		for _, c := range clears {
			if x > c.x-c.r && x < c.x+c.r && z > c.z-c.r && z < c.z+c.r {
				return true
			}
		}
		return false
	}

	var out []Place
	seed := w.Seed
	for vi, veg := range tables.rules {
		if !veg.Enable || !g.haveBiome(veg.Biome) {
			continue
		}
		if opt.run != nil && !opt.run(vi, veg) {
			continue
		}
		rng := unityrandom.New(seed + int32(zx)*4271 + int32(zz)*9187 + veg.PrefabHash)
		count := int32(1)
		if veg.Max < 1 {
			if rng.Value() > veg.Max {
				continue
			}
		} else {
			count = rng.RangeInt(int32(veg.Min), int32(veg.Max)+1)
		}
		cosMax := float32(math.Cos(math.Pi / 180 * float64(veg.MaxTilt)))
		cosMin := float32(math.Cos(math.Pi / 180 * float64(veg.MinTilt)))
		span := 32 - veg.GroupRadius
		var surround []float32
		attempts := count
		if veg.ForcePlacement {
			attempts = count * 50
		}
		placed := int32(0)
		soup := prefabSoup(veg.Prefab)
		draw := keep(veg)
		for i := int32(0); i < attempts; i++ {
			gx := float64(rng.RangeFloat(float32(cx)-span, float32(cx)+span))
			gz := float64(rng.RangeFloat(float32(cz)-span, float32(cz)+span))
			sector := biomes.SectorAt(float32(gx), float32(gz))
			if !altAllows(sector, veg, tables) {
				continue
			}
			members := rng.RangeInt(veg.GroupSizeMin, veg.GroupSizeMax+1)
			any := false
			for m := int32(0); m < members; m++ {
				px, pz := gx, gz
				if m != 0 {
					// GetRandomPointInRadius.
					f := float64(rng.Value()) * math.Pi * 2
					r := float64(rng.RangeFloat(0, veg.GroupRadius))
					px = gx + float32s(math.Sin(f))*r
					pz = gz + float32s(math.Cos(f))*r
				}
				yaw := float64(rng.RangeInt(0, 360))
				scale := rng.RangeFloat(veg.ScaleMin, veg.ScaleMax)
				tiltX := float64(rng.RangeFloat(-veg.RandTilt, veg.RandTilt))
				tiltZ := float64(rng.RangeFloat(-veg.RandTilt, veg.RandTilt))
				if veg.BlockCheck && standing.blocked(px, pz) {
					continue
				}
				py, normal, biome := g.groundData(px, pz)
				if veg.Biome&biome == 0 || veg.BiomeArea&g.biomeArea() == 0 {
					continue
				}
				if veg.SnapToStaticSolid {
					if h, ok := standing.staticSolidHeight(px, pz); ok {
						py = h
						normal = [3]float64{0, 1, 0}
					}
				}
				alt := float32(py) - float32(worldgen.WaterLevel)
				if alt < veg.MinAltitude || alt > veg.MaxAltitude {
					continue
				}
				if veg.MinVegetation != veg.MaxVegetation {
					mask := g.vegetationMask(px, pz)
					if mask > veg.MaxVegetation || mask < veg.MinVegetation {
						continue
					}
				}
				if veg.MinOceanDepth != veg.MaxOceanDepth {
					depth := g.oceanDepth(px, pz)
					if depth < veg.MinOceanDepth || depth > veg.MaxOceanDepth {
						continue
					}
				}
				if float32(normal[1]) < cosMax || float32(normal[1]) > cosMin {
					continue
				}
				if veg.TerrainDeltaRadius > 0 {
					delta := g.terrainDelta(rng, px, pz, veg.TerrainDeltaRadius)
					if delta > veg.MaxTerrainDelta || delta < veg.MinTerrainDelta {
						continue
					}
				}
				if veg.MinDistanceFromCenter > 0 || veg.MaxDistanceFromCenter > 0 {
					d := float32(math.Sqrt(px*px + pz*pz))
					if (veg.MinDistanceFromCenter > 0 && d < veg.MinDistanceFromCenter) || (veg.MaxDistanceFromCenter > 0 && d > veg.MaxDistanceFromCenter) {
						continue
					}
				}
				if veg.InForest {
					f := worldgen.ForestFactor(worldgen.Vec3{X: float32(px), Z: float32(pz)})
					if f < veg.ForestTresholdMin || f > veg.ForestTresholdMax {
						continue
					}
				}
				if veg.SurroundCheckVegetation {
					total := float32(0)
					for layer := 0; layer < veg.SurroundCheckLayers; layer++ {
						dist := float32(layer+1) / float32(veg.SurroundCheckLayers) * veg.SurroundCheckDistance
						for k := 0; k < 6; k++ {
							f := float64(k) / 6 * math.Pi * 2
							mask := g.vegetationMask(px+float64(float32(math.Sin(f))*dist), pz+float64(float32(math.Cos(f))*dist))
							total += mask * ((1 - dist) / (veg.SurroundCheckDistance * 2))
						}
					}
					surround = append(surround, total)
					if len(surround) < 10 {
						continue
					}
					best, sum := surround[0], float32(0)
					for _, v := range surround {
						best = max(best, v)
						sum += v
					}
					avg := sum / float32(len(surround))
					if total < avg+(best-avg)*veg.SurroundBetterThanAverage {
						continue
					}
				}
				if insideClear(px, pz) {
					continue
				}
				if veg.SnapToWater {
					py = worldgen.WaterLevel
				}
				py += float64(veg.GroundOffset)
				rot := euler(tiltX, yaw, tiltZ)
				if veg.ChanceToUseGroundTilt > 0 && rng.Value() <= veg.ChanceToUseGroundTilt {
					fwd := axisAngle([3]float64{0, 1, 0}, yaw).rotate([3]float64{0, 0, 1})
					rot = lookRotation(cross3(normal, fwd), normal)
				}
				// Instantiate: what the prefab's components draw as they wake.
				for k := 0; k < veg.SpawnDraws; k++ {
					rng.Value()
				}
				pos := [3]float64{px, py, pz}
				standing.add(soup, pos, rot, float64(scale))
				if draw {
					out = append(out, Place{
						Name: veg.Prefab, Site: veg.Name, At: pos,
						Rot:   [4]float64(rot),
						Scale: [3]float64{float64(scale), float64(scale), float64(scale)},
						Kind:  "vegetation",
					})
				}
				any = true
			}
			if any {
				placed++
			}
			if placed >= count {
				break
			}
		}
	}
	return out
}

// float32s rounds a double through float32, as the game's Mathf does.
func float32s(v float64) float64 { return float64(float32(v)) }

// altAllows is the alt-biome gate on a group: a rule an alt biome added
// only places in a sector carrying it, and no sector's alt biome may
// block the rule by name.
func altAllows(sector *worldgen.BiomeSector, veg *VegRule, tables *vegTables) bool {
	if sector == nil {
		return veg.AltBiomeParent == ""
	}
	if veg.AltBiomeParent != "" {
		found := false
		for _, a := range sector.AltBiomes {
			if a.Name == veg.AltBiomeParent {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	for _, a := range sector.AltBiomes {
		if tables.blocks[a.Name][veg.Name] {
			return false
		}
	}
	return true
}

// vegCache is the zones' vegetation a world holds, see vegetationOf.
type vegCache struct {
	vegMu sync.Mutex
	veg   map[[2]int][]Place
}

// VegetationNames is every prefab the rules can place, for warming a
// viewer's models.
func VegetationNames() []string {
	seen := map[string]bool{}
	var out []string
	for _, r := range VegetationRules() {
		if r.Enable && !seen[r.Prefab] {
			seen[r.Prefab] = true
			out = append(out, r.Prefab)
		}
	}
	sort.Strings(out)
	return out
}
