package main

import (
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
)

// loadFloors reads how deep each location digs, from valheim-world's
// sites.tsv: the TerrainModifier components on the location prefab. A
// level modifier sets the ground within its radius to its own height plus
// its offset; a smooth modifier pulls the ground toward its height. The
// lowest of those is the floor the icon draws down to, so a pond, a tar
// pit or a well shows its bottom while a crypt's buried hull stays
// buried. Locations that only raise the ground, or have no modifiers,
// draw down to y=0.
func loadFloors(path string) (map[string]float64, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	floors := map[string]float64{}
	for _, line := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(line, "#") || line == "" {
			continue
		}
		f := strings.Split(line, "\t")
		if len(f) < 5 || f[4] == "-" {
			continue
		}
		floor := 0.0
		for _, op := range strings.Split(f[4], ";") {
			v := strings.Split(op, ",")
			if len(v) < 11 {
				return nil, fmt.Errorf("%s: %s: terrain op %q has %d fields, want 11", path, f[0], op, len(v))
			}
			y, levelRadius, levelOffset, smoothRadius := num(v[1]), num(v[3]), num(v[4]), num(v[6])
			if levelRadius > 0 {
				floor = math.Min(floor, y+levelOffset)
			}
			if smoothRadius > 0 {
				floor = math.Min(floor, y)
			}
		}
		if floor < 0 {
			floors[f[0]] = floor
		}
	}
	return floors, nil
}

func num(s string) float64 {
	v, _ := strconv.ParseFloat(s, 64)
	return v
}
