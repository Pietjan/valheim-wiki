package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/pietjan/valheim-wiki/internal/assets"
	"github.com/pietjan/valheim-wiki/internal/model"
)

// extractLocations reads the ZoneLocation rules and the location prefabs.
//
// The rules are what ZoneSystem.SetupLocations concatenates at start-up:
// ZoneSystem.m_locations as serialised (the base game), then
// LocationList.m_locations of every _LocationList_* prefab in m_sortOrder
// order, then AltBiome.m_addLocations of every alt biome, keyed by the
// prefab's name (a later rule for the same prefab is ignored with an
// error in the log). The rule names its prefab twice: m_prefabName
// is a cached string that can be stale, m_prefab is a SoftReference
// whose asset ID the SoftRef manifest maps to a path, so the GUID wins.
//
// The prefab adds what the rule does not say: the Location component
// (interior, no-build, discovery label, enemy level overrides), the
// DungeonGenerator of an interior, and the contents counted by
// contentsOf. The map icon is the Minimap's own sprite for the prefab.
func extractLocations(x *X) error {
	guids, err := softGUIDs(x.SoftRef)
	if err != nil {
		return err
	}
	var rules []ruleSource
	seen := map[string]bool{}

	// ZoneSystem.m_locations: the base game's rules are serialised on
	// the ZoneSystem itself, and SetupLocations appends the lists to
	// them, so they come first. The bundles hold the component twice:
	// on the _ZoneSystem prefab and on its instance under _GameMain,
	// which is the one the game runs with its overrides, so it wins.
	type zone struct {
		root, name string
		rules      []any
	}
	var zones []zone
	x.EachMono([]string{"m_locations", "m_locationLists"}, func(cab string, id int64, t map[string]any) {
		root, g := x.Owner(cab, t)
		zones = append(zones, zone{root, x.Name(g), L(t, "m_locations")})
	})
	sort.SliceStable(zones, func(i, j int) bool { return zones[i].root == "_GameMain" && zones[j].root != "_GameMain" })
	for _, z := range zones {
		fmt.Printf("    %d rules on ZoneSystem %s in %s\n", len(z.rules), z.name, z.root)
		for _, r := range z.rules {
			rules = append(rules, ruleSource{assets.AsMap(r), z.name, ""})
		}
	}

	// LocationList components: m_locations beside m_sortOrder.
	type list struct {
		order int
		name  string
		rules []any
	}
	var lists []list
	x.EachMono([]string{"m_locations", "m_sortOrder"}, func(cab string, id int64, t map[string]any) {
		name, _ := x.Owner(cab, t)
		lists = append(lists, list{I(t, "m_sortOrder"), name, L(t, "m_locations")})
	})
	sort.SliceStable(lists, func(i, j int) bool {
		if lists[i].order != lists[j].order {
			return lists[i].order < lists[j].order
		}
		return lists[i].name < lists[j].name
	})
	for _, l := range lists {
		fmt.Printf("    %d rules in %s (sort order %d)\n", len(l.rules), l.name, l.order)
		for _, r := range l.rules {
			rules = append(rules, ruleSource{assets.AsMap(r), l.name, ""})
		}
	}

	// AltBiomeList.m_alts[].m_addLocations.
	x.EachMono([]string{"m_alts"}, func(cab string, id int64, t map[string]any) {
		listName, _ := x.Owner(cab, t)
		for _, a := range L(t, "m_alts") {
			am := assets.AsMap(a)
			for _, r := range L(am, "m_addLocations") {
				rules = append(rules, ruleSource{assets.AsMap(r), listName, S(am, "m_name")})
			}
		}
	})

	dropped := 0
	for _, r := range rules {
		// A rule whose SoftReference is unassigned (all-zero GUID) is a
		// legacy entry: SetupLocations registers a rule only when it is
		// enabled or its prefab is valid, so a disabled one without a
		// prefab never exists in the game, whatever stale m_prefabName it
		// still carries (Castle, TrollCave, SunkenCrypt1, ...).
		if guid := assetGUID(M(r.rule, "m_prefab")); guid == "" || guid == strings.Repeat("0", 32) {
			if !B(r.rule, "m_enable") {
				dropped++
				continue
			}
			fmt.Printf("    enabled rule %q in %s has no prefab assigned; keeping its cached name %q\n", S(r.rule, "m_name"), r.list, S(r.rule, "m_prefabName"))
		}
		prefab := x.softPrefab(guids, M(r.rule, "m_prefab"), S(r.rule, "m_prefabName"))
		if prefab == "" {
			fmt.Printf("    rule %q in %s has no prefab\n", S(r.rule, "m_name"), r.list)
			continue
		}
		if seen[prefab] {
			fmt.Printf("    duplicate rule for %s (%q in %s) ignored, as the game does\n", prefab, S(r.rule, "m_name"), r.list)
			continue
		}
		seen[prefab] = true
		loc := readRule(r.rule, prefab)
		loc.List = r.list
		loc.AltBiome = r.alt
		x.DB.Locations[prefab] = loc
	}

	fmt.Printf("    %d disabled rules without a prefab dropped, as the game does\n", dropped)

	for _, prefab := range model.SortedKeys(x.DB.Locations) {
		loc := x.DB.Locations[prefab]
		root, ok := x.locationRoot(prefab)
		if !ok {
			fmt.Printf("    %s: prefab not loaded\n", prefab)
		} else {
			x.readLocationPrefab(root, loc)
		}
		token := ""
		if r, ok := x.names[prefab]; ok && r.token != "-" {
			token = r.token
		} else if strings.HasPrefix(loc.DiscoverLabel, "$") {
			token = loc.DiscoverLabel
		}
		loc.Token = token
		loc.Name = x.NameOf(prefab, loc.DiscoverLabel)
		loc.DiscoverLabel = x.Text(loc.DiscoverLabel)
		loc.Kind = slug(loc.Name)
	}

	x.readMapIcons()
	return nil
}

// ruleSource is a ZoneLocation with where it came from.
type ruleSource struct {
	rule map[string]any
	list string
	alt  string
}

// readRule maps a ZoneSystem.ZoneLocation onto the model.
func readRule(r map[string]any, prefab string) *model.Location {
	loc := &model.Location{
		Prefab:                    prefab,
		Enabled:                   B(r, "m_enable"),
		Biomes:                    Biomes(I(r, "m_biome")),
		BiomeArea:                 locBiomeArea(I(r, "m_biomeArea")),
		Quantity:                  I(r, "m_quantity"),
		Prioritized:               B(r, "m_prioritized"),
		CenterFirst:               B(r, "m_centerFirst"),
		Unique:                    B(r, "m_unique"),
		Group:                     S(r, "m_group"),
		MinDistanceFromSimilar:    F(r, "m_minDistanceFromSimilar"),
		GroupMax:                  S(r, "m_groupMax"),
		MaxDistanceFromSimilar:    F(r, "m_maxDistanceFromSimilar"),
		IconAlways:                B(r, "m_iconAlways"),
		IconPlaced:                B(r, "m_iconPlaced"),
		RandomRotation:            B(r, "m_randomRotation"),
		SlopeRotation:             B(r, "m_slopeRotation"),
		SnapToWater:               B(r, "m_snapToWater"),
		InteriorRadius:            F(r, "m_interiorRadius"),
		ExteriorRadius:            F(r, "m_exteriorRadius"),
		ClearArea:                 B(r, "m_clearArea"),
		MinTerrainDelta:           F(r, "m_minTerrainDelta"),
		MaxTerrainDelta:           F(r, "m_maxTerrainDelta"),
		MinVegetation:             F(r, "m_minimumVegetation"),
		MaxVegetation:             F(r, "m_maximumVegetation"),
		InForest:                  B(r, "m_inForest"),
		ForestMin:                 F(r, "m_forestTresholdMin"),
		ForestMax:                 F(r, "m_forestTresholdMax"),
		MinDistanceFromCenter:     F(r, "m_minDistanceFromCenter"),
		MaxDistanceFromCenter:     F(r, "m_maxDistanceFromCenter"),
		MinDistance:               F(r, "m_minDistance"),
		MaxDistance:               F(r, "m_maxDistance"),
		MinAltitude:               F(r, "m_minAltitude"),
		MaxAltitude:               F(r, "m_maxAltitude"),
		SurroundCheckVegetation:   B(r, "m_surroundCheckVegetation"),
		SurroundCheckDistance:     F(r, "m_surroundCheckDistance"),
		SurroundCheckLayers:       I(r, "m_surroundCheckLayers"),
		SurroundBetterThanAverage: F(r, "m_surroundBetterThanAverage"),
	}
	if name := S(r, "m_name"); name != prefab {
		loc.Rule = name
	}
	return loc
}

// locBiomeArea names a ZoneLocation.m_biomeArea. The game tests it as a
// mask against the zone's area (Edge = 1 or Median = 2): the serialised
// values are 1, 2, 3, 6, 7 and -1 (every bit), and 0, which matches no
// zone, so that rule is never placed (GoblinCamp2_1).
func locBiomeArea(v int) string {
	switch v & 3 {
	case 1:
		return "Edge"
	case 2:
		return "Median"
	case 3:
		return "Everything"
	}
	return "None"
}

// locationRoot is the prefab's root with the Location component. A
// name may be held by several roots (the Bonemass location and the
// Bonemass creature), so the component decides, not the load order.
func (x *X) locationRoot(prefab string) (assets.Ref, bool) {
	for _, g := range x.RootsAll[prefab] {
		if _, ok := x.Find(g, "m_exteriorRadius", "m_clearArea"); ok {
			return g, true
		}
	}
	g, ok := x.Roots[prefab]
	if ok {
		fmt.Printf("    %s: no root with a Location component; reading %s as is\n", prefab, x.Name(g))
	}
	return g, ok
}

// readLocationPrefab fills what the prefab itself says.
func (x *X) readLocationPrefab(root assets.Ref, loc *model.Location) {
	if t, ok := x.Find(root, "m_exteriorRadius", "m_clearArea"); ok {
		loc.HasInterior = B(t.Tree, "m_hasInterior")
		loc.InteriorEnv = S(t.Tree, "m_interiorEnvironment")
		loc.NoBuild = B(t.Tree, "m_noBuild")
		loc.NoBuildRadius = F(t.Tree, "m_noBuildRadiusOverride")
		loc.ApplyRandomDamage = B(t.Tree, "m_applyRandomDamage")
		loc.DiscoverLabel = S(t.Tree, "m_discoverLabel")
		if v := I(t.Tree, "m_enemyMinLevelOverride"); v >= 0 {
			loc.EnemyMinLevel = v
		}
		if v := I(t.Tree, "m_enemyMaxLevelOverride"); v >= 0 {
			loc.EnemyMaxLevel = v
		}
		if v := F(t.Tree, "m_enemyLevelUpOverride"); v >= 0 {
			loc.EnemyLevelUp = v
		}
	}
	x.Walk(root, func(d int, g assets.Ref) bool {
		if loc.Dungeon != nil {
			return false
		}
		if t, ok := x.Find(g, "m_algorithm", "m_themes"); ok {
			loc.Dungeon = x.readDungeon(g, t.Tree)
			return false
		}
		return true
	})
	loc.Contents = x.contentsOf(root)
}

// readDungeon maps a DungeonGenerator onto the model.
func (x *X) readDungeon(g assets.Ref, t map[string]any) *model.Dungeon {
	d := &model.Dungeon{
		Prefab:            x.Name(g),
		Algorithm:         [...]string{"Dungeon", "CampGrid", "CampRadial"}[I(t, "m_algorithm")%3],
		Themes:            x.themeNames(I(t, "m_themes")),
		MinRooms:          I(t, "m_minRooms"),
		MaxRooms:          I(t, "m_maxRooms"),
		MinRequiredRooms:  I(t, "m_minRequiredRooms"),
		RequiredRooms:     Strs(t["m_requiredRooms"]),
		CampRadiusMin:     F(t, "m_campRadiusMin"),
		CampRadiusMax:     F(t, "m_campRadiusMax"),
		TileWidth:         F(t, "m_tileWidth"),
		GridSize:          I(t, "m_gridSize"),
		SpawnChance:       F(t, "m_spawnChance"),
		PerimeterSections: I(t, "m_perimeterSections"),
		DoorChance:        F(t, "m_doorChance"),
	}
	for _, dd := range L(t, "m_doorTypes") {
		dm := assets.AsMap(dd)
		prefab := x.GameObjectName(g.Cab, dm["m_prefab"])
		if prefab == "" {
			continue
		}
		d.Doors = append(d.Doors, model.Door{Prefab: prefab, ConnectionType: S(dm, "m_connectionType"), Chance: F(dm, "m_chance")})
	}
	return d
}

// readMapIcons reads Minimap.m_locationIcons: the sprite the map draws
// for a location whose rule has iconAlways or iconPlaced.
func (x *X) readMapIcons() {
	done := false
	x.EachMono([]string{"m_locationIcons", "m_icons"}, func(cab string, id int64, t map[string]any) {
		if done {
			return
		}
		icons := L(t, "m_locationIcons")
		if len(icons) == 0 {
			return
		}
		done = true
		for _, e := range icons {
			em := assets.AsMap(e)
			ref, ok := x.Deref(cab, em["m_icon"])
			if !ok {
				continue
			}
			sprite := S(x.Tree(ref), "m_Name")
			if loc, ok := x.DB.Locations[S(em, "m_name")]; ok && sprite != "" {
				loc.MapIcon = sprite
			}
			x.WantSprite("map", sprite, ref)
		}
	})
}

// softGUIDs reads the "asset locations:" section of SoftRef/manifest:
// asset ID -> path in bundle. The ID is the GUID string Unity prints,
// which assetGUID derives from the four serialised words.
func softGUIDs(softRef string) (map[string]string, error) {
	fh, err := os.Open(filepath.Join(softRef, "manifest"))
	if err != nil {
		return nil, err
	}
	defer fh.Close()
	out := map[string]string{}
	id := ""
	sc := bufio.NewScanner(fh)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		line := strings.TrimRight(strings.TrimPrefix(sc.Text(), "\ufeff"), "\r")
		if strings.HasPrefix(line, "- asset ID: ") {
			id = strings.TrimPrefix(line, "- asset ID: ")
		} else if strings.HasPrefix(line, "  path in bundle: ") && id != "" {
			out[id] = strings.TrimPrefix(line, "  path in bundle: ")
			id = ""
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%s: no 'asset locations' section; the manifest format has changed", filepath.Join(softRef, "manifest"))
	}
	return out, nil
}

// assetGUID is the manifest's spelling of a serialised AssetID: the
// four 32-bit words in reverse order, each as eight hex digits
// (a859725952215ba4bb2915451a24678d is v3 v2 v1 v0).
func assetGUID(id map[string]any) string {
	if s, ok := id["m_assetID"].(string); ok {
		return s
	}
	a := assets.AsMap(id["m_assetID"])
	if a == nil {
		return ""
	}
	return fmt.Sprintf("%08x%08x%08x%08x", assets.AsUint32(a["v3"]), assets.AsUint32(a["v2"]), assets.AsUint32(a["v1"]), assets.AsUint32(a["v0"]))
}

// softPrefab resolves a SoftReference<GameObject> to the prefab name:
// the manifest path's base name, or the cached name when the GUID is
// unknown (or all zero: no prefab assigned).
func (x *X) softPrefab(guids map[string]string, ref map[string]any, cached string) string {
	guid := assetGUID(ref)
	if p, ok := guids[guid]; ok {
		return strings.TrimSuffix(filepath.Base(p), ".prefab")
	}
	if guid != "" && guid != strings.Repeat("0", 32) {
		fmt.Printf("    asset %s (%q) not in the manifest; using the cached name\n", guid, cached)
	}
	return cached
}

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

// slug is the kebab-case of an English name: the grouping key that puts
// every variant of a location (WoodHouse1..13 "Abandoned house", Crypt2/
// 3/4 "Burial Chambers") on one page.
func slug(name string) string {
	return strings.Trim(nonSlug.ReplaceAllString(strings.ToLower(name), "-"), "-")
}
