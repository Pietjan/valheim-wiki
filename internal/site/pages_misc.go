package site

import (
	"sort"
	"strings"

	"github.com/pietjan/valheim-wiki/internal/model"
)

// EffectPage is a status effect's page.
type EffectPage struct {
	Effect  *model.Effect
	Thing   *Thing
	Granted []EffectLink
	Boss    *model.Creature
}

func (s *Site) pagesEffects() error {
	ix := s.Index
	var powers, others []*model.Effect
	for _, e := range ix.Effects {
		if e.GuardianOf != "" {
			powers = append(powers, e)
		} else {
			others = append(others, e)
		}
	}
	if err := s.write("effects/", "effects", &Page{Site: s, Title: "Status effects", Description: "Every status effect in Valheim: the Forsaken powers, set bonuses, meads, food and the weather's touch, with their exact numbers.", Section: "items", Path: "effects/",
		Crumbs: []Crumb{{"Items", "items/"}, {"Status effects", ""}}, Data: map[string]any{"Powers": powers, "Others": others}}); err != nil {
		return err
	}
	for _, e := range ix.Effects {
		t := ix.Things["effect:"+e.ID]
		pg := &EffectPage{Effect: e, Thing: t, Granted: ix.Effects_[e.ID]}
		if e.GuardianOf != "" {
			pg.Boss = s.DB.Creatures[e.GuardianOf]
		}
		desc := e.Tooltip
		if desc == "" {
			desc = e.Name + ", a status effect in Valheim."
		}
		if err := s.write(t.URL, "effect", &Page{Site: s, Title: e.Name, Description: desc, Section: "items", Path: t.URL, Icon: s.absIcon(t.Icon),
			Crumbs: []Crumb{{"Items", "items/"}, {"Status effects", "effects/"}, {e.Name, ""}}, Data: pg}); err != nil {
			return err
		}
	}
	// Skills.
	return s.write("skills/", "skills", &Page{Site: s, Title: "Skills", Description: "The skills of Valheim and what each one trains.", Section: "items", Path: "skills/",
		Crumbs: []Crumb{{"Items", "items/"}, {"Skills", ""}}, Data: s.DB.Skills})
}

// TraderPage is a trader's page.
type TraderPage struct {
	Trader   *model.Trader
	Thing    *Thing
	Creature *model.Creature
	Location *model.Location
	Kind     *Kind
}

func (s *Site) pagesTraders() error {
	ix := s.Index
	var pages []*TraderPage
	for _, t := range s.DB.Traders {
		pg := &TraderPage{Trader: t, Thing: ix.Things[t.Prefab], Creature: s.DB.Creatures[t.Prefab], Location: s.DB.Locations[t.Location], Kind: ix.KindOf[t.Location]}
		pages = append(pages, pg)
	}
	if err := s.write("traders/", "traders", &Page{Site: s, Title: "Traders", Description: "Haldor, Hildir and the Bog Witch: what each sells, for how much, and what you must do first.", Section: "items", Path: "traders/",
		Crumbs: []Crumb{{"Items", "items/"}, {"Traders", ""}}, Data: pages}); err != nil {
		return err
	}
	for _, pg := range pages {
		t := pg.Thing
		page := &Page{Site: s, Title: pg.Trader.Name, Description: pg.Trader.Name + " in Valheim: every ware, its price, and where the camp stands in your world.", Section: "items", Path: t.URL, Icon: s.absIcon(t.Icon),
			Crumbs: []Crumb{{"Items", "items/"}, {"Traders", "traders/"}, {pg.Trader.Name, ""}}, Data: pg}
		if pg.Trader.Location != "" {
			page.YourWorld = &YourWorld{Kind: "trader", Prefabs: []string{pg.Trader.Location}, Label: pg.Trader.Name}
		}
		if err := s.write(t.URL, "trader", page); err != nil {
			return err
		}
	}
	return nil
}

// EventView is a raid with its creatures resolved.
type EventView struct {
	Event     *model.Event
	Creatures []*model.Creature
	Spawns    []*model.Spawn
}

func (s *Site) pagesEvents() error {
	byID := map[string]*model.Spawn{}
	for _, sp := range s.DB.Spawns {
		byID[sp.ID] = sp
	}
	var views []EventView
	for _, e := range s.DB.Events {
		if !e.Enabled || !e.Random {
			continue // the boss fights and Fimbulvinter are events, not raids
		}
		v := EventView{Event: e}
		seen := map[string]bool{}
		for _, id := range e.Spawns {
			sp := byID[id]
			if sp == nil {
				continue
			}
			v.Spawns = append(v.Spawns, sp)
			if c := s.DB.Creatures[sp.Prefab]; c != nil && !seen[sp.Prefab] {
				seen[sp.Prefab] = true
				v.Creatures = append(v.Creatures, c)
			}
		}
		views = append(views, v)
	}
	sort.SliceStable(views, func(i, j int) bool { return !views[i].Event.Random && views[j].Event.Random })
	return s.write("events/", "events", &Page{Site: s, Title: "Raids and events", Description: "Every raid in Valheim: what triggers it, where it can happen, what comes, and for how long.", Section: "creatures", Path: "events/",
		Crumbs: []Crumb{{"Creatures", "creatures/"}, {"Raids", ""}}, Data: views})
}

// HomeData is the front page.
type HomeData struct {
	Counts map[string]int
	Biomes []*model.Biome
	Bosses []*model.Creature
}

func (s *Site) pageHome() error {
	ix := s.Index
	visible := 0
	for _, it := range ix.Items {
		if s.visible(it) {
			visible++
		}
	}
	var bosses []*model.Creature
	for _, c := range ix.Creatures {
		if c.Boss && c.BossOrder > 0 {
			bosses = append(bosses, c)
		}
	}
	sort.Slice(bosses, func(i, j int) bool { return bosses[i].BossOrder < bosses[j].BossOrder })
	d := &HomeData{Counts: map[string]int{"items": visible, "pieces": len(ix.Pieces), "creatures": len(ix.Creatures), "locations": len(ix.KindList), "effects": len(ix.Effects), "resources": len(ix.Resources), "achievements": len(s.DB.Achievements)}, Biomes: s.DB.Biomes, Bosses: bosses}
	return s.write("", "home", &Page{Site: s, Title: "", Description: "Vegvísir is a Valheim wiki generated from the game's own data, with a seed viewer that shows where everything is in your world.", Section: "home", Path: "", Data: d})
}

func (s *Site) pageWorld() error {
	return s.write("world/", "world", &Page{Site: s, Title: "Your world", Description: "A map of any Valheim seed, computed in your browser: biomes, rivers, bosses, traders, dungeons and every other place the world generator sets down.", Section: "world", Path: "world/"})
}

func (s *Site) pageSearch() error {
	return s.write("search/", "search", &Page{Site: s, Title: "Search", Description: "Search the wiki.", Section: "search", Path: "search/"})
}

func (s *Site) pageAbout() error {
	return s.write("about/", "about", &Page{Site: s, Title: "About", Description: "Where the data comes from and how the site is built.", Section: "about", Path: "about/"})
}

// SearchEntry is one row of search.json.
type SearchEntry struct {
	N string `json:"n"`           // name
	U string `json:"u"`           // url (site-relative)
	K string `json:"k"`           // kind
	S string `json:"s,omitempty"` // sub line
	I string `json:"i,omitempty"` // icon path under icons/
	T string `json:"t,omitempty"` // extra terms (prefab)
}

// writeSiteData writes search.json and the map's location catalogue.
func (s *Site) writeSiteData() error {
	ix := s.Index
	var entries []SearchEntry
	seen := map[string]bool{}
	for _, t := range ix.Things {
		if t.Kind == "item" {
			if it := s.DB.Items[t.Prefab]; it != nil && !s.visible(it) {
				continue
			}
		}
		if seen[t.URL] {
			continue
		}
		seen[t.URL] = true
		entries = append(entries, SearchEntry{N: t.Name, U: t.URL, K: t.Kind, S: t.Sub, I: t.Icon, T: t.Prefab})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].N != entries[j].N {
			return entries[i].N < entries[j].N
		}
		return entries[i].U < entries[j].U
	})
	if err := s.writeJSON("search.json", entries); err != nil {
		return err
	}
	// The map's catalogue: every location prefab the engine can place,
	// with its kind, name, biome, icon and category for the layer list.
	type mapLoc struct {
		Prefab string `json:"prefab"`
		Kind   string `json:"kind"`
		Name   string `json:"name"`
		URL    string `json:"url"`
		Icon   string `json:"icon,omitempty"`
		Biome  string `json:"biome,omitempty"`
		Cat    string `json:"cat"`
		Pri    int    `json:"pri"`
	}
	var locs []mapLoc
	for _, l := range ix.Locations {
		k := ix.KindOf[l.Prefab]
		if k == nil {
			continue
		}
		ml := mapLoc{Prefab: l.Prefab, Kind: k.Slug, Name: k.Name, URL: "locations/" + k.Slug + "/"}
		if s.hasIcon("locations", l.Prefab) {
			ml.Icon = "locations/" + l.Prefab
		} else if k.Icon != "" {
			ml.Icon = "locations/" + k.Icon
		}
		if len(l.Biomes) > 0 {
			ml.Biome = l.Biomes[0]
		}
		ml.Cat, ml.Pri = locationCategory(l, s.DB)
		locs = append(locs, ml)
	}
	return s.writeJSON("world/locations.json", locs)
}

// locationCategory sorts a location into a map layer and a draw priority.
func locationCategory(l *model.Location, db *model.DB) (string, int) {
	if l.Contents != nil && l.Contents.Offering != nil {
		return "boss", 100
	}
	switch l.Prefab {
	case "StartTemple":
		return "start", 110
	case "Vendor_BlackForest", "Hildir_camp", "BogWitch_Camp", "AncientUpgradeStation":
		return "trader", 95
	case "Mistlands_DvergrBossEntrance1", "FaderLocation", "DN_Bossroom", "Hildir_cave", "Hildir_crypt", "Hildir_plainsfortress", "PlaceofMystery1", "PlaceofMystery2", "PlaceofMystery3":
		return "boss", 98
	}
	if l.HasInterior || l.Dungeon != nil && l.Dungeon.Algorithm == "Dungeon" {
		return "dungeon", 80
	}
	if l.Dungeon != nil {
		return "camp", 70
	}
	if strings.HasPrefix(l.Prefab, "Runestone") || strings.Contains(l.Prefab, "Lorestone") {
		return "runestone", 40
	}
	if l.Contents != nil && len(l.Contents.Chests) > 0 {
		return "ruin", 60
	}
	return "other", 30
}
