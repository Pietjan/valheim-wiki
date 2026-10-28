package scene

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"fmt"
	"io"
	"io/fs"
	"math"
	"sort"
	"sync"
)

// The placement ray.
//
// ZoneSystem.IsBlocked drops a ray from two kilometres up onto a point and
// asks whether it hits anything on the Default, static_solid, Default_small
// or piece layers: a rock, a tree already standing, a location's wall. A
// tree's collider is a coarse hull of trunk and crown, so a sapling cannot
// be placed under a beech, and that is a large part of why the game's
// forests look as they do. GetStaticSolidHeight is the same ray against
// static_solid alone, for what grows on rocks.
//
// The colliders come out of the running game as triangle soups in the
// prefab's frame (vegetation/colliders/<prefab>.bin: a magic, a version, a
// vertex count, a triangle count, the vertices, the indices, and from
// version two a layer byte a triangle), tessellated where the game has a
// box, a sphere or a capsule. A location's blocker is its whole-site
// render mesh, which is coarser than its colliders but stands where they
// do.

// Unity's layers in the mask, by number.
const (
	layerDefault      = 0
	layerPiece        = 10
	layerStaticSolid  = 15
	layerDefaultSmall = 20
)

// colliderSoup is one prefab's blocking triangles with a tree over them.
type colliderSoup struct {
	verts []float32
	tris  []uint32
	layer []uint8
	nodes []bvhNode
	// order is triangle numbers as the leaves list them.
	order    []uint32
	min, max [3]float32
}

type bvhNode struct {
	min, max    [3]float32
	left, right int32 // children, or -1 at a leaf
	first, n    int32 // the leaf's run in order
}

// readSoup decodes a colliders file.
func readSoup(data []byte) (*colliderSoup, error) {
	if len(data) < 16 || binary.LittleEndian.Uint32(data) != 0x4c4f4356 {
		return nil, fmt.Errorf("not a colliders file")
	}
	version := binary.LittleEndian.Uint32(data[4:])
	nv := int(binary.LittleEndian.Uint32(data[8:]))
	nt := int(binary.LittleEndian.Uint32(data[12:]))
	need := 16 + nv*12 + nt*12
	if version >= 2 {
		need += nt
	}
	if len(data) < need {
		return nil, fmt.Errorf("colliders file is short: %d of %d bytes", len(data), need)
	}
	s := &colliderSoup{verts: make([]float32, nv*3), tris: make([]uint32, nt*3), layer: make([]uint8, nt)}
	at := 16
	for i := range s.verts {
		s.verts[i] = math.Float32frombits(binary.LittleEndian.Uint32(data[at:]))
		at += 4
	}
	for i := range s.tris {
		s.tris[i] = binary.LittleEndian.Uint32(data[at:])
		at += 4
		if int(s.tris[i]) >= nv {
			return nil, fmt.Errorf("index %d past %d vertices", s.tris[i], nv)
		}
	}
	if version >= 2 {
		copy(s.layer, data[at:at+nt])
	}
	s.build()
	return s, nil
}

// soupOfBoxes is a site's solids as a blocker on the piece layer: each
// box's twelve triangles, from its centre and three half-axis vectors.
func soupOfBoxes(boxes []SolidBox) *colliderSoup {
	if len(boxes) == 0 {
		return nil
	}
	s := &colliderSoup{}
	for _, b := range boxes {
		base := uint32(len(s.verts) / 3)
		for i := 0; i < 8; i++ {
			sx, sy, sz := float32(1), float32(1), float32(1)
			if i&1 != 0 {
				sx = -1
			}
			if i&2 != 0 {
				sy = -1
			}
			if i&4 != 0 {
				sz = -1
			}
			for k := 0; k < 3; k++ {
				s.verts = append(s.verts, b.Centre[k]+sx*b.A[k]+sy*b.B[k]+sz*b.C[k])
			}
		}
		// The faces, by corner index bits (1 flips A, 2 flips B, 4 flips C).
		for _, f := range [6][4]uint32{{0, 2, 6, 4}, {1, 5, 7, 3}, {0, 4, 5, 1}, {2, 3, 7, 6}, {0, 1, 3, 2}, {4, 6, 7, 5}} {
			s.tris = append(s.tris, base+f[0], base+f[1], base+f[2], base+f[0], base+f[2], base+f[3])
			s.layer = append(s.layer, layerPiece, layerPiece)
		}
	}
	s.build()
	return s
}

func (s *colliderSoup) tri(i uint32, k int) [3]float32 {
	v := s.tris[i*3+uint32(k)] * 3
	return [3]float32{s.verts[v], s.verts[v+1], s.verts[v+2]}
}

// build makes the tree: a median split on the longest axis of the
// triangles' centroids, leaves of four.
func (s *colliderSoup) build() {
	n := len(s.tris) / 3
	s.order = make([]uint32, n)
	cent := make([][3]float32, n)
	for i := 0; i < n; i++ {
		s.order[i] = uint32(i)
		a, b, c := s.tri(uint32(i), 0), s.tri(uint32(i), 1), s.tri(uint32(i), 2)
		for k := 0; k < 3; k++ {
			cent[i][k] = (a[k] + b[k] + c[k]) / 3
		}
	}
	s.nodes = s.nodes[:0]
	if n == 0 {
		s.min, s.max = [3]float32{}, [3]float32{}
		return
	}
	root := s.split(cent, 0, n)
	s.min, s.max = s.nodes[root].min, s.nodes[root].max
}

func (s *colliderSoup) split(cent [][3]float32, first, end int) int32 {
	id := int32(len(s.nodes))
	s.nodes = append(s.nodes, bvhNode{left: -1, right: -1, first: int32(first), n: int32(end - first)})
	node := &s.nodes[id]
	node.min = [3]float32{math.MaxFloat32, math.MaxFloat32, math.MaxFloat32}
	node.max = [3]float32{-math.MaxFloat32, -math.MaxFloat32, -math.MaxFloat32}
	cmin, cmax := node.min, node.max
	for i := first; i < end; i++ {
		t := s.order[i]
		for k := 0; k < 3; k++ {
			p := s.tri(t, k)
			for a := 0; a < 3; a++ {
				node.min[a] = min(node.min[a], p[a])
				node.max[a] = max(node.max[a], p[a])
			}
		}
		for a := 0; a < 3; a++ {
			cmin[a] = min(cmin[a], cent[t][a])
			cmax[a] = max(cmax[a], cent[t][a])
		}
	}
	if end-first <= 4 {
		return id
	}
	axis := 0
	for a := 1; a < 3; a++ {
		if cmax[a]-cmin[a] > cmax[axis]-cmin[axis] {
			axis = a
		}
	}
	if cmax[axis]-cmin[axis] == 0 {
		return id
	}
	run := s.order[first:end]
	sort.Slice(run, func(i, j int) bool { return cent[run[i]][axis] < cent[run[j]][axis] })
	mid := (first + end) / 2
	left := s.split(cent, first, mid)
	right := s.split(cent, mid, end)
	s.nodes[id].left, s.nodes[id].right = left, right
	return id
}

// nearest is the first triangle along the ray o + t d, t in [0, tmax], on
// a layer in mask, as its t; false for none. In the soup's own frame.
func (s *colliderSoup) nearest(o, d [3]float32, tmax float32, mask uint32) (float32, bool) {
	if len(s.nodes) == 0 {
		return 0, false
	}
	inv := [3]float32{}
	for a := 0; a < 3; a++ {
		if d[a] != 0 {
			inv[a] = 1 / d[a]
		} else {
			inv[a] = float32(math.Inf(1))
		}
	}
	best, hit := tmax, false
	stack := []int32{0}
	for len(stack) > 0 {
		id := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		node := &s.nodes[id]
		if !slab(o, inv, node.min, node.max, best) {
			continue
		}
		if node.left < 0 {
			for i := node.first; i < node.first+node.n; i++ {
				t := s.order[i]
				if mask&(1<<s.layer[t]) == 0 {
					continue
				}
				if tt, ok := rayTriangle(o, d, s.tri(t, 0), s.tri(t, 1), s.tri(t, 2)); ok && tt >= 0 && tt < best {
					best, hit = tt, true
				}
			}
			continue
		}
		stack = append(stack, node.left, node.right)
	}
	return best, hit
}

// slab is whether the ray meets the box before tmax.
func slab(o, inv, bmin, bmax [3]float32, tmax float32) bool {
	t0, t1 := float32(0), tmax
	for a := 0; a < 3; a++ {
		lo := (bmin[a] - o[a]) * inv[a]
		hi := (bmax[a] - o[a]) * inv[a]
		if lo > hi {
			lo, hi = hi, lo
		}
		// A zero direction component gives ±Inf or NaN: NaN compares false
		// and must not narrow the slab, Inf narrows it to nothing.
		if lo == lo && lo > t0 {
			t0 = lo
		}
		if hi == hi && hi < t1 {
			t1 = hi
		}
		if t0 > t1 {
			return false
		}
	}
	return true
}

// rayTriangle is Möller–Trumbore, both faces.
func rayTriangle(o, d, a, b, c [3]float32) (float32, bool) {
	e1 := [3]float32{b[0] - a[0], b[1] - a[1], b[2] - a[2]}
	e2 := [3]float32{c[0] - a[0], c[1] - a[1], c[2] - a[2]}
	p := cross(d, e2)
	det := dot(e1, p)
	if det > -1e-9 && det < 1e-9 {
		return 0, false
	}
	inv := 1 / det
	s := [3]float32{o[0] - a[0], o[1] - a[1], o[2] - a[2]}
	u := dot(s, p) * inv
	if u < 0 || u > 1 {
		return 0, false
	}
	q := cross(s, e1)
	v := dot(d, q) * inv
	if v < 0 || u+v > 1 {
		return 0, false
	}
	return dot(e2, q) * inv, true
}

func cross(a, b [3]float32) [3]float32 {
	return [3]float32{a[1]*b[2] - a[2]*b[1], a[2]*b[0] - a[0]*b[2], a[0]*b[1] - a[1]*b[0]}
}

func dot(a, b [3]float32) float32 { return a[0]*b[0] + a[1]*b[1] + a[2]*b[2] }

// soups is the prefabs' colliders, read on first use, and the site meshes
// turned into blockers.
type soupCache struct {
	mu     sync.Mutex
	byName map[string]*colliderSoup
}

var soups = soupCache{byName: map[string]*colliderSoup{}}

// prefabSoup is a vegetation prefab's blockers, nil when it has none.
func prefabSoup(prefab string) *colliderSoup {
	soups.mu.Lock()
	defer soups.mu.Unlock()
	if s, ok := soups.byName[prefab]; ok {
		return s
	}
	var s *colliderSoup
	if packed, err := fs.ReadFile(vegetationFS, "vegetation/colliders/"+prefab+".bin.gz"); err == nil {
		zr, err := gzip.NewReader(bytes.NewReader(packed))
		if err != nil {
			panic("scene: vegetation/colliders/" + prefab + ".bin.gz: " + err.Error())
		}
		data, err := io.ReadAll(zr)
		if err != nil {
			panic("scene: vegetation/colliders/" + prefab + ".bin.gz: " + err.Error())
		}
		if s, err = readSoup(data); err != nil {
			panic("scene: vegetation/colliders/" + prefab + ".bin: " + err.Error())
		}
	}
	soups.byName[prefab] = s
	return s
}

// siteSoup is a site's solids as a blocker, nil for a site without any.
// Keyed apart from the prefabs' own soups.
func siteSoup(prefab string) *colliderSoup {
	key := prefab + SiteSuffix
	soups.mu.Lock()
	defer soups.mu.Unlock()
	if s, ok := soups.byName[key]; ok {
		return s
	}
	s := soupOfBoxes(SiteSolids(prefab))
	soups.byName[key] = s
	return s
}

// A blocker is one standing thing: a soup placed in the world.
type blocker struct {
	soup  *colliderSoup
	pos   [3]float64
	rot   quat
	scale float64
	// The soup's box in the world, for a quick refusal.
	xmin, xmax, zmin, zmax float64
}

func newBlocker(s *colliderSoup, pos [3]float64, rot quat, scale float64) blocker {
	b := blocker{soup: s, pos: pos, rot: rot, scale: scale}
	b.xmin, b.zmin = math.Inf(1), math.Inf(1)
	b.xmax, b.zmax = math.Inf(-1), math.Inf(-1)
	for i := 0; i < 8; i++ {
		c := [3]float64{float64(s.min[0]), float64(s.min[1]), float64(s.min[2])}
		if i&1 != 0 {
			c[0] = float64(s.max[0])
		}
		if i&2 != 0 {
			c[1] = float64(s.max[1])
		}
		if i&4 != 0 {
			c[2] = float64(s.max[2])
		}
		w := rot.rotate([3]float64{c[0] * scale, c[1] * scale, c[2] * scale})
		x, z := w[0]+pos[0], w[2]+pos[2]
		b.xmin, b.xmax = math.Min(b.xmin, x), math.Max(b.xmax, x)
		b.zmin, b.zmax = math.Min(b.zmin, z), math.Max(b.zmax, z)
	}
	return b
}

// drop is the first thing the ray from (x, top, z) straight down meets
// within reach, on a layer in mask: the height it meets it at. The game's
// ray starts two kilometres up and runs ten.
func (b *blocker) drop(x, top, z, reach float64, mask uint32) (float64, bool) {
	if x < b.xmin || x > b.xmax || z < b.zmin || z > b.zmax {
		return 0, false
	}
	inv := b.rot.conj()
	o := inv.rotate([3]float64{(x - b.pos[0]) / b.scale, (top - b.pos[1]) / b.scale, (z - b.pos[2]) / b.scale})
	d := inv.rotate([3]float64{0, -1 / b.scale, 0})
	t, ok := b.soup.nearest(
		[3]float32{float32(o[0]), float32(o[1]), float32(o[2])},
		[3]float32{float32(d[0]), float32(d[1]), float32(d[2])},
		float32(reach), mask)
	if !ok {
		return 0, false
	}
	return top - float64(t), true
}

// blockers is what stands in a zone so far, in placement order.
type blockers struct {
	list []blocker
}

func (bs *blockers) add(s *colliderSoup, pos [3]float64, rot quat, scale float64) {
	if s == nil || len(s.nodes) == 0 {
		return
	}
	bs.list = append(bs.list, newBlocker(s, pos, rot, scale))
}

const (
	rayTop   = 2000.0
	rayReach = 10000.0
	// blockMask is ZoneSystem.m_blockRayMask.
	blockMask uint32 = 1<<layerDefault | 1<<layerStaticSolid | 1<<layerDefaultSmall | 1<<layerPiece
)

// blocked is ZoneSystem.IsBlocked.
func (bs *blockers) blocked(x, z float64) bool {
	for i := range bs.list {
		if _, ok := bs.list[i].drop(x, rayTop, z, rayReach, blockMask); ok {
			return true
		}
	}
	return false
}

// staticSolidHeight is ZoneSystem.GetStaticSolidHeight: the highest
// static_solid surface under the ray, and its normal taken as straight up
// since the soups carry none.
func (bs *blockers) staticSolidHeight(x, z float64) (float64, bool) {
	best, hit := math.Inf(-1), false
	for i := range bs.list {
		if y, ok := bs.list[i].drop(x, rayTop, z, rayReach, 1<<layerStaticSolid); ok && y > best {
			best, hit = y, true
		}
	}
	return best, hit
}

// quat is a rotation as Unity holds it: x, y, z, w.
type quat [4]float64

var quatIdentity = quat{0, 0, 0, 1}

func (q quat) conj() quat { return quat{-q[0], -q[1], -q[2], q[3]} }

func (q quat) mul(r quat) quat {
	return quat{
		q[3]*r[0] + q[0]*r[3] + q[1]*r[2] - q[2]*r[1],
		q[3]*r[1] - q[0]*r[2] + q[1]*r[3] + q[2]*r[0],
		q[3]*r[2] + q[0]*r[1] - q[1]*r[0] + q[2]*r[3],
		q[3]*r[3] - q[0]*r[0] - q[1]*r[1] - q[2]*r[2],
	}
}

// rotate is q v q*.
func (q quat) rotate(v [3]float64) [3]float64 {
	x, y, z, w := q[0], q[1], q[2], q[3]
	// t = 2 cross(q.xyz, v); v' = v + w t + cross(q.xyz, t)
	tx := 2 * (y*v[2] - z*v[1])
	ty := 2 * (z*v[0] - x*v[2])
	tz := 2 * (x*v[1] - y*v[0])
	return [3]float64{
		v[0] + w*tx + (y*tz - z*ty),
		v[1] + w*ty + (z*tx - x*tz),
		v[2] + w*tz + (x*ty - y*tx),
	}
}

func axisAngle(ax [3]float64, deg float64) quat {
	half := deg * math.Pi / 360
	s := math.Sin(half)
	return quat{ax[0] * s, ax[1] * s, ax[2] * s, math.Cos(half)}
}

// euler is Quaternion.Euler: z about z, then x about x, then y about y.
func euler(x, y, z float64) quat {
	return axisAngle([3]float64{0, 1, 0}, y).mul(axisAngle([3]float64{1, 0, 0}, x)).mul(axisAngle([3]float64{0, 0, 1}, z))
}

// lookRotation is Quaternion.LookRotation: z along forward, y as near up
// as it can be.
func lookRotation(forward, up [3]float64) quat {
	f := norm3(forward)
	r := norm3(cross3(up, f))
	u := cross3(f, r)
	// The rotation matrix's columns are r, u, f; to a quaternion.
	m00, m01, m02 := r[0], u[0], f[0]
	m10, m11, m12 := r[1], u[1], f[1]
	m20, m21, m22 := r[2], u[2], f[2]
	tr := m00 + m11 + m22
	var q quat
	switch {
	case tr > 0:
		s := math.Sqrt(tr+1) * 2
		q = quat{(m21 - m12) / s, (m02 - m20) / s, (m10 - m01) / s, 0.25 * s}
	case m00 > m11 && m00 > m22:
		s := math.Sqrt(1+m00-m11-m22) * 2
		q = quat{0.25 * s, (m01 + m10) / s, (m02 + m20) / s, (m21 - m12) / s}
	case m11 > m22:
		s := math.Sqrt(1+m11-m00-m22) * 2
		q = quat{(m01 + m10) / s, 0.25 * s, (m12 + m21) / s, (m02 - m20) / s}
	default:
		s := math.Sqrt(1+m22-m00-m11) * 2
		q = quat{(m02 + m20) / s, (m12 + m21) / s, 0.25 * s, (m10 - m01) / s}
	}
	return q
}

func cross3(a, b [3]float64) [3]float64 {
	return [3]float64{a[1]*b[2] - a[2]*b[1], a[2]*b[0] - a[0]*b[2], a[0]*b[1] - a[1]*b[0]}
}

func norm3(v [3]float64) [3]float64 {
	l := math.Sqrt(v[0]*v[0] + v[1]*v[1] + v[2]*v[2])
	if l == 0 {
		return [3]float64{0, 0, 1}
	}
	return [3]float64{v[0] / l, v[1] / l, v[2] / l}
}
