// Package vmsh reads the prefab meshes the scene draws — meshes/*.bin,
// written by cmd/unity-meshes — so that the same geometry can be walked on.
//
// One file per prefab: the renderers' meshes at level of detail zero, in the
// prefab's own frame, with the triangles filed under the material that paints
// them, and from version 3 the parts of it the game drops onto the ground
// once the prefab is placed. The scene and the navmesh read the same bytes;
// there is no second table of what a thing is shaped like.
package vmsh

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sync"
)

// magic is "VMSH" little-endian, as the writer spells it.
const magic = 0x48534d56

// Run is one material's triangles: the indices from First, Count of them.
type Run struct {
	First, Count uint32
	Material     string
}

// Part is a run of vertices the game moves to the ground where the prefab
// stands: VertCount of them from VertFirst. Their SnapToGround transform
// sits at Anchor in the prefab's frame, and the game puts it at the ground's
// height under that point plus Offset once the prefab is placed, everything
// under it moving along. So a placed part is the prefab's geometry shifted
// in world y by (ground + Offset) - (the anchor's placed height); nothing
// else about it changes. See scene.Settle.
type Part struct {
	Anchor    [3]float32
	Offset    float32
	VertFirst uint32
	VertCount uint32
}

// Mesh is one prefab's geometry in its own frame.
type Mesh struct {
	// Verts are x, y, z triples in Unity's frame, metres.
	Verts []float32
	// Idx indexes Verts, three per triangle.
	Idx  []uint32
	Runs []Run
	// UV is a u, v pair per vertex from version 4, nil before.
	UV []float32
	// Parts are the snapped runs of Verts, in file order; each vertex is in
	// at most one.
	Parts []Part
}

// PartOf is which part a vertex belongs to, or -1 for the body of the mesh.
func (m *Mesh) PartOf(vertex uint32) int {
	for i, p := range m.Parts {
		if vertex >= p.VertFirst && vertex < p.VertFirst+p.VertCount {
			return i
		}
	}
	return -1
}

// Load reads one mesh file.
func Load(path string) (*Mesh, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(b) < 16 || binary.LittleEndian.Uint32(b) != magic {
		return nil, fmt.Errorf("vmsh: %s is not a mesh file", path)
	}
	version := binary.LittleEndian.Uint32(b[4:])
	if version < 2 || version > 4 {
		return nil, fmt.Errorf("vmsh: %s is version %d, want 2 to 4", path, version)
	}
	nv := int(binary.LittleEndian.Uint32(b[8:]))
	ni := int(binary.LittleEndian.Uint32(b[12:]))
	at := 16
	need := at + nv*12 + ni*4 + 4
	if len(b) < need {
		return nil, fmt.Errorf("vmsh: %s truncated", path)
	}
	m := &Mesh{Verts: make([]float32, nv*3), Idx: make([]uint32, ni)}
	for i := range m.Verts {
		m.Verts[i] = float32frombits(binary.LittleEndian.Uint32(b[at:]))
		at += 4
	}
	for i := range m.Idx {
		m.Idx[i] = binary.LittleEndian.Uint32(b[at:])
		at += 4
	}
	nr := int(binary.LittleEndian.Uint32(b[at:]))
	at += 4
	for i := 0; i < nr; i++ {
		if len(b) < at+12 {
			return nil, fmt.Errorf("vmsh: %s truncated in run %d", path, i)
		}
		r := Run{First: binary.LittleEndian.Uint32(b[at:]), Count: binary.LittleEndian.Uint32(b[at+4:])}
		n := int(binary.LittleEndian.Uint32(b[at+8:]))
		at += 12
		if len(b) < at+n {
			return nil, fmt.Errorf("vmsh: %s truncated in run %d's name", path, i)
		}
		r.Material = string(b[at : at+n])
		at += n
		if int(r.First)+int(r.Count) > ni {
			return nil, fmt.Errorf("vmsh: %s run %d overruns the indices", path, i)
		}
		m.Runs = append(m.Runs, r)
	}
	if version < 3 {
		return m, nil
	}
	if len(b) < at+4 {
		return nil, fmt.Errorf("vmsh: %s truncated before its parts", path)
	}
	np := int(binary.LittleEndian.Uint32(b[at:]))
	at += 4
	for i := 0; i < np; i++ {
		if len(b) < at+24 {
			return nil, fmt.Errorf("vmsh: %s truncated in part %d", path, i)
		}
		p := Part{
			Anchor: [3]float32{
				float32frombits(binary.LittleEndian.Uint32(b[at:])),
				float32frombits(binary.LittleEndian.Uint32(b[at+4:])),
				float32frombits(binary.LittleEndian.Uint32(b[at+8:])),
			},
			Offset:    float32frombits(binary.LittleEndian.Uint32(b[at+12:])),
			VertFirst: binary.LittleEndian.Uint32(b[at+16:]),
			VertCount: binary.LittleEndian.Uint32(b[at+20:]),
		}
		at += 24
		if int(p.VertFirst)+int(p.VertCount) > nv {
			return nil, fmt.Errorf("vmsh: %s part %d overruns the vertices", path, i)
		}
		m.Parts = append(m.Parts, p)
	}
	// Version 4: a u, v pair per vertex after the parts.
	if version >= 4 && len(b) >= at+nv*8 {
		m.UV = make([]float32, nv*2)
		for i := range m.UV {
			m.UV[i] = float32frombits(binary.LittleEndian.Uint32(b[at:]))
			at += 4
		}
	}
	return m, nil
}

// Library is a directory of mesh files, read on demand and kept.
type Library struct {
	dir string

	mu    sync.Mutex
	cache map[string]*Mesh
	miss  map[string]bool
}

// NewLibrary opens a directory of mesh files; nothing is read until asked.
func NewLibrary(dir string) *Library {
	return &Library{dir: dir, cache: map[string]*Mesh{}, miss: map[string]bool{}}
}

// Has reports whether the library holds a mesh of that name.
func (l *Library) Has(name string) bool {
	if l == nil {
		return false
	}
	_, ok := l.Get(name)
	return ok
}

// HasEffect reports whether the library holds particle systems for a
// prefab, fx/<name>.json: what valheim-npc's extractor writes with -fx.
func (l *Library) HasEffect(name string) bool {
	if l == nil {
		return false
	}
	_, err := os.Stat(filepath.Join(l.dir, "fx", name+".json"))
	return err == nil
}

// Get returns the named prefab's mesh, or false when there is no file for
// it. A file that fails to read is treated as absent and not retried.
func (l *Library) Get(name string) (*Mesh, bool) {
	if l == nil {
		return nil, false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if m, ok := l.cache[name]; ok {
		return m, true
	}
	if l.miss[name] {
		return nil, false
	}
	m, err := Load(filepath.Join(l.dir, name+".bin"))
	if err != nil {
		l.miss[name] = true
		return nil, false
	}
	l.cache[name] = m
	return m, true
}

// Parts is the named prefab's snapped parts, or nil when it has none or
// there is no file for it.
func (l *Library) Parts(name string) []Part {
	m, ok := l.Get(name)
	if !ok {
		return nil
	}
	return m.Parts
}

// The scene paints a material by its name, and greenery is the one class of
// material nothing walks into: a canopy is seen through, and walked under.
// These two patterns are the scene's own (MATERIAL_TONES in scene.js), and
// TestFoliageRuleIsTheScenes holds them to it — a burnt Ashlands tree is bare
// wood and stays solid, exactly as the scene paints it.
var (
	burnt    = regexp.MustCompile(`(?i)ashlandstree|ashlandsbranch|ashlandslog|charred|ember|burnt`)
	greenery = regexp.MustCompile(`(?i)leaf|leaves|needle|foliage|branch|shrub|bush|fern|grass|vine|ivy|moss`)
)

// Foliage reports whether a material is greenery: drawn see-through by the
// scene, and not solid to a walker.
func Foliage(material string) bool {
	return !burnt.MatchString(material) && greenery.MatchString(material)
}
