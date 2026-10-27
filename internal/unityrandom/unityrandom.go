// Package unityrandom reproduces UnityEngine.Random.
//
// Valheim derives its per-world constants from it — the five terrain offsets,
// the river seed and the stream seed — and then generates every river and
// stream from those seeds. Terrain cannot be computed outside the game without
// it, which is why the offsets were previously read out of a running server and
// why rivers were not ported at all.
//
// Unity's generator is xorshift128, and it is native code with no documented
// specification. Everything here was derived from a state trace taken out of the
// running game rather than from an algorithm found elsewhere: the internal state
// was captured after InitState and after every call, so each step could be
// checked rather than inferred from outputs alone.
//
// The three mappings are all measured, and none of them is the obvious guess:
//
//   - Range(int, int) takes the raw generator word modulo the span. It is not
//     value scaled into the interval, which would give different numbers.
//   - Range(float, float) interpolates by 1-value, so it runs *backwards*
//     relative to value.
//   - value keeps the low 23 bits, the float32 mantissa width.
//
// The port is verified against the constants of a real world: seeding with
// npcdev's seed and making the seven calls WorldGenerator makes must reproduce
// the offsets and both river seeds already measured from the game.
package unityrandom

import "math"

// Random is a UnityEngine.Random sequence. Not safe for concurrent use, and
// deliberately not a global: Valheim saves and restores the global state around
// world generation, so a shared instance would be wrong here.
type Random struct {
	s0, s1, s2, s3 uint32
}

// initMultiplier is Knuth's MMIX-style constant, the same one Mersenne Twister
// uses to expand a single seed word into a full state.
const initMultiplier = 1812433253

// New seeds a generator the way UnityEngine.Random.InitState does: the seed
// becomes the first state word, and the rest are stretched from it.
func New(seed int32) *Random {
	r := &Random{s0: uint32(seed)}
	r.s1 = r.s0*initMultiplier + 1
	r.s2 = r.s1*initMultiplier + 1
	r.s3 = r.s2*initMultiplier + 1
	return r
}

// next advances the state one xorshift128 step and returns the new word.
func (r *Random) next() uint32 {
	t := r.s0
	t ^= t << 11
	t ^= t >> 8

	r.s0, r.s1, r.s2 = r.s1, r.s2, r.s3
	r.s3 = r.s3 ^ (r.s3 >> 19) ^ t

	return r.s3
}

// Value is UnityEngine.Random.value: a float32 in [0, 1].
func (r *Random) Value() float32 {
	// Divided by 2^23-1, not 2^23 — so the range is closed at both ends and an
	// all-ones draw returns exactly 1.0. The difference is one part in eight
	// million, invisible in decimal at float32 precision and confirmed only by
	// comparing raw bits against the game.
	const mask = 1<<23 - 1
	return float32(r.next()&mask) / float32(mask)
}

// RangeInt is UnityEngine.Random.Range(int, int): min inclusive, max exclusive.
func (r *Random) RangeInt(min, max int32) int32 {
	// Computed in uint32 so the full int32 range works. Valheim asks for
	// Range(int.MinValue, int.MaxValue) when drawing the river seeds, and a
	// signed max-min would overflow on exactly that call.
	span := uint32(max) - uint32(min)
	if span == 0 {
		return min
	}
	return int32(uint32(min) + r.next()%span)
}

// RangeFloat is UnityEngine.Random.Range(float, float).
func (r *Random) RangeFloat(min, max float32) float32 {
	v := r.Value()

	// Two things here are load-bearing, and neither is guessable.
	//
	// The direction: a value near 1 gives a result near min, so this runs
	// opposite to the obvious Lerp(min, max, value). That alone produces the
	// same distribution and a different sequence — statistically perfect,
	// and every river in the wrong place.
	//
	// The arrangement: min*v + max*(1-v) is algebraically identical to
	// max + (min-max)*v and to min + (max-min)*(1-v), but not identical in
	// float32. Those forms round differently, and on measured draws they
	// disagree with the game in the last bit. Written this way they match it
	// exactly.
	return min*v + max*(1-v)
}

// InsideUnitCircle is UnityEngine.Random.insideUnitCircle: a point in the unit
// disc, as two draws.
//
// Native code, so measured rather than read: the state before each call and
// the point it returned were traced out of the game for three seeds, and the
// only form that reproduces every one is an angle from Range(0, 2π) followed
// by a radius of Range(0, 1) to the power one half. The order matters, and
// so does using Range for the radius rather than value — the two run in
// opposite directions, and the polar form the felling code used before this
// existed gives the right distribution and the wrong points.
func (r *Random) InsideUnitCircle() (x, y float32) {
	angle := r.RangeFloat(0, 2*math.Pi)
	radius := float32(math.Pow(float64(r.RangeFloat(0, 1)), 0.5))
	return float32(math.Cos(float64(angle))) * radius, float32(math.Sin(float64(angle))) * radius
}
