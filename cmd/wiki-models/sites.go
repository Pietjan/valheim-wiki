package main

import (
	"encoding/binary"
	"math"

	"github.com/pietjan/valheim-wiki/internal/vmsh"
)

// A whole-site model as extracted is every plank of every piece: a camp
// is tens of thousands of triangles, a fortress millions. For the web
// they are packed as VMSQ — positions quantised to sixteen bits over the
// model's bounds, texture coordinates to sixteen bits, indices to sixteen
// where they fit — after merging the vertices that coincide, and, where
// that is still over the cap, snapping vertices to a coarser grid until
// it is under. A site that will not come under the cap is left out, and
// the view stands nothing there but its marker.
//
//	"VMSQ" version(1) nv ni nr
//	min[3] float32, scale[3] float32       position = min + q / 65535 * scale
//	nv × 3 uint16                           positions
//	nv × 2 int16                            uv × 2048
//	ni × uint16 (nv ≤ 65535) or uint32      indices
//	nr × { first uint32, count uint32, len uint32, name }

const magicVMSQ = 0x51534d56

// siteCap is the most bytes a packed site may take.
const siteCap = 1_800_000

// clusterCells are the grids tried, finest first, when a site is over
// the cap as it is.
var clusterCells = []float64{0.25, 0.5, 1.0}

// packSite packs a site mesh, or returns nil when it cannot come under
// the cap.
func packSite(m *vmsh.Mesh) []byte {
	for _, cell := range append([]float64{0.005}, clusterCells...) {
		c := cluster(m, cell)
		if b := encodeVMSQ(c); len(b) <= siteCap {
			return b
		}
	}
	return nil
}

// cluster merges the vertices of a run that fall in one cell of the grid
// and one 1/16 cell of texture space, averaging their positions, and
// drops the triangles that collapse. A vertex shared by runs is split,
// since the runs paint it differently.
func cluster(m *vmsh.Mesh, cell float64) *vmsh.Mesh {
	type key struct {
		x, y, z int32
		run     int
		u, v    int16
	}
	ids := map[key]int{}
	var sum [][5]float64
	var count []int
	out := &vmsh.Mesh{}
	for ri, r := range m.Runs {
		first := uint32(len(out.Idx))
		for t := int(r.First) / 3; t < int(r.First+r.Count)/3; t++ {
			var tri [3]int
			for k := 0; k < 3; k++ {
				vi := int(m.Idx[3*t+k])
				x, y, z := float64(m.Verts[3*vi]), float64(m.Verts[3*vi+1]), float64(m.Verts[3*vi+2])
				var u, v float64
				if m.UV != nil {
					u, v = float64(m.UV[2*vi]), float64(m.UV[2*vi+1])
				}
				kk := key{int32(math.Floor(x / cell)), int32(math.Floor(y / cell)), int32(math.Floor(z / cell)), ri, int16(math.Floor(u * 16)), int16(math.Floor(v * 16))}
				id, ok := ids[kk]
				if !ok {
					id = len(sum)
					ids[kk] = id
					sum = append(sum, [5]float64{})
					count = append(count, 0)
				}
				s := &sum[id]
				s[0] += x
				s[1] += y
				s[2] += z
				s[3] += u
				s[4] += v
				count[id]++
				tri[k] = id
			}
			if tri[0] == tri[1] || tri[1] == tri[2] || tri[0] == tri[2] {
				continue
			}
			out.Idx = append(out.Idx, uint32(tri[0]), uint32(tri[1]), uint32(tri[2]))
		}
		if n := uint32(len(out.Idx)) - first; n > 0 {
			out.Runs = append(out.Runs, vmsh.Run{First: first, Count: n, Material: m.Runs[ri].Material})
		}
	}
	out.Verts = make([]float32, 3*len(sum))
	out.UV = make([]float32, 2*len(sum))
	for i, s := range sum {
		n := float64(count[i])
		out.Verts[3*i], out.Verts[3*i+1], out.Verts[3*i+2] = float32(s[0]/n), float32(s[1]/n), float32(s[2]/n)
		out.UV[2*i], out.UV[2*i+1] = float32(s[3]/n), float32(s[4]/n)
	}
	return out
}

func encodeVMSQ(m *vmsh.Mesh) []byte {
	nv := len(m.Verts) / 3
	var min, max [3]float32
	for k := 0; k < 3; k++ {
		min[k], max[k] = math.MaxFloat32, -math.MaxFloat32
	}
	for i := 0; i < nv; i++ {
		for k := 0; k < 3; k++ {
			v := m.Verts[3*i+k]
			if v < min[k] {
				min[k] = v
			}
			if v > max[k] {
				max[k] = v
			}
		}
	}
	var scale [3]float32
	for k := 0; k < 3; k++ {
		scale[k] = max[k] - min[k]
		if scale[k] <= 0 {
			scale[k] = 1
		}
	}
	wide := nv > 65535
	idxBytes := 2
	if wide {
		idxBytes = 4
	}
	size := 20 + 24 + nv*6 + nv*4 + len(m.Idx)*idxBytes
	for _, r := range m.Runs {
		size += 12 + len(r.Material)
	}
	b := make([]byte, 0, size)
	put32 := func(v uint32) { b = binary.LittleEndian.AppendUint32(b, v) }
	putf := func(v float32) { put32(math.Float32bits(v)) }
	put32(magicVMSQ)
	put32(1)
	put32(uint32(nv))
	put32(uint32(len(m.Idx)))
	put32(uint32(len(m.Runs)))
	for k := 0; k < 3; k++ {
		putf(min[k])
	}
	for k := 0; k < 3; k++ {
		putf(scale[k])
	}
	for i := 0; i < nv; i++ {
		for k := 0; k < 3; k++ {
			q := math.Round(float64(m.Verts[3*i+k]-min[k]) / float64(scale[k]) * 65535)
			b = binary.LittleEndian.AppendUint16(b, uint16(math.Max(0, math.Min(65535, q))))
		}
	}
	for i := 0; i < nv; i++ {
		for k := 0; k < 2; k++ {
			var u float64
			if m.UV != nil {
				u = float64(m.UV[2*i+k])
			}
			q := math.Round(u * 2048)
			b = binary.LittleEndian.AppendUint16(b, uint16(int16(math.Max(-32768, math.Min(32767, q)))))
		}
	}
	for _, i := range m.Idx {
		if wide {
			put32(i)
		} else {
			b = binary.LittleEndian.AppendUint16(b, uint16(i))
		}
	}
	for _, r := range m.Runs {
		put32(r.First)
		put32(r.Count)
		put32(uint32(len(r.Material)))
		b = append(b, r.Material...)
	}
	return b
}
