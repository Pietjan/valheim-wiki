//go:build js && wasm

// Command world-wasm is the seed engine the wiki runs in the browser: the
// seed → world port compiled to WebAssembly and driven from a Web Worker.
//
// It registers one global, vegvisirWorld, on the worker's scope:
//
//	version()                        the engine version, the cache key beside the seed
//	open(seed, mapSize)              computes a world, streaming it out through vegvisirEmit
//	adopt(seed, mapSize, size, ids, names)   a cached world's sector plane, so point() can name sectors
//	point(seed, x, z)                one position's biome, height, depth, forest and sector
//	tile(seed, cx, cz, span, n)      a close-up grid of the ground, for the 3D view
//
// open runs to completion inside the call — twenty-odd seconds — and
// hands each message to the worker's vegvisirEmit(message, transfer) as
// it is ready, which the worker stores and relays to the page. The
// messages are docs/architecture.md's worker → page protocol, every one
// carrying the seed and mapSize it belongs to; the typed arrays in them
// are fresh buffers listed in transfer, to hand to the page without a
// second copy.
//
// Build: GOOS=js GOARCH=wasm go build -ldflags='-s -w' -o web/static/world/world.wasm ./cmd/world-wasm
package main

import (
	"encoding/binary"
	"fmt"
	"math"
	"syscall/js"
	"time"

	"github.com/pietjan/valheim-wiki/cmd/world-wasm/engine"
	"github.com/pietjan/valheim-wiki/internal/worldmap"
)

var (
	global       = js.Global()
	object       = global.Get("Object")
	array        = global.Get("Array")
	uint8Array   = global.Get("Uint8Array")
	uint16Array  = global.Get("Uint16Array")
	float32Array = global.Get("Float32Array")
)

// current is the world the last open or adopt made, which point answers
// for; a point about another seed gets a world with nothing built, which
// is enough for everything but sector names.
var current *engine.World

func main() {
	api := object.New()
	api.Set("version", js.FuncOf(func(js.Value, []js.Value) any { return engine.Version() }))
	api.Set("open", js.FuncOf(open))
	api.Set("adopt", js.FuncOf(adopt))
	api.Set("point", js.FuncOf(point))
	api.Set("tile", js.FuncOf(tile))
	global.Set("vegvisirWorld", api)
	if ready := global.Get("vegvisirReady"); ready.Type() == js.TypeFunction {
		ready.Invoke()
	}
	select {}
}

// emit hands one message and the buffers it owns to the worker.
func emit(msg map[string]any, transfer ...js.Value) {
	list := array.New(len(transfer))
	for i, t := range transfer {
		list.SetIndex(i, t)
	}
	global.Call("vegvisirEmit", js.ValueOf(msg), list)
}

// Typed arrays from Go slices: one copy each, into a buffer the page can
// own. The multi-byte ones go through a byte view, little-endian as
// WebAssembly is, and are then re-viewed as their own type.

func bytesJS(b []byte) js.Value {
	a := uint8Array.New(len(b))
	js.CopyBytesToJS(a, b)
	return a
}

func uint16JS(v []uint16) js.Value {
	b := make([]byte, 2*len(v))
	for i, x := range v {
		binary.LittleEndian.PutUint16(b[2*i:], x)
	}
	return uint16Array.New(bytesJS(b).Get("buffer"))
}

func float32JS(v []float32) js.Value {
	b := make([]byte, 4*len(v))
	for i, x := range v {
		binary.LittleEndian.PutUint32(b[4*i:], math.Float32bits(x))
	}
	return float32Array.New(bytesJS(b).Get("buffer"))
}

func uint16Go(a js.Value) []uint16 {
	view := uint8Array.New(a.Get("buffer"), a.Get("byteOffset"), a.Get("byteLength"))
	b := make([]byte, view.Length())
	js.CopyBytesToGo(b, view)
	out := make([]uint16, len(b)/2)
	for i := range out {
		out[i] = binary.LittleEndian.Uint16(b[2*i:])
	}
	return out
}

func strings(a js.Value) []string {
	out := make([]string, a.Length())
	for i := range out {
		out[i] = a.Index(i).String()
	}
	return out
}

func intArg(args []js.Value, i int) int {
	if i < len(args) && args[i].Type() == js.TypeNumber {
		return args[i].Int()
	}
	return 0
}

// open computes a world and streams it: progress, info, map, sectors,
// rivers, the sites rule by rule, done. A failure is one error message
// instead of a dead worker.
func open(_ js.Value, args []js.Value) any {
	seed := args[0].String()
	mapSize := intArg(args, 1)
	defer func() {
		if r := recover(); r != nil {
			emit(map[string]any{"type": "error", "seed": seed, "mapSize": mapSize, "message": fmt.Sprint(r)})
		}
	}()

	w, err := engine.New(seed, mapSize)
	if err != nil {
		emit(map[string]any{"type": "error", "seed": seed, "mapSize": mapSize, "message": err.Error()})
		return nil
	}
	current = w
	start := time.Now()
	info := w.Info()
	mapSize = info.MapSize
	msg := func(kind string) map[string]any {
		return map[string]any{"type": kind, "seed": seed, "mapSize": mapSize}
	}
	elapsed := func() int { return int(time.Since(start) / time.Millisecond) }
	progress := func(phase string, index, total, placed int) {
		m := msg("progress")
		m["phase"], m["index"], m["total"], m["placed"], m["elapsed"] = phase, index, total, placed, elapsed()
		emit(m)
	}

	progress("world", 0, 0, 0)
	w.Prepare()
	info = w.Info()
	m := msg("info")
	m["seedNumber"] = info.SeedNumber
	m["engine"] = info.Engine
	m["radius"] = info.Radius
	m["worldRadius"] = info.WorldRadius
	m["water"] = info.Water
	// The height plane's encoding, so the page decodes it with the raster's
	// own numbers rather than a copy of them.
	m["heightOffset"] = worldmap.HeightOffset
	m["heightScale"] = worldmap.HeightScale
	m["palette"] = bytesJS(info.Palette[:])
	m["sectorCount"] = info.SectorCount
	m["altSectorCount"] = info.AltSectorCount
	emit(m)

	progress("map", 0, 0, 0)
	raster := w.Map()
	heights, biomes, mask := uint16JS(raster.Heights), bytesJS(raster.Biomes), bytesJS(raster.Mask)
	m = msg("map")
	m["size"], m["heights"], m["biomes"], m["mask"] = raster.Size, heights, biomes, mask
	emit(m, heights.Get("buffer"), biomes.Get("buffer"), mask.Get("buffer"))

	sectors := w.Sectors()
	list := make([]any, len(sectors.List))
	for i, s := range sectors.List {
		names := make([]any, len(s.Names))
		for j, n := range s.Names {
			names[j] = n
		}
		list[i] = map[string]any{"id": s.ID, "biome": s.Biome, "names": names, "x": s.X, "z": s.Z}
	}
	ids := uint16JS(sectors.IDs)
	m = msg("sectors")
	m["size"], m["ids"], m["list"] = sectors.Size, ids, list
	emit(m, ids.Get("buffer"))

	rivers := w.Rivers()
	lines := func(in []engine.Line) []any {
		out := make([]any, len(in))
		for i, l := range in {
			out[i] = []any{l[0], l[1], l[2], l[3], l[4], l[5]}
		}
		return out
	}
	m = msg("rivers")
	m["rivers"], m["streams"] = lines(rivers.Rivers), lines(rivers.Streams)
	emit(m)

	progress("sites", 0, 0, 0)
	placed := 0
	report := w.Place(func(rs engine.RuleSites) {
		placed += len(rs.XZY) / 3
		xyz := float32JS(rs.XZY)
		m := msg("sites")
		m["rule"], m["prefab"], m["index"], m["total"] = rs.Rule, rs.Prefab, rs.Index, rs.Total
		m["xyz"], m["placed"], m["elapsed"] = xyz, placed, elapsed()
		emit(m, xyz.Get("buffer"))
	})
	lines2 := make([]any, len(report))
	for i, r := range report {
		lines2[i] = map[string]any{"rule": r.Rule, "prefab": r.Prefab, "wanted": r.Wanted, "placed": r.Placed}
	}
	m = msg("done")
	m["report"], m["took"], m["placed"] = lines2, elapsed(), placed
	emit(m)
	return nil
}

// adopt makes a cached world current: its seed and the sector plane the
// cache kept, so point names sectors without building the biome map.
func adopt(_ js.Value, args []js.Value) any {
	seed := args[0].String()
	w, err := engine.New(seed, intArg(args, 1))
	if err != nil {
		return err.Error()
	}
	s := &engine.Sectors{Size: intArg(args, 2), IDs: uint16Go(args[3])}
	if len(s.IDs) != s.Size*s.Size {
		return fmt.Sprintf("sector plane is %d ids for %d a side", len(s.IDs), s.Size)
	}
	names := args[4]
	for i := 0; i < names.Length(); i++ {
		s.List = append(s.List, engine.Sector{ID: i + 1, Names: strings(names.Index(i))})
	}
	w.Adopt(s)
	current = w
	return nil
}

// world is the current world for a seed: the open or adopted one, else
// a fresh one with nothing built, which answers heights and biomes but
// names no sectors.
func world(seed string) (*engine.World, error) {
	if current != nil && current.Seed() == seed {
		return current, nil
	}
	w, err := engine.New(seed, 0)
	if err != nil {
		return nil, err
	}
	current = w
	return w, nil
}

// tile samples a close-up of the ground: {n, heights: Float32Array,
// biomes: Uint8Array, mask: Uint8Array RGBA}, row zero north, or {error}.
func tile(_ js.Value, args []js.Value) any {
	w, err := world(args[0].String())
	if err != nil {
		return map[string]any{"error": err.Error()}
	}
	t := w.Tile(args[1].Float(), args[2].Float(), args[3].Float(), intArg(args, 4))
	return map[string]any{
		"n": t.N, "cx": t.CX, "cz": t.CZ, "span": t.Span,
		"heights": float32JS(t.Heights), "biomes": bytesJS(t.Biomes), "mask": bytesJS(t.Mask),
	}
}

// point answers for one position of a seed.
func point(_ js.Value, args []js.Value) any {
	seed := args[0].String()
	if _, err := world(seed); err != nil {
		return map[string]any{"error": err.Error()}
	}
	p := current.Point(args[1].Float(), args[2].Float())
	out := map[string]any{"x": p.X, "z": p.Z, "ok": p.OK, "biome": p.Biome, "height": nil, "depth": p.Depth, "forest": p.Forest, "sector": nil}
	if p.OK {
		out["height"] = p.Height
	}
	if p.Sector != nil {
		names := make([]any, len(p.Sector))
		for i, n := range p.Sector {
			names[i] = n
		}
		out["sector"] = names
	}
	return out
}
