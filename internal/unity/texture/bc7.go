package texture

import "image/color"

// BC7, the block format Valheim's icon atlas is compressed with: 128 bits
// per 4x4 block, one of eight modes, one to three subsets each with two
// endpoints, and per-pixel indices into the line between them. A port of
// the decoder in texture2ddecoder, which is what UnityPy uses, so the
// pixels agree with it exactly; the partition and anchor tables are the
// standard's, read out of that source rather than typed from memory.

// bc7Mode is one mode's layout.
type bc7Mode struct {
	subsets, partitionBits, rotationBits, indexSelectionBits uint8
	colorBits, alphaBits, endpointPBits, sharedPBits         uint8
	indexBits                                                [2]uint8
}

var bc7Modes = [8]bc7Mode{
	{3, 4, 0, 0, 4, 0, 1, 0, [2]uint8{3, 0}},
	{2, 6, 0, 0, 6, 0, 0, 1, [2]uint8{3, 0}},
	{3, 6, 0, 0, 5, 0, 0, 0, [2]uint8{2, 0}},
	{2, 6, 0, 0, 7, 0, 1, 0, [2]uint8{2, 0}},
	{1, 0, 2, 1, 5, 6, 0, 0, [2]uint8{2, 3}},
	{1, 0, 2, 0, 7, 8, 0, 0, [2]uint8{2, 2}},
	{1, 0, 0, 0, 7, 7, 1, 0, [2]uint8{4, 0}},
	{2, 6, 0, 0, 5, 5, 1, 0, [2]uint8{2, 0}},
}

// bc7Weights are the interpolation factors for 2-, 3- and 4-bit indices.
var bc7Weights = [3][16]uint8{
	{0, 21, 43, 64},
	{0, 9, 18, 27, 37, 46, 55, 64},
	{0, 4, 9, 13, 17, 21, 26, 30, 34, 38, 43, 47, 51, 55, 60, 64},
}

// bc7Partitions2 is the subset of each pixel for the 64 two-subset
// partitions, one bit per pixel.
var bc7Partitions2 = [64]uint16{
	0xcccc, 0x8888, 0xeeee, 0xecc8, 0xc880, 0xfeec, 0xfec8, 0xec80,
	0xc800, 0xffec, 0xfe80, 0xe800, 0xffe8, 0xff00, 0xfff0, 0xf000,
	0xf710, 0x008e, 0x7100, 0x08ce, 0x008c, 0x7310, 0x3100, 0x8cce,
	0x088c, 0x3110, 0x6666, 0x366c, 0x17e8, 0x0ff0, 0x718e, 0x399c,
	0xaaaa, 0xf0f0, 0x5a5a, 0x33cc, 0x3c3c, 0x55aa, 0x9696, 0xa55a,
	0x73ce, 0x13c8, 0x324c, 0x3bdc, 0x6996, 0xc33c, 0x9966, 0x0660,
	0x0272, 0x04e4, 0x4e40, 0x2720, 0xc936, 0x936c, 0x39c6, 0x639c,
	0x9336, 0x9cc6, 0x817e, 0xe718, 0xccf0, 0x0fcc, 0x7744, 0xee22,
}

// bc7Anchors2 is the anchor pixel of the second subset per partition.
var bc7Anchors2 = [64]uint8{
	15, 15, 15, 15, 15, 15, 15, 15, 15, 15, 15, 15, 15, 15, 15, 15,
	15, 2, 8, 2, 2, 8, 8, 15, 2, 8, 2, 2, 8, 8, 2, 2,
	15, 15, 6, 8, 2, 8, 15, 15, 2, 8, 2, 2, 2, 15, 15, 6,
	6, 2, 6, 8, 15, 15, 2, 2, 15, 15, 15, 15, 15, 2, 2, 15,
}

// bc7Partitions3 is the subset of each pixel for the 64 three-subset
// partitions, two bits per pixel.
var bc7Partitions3 = [64]uint32{
	0xaa685050, 0x6a5a5040, 0x5a5a4200, 0x5450a0a8, 0xa5a50000, 0xa0a05050, 0x5555a0a0, 0x5a5a5050,
	0xaa550000, 0xaa555500, 0xaaaa5500, 0x90909090, 0x94949494, 0xa4a4a4a4, 0xa9a59450, 0x2a0a4250,
	0xa5945040, 0x0a425054, 0xa5a5a500, 0x55a0a0a0, 0xa8a85454, 0x6a6a4040, 0xa4a45000, 0x1a1a0500,
	0x0050a4a4, 0xaaa59090, 0x14696914, 0x69691400, 0xa08585a0, 0xaa821414, 0x50a4a450, 0x6a5a0200,
	0xa9a58000, 0x5090a0a8, 0xa8a09050, 0x24242424, 0x00aa5500, 0x24924924, 0x24499224, 0x50a50a50,
	0x500aa550, 0xaaaa4444, 0x66660000, 0xa5a0a5a0, 0x50a050a0, 0x69286928, 0x44aaaa44, 0x66666600,
	0xaa444444, 0x54a854a8, 0x95809580, 0x96969600, 0xa85454a8, 0x80959580, 0xaa141414, 0x96960000,
	0xaaaa1414, 0xa05050a0, 0xa0a5a5a0, 0x96000000, 0x40804080, 0xa9a8a9a8, 0xaaaaaa44, 0x2a4a5254,
}

// bc7Anchors3 is the anchor pixel of the second and third subsets per
// partition.
var bc7Anchors3 = [2][64]uint8{
	{
		3, 3, 15, 15, 8, 3, 15, 15, 8, 8, 6, 6, 6, 5, 3, 3,
		3, 3, 8, 15, 3, 3, 6, 10, 5, 8, 8, 6, 8, 5, 15, 15,
		8, 15, 3, 5, 6, 10, 8, 15, 15, 3, 15, 5, 15, 15, 15, 15,
		3, 15, 5, 5, 5, 8, 5, 10, 5, 10, 8, 13, 15, 12, 3, 3,
	},
	{
		15, 8, 8, 3, 15, 15, 3, 8, 15, 15, 15, 15, 15, 15, 15, 8,
		15, 8, 15, 3, 15, 8, 15, 8, 3, 15, 6, 10, 15, 15, 10, 8,
		15, 3, 15, 10, 10, 8, 9, 10, 6, 15, 8, 15, 3, 6, 6, 8,
		15, 3, 15, 15, 15, 15, 15, 15, 15, 15, 15, 15, 3, 15, 15, 8,
	},
}

// bits reads a block little-endian, low bit first.
type bits struct {
	b   []byte
	pos int
}

func (r *bits) read(n uint8) uint32 {
	v := r.peek(0, n)
	r.pos += int(n)
	return v
}

func (r *bits) peek(offset int, n uint8) uint32 {
	if n == 0 {
		return 0
	}
	at := r.pos + offset
	var v uint32
	for i := 0; i < int(n); i++ {
		bit := at + i
		if bit>>3 < len(r.b) && r.b[bit>>3]&(1<<uint(bit&7)) != 0 {
			v |= 1 << uint(i)
		}
	}
	return v
}

func expandQuantized(v uint8, bits uint8) uint8 {
	v <<= 8 - bits
	return v | v>>bits
}

func decodeBC7(b []byte, out *[16]color.NRGBA) {
	r := &bits{b: b[:16]}
	mode := 0
	for mode < 8 && r.read(1) == 0 {
		mode++
	}
	if mode == 8 {
		*out = [16]color.NRGBA{}
		return
	}
	mi := bc7Modes[mode]
	pBits := mi.endpointPBits
	if pBits == 0 {
		pBits = mi.sharedPBits
	}
	partition := uint8(r.read(mi.partitionBits))
	rotation := uint8(r.read(mi.rotationBits))
	indexSelection := uint8(r.read(mi.indexSelectionBits))

	var epR, epG, epB, epA [6]uint8
	for i := 0; i < int(mi.subsets); i++ {
		epR[i*2] = uint8(r.read(mi.colorBits) << pBits)
		epR[i*2+1] = uint8(r.read(mi.colorBits) << pBits)
	}
	for i := 0; i < int(mi.subsets); i++ {
		epG[i*2] = uint8(r.read(mi.colorBits) << pBits)
		epG[i*2+1] = uint8(r.read(mi.colorBits) << pBits)
	}
	for i := 0; i < int(mi.subsets); i++ {
		epB[i*2] = uint8(r.read(mi.colorBits) << pBits)
		epB[i*2+1] = uint8(r.read(mi.colorBits) << pBits)
	}
	if mi.alphaBits != 0 {
		for i := 0; i < int(mi.subsets); i++ {
			epA[i*2] = uint8(r.read(mi.alphaBits) << pBits)
			epA[i*2+1] = uint8(r.read(mi.alphaBits) << pBits)
		}
	} else {
		for i := range epA {
			epA[i] = 0xff
		}
	}
	if pBits != 0 {
		for i := 0; i < int(mi.subsets); i++ {
			pa := uint8(r.read(pBits))
			pb := pa
			if mi.sharedPBits == 0 {
				pb = uint8(r.read(pBits))
			}
			epR[i*2] |= pa
			epR[i*2+1] |= pb
			epG[i*2] |= pa
			epG[i*2+1] |= pb
			epB[i*2] |= pa
			epB[i*2+1] |= pb
			epA[i*2] |= pa
			epA[i*2+1] |= pb
		}
	}
	colorBits := mi.colorBits + pBits
	for i := 0; i < int(mi.subsets)*2; i++ {
		epR[i] = expandQuantized(epR[i], colorBits)
		epG[i] = expandQuantized(epG[i], colorBits)
		epB[i] = expandQuantized(epB[i], colorBits)
	}
	if mi.alphaBits != 0 {
		alphaBits := mi.alphaBits + pBits
		for i := 0; i < int(mi.subsets)*2; i++ {
			epA[i] = expandQuantized(epA[i], alphaBits)
		}
	}

	hasSecond := mi.indexBits[1] != 0
	factors := [2]*[16]uint8{&bc7Weights[mi.indexBits[0]-2], nil}
	if hasSecond {
		factors[1] = &bc7Weights[mi.indexBits[1]-2]
	} else {
		factors[1] = factors[0]
	}
	offset := [2]int{0, int(mi.subsets) * (16*int(mi.indexBits[0]) - 1)}

	for idx := 0; idx < 16; idx++ {
		var subset, anchorAt uint8
		switch mi.subsets {
		case 2:
			subset = uint8((bc7Partitions2[partition] >> uint(idx)) & 1)
			if subset != 0 {
				anchorAt = bc7Anchors2[partition]
			}
		case 3:
			subset = uint8((bc7Partitions3[partition] >> uint(2*idx)) & 3)
			if subset != 0 {
				anchorAt = bc7Anchors3[subset-1][partition]
			}
		}
		var anchor uint8
		if uint8(idx) == anchorAt {
			anchor = 1
		}
		num := [2]uint8{mi.indexBits[0] - anchor, 0}
		if hasSecond {
			num[1] = mi.indexBits[1] - anchor
		}
		var index [2]uint8
		index[0] = uint8(r.peek(offset[0], num[0]))
		if hasSecond {
			index[1] = uint8(r.peek(offset[1], num[1]))
		} else {
			index[1] = index[0]
		}
		offset[0] += int(num[0])
		offset[1] += int(num[1])

		fc := factors[indexSelection][index[indexSelection]]
		fa := factors[1-indexSelection][index[1-indexSelection]]
		fca, fcb := uint16(64-fc), uint16(fc)
		faa, fab := uint16(64-fa), uint16(fa)
		s := int(subset) * 2
		rr := uint8((uint16(epR[s])*fca + uint16(epR[s+1])*fcb + 32) >> 6)
		gg := uint8((uint16(epG[s])*fca + uint16(epG[s+1])*fcb + 32) >> 6)
		bb := uint8((uint16(epB[s])*fca + uint16(epB[s+1])*fcb + 32) >> 6)
		aa := uint8((uint16(epA[s])*faa + uint16(epA[s+1])*fab + 32) >> 6)
		switch rotation {
		case 1:
			aa, rr = rr, aa
		case 2:
			aa, gg = gg, aa
		case 3:
			aa, bb = bb, aa
		}
		out[idx] = color.NRGBA{rr, gg, bb, aa}
	}
}
