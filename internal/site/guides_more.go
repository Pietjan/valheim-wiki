package site

import (
	"sort"
	"strings"

	"github.com/pietjan/valheim-wiki/internal/model"
)

// The taming, mead and progression guides.

// TameRow is one creature that can be tamed, hatched or raised.
type TameRow struct {
	Creature  *model.Creature
	Biome     string
	Stage     int
	Eats      []string // item prefabs
	Offspring *model.Creature
	GrowTime  float64 // the young's time to grow, seconds
	Egg       *model.Item
	EggTime   float64
	Saddle    *model.Item
	Young     *model.Creature // what grows into it
	Lays      *model.Item     // what it lays without a partner
}

// TamingGuide is the taming guide.
type TamingGuide struct {
	Wild   []TameRow // tamed by feeding in the wild
	Raised []TameRow // hatched or born tame
}

func (s *Site) tamingGuide() *TamingGuide {
	ix := s.Index
	g := s.stagerFor()
	guide := &TamingGuide{}
	young := map[string]*model.Creature{}
	for _, c := range s.DB.Creatures {
		if c.Growup != nil && c.Growup.Grown != "" {
			young[c.Growup.Grown] = c
		}
	}
	for _, c := range ix.Creatures {
		if c.Tameable == nil && c.Egg == nil {
			continue
		}
		// Summons — the spiritcaller's beasts, the skeleton — are tame
		// from the first and not kept.
		if c.Tameable != nil && c.Tameable.Unsummon > 0 || strings.Contains(c.Prefab, "_spiritcaller") {
			continue
		}
		row := TameRow{Creature: c, Biome: ix.Primary[c.Prefab], Stage: g.creature(c.Prefab), Young: young[c.Prefab]}
		if row.Young != nil && row.Young.Growup != nil && row.GrowTime == 0 {
			row.GrowTime = row.Young.Growup.GrowTime
		}
		if c.Procreation != nil && c.Procreation.NoPartnerOffspring != "" {
			row.Lays = s.DB.Items[c.Procreation.NoPartnerOffspring]
		}
		if row.Biome == "" && row.Stage != stageUnknown {
			row.Biome = model.Biomes[row.Stage]
		}
		if c.AI != nil {
			row.Eats = c.AI.ConsumeItems
		}
		if c.Procreation != nil && c.Procreation.Offspring != "" {
			row.Offspring = s.DB.Creatures[c.Procreation.Offspring]
			if row.Offspring != nil && row.Offspring.Growup != nil {
				row.GrowTime = row.Offspring.Growup.GrowTime
			}
			if row.Offspring == nil {
				// An egg rather than a young: the asksvin's.
				row.Lays = s.DB.Items[c.Procreation.Offspring]
			}
		}
		if c.Egg != nil {
			row.Egg = s.DB.Items[c.Egg.From]
			row.EggTime = c.Egg.GrowTime
		}
		if c.Tameable != nil && c.Tameable.Saddle != "" {
			row.Saddle = s.DB.Items[c.Tameable.Saddle]
		}
		if c.Tameable != nil && !c.Tameable.StartsTamed && c.Egg == nil {
			guide.Wild = append(guide.Wild, row)
		} else {
			guide.Raised = append(guide.Raised, row)
		}
	}
	byStage := func(rows []TameRow) {
		sort.SliceStable(rows, func(i, j int) bool {
			if rows[i].Stage != rows[j].Stage {
				return rows[i].Stage < rows[j].Stage
			}
			return rows[i].Creature.Name < rows[j].Creature.Name
		})
	}
	byStage(guide.Wild)
	byStage(guide.Raised)
	return guide
}

// MeadRow is one mead or potion and what it does.
type MeadRow struct {
	Item   *model.Item
	Effect *model.Effect
	Biome  string
	Stage  int
	Group  string
}

// meadGroups names the effect categories in the order shown.
var meadGroups = []struct{ cat, name string }{
	{"healthpotion", "Healing"}, {"staminapotion", "Stamina"}, {"eitrpotion", "Eitr"},
	{"resistance", "Resistance"}, {"", "Other"},
}

func (s *Site) meadGuide() []MeadRow {
	g := s.stagerFor()
	var rows []MeadRow
	for _, it := range s.Index.Items {
		if it.ConsumeEffect == "" || !s.visible(it) {
			continue
		}
		e := s.DB.Effects[it.ConsumeEffect]
		if e == nil || e.ID == "Puke" {
			continue
		}
		row := MeadRow{Item: it, Effect: e, Stage: g.item(it.Prefab)}
		if row.Stage != stageUnknown {
			row.Biome = model.Biomes[row.Stage]
		}
		cat := e.Category
		switch {
		case cat == "healthpotion", cat == "staminapotion", cat == "eitrpotion":
		case len(e.Mods) > 0 || strings.Contains(strings.ToLower(e.ID), "resist"):
			cat = "resistance"
		default:
			cat = ""
		}
		for _, mg := range meadGroups {
			if mg.cat == cat {
				row.Group = mg.name
			}
		}
		rows = append(rows, row)
	}
	order := map[string]int{}
	for i, mg := range meadGroups {
		order[mg.name] = i
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if order[rows[i].Group] != order[rows[j].Group] {
			return order[rows[i].Group] < order[rows[j].Group]
		}
		if rows[i].Stage != rows[j].Stage {
			return rows[i].Stage < rows[j].Stage
		}
		return rows[i].Item.Name < rows[j].Item.Name
	})
	return rows
}

// ProgressStage is one biome's checklist.
type ProgressStage struct {
	Biome      string
	Boss       *model.Creature
	Stations   []*model.Piece // stations and converters first buildable here
	Materials  []*model.Item  // materials first had here
	Creatures  []*model.Creature
	Raids      []*model.Event // raids the boss's defeat unlocks
	Food       *FoodStage
	Gear       *GearStage
	Comfort    int
	MeadCount  int
	TameCount  int
	NewMeads   []*model.Item
	NewTames   []*model.Creature
	NewWeapons int
}

func (s *Site) progressionGuide() []ProgressStage {
	ix := s.Index
	g := s.stagerFor()
	food := s.foodGuide()
	gear := s.gearGuide()
	comfort := s.comfortGuide()
	meads := s.meadGuide()
	tames := s.tamingGuide()
	bosses := map[string]*model.Creature{}
	for _, c := range ix.Creatures {
		if c.Boss && c.BossOrder >= 1 && c.BossOrder <= len(model.Biomes) {
			bosses[model.Biomes[c.BossOrder-1]] = c
		}
	}
	var out []ProgressStage
	for bi, biome := range model.Biomes {
		if biome == "ocean" {
			continue
		}
		st := ProgressStage{Biome: biome, Boss: bosses[biome], Food: food.ByBiome[biome], Gear: gear.ByBiome[biome]}
		for _, cs := range comfort {
			if cs.Biome == biome {
				st.Comfort = cs.Total
			}
		}
		seen := map[string]bool{}
		for _, k := range model.SortedKeys(s.DB.Pieces) {
			p := s.DB.Pieces[k]
			if seen[p.Name] || !p.Enabled {
				continue
			}
			if s.DB.Stations[p.Prefab] == nil {
				cv := s.DB.Converters[p.Prefab]
				if cv == nil {
					continue
				}
				switch cv.Kind {
				case "smelter", "cooking", "fermenter", "beehive", "sap", "incinerator":
				default:
					continue // fires, feasts, turrets and traps are not what a stage opens
				}
			}
			if g.piece(p.Prefab) == bi {
				seen[p.Name] = true
				st.Stations = append(st.Stations, p)
			}
		}
		for _, it := range ix.Items {
			if it.Type != "Material" || !s.visible(it) || it.DLC != "" || it.Upgrader != nil || strings.HasPrefix(it.Prefab, "Feast") {
				continue
			}
			if g.item(it.Prefab) == bi {
				st.Materials = append(st.Materials, it)
			}
		}
		for _, c := range ix.Creatures {
			if ix.Primary[c.Prefab] == biome && !(c.Boss && c.BossOrder > 0) {
				st.Creatures = append(st.Creatures, c)
			}
		}
		sort.SliceStable(st.Creatures, func(i, j int) bool { return st.Creatures[i].Health > st.Creatures[j].Health })
		if st.Boss != nil && st.Boss.DefeatKey != "" {
			for _, e := range s.DB.Events {
				if !e.Enabled || !e.Random {
					continue
				}
				for _, k := range e.RequiredKeys {
					if k == st.Boss.DefeatKey {
						st.Raids = append(st.Raids, e)
						break
					}
				}
			}
		}
		for _, m := range meads {
			if m.Stage == bi {
				st.NewMeads = append(st.NewMeads, m.Item)
			}
		}
		for _, t := range append(append([]TameRow{}, tames.Wild...), tames.Raised...) {
			if t.Stage == bi {
				st.NewTames = append(st.NewTames, t.Creature)
			}
		}
		out = append(out, st)
	}
	return out
}

// pagesGuideMore writes the taming, mead and progression guides.
func (s *Site) pagesGuideMore() error {
	if err := s.write("guides/taming/", "taming", &Page{Site: s, Title: "Taming and breeding",
		Description: "Every creature of Valheim that can be tamed, hatched or raised: what it eats, how long taming takes, how it breeds and how fast the young grow, and what it can be ridden with.",
		Section:     "guides", Path: "guides/taming/", Crumbs: []Crumb{{"Guides", "guides/"}, {"Taming", ""}}, Data: s.tamingGuide()}); err != nil {
		return err
	}
	if err := s.write("guides/meads/", "meads", &Page{Site: s, Title: "Meads and potions",
		Description: "Every mead and potion of Valheim by what it does and when it can first be brewed: healing, stamina, eitr, resistances and the rest, with the game's own numbers.",
		Section:     "guides", Path: "guides/meads/", Crumbs: []Crumb{{"Guides", "guides/"}, {"Meads", ""}}, Data: s.meadGuide()}); err != nil {
		return err
	}
	return s.write("guides/progression/", "progression", &Page{Site: s, Title: "Progression",
		Description: "A checklist for each biome of Valheim: the boss, the stations and materials it unlocks, the creatures that live there, the raids its boss sets loose, and the food, gear and comfort to have by then.",
		Section:     "guides", Path: "guides/progression/", Crumbs: []Crumb{{"Guides", "guides/"}, {"Progression", ""}}, Data: s.progressionGuide()})
}
