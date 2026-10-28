package render

import (
	"math"

	"github.com/pietjan/valheim-wiki/internal/vmsh"
)

// interiorY is where the game parks a dungeon's interior: 5000 m up. A
// location prefab's exterior never reaches a tenth of that.
const interiorY = 500

// Transform places a mesh in the scene: scale, then rotate, then move,
// as a Unity Transform composes.
type Transform struct {
	Pos   [3]float64
	Rot   [4]float64 // quaternion x, y, z, w
	Scale [3]float64
}

// Identity leaves a mesh where it is.
var Identity = Transform{Rot: [4]float64{0, 0, 0, 1}, Scale: [3]float64{1, 1, 1}}

func (t Transform) apply(p [3]float64) [3]float64 {
	p = [3]float64{p[0] * t.Scale[0], p[1] * t.Scale[1], p[2] * t.Scale[2]}
	return add3(t.Pos, rotate(t.Rot, p))
}

// Child is the transform of c placed under t, as a child Transform
// composes with its parent.
func (t Transform) Child(c Transform) Transform {
	return Transform{
		Pos:   t.apply(c.Pos),
		Rot:   Mul(t.Rot, c.Rot),
		Scale: [3]float64{t.Scale[0] * c.Scale[0], t.Scale[1] * c.Scale[1], t.Scale[2] * c.Scale[2]},
	}
}

// run is one material's triangles in the scene's index list.
type run struct {
	first, count int
	mat          *Material
}

// Scene is the geometry one icon draws: every kept triangle of every mesh
// added to it, in the Unity frame, with the smooth normals the files lack.
type Scene struct {
	pos  []float32 // x, y, z per vertex
	uv   []float32 // u, v per vertex
	nrm  []float32 // unit normal per vertex, zero where no face touches it
	idx  []uint32
	runs []run
	// Floor is the ground level, metres: what lies under it is buried
	// and not drawn. Zero unless the location digs.
	Floor float64
	// Omit names materials Add leaves out of this scene on top of what
	// the manifest skips: the crows circling a ruin, say.
	Omit map[string]bool
	// Translucent keeps the see-through flat colours, drawn opaque: a
	// creature made of one (the frost blob) is nothing without it, while
	// a building reads better without its windows and blocker walls.
	Translucent bool

	// Min and Max bound the vertices of the kept triangles that stand
	// above Floor, after Finish.
	Min, Max [3]float64
	tris     int
}

// Triangles is how many the scene keeps.
func (s *Scene) Triangles() int { return s.tris }

// Add appends a mesh under a transform. Parts the game drops onto the
// ground are settled to y=0 first (the flat ground under the prefab's
// origin), triangles that reach the parked interior are left out, and so
// are materials the game does not draw as a surface.
func (s *Scene) Add(m *vmsh.Mesh, mats *Manifest, xf Transform) {
	base := uint32(len(s.pos) / 3)
	nv := len(m.Verts) / 3
	// A snapped part's anchor goes to ground + Offset: everything in it
	// shifts in y by Offset - anchor.y, see vmsh.Part.
	shift := make([]float32, nv)
	for _, p := range m.Parts {
		dy := p.Offset - p.Anchor[1]
		for v := p.VertFirst; v < p.VertFirst+p.VertCount; v++ {
			shift[v] = dy
		}
	}
	for v := 0; v < nv; v++ {
		p := xf.apply([3]float64{float64(m.Verts[v*3]), float64(m.Verts[v*3+1] + shift[v]), float64(m.Verts[v*3+2])})
		s.pos = append(s.pos, float32(p[0]), float32(p[1]), float32(p[2]))
		if m.UV != nil {
			s.uv = append(s.uv, m.UV[v*2], m.UV[v*2+1])
		} else {
			s.uv = append(s.uv, 0, 0)
		}
	}
	for _, r := range m.Runs {
		mat := mats.Material(r.Material)
		if mat.Skip || s.Omit[r.Material] || (mat.Translucent && !s.Translucent) {
			continue
		}
		first := len(s.idx)
		end := int(r.First + r.Count)
		for i := int(r.First); i+2 < end; i += 3 {
			a, b, c := m.Idx[i], m.Idx[i+1], m.Idx[i+2]
			if m.Verts[a*3+1] > interiorY || m.Verts[b*3+1] > interiorY || m.Verts[c*3+1] > interiorY {
				continue
			}
			s.idx = append(s.idx, base+a, base+b, base+c)
		}
		if n := len(s.idx) - first; n > 0 {
			s.runs = append(s.runs, run{first: first, count: n, mat: mat})
		}
	}
}

// Finish computes the normals and the bounds once everything is added.
// Normals are the area-weighted sum of the face normals over shared
// indices, as three's computeVertexNormals does for the scene; Unity's
// clockwise-front winding makes cross(b-a, c-a) the outward normal in its
// left-handed frame.
func (s *Scene) Finish() {
	s.nrm = make([]float32, len(s.pos))
	s.tris = len(s.idx) / 3
	for i := 0; i+2 < len(s.idx); i += 3 {
		a, b, c := s.idx[i], s.idx[i+1], s.idx[i+2]
		n := s.faceNormal(a, b, c)
		for _, v := range [3]uint32{a, b, c} {
			s.nrm[v*3] += float32(n[0])
			s.nrm[v*3+1] += float32(n[1])
			s.nrm[v*3+2] += float32(n[2])
		}
	}
	for v := 0; v+2 < len(s.nrm); v += 3 {
		l := math.Sqrt(float64(s.nrm[v]*s.nrm[v] + s.nrm[v+1]*s.nrm[v+1] + s.nrm[v+2]*s.nrm[v+2]))
		if l > 0 {
			s.nrm[v] /= float32(l)
			s.nrm[v+1] /= float32(l)
			s.nrm[v+2] /= float32(l)
		}
	}
	s.Min = [3]float64{math.Inf(1), math.Inf(1), math.Inf(1)}
	s.Max = [3]float64{math.Inf(-1), math.Inf(-1), math.Inf(-1)}
	for _, v := range s.idx {
		p := s.vertex(v)
		if p[1] < s.Floor {
			continue
		}
		for k := range p {
			s.Min[k] = math.Min(s.Min[k], p[k])
			s.Max[k] = math.Max(s.Max[k], p[k])
		}
	}
}

// faceNormal is the unnormalised (area-weighted) outward normal.
func (s *Scene) faceNormal(a, b, c uint32) [3]float64 {
	pa, pb, pc := s.vertex(a), s.vertex(b), s.vertex(c)
	return cross(sub3(pb, pa), sub3(pc, pa))
}

func (s *Scene) vertex(v uint32) [3]float64 {
	return [3]float64{float64(s.pos[v*3]), float64(s.pos[v*3+1]), float64(s.pos[v*3+2])}
}

// eachVertex calls fn for every vertex a kept triangle uses, once per use.
func (s *Scene) eachVertex(fn func(p [3]float64)) {
	for _, v := range s.idx {
		fn(s.vertex(v))
	}
}

func add3(a, b [3]float64) [3]float64 { return [3]float64{a[0] + b[0], a[1] + b[1], a[2] + b[2]} }
func sub3(a, b [3]float64) [3]float64 { return [3]float64{a[0] - b[0], a[1] - b[1], a[2] - b[2]} }
func scale3(a [3]float64, k float64) [3]float64 {
	return [3]float64{a[0] * k, a[1] * k, a[2] * k}
}
func dot(a, b [3]float64) float64 { return a[0]*b[0] + a[1]*b[1] + a[2]*b[2] }
func cross(a, b [3]float64) [3]float64 {
	return [3]float64{a[1]*b[2] - a[2]*b[1], a[2]*b[0] - a[0]*b[2], a[0]*b[1] - a[1]*b[0]}
}
func norm3(a [3]float64) [3]float64 {
	l := math.Sqrt(dot(a, a))
	if l == 0 {
		return a
	}
	return scale3(a, 1/l)
}

// rotate is q v q*: the vector under the quaternion.
func rotate(q [4]float64, v [3]float64) [3]float64 {
	x, y, z, w := q[0], q[1], q[2], q[3]
	tx := 2 * (y*v[2] - z*v[1])
	ty := 2 * (z*v[0] - x*v[2])
	tz := 2 * (x*v[1] - y*v[0])
	return [3]float64{
		v[0] + w*tx + (y*tz - z*ty),
		v[1] + w*ty + (z*tx - x*tz),
		v[2] + w*tz + (x*ty - y*tx),
	}
}

// Mul composes two rotations: a then b, as Unity's a * b applies b first.
func Mul(a, b [4]float64) [4]float64 {
	return [4]float64{
		a[3]*b[0] + a[0]*b[3] + a[1]*b[2] - a[2]*b[1],
		a[3]*b[1] - a[0]*b[2] + a[1]*b[3] + a[2]*b[0],
		a[3]*b[2] + a[0]*b[1] - a[1]*b[0] + a[2]*b[3],
		a[3]*b[3] - a[0]*b[0] - a[1]*b[1] - a[2]*b[2],
	}
}
