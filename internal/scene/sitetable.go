package scene

import (
	"bufio"
	_ "embed"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// The reshaping a site does to the ground it stands on.
//
// A location prefab carries TerrainModifier components — flatten a disc
// to the site's height, feather the rim, paint dirt or paving — and the
// game applies them to the zone's heightmap when the site is built. The
// seed says nothing about them, so they are baked from the client's
// prefabs into sites.tsv, the same table valheim-npc reads its camps'
// levelling from, one line per location: the ops as x,y,z offsets in the
// prefab's frame with the radii, the offset, the square flag, the power
// and the paint, and the modifier's m_sortOrder.

//go:embed sites.tsv
var siteTable string

// TerrainOp is one modifier: where it sits in the prefab's frame and what
// it does. A radius of nought means it does not do that one.
type TerrainOp struct {
	X, Y, Z      float32
	LevelRadius  float32
	LevelOffset  float32
	Square       bool
	SmoothRadius float32
	SmoothPower  float32
	PaintRadius  float32
	Paint        Paint
	// SortOrder is m_sortOrder, the first key the game orders modifiers by.
	SortOrder int
}

// Paint is what a modifier paints the ground: the game's paint mask
// colours, as the terrain shader and the grass read them.
type Paint uint8

const (
	PaintNone Paint = iota
	PaintDirt
	PaintCultivated
	PaintPaved
	PaintCleared // vegetation cleared, ground as it was
	// PaintReset takes back whatever paint an earlier modifier left. It is
	// never stored: a vertex it reaches holds PaintNone.
	PaintReset
)

// SolidBox is one box of a site's solid geometry, in the site's own frame:
// the centre and three half-axis vectors. The table lists only geometry
// with no ZNetView of its own.
type SolidBox struct {
	Centre, A, B, C [3]float32
}

type siteEntry struct {
	ops    []TerrainOp
	solids []SolidBox
}

var siteEntries = sync.OnceValue(func() map[string]*siteEntry {
	out := map[string]*siteEntry{}
	s := bufio.NewScanner(strings.NewReader(siteTable))
	s.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for s.Scan() {
		line := s.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		f := strings.Split(line, "\t")
		if len(f) < 5 {
			continue
		}
		e := &siteEntry{}
		out[f[0]] = e
		if len(f) > 5 && f[5] != "-" && f[5] != "" {
			for _, box := range strings.Split(f[5], ";") {
				n := strings.Split(box, ",")
				if len(n) != 12 {
					continue
				}
				var b SolidBox
				for k := 0; k < 3; k++ {
					b.Centre[k], b.A[k], b.B[k], b.C[k] = num(n[k]), num(n[3+k]), num(n[6+k]), num(n[9+k])
				}
				e.solids = append(e.solids, b)
			}
		}
		if f[4] == "-" {
			continue
		}
		var ops []TerrainOp
		for _, op := range strings.Split(f[4], ";") {
			n := strings.Split(op, ",")
			if len(n) != 11 {
				continue
			}
			ops = append(ops, TerrainOp{
				X: num(n[0]), Y: num(n[1]), Z: num(n[2]),
				LevelRadius: num(n[3]), LevelOffset: num(n[4]), Square: n[5] == "1",
				SmoothRadius: num(n[6]), SmoothPower: num(n[7]),
				PaintRadius: num(n[8]), Paint: paintOf(n[9]),
				SortOrder: int(num(n[10])),
			})
		}
		e.ops = ops
	}
	return out
})

func num(s string) float32 {
	v, _ := strconv.ParseFloat(s, 32)
	return float32(v)
}

func paintOf(s string) Paint {
	switch s {
	case "Dirt":
		return PaintDirt
	case "Cultivate":
		return PaintCultivated
	case "Paved":
		return PaintPaved
	case "ClearVegetation":
		return PaintCleared
	case "Reset":
		return PaintReset
	}
	return PaintNone
}

// SiteOps is a location's modifiers by prefab name, nil for none.
func SiteOps(prefab string) []TerrainOp {
	if e := siteEntries()[prefab]; e != nil {
		return e.ops
	}
	return nil
}

// SiteSolids is a location's solid boxes by prefab name, nil for none.
func SiteSolids(prefab string) []SolidBox {
	if e := siteEntries()[prefab]; e != nil {
		return e.solids
	}
	return nil
}

// placedOp is one site's modifier carried into the world: turned by the
// site's facing, moved to where it stands, its heights in world metres.
type placedOp struct {
	X, Z   float64
	Y      float64 // the modifier's own height: the site's plus its offset
	Target float64 // what level and smooth pull the ground to: Y plus the level offset
	Op     TerrainOp
	Order  int // m_sortOrder
}

// reach is how far from its centre an op moves or paints ground.
func (p placedOp) reach() float64 {
	r := p.Op.LevelRadius
	if p.Op.SmoothRadius > r {
		r = p.Op.SmoothRadius
	}
	if p.Op.PaintRadius > r {
		r = p.Op.PaintRadius
	}
	return float64(r)
}

// opsOf is every modifier a site applies: the location's own. (Its
// generator's rooms' are not reproduced here.) Not in any order: the game
// orders modifiers across every site at once, so ordering is sortOps'
// job, over all of them that reach a piece of ground.
func (w *World) opsOf(s Site) []placedOp {
	return placeOps(s, w.groundOrWater(s), w.Facing(s))
}

// sortOps puts modifiers in the order Heightmap.ApplyModifiers runs them:
// TerrainModifier.GetAllInstances sorts by SortByModifiers, which is the
// player-made flag, then m_sortOrder, then creation time, then the squared
// length of transform.position. None of these is player-made and none has
// a ZDO, so creation time is nought for all and the order is m_sortOrder,
// then distance from the world's origin. That is across sites: where two
// sites' modifiers overlap - a camp's flatten and the mound of the witch's
// hut beside it - which one wins depends on where each stands, not on
// which site they belong to.
//
// The squared length is taken in float32, as Unity takes it; List.Sort is
// not stable, but two modifiers at the same point are the only tie.
func sortOps(ops []placedOp) {
	sq := func(p placedOp) float32 {
		x, y, z := float32(p.X), float32(p.Y), float32(p.Z)
		return x*x + y*y + z*z
	}
	sort.SliceStable(ops, func(i, j int) bool {
		if ops[i].Order != ops[j].Order {
			return ops[i].Order < ops[j].Order
		}
		return sq(ops[i]) < sq(ops[j])
	})
}

// placeOps is a site's ops in the world, as rooms.Place does it in
// valheim-npc: Unity's yaw about y applied to each offset, from the
// height the site stands at.
func placeOps(s Site, ground, yaw float64) []placedOp {
	ops := SiteOps(s.Prefab)
	if len(ops) == 0 {
		return nil
	}
	rad := yaw * math.Pi / 180
	sin, cos := math.Sin(rad), math.Cos(rad)
	out := make([]placedOp, 0, len(ops))
	for _, op := range ops {
		dx := float64(op.X)*cos + float64(op.Z)*sin
		dz := -float64(op.X)*sin + float64(op.Z)*cos
		y := ground + float64(op.Y)
		out = append(out, placedOp{
			X: float64(s.X) + dx, Z: float64(s.Z) + dz,
			Y: y, Target: y + float64(op.LevelOffset), Op: op, Order: op.SortOrder,
		})
	}
	return out
}
