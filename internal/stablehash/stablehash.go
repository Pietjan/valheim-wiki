// Package stablehash reproduces Valheim's String.GetStableHashCode, the hash
// the game seeds its random streams with: the world seed is one, and every
// location prefab and alt biome mixes its name in through this.
package stablehash

import "unicode/utf16"

// Code reproduces Valheim's String.GetStableHashCode extension
// (assembly_utils/StringExtensionMethods.cs).
//
// It is a djb2 variant running two interleaved accumulators over the string's
// UTF-16 code units — even indices into one, odd into the other — combined at
// the end. Three details are load-bearing and easy to get wrong:
//
//   - It iterates UTF-16 code units, not bytes and not runes. Characters
//     outside the BMP occupy two units and are hashed as their surrogate pair.
//   - All arithmetic is int32 with silent wraparound, including the final
//     multiply.
//   - A NUL code unit terminates the loop early, so hashing stops at the first
//     zero rather than running to the end of the string.
func Code(s string) int32 {
	units := utf16.Encode([]rune(s))

	num := int32(5381)
	num2 := num

	for i := 0; i < len(units) && units[i] != 0; i += 2 {
		num = ((num << 5) + num) ^ int32(units[i])
		if i == len(units)-1 || units[i+1] == 0 {
			break
		}
		num2 = ((num2 << 5) + num2) ^ int32(units[i+1])
	}

	return num + num2*1566083941
}
