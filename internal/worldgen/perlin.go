// Package worldgen reproduces Valheim's terrain generation.
//
// The obstacle was Mathf.PerlinNoise: native Unity code with no published
// source. It cannot be approximated, because GetBaseHeight multiplies noise
// terms by each other and by running totals, so error compounds into a
// different world rather than a rougher one — and since 1.0 it cannot even be
// close: location placement classifies every point of a 2048² grid against
// fixed thresholds, and a point an ulp on the wrong side is a different biome,
// which lengthens that biome's point list by one and moves every location
// drawn from it.
//
// For a while it was reproduced by measurement — a table of "effective"
// gradients read out of the running game near each lattice point, good to
// about 1.5e-5. That table is what gave the game away: it held exactly eight
// directions, the axis ones 0.6743 long and the diagonals 0.9536, which are
// 1/1.483 and √2/1.483. Those are the gradients of Perlin's 2002 improved
// noise, and 0.69/1.483 is the value at a lattice point, so Unity's function
// is the reference implementation with its own output mapping on the end.
// Fitted against 9,400 bit-exact samples dumped from the game, the form below
// matches every one.
//
// See docs/terrain.md.
package worldgen

import "math"

// permutation is Ken Perlin's reference table, as published with the 2002
// improved noise, doubled so the hash lookups never wrap.
var permutation = func() [512]int {
	ref := [256]int{151, 160, 137, 91, 90, 15, 131, 13, 201, 95, 96, 53, 194, 233, 7, 225, 140, 36, 103, 30, 69, 142, 8, 99, 37, 240, 21, 10, 23, 190, 6, 148, 247, 120, 234, 75, 0, 26, 197, 62, 94, 252, 219, 203, 117, 35, 11, 32, 57, 177, 33, 88, 237, 149, 56, 87, 174, 20, 125, 136, 171, 168, 68, 175, 74, 165, 71, 134, 139, 48, 27, 166, 77, 146, 158, 231, 83, 111, 229, 122, 60, 211, 133, 230, 220, 105, 92, 41, 55, 46, 245, 40, 244, 102, 143, 54, 65, 25, 63, 161, 1, 216, 80, 73, 209, 76, 132, 187, 208, 89, 18, 169, 200, 196, 135, 130, 116, 188, 159, 86, 164, 100, 109, 198, 173, 186, 3, 64, 52, 217, 226, 250, 124, 123, 5, 202, 38, 147, 118, 126, 255, 82, 85, 212, 207, 206, 59, 227, 47, 16, 58, 17, 182, 189, 28, 42, 223, 183, 170, 213, 119, 248, 152, 2, 44, 154, 163, 70, 221, 153, 101, 155, 167, 43, 172, 9, 129, 22, 39, 253, 19, 98, 108, 110, 79, 113, 224, 232, 178, 185, 112, 104, 218, 246, 97, 228, 251, 34, 242, 193, 238, 210, 144, 12, 191, 179, 162, 241, 81, 51, 145, 235, 249, 14, 239, 107, 49, 192, 214, 31, 181, 199, 106, 157, 184, 84, 204, 176, 115, 121, 50, 45, 127, 4, 150, 254, 138, 236, 205, 93, 222, 114, 67, 29, 24, 72, 243, 141, 128, 195, 78, 66, 215, 61, 156, 180}
	var p [512]int
	for i := range p {
		p[i] = ref[i&255]
	}
	return p
}()

// perlinBase is Mathf.PerlinNoise at a lattice point: the noise is zero there,
// so this is the output mapping's constant, 0.69/1.483.
const perlinBase = float32(0.69) / 1.483

// fade is Perlin's improved interpolation curve, 6t⁵-15t⁴+10t³, in float32 and
// in the reference's operation order.
func fade(t float32) float32 { return t * t * t * (t*(t*6-15) + 10) }

func lerp32Perlin(t, a, b float32) float32 { return a + t*(b-a) }

// grad is the reference's gradient function with z fixed at zero: the twelve
// edge directions of the cube collapse to the eight of the square, the axis
// ones counted more often than the diagonals.
func grad(hash int, x, y float32) float32 {
	h := hash & 15
	u, v := x, y
	if h >= 8 {
		u = y
	}
	if h >= 4 {
		if h == 12 || h == 14 {
			v = x
		} else {
			v = 0
		}
	}
	if h&1 != 0 {
		u = -u
	}
	if h&2 != 0 {
		v = -v
	}
	return u + v
}

// PerlinNoise reproduces UnityEngine.Mathf.PerlinNoise, bit for bit.
//
// Three things about it are Unity's rather than Perlin's, and each was
// found by fitting rather than read anywhere:
//
//   - The inputs are taken as absolute values, so the function is mirrored
//     about each axis: noise(-x, y) == noise(x, y).
//   - The 2D noise is the 3D reference at z = 0: the lattice hash goes
//     through the permutation once more than a 2D version would, which
//     selects different gradients from the same table.
//   - The [-1, 1] result is mapped by (n + 0.69) / 1.483, in float32.
func PerlinNoise(x, y float32) float32 {
	if x < 0 {
		x = -x
	}
	if y < 0 {
		y = -y
	}

	// Truncation and floor agree for non-negative inputs; the period of 256
	// comes from the mask.
	fx, fy := float32(math.Floor(float64(x))), float32(math.Floor(float64(y)))
	ix, iy := int(fx)&255, int(fy)&255
	x -= fx
	y -= fy

	u, v := fade(x), fade(y)

	p := &permutation
	a := p[ix] + iy
	aa, ab := p[p[a]], p[p[a+1]]
	b := p[ix+1] + iy
	ba, bb := p[p[b]], p[p[b+1]]

	n := lerp32Perlin(v,
		lerp32Perlin(u, grad(aa, x, y), grad(ba, x-1, y)),
		lerp32Perlin(u, grad(ab, x, y-1), grad(bb, x-1, y-1)))

	return (n + 0.69) / 1.483
}
