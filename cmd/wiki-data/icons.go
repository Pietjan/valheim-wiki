package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/pietjan/valheim-wiki/internal/assets"
)

// extractIcons is the sprite pass: it writes every sprite the earlier
// extractors asked for with WantSprite as assets/icons/<kind>/<name>.png
// and records in the DB which files exist, so the site never links an
// icon that was not written. The map kind also gets every mapicon_*
// sprite and map_marker of the UI atlas, found by name, so the site has
// the game's pins (bed, portal, death, boss, ...) whatever the Minimap
// component lists. Files are only ever overwritten, never removed.
func extractIcons(x *X) error {
	if x.sprites["map"] == nil {
		x.sprites["map"] = map[string]assets.Ref{}
	}
	x.findMapSprites(x.sprites["map"])

	kinds := make([]string, 0, len(x.sprites))
	for k := range x.sprites {
		kinds = append(kinds, k)
	}
	sort.Strings(kinds)
	var report []string
	for _, kind := range kinds {
		wanted := map[assets.Ref][]string{}
		for name, ref := range x.sprites[kind] {
			wanted[ref] = append(wanted[ref], name)
		}
		if len(wanted) == 0 {
			continue
		}
		written, err := x.writeSprites(wanted, filepath.Join(x.IconDir, kind))
		if err != nil {
			return fmt.Errorf("%s: %w", kind, err)
		}
		names := make([]string, 0, len(written))
		for n := range written {
			names = append(names, n)
		}
		sort.Strings(names)
		x.DB.Icons[kind] = names
		x.reconcileIcons(kind, written)
		report = append(report, fmt.Sprintf("%s: %d of %d sprites", kind, len(written), len(x.sprites[kind])))
	}
	return x.writeProvenance(report)
}

// findMapSprites adds every sprite of the loaded bundles whose name is a
// map icon: mapicon_* and map_marker, the UI atlas's own names.
func (x *X) findMapSprites(into map[string]assets.Ref) {
	for _, cab := range x.Env.Order {
		f := x.Env.Files[cab]
		for _, id := range f.Order {
			if f.Objects[id].ClassID != 213 {
				continue
			}
			t := x.Env.Tree(cab, id)
			name := S(t, "m_Name")
			if strings.HasPrefix(name, "mapicon") || name == "map_marker" {
				if _, ok := into[name]; !ok {
					into[name] = assets.Ref{Cab: cab, ID: id}
				}
			}
		}
	}
}

// reconcileIcons sets the per-thing icon flags from what was written.
// An item's icons are <prefab>.png and <prefab>_<n>.png for its further
// variants or qualities.
func (x *X) reconcileIcons(kind string, written map[string]bool) {
	switch kind {
	case "items":
		for prefab, it := range x.DB.Items {
			n := 0
			if written[prefab] {
				n++
			}
			for i := 1; written[fmt.Sprintf("%s_%d", prefab, i)]; i++ {
				n++
			}
			it.Icons = n
		}
	case "pieces":
		for prefab, p := range x.DB.Pieces {
			p.Icon = written[prefab]
		}
	case "stations":
		for prefab, s := range x.DB.Stations {
			s.Icon = written[prefab]
		}
	case "effects":
		for id, e := range x.DB.Effects {
			e.Icon = written[id]
		}
	case "skills":
		for _, s := range x.DB.Skills {
			s.Icon = written[s.ID]
		}
	}
}

// writeProvenance records where the icons came from.
func (x *X) writeProvenance(report []string) error {
	if err := os.MkdirAll(x.IconDir, 0o755); err != nil {
		return err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Extracted %s by wiki-data from the Valheim client's asset bundles.\n", time.Now().Format(time.RFC3339))
	fmt.Fprintf(&b, "Source: %s\n", x.SoftRef)
	fmt.Fprintf(&b, "Unity: %s\n", x.unityVersion())
	b.WriteString("The sprites are Iron Gate's art, cropped from the game's atlases as the game draws them.\n")
	for _, r := range report {
		fmt.Fprintf(&b, "%s\n", r)
	}
	return os.WriteFile(filepath.Join(x.IconDir, "PROVENANCE.txt"), []byte(b.String()), 0o644)
}
