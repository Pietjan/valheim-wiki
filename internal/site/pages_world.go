package site

import (
	"sort"
	"strings"

	"github.com/pietjan/valheim-wiki/internal/model"
)

// BiomePage is a biome's page.
type BiomePage struct {
	*BiomeIndex
	Boss         *model.Creature
	BossLocation *model.Location
	Envs         []EnvView
	Materials    []*model.Item
	Pickables    []*model.Resource
	Trees        []*model.Resource
	Rocks        []*model.Resource
	Other        []*model.Resource
	Thing        *Thing
}

// EnvView is one weather with its share.
type EnvView struct {
	Env    *model.Env
	Name   string
	Share  float64
	Traits []string
}

func (s *Site) pagesBiomes() error {
	ix := s.Index
	var pages []*BiomePage
	for _, b := range s.DB.Biomes {
		bi := ix.ByBiome[b.ID]
		pg := &BiomePage{BiomeIndex: bi, Thing: ix.Things["biome:"+b.ID]}
		pg.Boss = s.DB.Creatures[b.Boss]
		pg.BossLocation = s.DB.Locations[b.BossLocation]
		// Weather: merge duplicate entries, share by weight.
		weights := map[string]float64{}
		var order []string
		total := 0.0
		for _, e := range b.Environments {
			if _, ok := weights[e.Env]; !ok {
				order = append(order, e.Env)
			}
			weights[e.Env] += e.Weight
			total += e.Weight
		}
		for _, name := range order {
			ev := EnvView{Env: s.DB.Environments[name], Name: name}
			if total > 0 {
				ev.Share = weights[name] / total
			}
			if ev.Env != nil {
				if ev.Env.Wet {
					ev.Traits = append(ev.Traits, "wet")
				}
				if ev.Env.Freezing {
					ev.Traits = append(ev.Traits, "freezing")
				} else if ev.Env.FreezingAtNight {
					ev.Traits = append(ev.Traits, "freezing at night")
				}
				if ev.Env.Cold {
					ev.Traits = append(ev.Traits, "cold")
				} else if ev.Env.ColdAtNight {
					ev.Traits = append(ev.Traits, "cold at night")
				}
				if ev.Env.AlwaysDark {
					ev.Traits = append(ev.Traits, "dark")
				}
			}
			pg.Envs = append(pg.Envs, ev)
		}
		sort.SliceStable(pg.Envs, func(i, j int) bool { return pg.Envs[i].Share > pg.Envs[j].Share })
		for _, r := range bi.Resources {
			switch r.Kind {
			case "pickable", "plant":
				pg.Pickables = append(pg.Pickables, r)
			case "tree", "log":
				pg.Trees = append(pg.Trees, r)
			case "rock":
				pg.Rocks = append(pg.Rocks, r)
			default:
				pg.Other = append(pg.Other, r)
			}
		}
		// Materials of the biome: what its creatures drop and its resources give.
		seen := map[string]bool{}
		add := func(prefab string) {
			it := s.DB.Items[prefab]
			if it == nil || seen[prefab] || it.Hidden || it.Type == "Trophy" {
				return
			}
			seen[prefab] = true
			pg.Materials = append(pg.Materials, it)
		}
		for _, r := range bi.Resources {
			if r.Item != "" {
				add(r.Item)
			}
			if r.Drops != nil {
				for _, d := range r.Drops.Drops {
					add(d.Item)
				}
			}
		}
		for _, c := range bi.Creatures {
			for _, d := range c.Drops {
				add(d.Item)
			}
		}
		sort.Slice(pg.Materials, func(i, j int) bool { return pg.Materials[i].Name < pg.Materials[j].Name })
		pages = append(pages, pg)
	}
	if err := s.write("biomes/", "biomes", &Page{Site: s, Title: "Biomes", Description: "The nine biomes of Valheim in order of danger: their creatures, resources, places, weather and bosses.", Section: "biomes", Path: "biomes/", Data: pages}); err != nil {
		return err
	}
	for _, pg := range pages {
		b := pg.Biome
		page := &Page{Site: s, Title: b.Name, Description: b.Name + " in Valheim: creatures, resources, locations, weather and the boss that rules it.", Section: "biomes", Path: "biomes/" + b.ID + "/",
			Crumbs: []Crumb{{"Biomes", "biomes/"}, {b.Name, ""}}, Data: pg, YourWorld: &YourWorld{Kind: "biome", Biomes: []string{b.ID}, Label: "the " + b.Name}}
		if err := s.write(page.Path, "biome", page); err != nil {
			return err
		}
	}
	return nil
}

// KindPage is a location kind's page.
type KindPage struct {
	Kind      *Kind
	Main      *model.Location // the variant whose rule describes the kind
	Contents  *model.Contents // merged over variants
	Rooms     []*model.Room   // dungeon rooms of its themes
	Creatures []*model.Creature
	Loot      []LootView
	Boss      *model.Creature
}

// LootView is one item found in the kind's chests, with where.
type LootView struct {
	Item   string
	Chests []string
	Min    int
	Max    int
}

func (s *Site) pagesLocations() error {
	ix := s.Index
	byBiome := map[string][]*Kind{}
	for _, k := range ix.KindList {
		for _, b := range k.Biomes {
			byBiome[b] = append(byBiome[b], k)
		}
	}
	var groups []struct {
		Biome string
		Name  string
		Kinds []*Kind
	}
	for _, b := range model.Biomes {
		if ks := byBiome[b]; len(ks) > 0 {
			groups = append(groups, struct {
				Biome string
				Name  string
				Kinds []*Kind
			}{b, model.BiomeName[b], ks})
		}
	}
	if err := s.write("locations/", "locations", &Page{Site: s, Title: "Locations", Description: "Every place the world generator puts in a Valheim world: dungeons, ruins, camps, altars and caves, by biome, with what they hold and how many a world gets.", Section: "locations", Path: "locations/",
		Data: map[string]any{"Groups": groups, "Count": len(ix.KindList)}}); err != nil {
		return err
	}
	roomsByTheme := map[string][]*model.Room{}
	for _, r := range s.DB.Rooms {
		if r.Enabled {
			roomsByTheme[r.Theme] = append(roomsByTheme[r.Theme], r)
		}
	}
	for _, k := range ix.KindList {
		pg := &KindPage{Kind: k, Main: k.Variants[0], Contents: mergeContents(k.Variants)}
		if pg.Contents != nil {
			pg.Contents.Pickables = s.mergeByName(pg.Contents.Pickables)
		}
		for _, v := range k.Variants {
			if v.Quantity > pg.Main.Quantity {
				pg.Main = v
			}
		}
		themes := map[string]bool{}
		for _, v := range k.Variants {
			if v.Dungeon != nil {
				for _, t := range v.Dungeon.Themes {
					themes[t] = true
				}
			}
		}
		for t := range themes {
			pg.Rooms = append(pg.Rooms, roomsByTheme[t]...)
		}
		sort.Slice(pg.Rooms, func(i, j int) bool { return pg.Rooms[i].Prefab < pg.Rooms[j].Prefab })
		// Creatures met here, from the kind's contents and its rooms.
		seenC := map[string]bool{}
		addC := func(p string) {
			if c := s.DB.Creatures[p]; c != nil && !seenC[p] {
				seenC[p] = true
				pg.Creatures = append(pg.Creatures, c)
			}
		}
		loot := map[string]*LootView{}
		addLoot := func(ch model.Chest) {
			if ch.Loot == nil {
				return
			}
			for _, d := range ch.Loot.Drops {
				lv := loot[d.Item]
				if lv == nil {
					lv = &LootView{Item: d.Item, Min: d.Min, Max: d.Max}
					loot[d.Item] = lv
				}
				if d.Min < lv.Min {
					lv.Min = d.Min
				}
				if d.Max > lv.Max {
					lv.Max = d.Max
				}
				if !contains(lv.Chests, ch.Prefab) {
					lv.Chests = append(lv.Chests, ch.Prefab)
				}
			}
		}
		collect := func(c *model.Contents) {
			if c == nil {
				return
			}
			for _, sp := range c.Spawners {
				addC(sp.Creature)
			}
			for _, n := range c.Nests {
				for _, e := range n.Creatures {
					addC(e.Creature)
				}
			}
			for _, cr := range c.Creatures {
				addC(cr.Creature)
			}
			for _, ch := range c.Chests {
				addLoot(ch)
			}
			if c.Offering != nil {
				pg.Boss = s.DB.Creatures[c.Offering.Boss]
			}
		}
		collect(pg.Contents)
		for _, r := range pg.Rooms {
			collect(r.Contents)
		}
		for _, item := range model.SortedKeys(loot) {
			pg.Loot = append(pg.Loot, *loot[item])
		}
		sort.Slice(pg.Loot, func(i, j int) bool {
			ni, nj := s.DB.Items[pg.Loot[i].Item], s.DB.Items[pg.Loot[j].Item]
			if ni == nil || nj == nil {
				return pg.Loot[i].Item < pg.Loot[j].Item
			}
			return ni.Name < nj.Name
		})
		var prefabs []string
		for _, v := range k.Variants {
			prefabs = append(prefabs, v.Prefab)
		}
		t := ix.Things[k.Variants[0].Prefab]
		page := &Page{Site: s, Title: k.Name, Description: k.Name + " in Valheim: which biome, how many per world, what lives and lies inside, and where to find one in your world.", Section: "locations", Path: "locations/" + k.Slug + "/", Icon: s.absIcon(t.Icon),
			Crumbs: []Crumb{{"Locations", "locations/"}, {k.Name, ""}}, Data: pg, YourWorld: &YourWorld{Kind: "location", Prefabs: prefabs, Label: strings.ToLower(k.Name)}}
		if err := s.write(page.Path, "location", page); err != nil {
			return err
		}
	}
	// Resources: an index and a page each.
	if err := s.pagesResources(); err != nil {
		return err
	}
	return nil
}

// mergeContents sums contents over a kind's variants.
func mergeContents(vs []*model.Location) *model.Contents {
	out := &model.Contents{}
	n := 0
	for _, v := range vs {
		if v.Contents == nil {
			continue
		}
		n++
		c := v.Contents
		out.Chests = mergeChests(out.Chests, c.Chests)
		out.Spawners = mergeSpawners(out.Spawners, c.Spawners)
		out.Nests = append(out.Nests, c.Nests...)
		out.Pickables = mergePickables(out.Pickables, c.Pickables)
		out.Loot = append(out.Loot, c.Loot...)
		out.Runestones = mergeRunestones(out.Runestones, c.Runestones)
		out.Vegvisirs = append(out.Vegvisirs, c.Vegvisirs...)
		if c.Offering != nil {
			out.Offering = c.Offering
		}
		out.Creatures = mergeCreaturesIn(out.Creatures, c.Creatures)
		out.Pieces = mergePiecesIn(out.Pieces, c.Pieces)
		out.Resources = mergeResourcesIn(out.Resources, c.Resources)
		out.Portal = out.Portal || c.Portal
	}
	if n == 0 {
		return nil
	}
	return out
}

func mergeChests(a, b []model.Chest) []model.Chest {
	for _, x := range b {
		found := false
		for i := range a {
			if a[i].Prefab == x.Prefab {
				if x.Count > a[i].Count {
					a[i].Count = x.Count
				}
				found = true
			}
		}
		if !found {
			a = append(a, x)
		}
	}
	return a
}

func mergeSpawners(a, b []model.Spawner) []model.Spawner {
	for _, x := range b {
		found := false
		for i := range a {
			if a[i].Creature == x.Creature && a[i].Respawn == x.Respawn {
				if x.Count > a[i].Count {
					a[i].Count = x.Count
				}
				found = true
			}
		}
		if !found {
			a = append(a, x)
		}
	}
	return a
}

func mergePickables(a, b []model.PickableIn) []model.PickableIn {
	for _, x := range b {
		found := false
		for i := range a {
			if a[i].Prefab == x.Prefab {
				if x.Count > a[i].Count {
					a[i].Count = x.Count
				}
				found = true
			}
		}
		if !found {
			a = append(a, x)
		}
	}
	return a
}

func mergeRunestones(a, b []model.Runestone) []model.Runestone {
	for _, x := range b {
		found := false
		for _, y := range a {
			if y.Text == x.Text && y.Label == x.Label {
				found = true
			}
		}
		if !found {
			a = append(a, x)
		}
	}
	return a
}

func mergeCreaturesIn(a, b []model.CreatureIn) []model.CreatureIn {
	for _, x := range b {
		found := false
		for i := range a {
			if a[i].Creature == x.Creature {
				if x.Count > a[i].Count {
					a[i].Count = x.Count
				}
				found = true
			}
		}
		if !found {
			a = append(a, x)
		}
	}
	return a
}

func mergePiecesIn(a, b []model.PieceIn) []model.PieceIn {
	for _, x := range b {
		found := false
		for i := range a {
			if a[i].Prefab == x.Prefab {
				if x.Count > a[i].Count {
					a[i].Count = x.Count
				}
				found = true
			}
		}
		if !found {
			a = append(a, x)
		}
	}
	return a
}

func mergeResourcesIn(a, b []model.ResourceIn) []model.ResourceIn {
	for _, x := range b {
		found := false
		for i := range a {
			if a[i].Prefab == x.Prefab {
				if x.Count > a[i].Count {
					a[i].Count = x.Count
				}
				found = true
			}
		}
		if !found {
			a = append(a, x)
		}
	}
	return a
}

// ResourcePage is a resource's page.
type ResourcePage struct {
	Resource *model.Resource
	Thing    *Thing
	Item     *model.Item
	Rules    []*model.VegRule
	Found    []*Kind
}

func (s *Site) pagesResources() error {
	ix := s.Index
	rulesByPrefab := map[string][]*model.VegRule{}
	for _, r := range s.DB.Vegetation {
		if r.Enabled {
			rulesByPrefab[r.Prefab] = append(rulesByPrefab[r.Prefab], r)
		}
	}
	foundIn := map[string][]*Kind{}
	for _, k := range ix.KindList {
		seen := map[string]bool{}
		for _, v := range k.Variants {
			if v.Contents == nil {
				continue
			}
			for _, p := range v.Contents.Pickables {
				if !seen[p.Prefab] {
					seen[p.Prefab] = true
					foundIn[p.Prefab] = append(foundIn[p.Prefab], k)
				}
			}
			for _, r := range v.Contents.Resources {
				if !seen[r.Prefab] {
					seen[r.Prefab] = true
					foundIn[r.Prefab] = append(foundIn[r.Prefab], k)
				}
			}
		}
	}
	kinds := []string{"pickable", "plant", "tree", "log", "rock", "destructible"}
	byKind := map[string][]*model.Resource{}
	for _, r := range ix.Resources {
		if t := ix.Things[r.Prefab]; t == nil || t.Kind != "resource" {
			continue
		}
		byKind[r.Kind] = append(byKind[r.Kind], r)
	}
	var groups []struct {
		ID        string
		Name      string
		Resources []*model.Resource
	}
	for _, k := range kinds {
		if rs := byKind[k]; len(rs) > 0 {
			groups = append(groups, struct {
				ID        string
				Name      string
				Resources []*model.Resource
			}{k, ResourceKindName(k), rs})
		}
	}
	if err := s.write("resources/", "resources", &Page{Site: s, Title: "Resources", Description: "What the land itself gives in Valheim: every berry bush, mushroom, flower, tree, ore vein and breakable thing, with its yield, respawn and biomes.", Section: "items", Path: "resources/",
		Crumbs: []Crumb{{"Items", "items/"}, {"Resources", ""}}, Data: groups}); err != nil {
		return err
	}
	for _, r := range ix.Resources {
		t := ix.Things[r.Prefab]
		if t == nil || t.Kind != "resource" {
			continue
		}
		pg := &ResourcePage{Resource: r, Thing: t, Item: s.DB.Items[r.Item], Rules: rulesByPrefab[r.Prefab], Found: foundIn[r.Prefab]}
		var prefabs []string
		for _, k := range pg.Found {
			for _, v := range k.Variants {
				prefabs = append(prefabs, v.Prefab)
			}
		}
		page := &Page{Site: s, Title: r.Name, Description: r.Name + " in Valheim: what it gives, how often it respawns and which biomes grow it.", Section: "items", Path: t.URL, Icon: s.absIcon(t.Icon),
			Crumbs: []Crumb{{"Items", "items/"}, {"Resources", "resources/"}, {r.Name, ""}}, Data: pg}
		if len(r.Biomes) > 0 || len(prefabs) > 0 {
			page.YourWorld = &YourWorld{Kind: "resource", Biomes: r.Biomes, Prefabs: prefabs, Label: strings.ToLower(r.Name)}
		}
		if err := s.write(t.URL, "resource", page); err != nil {
			return err
		}
	}
	return nil
}

// mergeByName folds pickables that read the same (the crypt's three
// "Skeletal Remains" prefabs) into one row.
func (s *Site) mergeByName(in []model.PickableIn) []model.PickableIn {
	var out []model.PickableIn
	index := map[string]int{}
	for _, p := range in {
		name := p.Prefab
		if t := s.Index.Things[p.Prefab]; t != nil {
			name = t.Name
		}
		if i, ok := index[name]; ok {
			out[i].Count += p.Count
			continue
		}
		index[name] = len(out)
		out = append(out, p)
	}
	return out
}
