package worldgen

import (
	"bufio"
	"math"
	"os"
	"strconv"
	"strings"
	"testing"
)

// The Ashlands height against the game's, bit for bit.
//
// The golden is tools/probes/ashlands-dump.cs run on npcdev: GetAshlandsHeight
// itself, the vegetation alpha it writes into the mask, and GetHeight — the
// same number scaled to world units through GetBiomeHeight's gaps — at every
// point of a 125 m grid over the biome and at 300 off-grid points. The
// function is double from the position in to the return, so nothing here is
// allowed a tolerance: the height is compared as the float32 it returns.
func TestAshlandsHeightMatchesTheGame(t *testing.T) {
	f, err := os.Open("testdata/ashlands_npcdev.tsv")
	if err != nil {
		t.Fatalf("open golden: %v", err)
	}
	defer f.Close()

	g := New(npcdev)

	var checked, above int
	var worst float64
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "# seed") {
			seed, _ := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "# seed")))
			if int32(seed) != npcdevSeed {
				t.Fatalf("golden is from seed %d, the test world is %d", seed, npcdevSeed)
			}
		}
		if strings.HasPrefix(line, "#") || line == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		x, _ := strconv.ParseFloat(fields[0], 32)
		y, _ := strconv.ParseFloat(fields[1], 32)
		wantH, _ := strconv.ParseFloat(fields[2], 32)
		wantVeg, _ := strconv.ParseFloat(fields[3], 32)
		wantHeight, _ := strconv.ParseFloat(fields[4], 32)
		checked++
		if wantHeight > WaterLevel {
			above++
		}

		h, veg := g.ashlands32(float32(x), float32(y))
		if h != float32(wantH) {
			t.Errorf("ashlands(%v, %v) = %v, game %v (off by %+.3g)", x, y, h, wantH, float64(h)-wantH)
		}
		if veg != float32(wantVeg) {
			t.Errorf("ashlands(%v, %v) vegetation = %v, game %v", x, y, veg, wantVeg)
		}

		if b := g.GetBiome(x, y); b != BiomeAshLands {
			t.Errorf("(%v, %v) classified %v, the game says AshLands", x, y, b)
			continue
		}
		got, ok := g.BiomeHeight(BiomeAshLands, x, y)
		if !ok && length(x, y) <= 10500 {
			t.Errorf("height at (%v, %v) reported unreliable", x, y)
		}
		if got != wantHeight {
			t.Errorf("height(%v, %v) = %v, game %v (off by %+.4f)", x, y, got, wantHeight, got-wantHeight)
		}
		if d := math.Abs(got - wantHeight); d > worst {
			worst = d
		}
	}

	if checked < 1000 {
		t.Fatalf("only %d samples", checked)
	}
	t.Logf("%d ashlands points, %d above the waterline, worst %.4g m", checked, above, worst)
}
