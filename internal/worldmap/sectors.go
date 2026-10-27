package worldmap

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/binary"
	"math"
	"strings"

	"github.com/pietjan/valheim-wiki/internal/worldgen"
)

// The alt-biome sectors: the game's biome variations, from the seed.
//
// Since 1.0 a biome's connected regions — its sectors, flood-filled on the
// same 2048-point grid the map is sampled on — can each carry alt biomes,
// the "Dark Meadows" or "Goblin Plains" that add and block vegetation,
// locations and spawns and rename the region on the in-game map. Which
// sectors carry which is rolled from the seed, see worldgen.RollAltBiomes,
// so like the map it is known before anyone has been there.
//
// What the browser gets is a plane of sector ids over the map's grid, zero
// where a sector carries nothing, so the painter can outline the ones that
// do; and a list of those sectors, so the page can name them. The plane's
// rows run north to south like the map's.

// AltSector is one sector that carries alt biomes.
type AltSector struct {
	// ID is the sector's number in the plane, from one.
	ID int
	// Biome is the sector's own biome; the alts vary it, they do not
	// replace it.
	Biome worldgen.Biome
	// Names are the alt biomes it carries, in the order they were rolled.
	Names []string
	// X and Z are where to write its name: the sector's own grid point
	// nearest its edge centroid, since the centroid of a crescent is not
	// in the crescent.
	X, Z float64
}

// Sectors is the alt-biome sectors of one world.
type Sectors struct {
	// Size is the plane's width and height, worldgen.BiomeMapSize.
	Size int
	// List is every sector carrying alt biomes, in id order.
	List []AltSector

	ids  []uint16
	data []byte
}

// IDAt is the sector id under a world position, zero where the sector
// carries no alt biome or the position is off the map: the plane's cell as
// the game's GetBiomeSector reads it, nearest grid point, row zero north.
func (s *Sectors) IDAt(wx, wz float64) int {
	n := float64(s.Size)
	x := int(math.Floor((wx + MapRadius) / (2 * MapRadius) * n))
	y := int(math.Floor((MapRadius - wz) / (2 * MapRadius) * n))
	if x < 0 || y < 0 || x >= s.Size || y >= s.Size {
		return 0
	}
	return int(s.ids[y*s.Size+x])
}

// The plane's encoding: a header, then size² little-endian uint16 ids,
// then one record per listed sector in id order — its biome index, and its
// name as a little-endian uint16 length and that many bytes of UTF-8 — so
// the browser can say what is under the cursor without a second fetch.
const (
	sectorsMagic      = "VSEC"
	sectorsVersion    = 1
	sectorsHeaderSize = 16
)

// Data is the plane, gzip-compressed, as /sectors.bin serves it.
func (s *Sectors) Data() []byte { return s.data }

// buildSectors builds the biome map, rolls the alt biomes onto it and
// encodes the plane. A second or two.
func buildSectors(gen *worldgen.Generator, seed int32) *Sectors {
	m := gen.BuildBiomeMap()
	m.RollAltBiomes(worldgen.DefaultAltBiomes(), seed)

	const n = worldgen.BiomeMapSize
	ids := make(map[*worldgen.BiomeSector]int)
	var list []AltSector
	for _, sec := range m.Sectors {
		if len(sec.AltBiomes) == 0 {
			continue
		}
		names := make([]string, len(sec.AltBiomes))
		for i, a := range sec.AltBiomes {
			names[i] = a.Name
		}
		ids[sec] = len(list) + 1
		list = append(list, AltSector{ID: len(list) + 1, Biome: sec.Biome, Names: names})
	}

	// The plane, and each sector's nearest own point to its centroid.
	raw := make([]byte, sectorsHeaderSize+2*n*n)
	copy(raw, sectorsMagic)
	raw[4] = sectorsVersion
	binary.LittleEndian.PutUint32(raw[8:], n)
	binary.LittleEndian.PutUint32(raw[12:], uint32(len(list)))
	plane := make([]uint16, n*n)
	best := make([]float64, len(list))
	for i := range best {
		best[i] = math.Inf(1)
	}
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			p := worldgen.BiomePoint{X: int16(x), Y: int16(y)}
			sec := m.SectorAtPoint(p)
			id := ids[sec]
			if id == 0 {
				continue
			}
			// Row zero is north, as the map's is.
			binary.LittleEndian.PutUint16(raw[sectorsHeaderSize+2*((n-1-y)*n+x):], uint16(id))
			plane[(n-1-y)*n+x] = uint16(id)
			wx, wz := p.World()
			d := math.Hypot(float64(wx-sec.Center.X), float64(wz-sec.Center.Y))
			if d < best[id-1] {
				best[id-1] = d
				list[id-1].X, list[id-1].Z = float64(wx), float64(wz)
			}
		}
	}

	for _, sec := range list {
		name := strings.Join(sec.Names, ", ")
		raw = append(raw, BiomeIndex(sec.Biome))
		raw = binary.LittleEndian.AppendUint16(raw, uint16(len(name)))
		raw = append(raw, name...)
	}

	var buf bytes.Buffer
	w, _ := gzip.NewWriterLevel(&buf, gzip.DefaultCompression)
	w.Write(raw)
	w.Close()
	return &Sectors{Size: n, List: list, ids: plane, data: buf.Bytes()}
}

// startSectors starts the build, once.
func (r *Renderer) startSectors() {
	r.secOnce.Do(func() {
		go func() {
			defer close(r.secDone)
			r.sec = buildSectors(r.gen, r.gen.Seed())
		}()
	})
}

// Sectors is the world's alt-biome sectors, built on the first call and
// waited for.
func (r *Renderer) Sectors(ctx context.Context) (*Sectors, error) {
	r.startSectors()
	select {
	case <-r.secDone:
		return r.sec, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// SectorsNow is the sectors if they are built, or nil while they are being
// built — and the first call is what starts the build. For a page that
// redraws every second anyway: it says nothing for a tick or two rather
// than holding the first paint.
func (r *Renderer) SectorsNow() *Sectors {
	r.startSectors()
	select {
	case <-r.secDone:
		return r.sec
	default:
		return nil
	}
}
