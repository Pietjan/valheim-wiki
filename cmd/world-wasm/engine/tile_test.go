package engine

import (
	"math"
	"testing"

	"github.com/pietjan/valheim-wiki/internal/worldmap"
)

// A tile needs no prepared world, so this runs in -short too.
func TestTile(t *testing.T) {
	w, err := New(testSeed, 0)
	if err != nil {
		t.Fatal(err)
	}
	const n = 32
	tile := w.Tile(2000, -1500, 800, n)
	if tile.N != n || len(tile.Heights) != n*n || len(tile.Biomes) != n*n || len(tile.Mask) != 4*n*n {
		t.Fatalf("tile shape %d %d %d %d", tile.N, len(tile.Heights), len(tile.Biomes), len(tile.Mask))
	}
	// Row zero is north, column zero west, samples at cell centres: the
	// corner sample is Point's answer there.
	cell := 800.0 / n
	for _, c := range []struct{ i, j int }{{0, 0}, {n - 1, n - 1}, {n / 2, 3}} {
		wx := 2000 - 400 + (float64(c.i)+0.5)*cell
		wz := -1500 + 400 - (float64(c.j)+0.5)*cell
		p := w.Point(wx, wz)
		k := c.j*n + c.i
		if !p.OK || math.Abs(float64(tile.Heights[k])-p.Height) > 1e-3 {
			t.Fatalf("sample %d,%d: tile %v, point %+v", c.i, c.j, tile.Heights[k], p)
		}
		if tile.Biomes[k] != worldmap.BiomeIndex(w.gen.GetBiome(wx, wz)) {
			t.Fatalf("sample %d,%d: biome %d", c.i, c.j, tile.Biomes[k])
		}
		if tile.Mask[4*k+3] != 255 {
			t.Fatalf("sample %d,%d: mask alpha %d", c.i, c.j, tile.Mask[4*k+3])
		}
	}
	// Past the edge: no biome, no mask, a height all the same.
	edge := w.Tile(10800, 0, 200, 4)
	for k := range edge.Biomes {
		if edge.Biomes[k] != 0 || edge.Mask[4*k+3] != 0 {
			t.Fatalf("beyond the world: biome %d alpha %d", edge.Biomes[k], edge.Mask[4*k+3])
		}
	}
	// Bounds hold.
	if big := w.Tile(0, 0, 100, 10000); big.N != MaxTileSide {
		t.Fatalf("side %d", big.N)
	}
	if small := w.Tile(0, 0, 100, 1); small.N != 2 {
		t.Fatalf("side %d", small.N)
	}
}
