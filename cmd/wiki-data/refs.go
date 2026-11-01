package main

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/pietjan/valheim-wiki/internal/assets"
)

// refs prints every component field in the loaded bundles that points at
// a prefab of one of the names, by the prefab root the field sits in:
// what places, drops, sells or otherwise refers to a thing. A debugging
// aid, run with -refs.
func (x *X) refs(names map[string]bool) error {
	type hit struct{ owner, cab, path string }
	var hits []hit
	seen := map[string]bool{}
	var walk func(cab, owner string, v any, path string)
	walk = func(cab, owner string, v any, path string) {
		switch t := v.(type) {
		case map[string]any:
			if _, ok := t["m_PathID"]; ok && len(t) <= 2 {
				if assets.AsInt64(t["m_PathID"]) == 0 {
					return
				}
				if n := x.GameObjectName(cab, t); names[n] {
					h := hit{owner, cab, path}
					key := h.owner + "|" + h.path
					if !seen[key] {
						seen[key] = true
						hits = append(hits, h)
					}
				}
				return
			}
			for k, c := range t {
				walk(cab, owner, c, path+"."+k)
			}
		case []any:
			for i, c := range t {
				walk(cab, owner, c, fmt.Sprintf("%s[%d]", path, i))
			}
		}
	}
	for _, cab := range x.Env.Order {
		f := x.Env.Files[cab]
		for _, id := range f.Order {
			o := f.Objects[id]
			if o.ClassID != 114 {
				continue
			}
			t := x.Env.Tree(cab, id)
			if t == nil {
				continue
			}
			owner, _ := x.Owner(cab, t)
			if owner == "" {
				owner = "?"
			}
			if names[owner] {
				continue // the thing's own components
			}
			walk(cab, owner, t, "")
		}
	}
	sort.Slice(hits, func(i, j int) bool { return hits[i].owner+hits[i].path < hits[j].owner+hits[j].path })
	fmt.Fprintf(os.Stdout, "==> %d references to %s\n", len(hits), strings.Join(sortedNames(names), ", "))
	for _, h := range hits {
		fmt.Fprintf(os.Stdout, "  %s  %s  (%s)\n", h.owner, h.path, h.cab)
	}
	return nil
}

func sortedNames(m map[string]bool) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
