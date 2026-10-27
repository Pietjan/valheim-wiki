package worldmap

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/binary"
	"io"
	"math"
	"strings"
	"testing"

	"github.com/pietjan/valheim-wiki/internal/worldgen"
)

// The plane and the list agree with the roll, and with each other: every
// listed sector's label point reads back as its own id, and every id in
// the plane is a listed sector.
func TestSectorsPlaneNamesTheRoll(t *testing.T) {
	const seed = -1365230248
	r := New(worldgen.New(worldgen.ConfigFromSeed(seed)), 32)
	if r.SectorsNow() != nil {
		t.Fatal("sectors before anyone built them")
	}
	s, err := r.Sectors(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if s.Size != worldgen.BiomeMapSize {
		t.Fatalf("plane is %d wide, want %d", s.Size, worldgen.BiomeMapSize)
	}
	if len(s.List) == 0 {
		t.Fatal("no alt-biome sectors on a seed that has them")
	}

	zr, err := gzip.NewReader(bytes.NewReader(s.Data()))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := io.ReadAll(zr)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw[:4]) != sectorsMagic || raw[4] != sectorsVersion {
		t.Fatalf("header %q v%d", raw[:4], raw[4])
	}
	if got := int(binary.LittleEndian.Uint32(raw[8:])); got != s.Size {
		t.Errorf("header says %d wide, plane is %d", got, s.Size)
	}
	if got := int(binary.LittleEndian.Uint32(raw[12:])); got != len(s.List) {
		t.Errorf("header counts %d sectors, list has %d", got, len(s.List))
	}
	plane := sectorsHeaderSize + 2*s.Size*s.Size
	if len(raw) < plane {
		t.Fatalf("plane is %d bytes, want at least %d", len(raw), plane)
	}
	// The names after the plane, one record per sector in id order.
	at := plane
	for _, sec := range s.List {
		if at+3 > len(raw) {
			t.Fatalf("names run out at sector %d", sec.ID)
		}
		if got := raw[at]; got != BiomeIndex(sec.Biome) {
			t.Errorf("sector %d: biome index %d, want %d", sec.ID, got, BiomeIndex(sec.Biome))
		}
		n := int(binary.LittleEndian.Uint16(raw[at+1:]))
		name := string(raw[at+3 : at+3+n])
		if want := strings.Join(sec.Names, ", "); name != want {
			t.Errorf("sector %d: named %q, want %q", sec.ID, name, want)
		}
		at += 3 + n
	}
	if at != len(raw) {
		t.Errorf("%d bytes after the names", len(raw)-at)
	}
	raw = raw[:plane]

	idAt := func(x, z float64) int {
		// The plane's cell for a world point: row zero north, 12 m cells
		// centred on the grid, as worldgen.BiomePoint.World places them.
		gx := int(math.Floor((x + MapRadius) / 12))
		gy := int(math.Floor((MapRadius - z) / 12))
		return int(binary.LittleEndian.Uint16(raw[sectorsHeaderSize+2*(gy*s.Size+gx):]))
	}
	for i, sec := range s.List {
		if sec.ID != i+1 {
			t.Errorf("sector %d listed as id %d", i+1, sec.ID)
		}
		if len(sec.Names) == 0 {
			t.Errorf("sector %d carries nothing", sec.ID)
		}
		if got := idAt(sec.X, sec.Z); got != sec.ID {
			t.Errorf("sector %d (%v) labelled at (%.0f, %.0f), where the plane says %d", sec.ID, sec.Names, sec.X, sec.Z, got)
		}
	}
	seen := make(map[int]bool)
	for i := sectorsHeaderSize; i < len(raw); i += 2 {
		if id := int(binary.LittleEndian.Uint16(raw[i:])); id != 0 {
			if id > len(s.List) {
				t.Fatalf("id %d in the plane, only %d listed", id, len(s.List))
			}
			seen[id] = true
		}
	}
	if len(seen) != len(s.List) {
		t.Errorf("%d ids in the plane, %d listed", len(seen), len(s.List))
	}
	t.Logf("%d alt-biome sectors", len(s.List))

	// Built once: the same answer, at once, from then on.
	if again := r.SectorsNow(); again != s {
		t.Error("a second ask built again")
	}
}
