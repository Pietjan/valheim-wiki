package worldgen

import (
	"bufio"
	"os"
	"strconv"
	"strings"
	"testing"
)

// The noise on its own, before the Ashlands layers anything on it: FastNoise
// configured as WorldGenerator configures it, sampled inside the running game
// by tools/probes/ashlands-dump.cs. It is a double generator, so the answer
// is bit-for-bit, and the sample spans the coordinates GetAshlandsHeight feeds
// it — a band near zero for the cell hashing and a few million for the rest.
func TestFastNoiseMatchesTheGame(t *testing.T) {
	f, err := os.Open("testdata/fastnoise.tsv")
	if err != nil {
		t.Fatalf("open golden: %v", err)
	}
	defer f.Close()

	var checked int
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "#") || line == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		x, _ := strconv.ParseFloat(fields[0], 64)
		y, _ := strconv.ParseFloat(fields[1], 64)
		wantCellular, _ := strconv.ParseFloat(fields[2], 64)
		wantSimplex, _ := strconv.ParseFloat(fields[3], 64)
		checked++

		if got := ashlandsNoise.Cellular(x, y); got != wantCellular {
			t.Errorf("cellular(%v, %v) = %v, game %v", x, y, got, wantCellular)
		}
		if got := ashlandsNoise.SimplexFractal(x, y); got != wantSimplex {
			t.Errorf("simplexFractal(%v, %v) = %v, game %v", x, y, got, wantSimplex)
		}
	}
	if checked < 100 {
		t.Fatalf("only %d samples", checked)
	}
	t.Logf("%d points, cellular and simplex fractal bit-exact", checked)
}
