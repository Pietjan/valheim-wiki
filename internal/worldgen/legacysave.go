package worldgen

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
)

// A world save from before 1.0 — the single uncompressed .db the game wrote
// until then — holds the two lists an upgrade replay needs: which sites were
// built, and which zones. The file is mostly ZDOs, and reading it properly
// means reading all of them; this reads the ZoneSystem block at its tail
// instead, which is what the game's own LoadOld reads:
//
//	int zones; (int x, int y) × zones
//	int pgwVersion; int locationVersion
//	int keys; string × keys
//	bool locationsGenerated
//	int sites; (string prefab, float x, y, z, bool built) × sites
//
// The block is found from its sites, whose first entry is the first site
// the placement loop ever placed — StartTemple, the first prioritised
// rule — and read backwards from there: the keys, the version words, and
// then the zones, whose count is wherever a word says exactly how many
// pairs lie between it and the version words and the list it opens holds
// the spawn zone. Checked on npcdev's pre-upgrade save: 11,296 sites, 1,151
// built, 4,627 zones, and the replay from them lands 9,360 of 9,369 sites
// on the server's spot.

// ReadLegacySave reads the built sites and the generated zones out of a
// pre-1.0 world save, resolving each site's prefab against rules.
func ReadLegacySave(save []byte, rules []*LocationRule) (*Existing, error) {
	byPrefab := make(map[string]*LocationRule, len(rules))
	for _, r := range rules {
		if r.Prefab != "" {
			byPrefab[r.Prefab] = r
		}
	}

	const first = "StartTemple"
	needle := append([]byte{byte(len(first))}, first...)
	at := -1
	for from := 0; ; {
		i := bytes.Index(save[from:], needle)
		if i < 0 {
			break
		}
		i += from
		if i >= 4 {
			if n := int(int32(binary.LittleEndian.Uint32(save[i-4:]))); n > 0 && n < 1<<17 {
				at = i - 4
				break
			}
		}
		from = i + 1
	}
	if at < 0 {
		return nil, fmt.Errorf("legacy save: no site list found")
	}

	// Sites, forward.
	p := at
	count := int(int32(binary.LittleEndian.Uint32(save[p:])))
	p += 4
	ex := &Existing{Generated: make(map[Zone]bool)}
	for i := 0; i < count; i++ {
		if p >= len(save) {
			return nil, fmt.Errorf("legacy save: site list runs off the end")
		}
		n := int(save[p])
		p++
		if p+n+13 > len(save) {
			return nil, fmt.Errorf("legacy save: site %d runs off the end", i)
		}
		name := string(save[p : p+n])
		p += n
		pos := Vec3{
			math.Float32frombits(binary.LittleEndian.Uint32(save[p:])),
			math.Float32frombits(binary.LittleEndian.Uint32(save[p+4:])),
			math.Float32frombits(binary.LittleEndian.Uint32(save[p+8:])),
		}
		built := save[p+12] == 1
		p += 13
		if !built {
			continue
		}
		rule := byPrefab[name]
		if rule == nil {
			return nil, fmt.Errorf("legacy save: built site %q has no rule in the 1.0 list", name)
		}
		ex.Built = append(ex.Built, LocationInstance{Rule: rule, Zone: ZoneOf(pos), Position: pos})
	}

	// Backwards from the sites: the generated flag, the keys, the two
	// version words, and then the zones, whose count is found by the one
	// position at which it accounts for exactly the pairs up to the words.
	end := at - 1 // the generated flag
	keysAt := -1
	for q := end - 4; q > end-8192 && q >= 0; q-- {
		n := int(int32(binary.LittleEndian.Uint32(save[q:])))
		if n < 0 || n > 256 {
			continue
		}
		r := q + 4
		ok := true
		for k := 0; k < n && ok; k++ {
			if r >= end {
				ok = false
				break
			}
			ln := int(save[r])
			r += 1 + ln
		}
		if ok && r == end {
			keysAt = q
			break
		}
	}
	if keysAt < 0 {
		return nil, fmt.Errorf("legacy save: no key list before the sites")
	}
	wordsAt := keysAt - 8

	// A small count matches by accident a few pairs in — the bytes before
	// the list are ZDO data — so every match is tried and the longest list
	// that holds the spawn zone and reads as zones is the one. The game's
	// own list has junk in it too: npcdev's holds 81 zones near 15,625,
	// which the game narrows to short on load, and so does this.
	zonesAt := -1
	for n := 0; ; n++ {
		c := wordsAt - 4 - 8*n
		if c < 0 {
			break
		}
		if int(int32(binary.LittleEndian.Uint32(save[c:]))) != n {
			continue
		}
		spawn, sane := false, 0
		for i := 0; i < n; i++ {
			x := int32(binary.LittleEndian.Uint32(save[c+4+8*i:]))
			y := int32(binary.LittleEndian.Uint32(save[c+8+8*i:]))
			if x == 0 && y == 0 {
				spawn = true
			}
			if x >= -200 && x <= 200 && y >= -200 && y <= 200 {
				sane++
			}
		}
		if spawn && sane*10 >= n*9 {
			zonesAt = c
		}
	}
	if zonesAt < 0 {
		return nil, fmt.Errorf("legacy save: no zone list before the version words")
	}
	zones := int(int32(binary.LittleEndian.Uint32(save[zonesAt:])))
	for i := 0; i < zones; i++ {
		q := zonesAt + 4 + 8*i
		x := int32(binary.LittleEndian.Uint32(save[q:]))
		y := int32(binary.LittleEndian.Uint32(save[q+4:]))
		ex.Generated[Zone{int16(x), int16(y)}] = true
	}
	return ex, nil
}
