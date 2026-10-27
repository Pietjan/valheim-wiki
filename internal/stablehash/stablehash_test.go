package stablehash

import "testing"

// Golden values produced by running Valheim's own GetStableHashCode under
// .NET 9. A wrong hash here is a wrong seed, and every map drawn from it is
// of some other world, so these are worth pinning exactly.
func TestCode(t *testing.T) {
	tests := []struct {
		in   string
		want int32
	}{
		{"", 371857150},
		{"a", 372029373},
		{"ab", 1093630535},
		{"PeerInfo", -725574882},
		{"CharacterID", 1191884308},
		{"Say", 696031011},
		// Exercises the UTF-16 path: 6 runes, 9 UTF-8 bytes, 6 UTF-16 units.
		{"naïve☃", 1387071926},
	}
	for _, tt := range tests {
		if got := Code(tt.in); got != tt.want {
			t.Errorf("Code(%q) = %d, want %d", tt.in, got, tt.want)
		}
	}
}

// A NUL terminates hashing early, so these must collide.
func TestCodeStopsAtNUL(t *testing.T) {
	if Code("abc\x00def") != Code("abc") {
		t.Error("hash did not stop at the NUL code unit")
	}
}
