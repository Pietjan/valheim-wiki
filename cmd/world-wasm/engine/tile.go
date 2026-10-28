package engine

import (
	"math"

	"github.com/pietjan/valheim-wiki/internal/worldmap"
)

// Tile is a close-up of the ground: N samples a side over Span metres
// centred on CX, CZ, row zero north as the map raster has it. Heights are
// metres as Generator.Height blends them, the ground the game builds;
// Biomes and Mask are the map raster's encodings at the same points, so
// the 3D view paints a tile with the rules it paints the map by.
type Tile struct {
	CX, CZ  float64
	Span    float64
	N       int
	Heights []float32
	Biomes  []uint8
	Mask    []uint8 // RGBA
}

// MaxTileSide bounds a tile: 512² is a quarter of a million height
// samples, a second or two in the browser.
const MaxTileSide = 512

// Tile samples the generator on a grid, for the 3D view's close-ups.
// Past the world's edge the height is the generator's open sea and the
// biome is none, as the map raster has it. Once the world is placed, the
// sites' terrain modifiers reshape the tile as they do the game's ground.
func (w *World) Tile(cx, cz, span float64, n int) *Tile {
	if n < 2 {
		n = 2
	}
	if n > MaxTileSide {
		n = MaxTileSide
	}
	if !(span > 0) {
		span = 1
	}
	t := &Tile{CX: cx, CZ: cz, Span: span, N: n, Heights: make([]float32, n*n), Biomes: make([]uint8, n*n), Mask: make([]uint8, 4*n*n)}
	cell := span / float64(n)
	for j := 0; j < n; j++ {
		wz := cz + span/2 - (float64(j)+0.5)*cell
		for i := 0; i < n; i++ {
			wx := cx - span/2 + (float64(i)+0.5)*cell
			k := j*n + i
			b := w.gen.GetBiome(wx, wz)
			h, ok := w.gen.Height(wx, wz)
			if !ok {
				h, _ = w.gen.BiomeHeight(b, wx, wz)
			}
			t.Heights[k] = float32(h)
			if math.Hypot(wx, wz) > worldmap.WorldRadius {
				continue
			}
			t.Biomes[k] = worldmap.BiomeIndex(b)
			r, g, bl := worldmap.MaskColor(w.gen, wx, wz, h, b)
			t.Mask[4*k], t.Mask[4*k+1], t.Mask[4*k+2], t.Mask[4*k+3] = r, g, bl, 255
		}
	}
	if sc := w.Scene(); sc != nil {
		sc.ApplyModifiers(t.Heights, n, cx-span/2+cell/2, cz+span/2-cell/2, cell, true)
	}
	return t
}
