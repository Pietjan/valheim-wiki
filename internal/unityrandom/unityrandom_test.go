package unityrandom

import (
	"math"
	"testing"
)

// All goldens in this file were captured from a running Valheim server with
// server_probe (see docs/dumps.md), including the internal state after every
// call. Testing the state and not just the outputs is what makes a failure
// point at the step that broke rather than at the end of the sequence.

func (r *Random) state() [4]int32 {
	return [4]int32{int32(r.s0), int32(r.s1), int32(r.s2), int32(r.s3)}
}

// InitState stretches one seed word into four. Three seeds, including a
// negative one, because the multiply wraps differently there.
func TestInitState(t *testing.T) {
	for _, tc := range []struct {
		seed int32
		want [4]int32
	}{
		{1, [4]int32{1, 1812433254, 1900727103, -603986212}},
		{7, [4]int32{7, -197869116, -1864477099, 1547603082}},
		{-3, [4]int32{-3, -1142332462, 1547551707, 824932456}},
		{-1365230248, [4]int32{-1365230248, 1937001401, -411639554, 1061204535}},
	} {
		if got := New(tc.seed).state(); got != tc.want {
			t.Errorf("New(%d) state = %v, want %v", tc.seed, got, tc.want)
		}
	}
}

// The state must march in lockstep with the game's, one step per call.
func TestStateAdvancesLikeTheGame(t *testing.T) {
	want := [][4]int32{
		{1812433254, 1900727103, -603986212, -603982422},
		{1900727103, -603986212, -603982422, -1948050678},
		{-603986212, -603982422, -1948050678, -1395185030},
		{-603982422, -1948050678, -1395185030, -2009403334},
		{-1948050678, -1395185030, -2009403334, -1404299726},
	}

	r := New(1)
	for i, w := range want {
		r.next()
		if got := r.state(); got != w {
			t.Fatalf("step %d: state = %v, want %v", i+1, got, w)
		}
	}
}

// Compared as raw bits, not decimals.
//
// The divisor is 2^23-1 rather than 2^23, a difference of one part in eight
// million. Both round to the same shortest decimal at float32 precision, so a
// test written against printed numbers cannot tell them apart — and the wrong
// one desynchronises every river drawn afterwards.
func TestValue(t *testing.T) {
	want := []uint32{0x3F7FEB56, 0x3F463616, 0x3F2E54F5, 0x3EEBC0EA, 0x3F182C65}

	r := New(1)
	for i, w := range want {
		if got := math.Float32bits(r.Value()); got != w {
			t.Errorf("value %d bits = %08X, want %08X", i, got, w)
		}
	}
}

// Range(int, int) takes the raw word modulo the span. Scaling value into the
// interval — the obvious implementation — produces a plausible but different
// sequence, so this pins the actual one.
func TestRangeInt(t *testing.T) {
	want := []int32{-5126, 6618, -7734, -6038}

	r := New(1)
	for i, w := range want {
		if got := r.RangeInt(-10000, 10000); got != w {
			t.Errorf("RangeInt %d = %d, want %d", i, got, w)
		}
	}
}

// Bits again, and here it is not a formality.
//
// The first draw is the one that discriminates: min*v+max*(1-v),
// max+(min-max)*v, min+(max-min)*(1-v) and max-(max-min)*v are the same
// expression in algebra and four different roundings in float32. All four agree
// with the game on draws 2 and 3. Only one agrees on draw 1.
//
// So a test that sampled a couple of values, or compared to five decimal places,
// would have accepted three wrong implementations.
func TestRangeFloat(t *testing.T) {
	want := []uint32{0x42700CEB, 0x428A0F19, 0x42918574}

	r := New(1)
	for i, w := range want {
		if got := math.Float32bits(r.RangeFloat(60, 100)); got != w {
			t.Errorf("RangeFloat %d bits = %08X, want %08X", i, got, w)
		}
	}
}

// The one that matters: WorldGenerator seeds with the world seed and makes
// exactly these seven calls. Reproducing them means every per-world constant —
// including the river and stream seeds everything downstream depends on — can
// be computed instead of read out of a running server.
//
// The expected values are npcdev's, independently measured long before this
// package existed and already checked into internal/protocol.
func TestWorldConstantsForNpcdev(t *testing.T) {
	r := New(-1365230248)

	got := struct {
		offset0, offset1, offset2, offset3 int32
		riverSeed, streamSeed              int32
		offset4                            int32
	}{
		offset0:    r.RangeInt(-10000, 10000),
		offset1:    r.RangeInt(-10000, 10000),
		offset2:    r.RangeInt(-10000, 10000),
		offset3:    r.RangeInt(-10000, 10000),
		riverSeed:  r.RangeInt(-2147483648, 2147483647),
		streamSeed: r.RangeInt(-2147483648, 2147483647),
		offset4:    r.RangeInt(-10000, 10000),
	}

	for _, tc := range []struct {
		name      string
		got, want int32
	}{
		{"offset0", got.offset0, 3178},
		{"offset1", got.offset1, -8412},
		{"offset2", got.offset2, 3297},
		{"offset3", got.offset3, 8915},
		{"riverSeed", got.riverSeed, 328821593},
		{"streamSeed", got.streamSeed, -1078268828},
		{"offset4", got.offset4, -8503},
	} {
		if tc.got != tc.want {
			t.Errorf("%s = %d, want %d", tc.name, tc.got, tc.want)
		}
	}
}

// Range over the full int32 span is the call that draws the river seeds, and it
// is where a signed max-min would overflow.
func TestRangeIntSpansTheWholeInt32Range(t *testing.T) {
	r := New(-1365230248)
	for i := 0; i < 4; i++ {
		r.RangeInt(-10000, 10000)
	}

	if got := r.RangeInt(-2147483648, 2147483647); got != 328821593 {
		t.Errorf("full-range RangeInt = %d, want 328821593", got)
	}
}

func TestRangeIntEmptySpan(t *testing.T) {
	if got := New(1).RangeInt(5, 5); got != 5 {
		t.Errorf("RangeInt(5,5) = %d, want 5", got)
	}
}
