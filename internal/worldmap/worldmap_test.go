package worldmap

import (
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"math"
	"testing"

	"github.com/pietjan/valheim-wiki/internal/worldgen"
)

const npcdevSeed = -1365230248

func npcdev(t *testing.T, size int) *Renderer {
	t.Helper()
	return New(worldgen.New(worldgen.ConfigFromSeed(npcdevSeed)), size)
}

func decoded(t *testing.T, r *Renderer) *Raster {
	t.Helper()
	data, err := r.Data(context.Background())
	if err != nil {
		t.Fatalf("Data: %v", err)
	}
	raster, err := Decode(data)
	if err != nil {
		t.Fatalf("the map does not read back: %v", err)
	}
	return raster
}

// North up, east right. Getting this wrong mirrors the map, which is the kind
// of bug that looks plausible until someone tries to navigate by it.
func TestProjectionOrientation(t *testing.T) {
	r := npcdev(t, 64)

	centreX, centreZ := r.Project(0, 0)
	if math.Abs(centreX-0.5) > 1e-9 || math.Abs(centreZ-0.5) > 1e-9 {
		t.Errorf("the origin projects to %.3f,%.3f, not the middle", centreX, centreZ)
	}

	eastX, _ := r.Project(5000, 0)
	if eastX <= 0.5 {
		t.Errorf("east is at %.3f, which is not to the right of centre", eastX)
	}

	// World Z increases northwards; image Y increases downwards.
	_, northZ := r.Project(0, 5000)
	if northZ >= 0.5 {
		t.Errorf("north is at %.3f, which is not above centre", northZ)
	}

	// The grid's edge is the image's edge, and the world ends inside it.
	edgeX, _ := r.Project(MapRadius, 0)
	if math.Abs(edgeX-1) > 1e-9 {
		t.Errorf("the grid edge projects to %.3f, not 1", edgeX)
	}
	worldX, _ := r.Project(WorldRadius, 0)
	if worldX >= 1 || worldX <= 0.9 {
		t.Errorf("the world edge projects to %.3f, which is not just inside the grid", worldX)
	}
}

func TestInWorld(t *testing.T) {
	r := npcdev(t, 64)

	if !r.InWorld(0, 0) {
		t.Error("the origin is not in the world")
	}
	if r.InWorld(WorldRadius+1, 0) {
		t.Error("a point past the edge is in the world")
	}
}

// The bytes have to say what they are, so a browser and a later start can
// both trust them.
func TestRendersAReadableRaster(t *testing.T) {
	raster := decoded(t, npcdev(t, 128))

	if raster.Size != 128 {
		t.Errorf("raster is %d samples wide, want 128", raster.Size)
	}
	if raster.Radius != MapRadius {
		t.Errorf("raster says the grid is %.0fm, want %.0f", raster.Radius, MapRadius)
	}
	if raster.Water != worldgen.WaterLevel {
		t.Errorf("raster says water is at %.1f, want %.1f", raster.Water, worldgen.WaterLevel)
	}
	if got, want := raster.Palette[BiomeIndex(worldgen.BiomeMeadows)], Palette[worldgen.BiomeMeadows]; got != want {
		t.Errorf("meadows are %v in the header, want %v", got, want)
	}
	if got, want := raster.Palette[0], Palette[worldgen.BiomeNone]; got != want {
		t.Errorf("the void is %v in the header, want %v", got, want)
	}
}

// Sampling a million points of terrain is not something to do twice.
func TestDataIsRenderedOnce(t *testing.T) {
	r := npcdev(t, 64)

	first, err := r.Data(context.Background())
	if err != nil {
		t.Fatalf("Data: %v", err)
	}
	second, _ := r.Data(context.Background())

	if &first[0] != &second[0] {
		t.Error("the map was re-encoded on the second call")
	}
}

// Restored takes last time's bytes on trust for the seed, and for nothing
// else: a picture from the old format, or a map at another size, is redrawn
// rather than served.
func TestRestoredChecksTheBytes(t *testing.T) {
	gen := worldgen.New(worldgen.ConfigFromSeed(npcdevSeed))
	data, _ := New(gen, 64).Data(context.Background())

	if _, err := Restored(gen, 64, data); err != nil {
		t.Errorf("the map it just drew is refused: %v", err)
	}
	if _, err := Restored(gen, 128, data); err == nil {
		t.Error("a 64px map was accepted as a 128px one")
	}
	if _, err := Restored(gen, 64, []byte("\x89PNG\r\n\x1a\n")); err == nil {
		t.Error("a PNG was accepted as a map")
	}
	// A right header on a short body: the last build's layout.
	short := truncated(t, data)
	if _, err := Restored(gen, 64, short); err == nil {
		t.Error("a map with the right header and a short body was accepted")
	}
	if _, err := Restored(gen, 64, nil); err == nil {
		t.Error("nothing was accepted as a map")
	}

	kept, _ := Restored(gen, 64, data)
	again, _ := kept.Data(context.Background())
	if &again[0] != &data[0] {
		t.Error("a restored map was drawn again")
	}
}

// The map has to be the world, not a picture of it: what a cell says about
// height, biome and mask is what the generator says at that cell's centre —
// the biome's own height, as Minimap.GenerateWorldMap stores it — to the
// precision the encoding keeps.
func TestCellsAreTheGeneratorsAnswers(t *testing.T) {
	r := npcdev(t, 256)
	raster := decoded(t, r)

	checked := 0
	for y := 0; y < r.size; y += 17 {
		for x := 0; x < r.size; x += 13 {
			wx := (float64(x)+0.5)/float64(r.size)*2*r.radius - r.radius
			wz := r.radius - (float64(y)+0.5)/float64(r.size)*2*r.radius
			if !r.InWorld(wx, wz) {
				if raster.Biome(x, y) != 0 {
					t.Errorf("cell %d,%d is outside the world and has biome %d", x, y, raster.Biome(x, y))
				}
				continue
			}
			checked++

			b := r.gen.GetBiome(wx, wz)
			h, _ := r.gen.BiomeHeight(b, wx, wz)
			if got := raster.Height(x, y); math.Abs(got-h) > 1/heightScale {
				t.Errorf("cell %d,%d is %.3fm, generator says %.3f", x, y, got, h)
			}
			if got, want := raster.Biome(x, y), BiomeIndex(b); got != want {
				t.Errorf("cell %d,%d is biome %d, generator says %d", x, y, got, want)
			}
			red, green, blue := maskColor(r.gen, wx, wz, h, b)
			gr, gg, gb := raster.Mask(x, y)
			if math.Abs(gr-float64(red)/255) > 1e-9 || math.Abs(gg-float64(green)/255) > 1e-9 || math.Abs(gb-float64(blue)/255) > 1e-9 {
				t.Errorf("cell %d,%d has mask %.3f,%.3f,%.3f, want %d,%d,%d", x, y, gr, gg, gb, red, green, blue)
			}
		}
	}
	if checked == 0 {
		t.Fatal("no cells fell inside the world")
	}
}

// The mask is Minimap.GetMaskColor, channel by channel.
func TestMaskIsTheGamesRule(t *testing.T) {
	gen := worldgen.New(worldgen.ConfigFromSeed(npcdevSeed))

	// Under water nothing grows, whatever the biome.
	if r, g, _ := maskColor(gen, 0, 0, 10, worldgen.BiomeBlackForest); r != 0 || g != 0 {
		t.Errorf("a drowned black forest has a mask of %d,%d", r, g)
	}
	// The black forest is forest everywhere above it.
	if r, _, _ := maskColor(gen, 0, 0, 40, worldgen.BiomeBlackForest); r != 255 {
		t.Errorf("the black forest is not forest: red %d", r)
	}
	// The meadows follow InForest, so somewhere in a 4km square both answers
	// occur.
	seen := map[uint8]bool{}
	for x := -2000.0; x <= 2000; x += 250 {
		for z := -2000.0; z <= 2000; z += 250 {
			r, _, _ := maskColor(gen, x, z, 40, worldgen.BiomeMeadows)
			seen[r] = true
			if r != 0 && r != 255 {
				t.Errorf("meadows at %.0f,%.0f have a partial forest of %d", x, z, r)
			}
		}
	}
	if !seen[0] || !seen[255] {
		t.Error("the meadows are all forest or all field")
	}
	// The mist is a ramp, not a switch.
	partial := false
	for x := -2000.0; x <= 2000; x += 50 {
		if _, g, _ := maskColor(gen, x, 0, 40, worldgen.BiomeMistlands); g != 0 && g != 255 {
			partial = true
		}
	}
	if !partial {
		t.Error("the mist never fades; it is a switch")
	}
}

// The map has to look like the world, not like noise. Valheim worlds are
// mostly ocean, but nowhere near all of it.
func TestTheMapLooksLikeTheWorld(t *testing.T) {
	raster := decoded(t, npcdev(t, 256))

	water, land := 0, 0
	for y := 0; y < raster.Size; y++ {
		for x := 0; x < raster.Size; x++ {
			if raster.Biome(x, y) == 0 {
				continue
			}
			if raster.Height(x, y) < raster.Water {
				water++
			} else {
				land++
			}
		}
	}

	if water+land == 0 {
		t.Fatal("no cells fell inside the world")
	}
	fraction := float64(land) / float64(water+land)
	if fraction < 0.05 || fraction > 0.6 {
		t.Errorf("%.0f%% of the world is land, which is not a Valheim world", 100*fraction)
	}
	if water == 0 {
		t.Error("no ocean at all")
	}
}

// Every biome the generator can return needs a colour and a slot in the
// header, or it draws as the void and reads as a hole in the map.
func TestEveryBiomeHasAColour(t *testing.T) {
	seen := map[uint8]worldgen.Biome{}
	for _, b := range []worldgen.Biome{
		worldgen.BiomeMeadows, worldgen.BiomeSwamp, worldgen.BiomeMountain,
		worldgen.BiomeBlackForest, worldgen.BiomePlains, worldgen.BiomeOcean,
		worldgen.BiomeAshLands, worldgen.BiomeDeepNorth, worldgen.BiomeMistlands,
		worldgen.BiomeNone,
	} {
		if _, ok := Palette[b]; !ok {
			t.Errorf("no colour for %s", b)
		}
		i := BiomeIndex(b)
		if i >= paletteSize {
			t.Errorf("%s is index %d, past the palette", b, i)
		}
		if other, dup := seen[i]; dup {
			t.Errorf("%s and %s share index %d", b, other, i)
		}
		seen[i] = b
	}
}

func BenchmarkRender(b *testing.B) {
	gen := worldgen.New(worldgen.ConfigFromSeed(npcdevSeed))
	gen.Height(0, 0) // rivers are generated once, and not part of the drawing

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		New(gen, 1024).render(context.Background())
	}
}

// truncated is a map's bytes with the header intact and the body cut: what
// a layout change leaves behind in a database.
func truncated(t *testing.T, data []byte) []byte {
	t.Helper()
	zr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := io.ReadAll(zr)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	w.Write(raw[:len(raw)-1000])
	w.Close()
	return buf.Bytes()
}
