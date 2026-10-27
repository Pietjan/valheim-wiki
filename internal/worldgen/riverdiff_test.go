package worldgen

import (
	"bufio"
	"math"
	"os"
	"strconv"
	"strings"
	"testing"
)

// TestRiverWeightAgainstTheGame diffs this package's river placement against
// the game's own GetRiverWeight, sampled off the live server.
//
// Skipped without the sample, which is a scratch artefact rather than a
// committed golden: see scratch/heightdiff/rivers.cs for how it is made.
//
//	go test ./internal/worldgen -run RiverWeight -v -args -sample=/path/rivers.csv
func TestRiverWeightAgainstTheGame(t *testing.T) {
	path := os.Getenv("RIVER_SAMPLE")
	if path == "" {
		t.Skip("set RIVER_SAMPLE to a csv of x,z,weight,width from the game")
	}

	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	g := New(ConfigFromSeed(-1365230248))
	d := g.ensureRivers()

	var n, missing, spurious, wrong int
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		p := strings.Split(strings.TrimSpace(sc.Text()), ",")
		if len(p) != 4 {
			continue
		}
		x, _ := strconv.ParseFloat(p[0], 64)
		z, _ := strconv.ParseFloat(p[1], 64)
		wantW, _ := strconv.ParseFloat(p[2], 64)
		wantWidth, _ := strconv.ParseFloat(p[3], 64)

		gotW, gotWidth := d.riverWeight(float32(x), float32(z))
		n++

		switch {
		case wantW > 0 && gotW == 0:
			missing++
		case wantW == 0 && gotW > 0:
			spurious++
		case math.Abs(float64(gotW)-wantW) > 0.01 || math.Abs(float64(gotWidth)-wantWidth) > 1:
			wrong++
			if wrong <= 5 {
				t.Logf("%.0f,%.0f game w=%.4f width=%.1f | port w=%.4f width=%.1f",
					x, z, wantW, wantWidth, gotW, gotWidth)
			}
		}
	}

	t.Logf("%d points: %d rivers the port misses, %d it invents, %d it gets the wrong weight for",
		n, missing, spurious, wrong)
	if missing+spurious+wrong > 0 {
		t.Errorf("river placement disagrees at %d of %d points", missing+spurious+wrong, n)
	}
}
