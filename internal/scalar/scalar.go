// Package scalar is the handful of scalar helpers every package kept a copy of.
//
// abs32 was written three times, lerp four times in two argument orders and
// clamp01 three times, each package its own, and min32 and max32 outlived
// the language growing min and max. One definition each, over any float, so
// a reader of one package knows what the same word means in the next.
package scalar

// Float is what these helpers are written over.
type Float interface{ ~float32 | ~float64 }

// Abs is the magnitude of v.
func Abs[T Float](v T) T {
	if v < 0 {
		return -v
	}
	return v
}

// Lerp is the point t of the way from a to b: a at 0, b at 1, and beyond
// either end for a t outside them.
func Lerp[T Float](a, b, t T) T { return a + (b-a)*t }

// Clamp01 pins v to the unit interval.
func Clamp01[T Float](v T) T {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
