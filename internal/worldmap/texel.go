package worldmap

import "github.com/pietjan/valheim-wiki/internal/worldgen"

// The per-texel arithmetic, exported for a renderer that keeps its own
// raster: the seed engine in cmd/world-wasm samples the same grid into
// typed arrays for the browser instead of into Renderer's gzip'd planes,
// and the numbers it stores have to be the ones Decode would read back.

// The height's encoding, (h + HeightOffset) * HeightScale into a uint16:
// 512 m below sea and 3500 m above at a sixteenth of a metre. The same two
// numbers Renderer writes into its header.
const (
	HeightOffset = heightOffset
	HeightScale  = heightScale
)

// EncodeHeight is a height in metres as the raster stores it, clamped to
// the uint16.
func EncodeHeight(h float64) uint16 { return encodeHeight(h) }

// MaskColor is Minimap.GetMaskColor for a texel: the biome and the biome
// height sampled there, and what the map shader reads from them besides
// colour and height. See maskColor for the rule.
func MaskColor(gen *worldgen.Generator, wx, wz, height float64, biome worldgen.Biome) (red, green, blue uint8) {
	return maskColor(gen, wx, wz, height, biome)
}

// TexelCentre is the world position a texel of a size-wide raster samples:
// the texel's centre over the MapRadius grid, row zero north, as Renderer
// samples it and as the browser reads it.
func TexelCentre(size, x, y int) (wx, wz float64) {
	n := float64(size)
	return (float64(x)+0.5)/n*2*MapRadius - MapRadius, MapRadius - (float64(y)+0.5)/n*2*MapRadius
}
