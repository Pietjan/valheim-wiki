// Command wiki-data reads Valheim's asset bundles and writes the wiki's
// data (data/*.json) and icons (assets/icons/**). It needs the game
// client: -softref points at valheim_Data/StreamingAssets/SoftRef.
//
// The mapping is the game's own: the components on the prefabs, the
// Recipe and StatusEffect assets, the lists on the system prefabs
// (_GameMain, _ZoneCtrl, _SpawnList_*, _LocationList_*), and the English
// text from resources.assets. Each domain is one extractor in its own
// file, registered in extractors below and run in order; later ones may
// read what earlier ones wrote into the DB.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/pietjan/valheim-wiki/internal/model"
)

// extractor fills one domain of the DB.
type extractor struct {
	name string
	run  func(x *X) error
}

// extractors in run order. Items first because almost everything else
// names items; locations and rooms last because they count what the
// earlier passes defined.
var extractors = []extractor{
	{"items", extractItems},
	{"recipes", extractRecipes},
	{"stations", extractStations},
	{"converters", extractConverters},
	{"traders", extractTraders},
	{"pieces", extractPieces},
	{"effects", extractEffects},
	{"skills", extractSkills},
	{"creatures", extractCreatures},
	{"spawns", extractSpawns},
	{"events", extractEvents},
	{"resources", extractResources},
	{"vegetation", extractVegetation},
	{"biomes", extractBiomes},
	{"altbiomes", extractAltBiomes},
	{"locations", extractLocations},
	{"rooms", extractRooms},
	{"names", fillNames},
	{"icons", extractIcons},
}

func main() {
	softRef := flag.String("softref", "", "the client's valheim_Data/StreamingAssets/SoftRef")
	dataDir := flag.String("data", "", "the client's valheim_Data (for resources.assets); default: two up from -softref")
	out := flag.String("out", "data", "output directory for the JSON")
	icons := flag.String("icons", "assets/icons", "output directory for the icons")
	game := flag.String("game", "", "the game version the bundles are from, recorded in meta.json")
	only := flag.String("only", "", "comma-separated extractor names to run (default all)")
	noIcons := flag.Bool("no-icons", false, "skip the icon pass")
	probe := flag.String("probe", "", "comma-separated prefab or asset names: dump every component's fields to -probe-out and exit")
	probeOut := flag.String("probe-out", "probe.txt", "where -probe writes")
	list := flag.Bool("list-bundles", false, "print which bundles would be loaded and exit")
	flag.Parse()
	if *softRef == "" {
		flag.Usage()
		os.Exit(2)
	}
	if *dataDir == "" {
		*dataDir = filepath.Dir(filepath.Dir(*softRef))
	}
	if err := run(*softRef, *dataDir, *out, *icons, *game, *only, !*noIcons, *probe, *probeOut, *list); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(softRef, dataDir, out, iconDir, game, only string, icons bool, probe, probeOut string, list bool) error {
	started := time.Now()
	x, err := open(softRef, dataDir, list)
	if err != nil {
		return err
	}
	if list {
		return nil
	}
	x.IconDir = iconDir
	x.memStat()
	fmt.Printf("==> %d serialised files, %d root prefabs, %d strings in %.0fs\n", len(x.Env.Files), len(x.Roots), len(x.Strings), time.Since(started).Seconds())

	if probe != "" {
		names := map[string]bool{}
		for _, n := range strings.Split(probe, ",") {
			names[strings.TrimSpace(n)] = true
		}
		return x.probe(names, probeOut)
	}

	want := map[string]bool{}
	for _, n := range strings.Split(only, ",") {
		if n = strings.TrimSpace(n); n != "" {
			want[n] = true
		}
	}
	for _, e := range extractors {
		if len(want) > 0 && !want[e.name] {
			continue
		}
		if e.name == "icons" && !icons {
			continue
		}
		t := time.Now()
		if err := e.run(x); err != nil {
			return fmt.Errorf("%s: %w", e.name, err)
		}
		fmt.Printf("==> %-11s %s (%.1fs)\n", e.name, x.summary(e.name), time.Since(t).Seconds())
	}

	x.DB.Meta = model.Meta{
		Game:      game,
		Extracted: time.Now().Format("2006-01-02"),
		Source:    "Valheim client, SoftRef bundles",
		Unity:     x.unityVersion(),
	}
	if err := x.DB.Write(out); err != nil {
		return err
	}
	fmt.Printf("==> wrote %s in %.0fs\n", out, time.Since(started).Seconds())
	return nil
}

// summary is a one-line count for the log.
func (x *X) summary(name string) string {
	db := x.DB
	switch name {
	case "items":
		return fmt.Sprintf("%d items", len(db.Items))
	case "recipes":
		return fmt.Sprintf("%d recipes", len(db.Recipes))
	case "stations":
		return fmt.Sprintf("%d stations", len(db.Stations))
	case "converters":
		return fmt.Sprintf("%d converters", len(db.Converters))
	case "traders":
		return fmt.Sprintf("%d traders", len(db.Traders))
	case "pieces":
		return fmt.Sprintf("%d pieces, %d tables", len(db.Pieces), len(db.PieceTables))
	case "effects":
		return fmt.Sprintf("%d effects", len(db.Effects))
	case "skills":
		return fmt.Sprintf("%d skills", len(db.Skills))
	case "creatures":
		return fmt.Sprintf("%d creatures", len(db.Creatures))
	case "spawns":
		return fmt.Sprintf("%d spawns", len(db.Spawns))
	case "events":
		return fmt.Sprintf("%d events", len(db.Events))
	case "resources":
		return fmt.Sprintf("%d resources", len(db.Resources))
	case "vegetation":
		return fmt.Sprintf("%d rules, %d clutter", len(db.Vegetation), len(db.Clutter))
	case "biomes":
		return fmt.Sprintf("%d biomes, %d environments", len(db.Biomes), len(db.Environments))
	case "altbiomes":
		return fmt.Sprintf("%d alt biomes", len(db.AltBiomes))
	case "locations":
		return fmt.Sprintf("%d locations", len(db.Locations))
	case "rooms":
		return fmt.Sprintf("%d rooms", len(db.Rooms))
	case "icons":
		n := 0
		var kinds []string
		for k, v := range db.Icons {
			n += len(v)
			kinds = append(kinds, fmt.Sprintf("%s %d", k, len(v)))
		}
		sort.Strings(kinds)
		return fmt.Sprintf("%d icons (%s)", n, strings.Join(kinds, ", "))
	}
	return ""
}
