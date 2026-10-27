// Package worldmap samples the world into a map, from the seed.
//
// This is the same trick the terrain work rests on: the world is a pure
// function of its seed, so the whole map can be computed without visiting any
// of it. The in-game minimap reveals itself as you explore; this one is
// complete from the first frame, because nothing here is exploration — it is
// arithmetic over internal/worldgen.
//
// What comes out is data rather than a picture, and it is the data the game's
// own map is made of. Minimap.GenerateWorldMap walks a 2048-texel grid at
// twelve metres and writes three textures: the biome's colour, the biome's
// height from GetBiomeHeight, and GetMaskColor's mask — forest in red, mist
// in green, lava and the Ashlands sea in blue. The map shader then paints
// trees, water, snow and clouds over those per screen pixel, which is why the
// in-game map is detailed at every zoom while its data is coarse. This
// package samples the same grid with the same functions; the painting happens
// in the browser, see cmd/npc-ui/static/worldmap.js, with the game's own
// textures when they have been extracted and a stand-in look when not.
//
// The palette is the one thing the server side cannot measure: it lives in
// the client's serialised Minimap component, which a dedicated server does
// not have. The values here are chosen to read well; cmd/unity-minimap reads
// the real ones out of the client's bundles and the panel takes those when
// they are present.
package worldmap

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/binary"
	"fmt"
	"image/color"
	"io"
	"math"
	"math/bits"
	"runtime"
	"sync"

	"github.com/pietjan/valheim-wiki/internal/worldgen"
)

// WorldRadius is where the world ends: beyond 10500 the generator reports a
// fixed depth and the game draws stars.
const WorldRadius = 10500.0

// MapRadius is how far the grid reaches: half of Minimap.m_textureSize times
// m_pixelSize, 2048 texels of twelve metres, which is the game's own map
// texture. Wider than the world on purpose, so the world is a disc with the
// void round it and the map shader's arithmetic, which is written against
// that texture, carries over unchanged.
const MapRadius = 12288.0

// Palette is one colour per biome: the base the browser paints from before
// shading, trees and snow go over it.
//
// Chosen for legibility on a small map: the biomes a player cares about telling
// apart at a glance are meadows against black forest, and swamp against both.
var Palette = map[worldgen.Biome]color.RGBA{
	worldgen.BiomeMeadows:     {124, 158, 74, 255},
	worldgen.BiomeBlackForest: {59, 82, 51, 255},
	worldgen.BiomeSwamp:       {94, 84, 61, 255},
	worldgen.BiomeMountain:    {226, 231, 236, 255},
	worldgen.BiomePlains:      {186, 168, 96, 255},
	worldgen.BiomeMistlands:   {74, 70, 88, 255},
	worldgen.BiomeDeepNorth:   {198, 214, 226, 255},
	worldgen.BiomeAshLands:    {130, 62, 47, 255},
	worldgen.BiomeOcean:       {46, 79, 122, 255},
	worldgen.BiomeNone:        {20, 22, 26, 255},
}

// The raster's encoding. All of it is written into the header, so a reader
// takes the numbers from the bytes rather than from here; these are what the
// writer uses.
const (
	magic = "VMAP"

	// version changes with the layout. Two: the mask's three planes replaced
	// a forest plane, and a map kept under the old layout would decode to
	// the wrong length.
	version = 2

	// heightOffset and heightScale turn a height in metres into the uint16
	// that is stored: (h + offset) * scale. 512m below sea and 3500m above
	// at a sixteenth of a metre, which is finer than a slope on a 10m grid
	// can use and coarse enough to compress.
	heightOffset = 512.0
	heightScale  = 16.0

	// paletteSize is how many biome indices the header carries colours for.
	// The biomes are bit flags and the index is the bit's position plus one,
	// so Mistlands (512) is the eleventh entry.
	paletteSize = 16

	headerSize = 32 + 3*paletteSize
)

// Renderer samples one world at one size. Safe for concurrent use; the raster
// is computed once and reused.
type Renderer struct {
	gen    *worldgen.Generator
	size   int
	radius float64

	once       sync.Once
	data       []byte
	err        error
	remembered bool

	// The alt-biome sectors, built apart from the raster and later: a page
	// asks for them when it first draws, see sectors.go.
	secOnce sync.Once
	secDone chan struct{}
	sec     *Sectors
}

// DefaultSize is what a Renderer samples at when it is not told otherwise:
// the game's own 2048, which with MapRadius makes a sample every twelve
// metres at the same points Minimap.GenerateWorldMap samples. It is not the
// resolution of the picture — the browser paints trees and coastlines at
// screen resolution from these samples, and a finer grid sharpens only the
// shape of a hill — so the only ceiling is what it costs to sample and to
// send.
//
// Measured on a twelve-core machine, sampling the whole grid:
//
//	1024px   0.5s   1.5 MB   24 m/sample
//	2048px   1.6s   4.7 MB   12 m/sample
//	4096px   5.6s  14.5 MB    6 m/sample
//
// Quadratic in the size, since every sample is an independent height query.
// The time is paid once at startup, before the UI serves anything, and the
// bytes are held for the life of the process and sent to each browser once.
const DefaultSize = 2048

// New returns a Renderer for a world, sampling size by size points.
func New(gen *worldgen.Generator, size int) *Renderer {
	if size <= 0 {
		size = DefaultSize
	}
	return &Renderer{gen: gen, size: size, radius: MapRadius, secDone: make(chan struct{})}
}

// Restored is a Renderer whose raster has already been sampled, handed back
// the bytes rather than the work.
//
// The map is a pure function of the seed and the size — the same two numbers
// give the same million samples every time — so redrawing it on every start
// is two seconds spent proving that. A caller that kept the bytes from last
// time passes them here and Data returns them without rendering.
//
// The bytes are checked for being a whole map of this size in this format
// and nothing more: nothing here can tell one seed's map from another's. See
// cmd/npc-ui, which keys them on the seed and the size together. Whole means
// inflated and counted, a tenth of a second, because a header that passed
// with a body that did not is a browser that says "wrong length" and draws
// nothing.
func Restored(gen *worldgen.Generator, size int, data []byte) (*Renderer, error) {
	r := New(gen, size)
	h, n, err := readHeaderAndLength(data)
	if err != nil {
		return nil, err
	}
	if h.size != r.size {
		return nil, fmt.Errorf("worldmap: kept map is %dpx, want %d", h.size, r.size)
	}
	if want := headerSize + 6*h.size*h.size; n != want {
		return nil, fmt.Errorf("worldmap: kept map is %d bytes, want %d", n, want)
	}
	r.data = data
	r.remembered = true
	r.once.Do(func() {})
	return r, nil
}

// Remembered reports whether the raster came from Restored rather than from
// sampling, so a caller that keeps maps knows this one is already kept.
func (r *Renderer) Remembered() bool { return r.remembered }

// Size is the raster's width and height in samples.
func (r *Renderer) Size() int { return r.size }

// Project maps a world position to a fraction of the image, x rightwards and
// z downwards, so (0, 0) is the top-left corner and (1, 1) the bottom-right.
// Fractions rather than pixels because the map is displayed at whatever size
// the page gives it.
func (r *Renderer) Project(x, z float64) (fx, fz float64) {
	return (x + r.radius) / (2 * r.radius), (r.radius - z) / (2 * r.radius)
}

// InWorld reports whether a position is inside the world circle.
func (r *Renderer) InWorld(x, z float64) bool {
	return math.Hypot(x, z) <= WorldRadius
}

// Data samples the map and encodes it, once. Later calls return the same
// bytes.
//
// The bytes are the raster gzip-compressed, which is how they are stored and
// how they go over the wire: twenty-five megabytes of grid at 2048 is a few
// once deflated, and a browser inflates it for free. Decode reads them back.
func (r *Renderer) Data(ctx context.Context) ([]byte, error) {
	r.once.Do(func() {
		raster := r.render(ctx)
		if ctx.Err() != nil {
			r.err = ctx.Err()
			return
		}
		var buf bytes.Buffer
		w, _ := gzip.NewWriterLevel(&buf, gzip.DefaultCompression)
		if _, err := w.Write(raster); err != nil {
			r.err = err
			return
		}
		if err := w.Close(); err != nil {
			r.err = err
			return
		}
		r.data = buf.Bytes()
	})
	return r.data, r.err
}

// render samples the map across every core, into the raster's byte layout.
//
// A million samples at roughly a microsecond of terrain each is a second of
// work single-threaded, which is worth splitting; the generator is safe for
// concurrent use and rows share nothing.
func (r *Renderer) render(ctx context.Context) []byte {
	n := r.size * r.size
	out := make([]byte, headerSize+6*n)
	r.writeHeader(out)
	height := make([]uint16, n)
	biome := out[headerSize+2*n : headerSize+3*n]
	mask := [3][]byte{
		out[headerSize+3*n : headerSize+4*n],
		out[headerSize+4*n : headerSize+5*n],
		out[headerSize+5*n:],
	}

	rows := make(chan int, r.size)
	for y := 0; y < r.size; y++ {
		rows <- y
	}
	close(rows)

	var wg sync.WaitGroup
	for i := 0; i < runtime.NumCPU(); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for y := range rows {
				if ctx.Err() != nil {
					return
				}
				r.row(height, biome, mask, y)
			}
		}()
	}
	wg.Wait()

	// Deltas along the row rather than values, for the compressor's sake:
	// neighbouring cells are within a metre of each other nearly everywhere,
	// so the differences are small numbers that repeat, where the values are
	// not. A third off the bytes, measured, and a prefix sum to undo. The
	// height's two bytes go in separate planes for the same reason — the low
	// byte is the noisy one and keeping the quiet high bytes together lets
	// them pack. The mask planes get the same treatment; two of them are
	// smooth and the third is runs of one value either way.
	lo, hi := out[headerSize:headerSize+n], out[headerSize+n:headerSize+2*n]
	for y := 0; y < r.size; y++ {
		var prev uint16
		for x := 0; x < r.size; x++ {
			i := y*r.size + x
			d := height[i] - prev
			prev = height[i]
			lo[i], hi[i] = byte(d), byte(d>>8)
		}
		for _, plane := range mask {
			var last byte
			for x := 0; x < r.size; x++ {
				i := y*r.size + x
				plane[i], last = plane[i]-last, plane[i]
			}
		}
	}

	return out
}

// row samples one row the way Minimap.GenerateWorldMap does: at the texel's
// centre, the biome there, GetBiomeHeight for that biome, and GetMaskColor
// from the two.
func (r *Renderer) row(height []uint16, biome []byte, mask [3][]byte, y int) {
	// Row zero is the north edge; the game's is the south, and the browser
	// reads it that way round.
	wz := r.radius - (float64(y)+0.5)/float64(r.size)*2*r.radius

	for x := 0; x < r.size; x++ {
		wx := (float64(x)+0.5)/float64(r.size)*2*r.radius - r.radius
		i := y*r.size + x

		b := r.gen.GetBiome(wx, wz)
		h, _ := r.gen.BiomeHeight(b, wx, wz)
		height[i] = encodeHeight(h)
		if math.Hypot(wx, wz) > WorldRadius {
			// Beyond the edge: the game's height is a fixed depth and its
			// biome is whatever GetBiome says for open sea, and the map
			// shader draws stars there regardless. No biome, so a painter
			// without the game's textures has a void to draw.
			continue
		}
		biome[i] = BiomeIndex(b)
		red, green, blue := maskColor(r.gen, wx, wz, h, b)
		mask[0][i], mask[1][i], mask[2][i] = red, green, blue
	}
}

// maskColor is Minimap.GetMaskColor: what the map shader reads per texel
// besides colour and height. Red is forest — the game's InForest for the
// meadows, a forest factor under 0.8 for the plains, everywhere in the black
// forest. Green is the mistlands' mist, fading in as the forest factor drops
// through 1.3 to 1.1. Blue is the Ashlands: the lava mask on land, and at
// sea how far past the biome's line the water is, which colours it.
func maskColor(gen *worldgen.Generator, wx, wz, height float64, biome worldgen.Biome) (red, green, blue uint8) {
	if height < worldgen.WaterLevel {
		return 0, 0, toByte(clamp01(worldgen.AshlandsOceanGradient(wx, wz)))
	}
	forest := func() float64 {
		return float64(worldgen.ForestFactor(worldgen.Vec3{X: float32(wx), Z: float32(wz)}))
	}
	switch biome {
	case worldgen.BiomeMeadows:
		if forest() < 1.15 {
			return 255, 0, 0
		}
	case worldgen.BiomePlains:
		if forest() < 0.8 {
			return 255, 0, 0
		}
	case worldgen.BiomeBlackForest:
		return 255, 0, 0
	case worldgen.BiomeMistlands:
		// Utils.SmoothStep: the Hermite step between the two.
		t := clamp01((forest() - 1.1) / (1.3 - 1.1))
		return 0, toByte(1 - t*t*(3-2*t)), 0
	case worldgen.BiomeAshLands:
		return 0, 0, toByte(float64(gen.AshlandsMask(wx, wz)))
	}
	return 0, 0, 0
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func toByte(v float64) uint8 {
	return uint8(math.Round(clamp01(v) * 255))
}

// BiomeIndex is the byte a biome is stored as: its flag's bit position plus
// one, and zero for no biome at all. Every biome the generator can name fits
// under paletteSize.
func BiomeIndex(b worldgen.Biome) uint8 {
	if b == 0 {
		return 0
	}
	return uint8(bits.TrailingZeros(uint(b)) + 1)
}

func encodeHeight(h float64) uint16 {
	v := math.Round((h + heightOffset) * heightScale)
	if v < 0 {
		return 0
	}
	if v > math.MaxUint16 {
		return math.MaxUint16
	}
	return uint16(v)
}

// header is what the front of the raster says about the rest of it.
type header struct {
	size         int
	radius       float64
	water        float64
	heightOffset float64
	heightScale  float64
	palette      [paletteSize]color.RGBA
}

// The layout, all little-endian:
//
//	0   4  "VMAP"
//	4   1  version
//	5   1  palette entries
//	6   2  reserved
//	8   4  size, samples per side
//	12  4  grid radius, float32 metres (MapRadius)
//	16  4  water level, float32 metres
//	20  4  height offset, float32
//	24  4  height scale, float32
//	28  4  reserved
//	32  48 palette, RGB per biome index
//	80     size² × byte: low byte of the height delta
//	       size² × byte: high byte of the height delta
//	       size² × byte: biome index
//	       size² × byte: mask red delta
//	       size² × byte: mask green delta
//	       size² × byte: mask blue delta
//
// A delta is the cell's value minus the one to its left, wrapping, with the
// first cell of each row against zero; see render. Height is uint16.
func (r *Renderer) writeHeader(out []byte) {
	copy(out, magic)
	out[4] = version
	out[5] = paletteSize
	binary.LittleEndian.PutUint32(out[8:], uint32(r.size))
	binary.LittleEndian.PutUint32(out[12:], math.Float32bits(float32(r.radius)))
	binary.LittleEndian.PutUint32(out[16:], math.Float32bits(float32(worldgen.WaterLevel)))
	binary.LittleEndian.PutUint32(out[20:], math.Float32bits(heightOffset))
	binary.LittleEndian.PutUint32(out[24:], math.Float32bits(heightScale))
	for biome, c := range Palette {
		i := 32 + 3*int(BiomeIndex(biome))
		out[i], out[i+1], out[i+2] = c.R, c.G, c.B
	}
}

// readHeaderAndLength inflates the bytes, keeping the header and counting
// the rest.
func readHeaderAndLength(data []byte) (header, int, error) {
	zr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return header{}, 0, fmt.Errorf("worldmap: not a map: %w", err)
	}
	raw := make([]byte, headerSize)
	if _, err := io.ReadFull(zr, raw); err != nil {
		return header{}, 0, fmt.Errorf("worldmap: not a map: %w", err)
	}
	h, err := parseHeader(raw)
	if err != nil {
		return header{}, 0, err
	}
	rest, err := io.Copy(io.Discard, zr)
	if err != nil {
		return header{}, 0, fmt.Errorf("worldmap: not a map: %w", err)
	}
	return h, headerSize + int(rest), nil
}

func parseHeader(raw []byte) (header, error) {
	if len(raw) < headerSize || string(raw[:4]) != magic {
		return header{}, fmt.Errorf("worldmap: not a map")
	}
	if raw[4] != version {
		return header{}, fmt.Errorf("worldmap: map format %d, want %d", raw[4], version)
	}
	if raw[5] != paletteSize {
		return header{}, fmt.Errorf("worldmap: map has %d palette entries, want %d", raw[5], paletteSize)
	}
	f := func(at int) float64 {
		return float64(math.Float32frombits(binary.LittleEndian.Uint32(raw[at:])))
	}
	h := header{
		size:         int(binary.LittleEndian.Uint32(raw[8:])),
		radius:       f(12),
		water:        f(16),
		heightOffset: f(20),
		heightScale:  f(24),
	}
	for i := range h.palette {
		h.palette[i] = color.RGBA{raw[32+3*i], raw[33+3*i], raw[34+3*i], 255}
	}
	if h.size <= 0 || h.size > 1<<14 {
		return header{}, fmt.Errorf("worldmap: map is %d samples wide", h.size)
	}
	return h, nil
}

// Raster is the map read back from its bytes: what the browser sees, in a
// form a Go test or tool can query.
type Raster struct {
	Size    int
	Radius  float64
	Water   float64
	Palette [paletteSize]color.RGBA

	height []uint16
	biome  []uint8
	mask   [3][]uint8

	heightOffset, heightScale float64
}

// Decode reads a Renderer's bytes back into a Raster.
func Decode(data []byte) (*Raster, error) {
	zr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("worldmap: not a map: %w", err)
	}
	raw, err := io.ReadAll(zr)
	if err != nil {
		return nil, fmt.Errorf("worldmap: not a map: %w", err)
	}
	h, err := parseHeader(raw)
	if err != nil {
		return nil, err
	}
	n := h.size * h.size
	if len(raw) != headerSize+6*n {
		return nil, fmt.Errorf("worldmap: map is %d bytes, want %d for %d samples", len(raw), headerSize+6*n, h.size)
	}
	r := &Raster{
		Size: h.size, Radius: h.radius, Water: h.water, Palette: h.palette,
		height: make([]uint16, n), biome: raw[headerSize+2*n : headerSize+3*n],
		mask: [3][]uint8{
			raw[headerSize+3*n : headerSize+4*n],
			raw[headerSize+4*n : headerSize+5*n],
			raw[headerSize+5*n:],
		},
		heightOffset: h.heightOffset, heightScale: h.heightScale,
	}
	lo, hi := raw[headerSize:headerSize+n], raw[headerSize+n:headerSize+2*n]
	for y := 0; y < h.size; y++ {
		var height uint16
		for x := 0; x < h.size; x++ {
			i := y*h.size + x
			height += uint16(lo[i]) | uint16(hi[i])<<8
			r.height[i] = height
		}
		for _, plane := range r.mask {
			var last byte
			for x := 0; x < h.size; x++ {
				i := y*h.size + x
				last += plane[i]
				plane[i] = last
			}
		}
	}
	return r, nil
}

// Height is the sampled height at a cell, in metres.
func (r *Raster) Height(x, y int) float64 {
	return float64(r.height[y*r.Size+x])/r.heightScale - r.heightOffset
}

// Biome is the biome index at a cell; see BiomeIndex.
func (r *Raster) Biome(x, y int) uint8 { return r.biome[y*r.Size+x] }

// Mask is the map mask at a cell, each channel in [0, 1]; see maskColor.
func (r *Raster) Mask(x, y int) (red, green, blue float64) {
	i := y*r.Size + x
	return float64(r.mask[0][i]) / 255, float64(r.mask[1][i]) / 255, float64(r.mask[2][i]) / 255
}
