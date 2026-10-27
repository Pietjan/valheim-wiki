package worldgen

import (
	"testing"
)

// The fresh 1.0 world's watercourses, from a probe that read them out of the
// running server. Streams are the sensitive part: 3,000 seeded attempts,
// each accepted or rejected on a pregeneration height against a threshold,
// so one height an ulp off moves every stream placed after it. Before the
// height functions were made bit-exact this diverged at stream 1,185 with
// Perlin exact and at 1,896 with the base height exact too.
func TestLocgenWatercoursesMatchTheGame(t *testing.T) {
	if testing.Short() {
		t.Skip("generates a whole world's rivers; skipped in -short")
	}
	g := New(ConfigFromSeed(locgenSeed))

	lakes := g.findLakes()
	n := 0
	locgenRows(t, "lakes.tsv", func(r tsvRow) {
		if n < len(lakes) && lakes[n] != (Vec2{r.float("x"), r.float("y")}) {
			t.Errorf("lake %d: %v, game (%v, %v)", n, lakes[n], r.float("x"), r.float("y"))
		}
		n++
	})
	if len(lakes) != n {
		t.Errorf("%d lakes, game has %d", len(lakes), n)
	}

	d := g.ensureRivers()
	for _, tc := range []struct {
		file string
		ours []River
	}{{"rivers.tsv", d.rivers}, {"streams.tsv", d.streams}} {
		i := 0
		first := -1
		locgenRows(t, tc.file, func(r tsvRow) {
			if i < len(tc.ours) && first < 0 {
				o := tc.ours[i]
				want := River{
					P0: Vec2{r.float("p0x"), r.float("p0y")}, P1: Vec2{r.float("p1x"), r.float("p1y")},
					Center:   Vec2{r.float("cx"), r.float("cy")},
					WidthMin: r.float("widthMin"), WidthMax: r.float("widthMax"),
					CurveWidth: r.float("curveWidth"), CurveWavelength: r.float("curveWavelength"),
				}
				if o != want {
					first = i
					t.Errorf("%s: first divergence at %d:\n  ours %+v\n  game %+v", tc.file, i, o, want)
				}
			}
			i++
		})
		if len(tc.ours) != i {
			t.Errorf("%s: %d, game has %d", tc.file, len(tc.ours), i)
		}
	}
}
