package site

import (
	"sort"
	"strings"

	"github.com/pietjan/valheim-wiki/internal/model"
)

// The food guide: for each biome, in the order a Viking meets them, the
// three foods to eat together for the most health, the most stamina, the
// most eitr, and the best of both, out of everything the game can have
// handed out by then.
//
// When a food becomes available is worked out from the data: a food is as
// late as the latest of its ingredients and the station that cooks it,
// an ingredient as early as the earliest creature that drops it, pickable
// that gives it, trader that sells it or chest that holds it, a station as
// late as the latest material it is built from. The ocean counts as the
// Black Forest, where the first boat and fishing rod come from.

// FoodCombo is three foods eaten together and what they add up to.
type FoodCombo struct {
	ID       string
	Label    string
	Foods    []*model.Item
	Health   float64
	Stamina  float64
	Eitr     float64
	Regen    float64 // health a tick, summed
	Duration float64 // the shortest of the three, seconds
}

// FoodStage is one biome's best meals and the foods it unlocks.
type FoodStage struct {
	Biome  string
	Combos []FoodCombo
	New    []*model.Item // foods first available at this stage, best first
	Count  int           // foods available by this stage
}

// FoodGuide is the whole guide, built once.
type FoodGuide struct {
	Stages  []FoodStage
	ByBiome map[string]*FoodStage
	Unknown []*model.Item // foods the data cannot place
}

const stageUnknown = 99

// stageOverrides are the earliest biomes for the few items the extraction
// gives no source for: the queen bee comes from a wild beehive's drops,
// which the extractor does not read, and raw fish from the fishing rod
// and bait Haldor sells.
var stageOverrides = map[string]int{
	"QueenBee":     0,
	"FishRaw":      1,
	"MushroomBlue": 1, // the burial chambers' blue mushrooms are not in the rooms' contents
	"OatSeeds":     7, // the Deep North's crops: their seeds' source is not extracted
	"PoteitrSeeds": 7,
	"DvergrNeedle": 5, // dropped by the Dvergr, whose drops are not extracted
}

// themeStage is the biome a dungeon theme belongs to, for what its rooms
// hold.
var themeStage = map[string]int{
	"MeadowsFarm": 0, "MeadowsVillage": 0, "ForestCrypt": 1, "ForestCryptHildir": 1, "SunkenCrypt": 2,
	"Cave": 3, "CaveHildir": 3, "GoblinCamp": 4, "PlainsFortHildir": 4, "DvergerTown": 5, "DvergerBoss": 5,
	"AshlandRuins": 6, "FortressRuins": 6, "MorkHalla": 6, "NorthVillage": 7, "HalfBurried": 7, "Hole": 7,
}

// keyStage is the stage a global key gates something to: the biome after
// the boss whose defeat sets it.
var keyStage = map[string]int{
	"defeated_eikthyr": 1, "defeated_gdking": 2, "defeated_bonemass": 3, "defeated_dragon": 4,
	"defeated_goblinking": 5, "defeated_queen": 6, "defeated_fader": 7,
	"KilledTroll": 1, "killed_surtling": 2, "defeated_serpent": 2, "KilledBat": 3,
}

// stager works out the stage of items and pieces, memoised.
type stager struct {
	s         *Site
	items     map[string]int
	pieces    map[string]int
	creatures map[string]int
	visiting  map[string]bool
	// grownFrom is the sapling pieces a pickable grows from; growsFrom
	// the young a grown creature was; found the earliest stage of the
	// places a pickable, resource, chest item or creature is met in,
	// locations and dungeon rooms both, by prefab (items as "item:" +
	// prefab).
	grownFrom map[string][]string
	growsFrom map[string][]string
	found     map[string]int
	// parentOf is the resources a resource comes from when felled or
	// broken: a birch's log, the log's halves.
	parentOf  map[string][]string
	resources map[string]int
}

// resource is the earliest stage a resource stands at: where its kind
// grows or is placed, or where whatever it is cut or broken from does.
func (g *stager) resource(p string) int {
	if v, ok := g.resources[p]; ok {
		return v
	}
	if g.visiting["r:"+p] {
		return stageUnknown
	}
	g.visiting["r:"+p] = true
	defer delete(g.visiting, "r:"+p)
	best := stageUnknown
	if r := g.s.DB.Resources[p]; r != nil {
		for _, b := range r.Biomes {
			best = minStage(best, biomeStage(b))
		}
	}
	if st, ok := g.found[p]; ok {
		best = minStage(best, st)
	}
	for _, parent := range g.parentOf[p] {
		best = minStage(best, g.resource(parent))
	}
	g.resources[p] = best
	return best
}

// note records the earliest stage something is met at.
func (g *stager) note(key string, st int) {
	if cur, ok := g.found[key]; !ok || st < cur {
		g.found[key] = st
	}
}

// contents files what a location or room holds, at its stage.
func (g *stager) contents(c *model.Contents, st int) {
	if c == nil || st == stageUnknown {
		return
	}
	for _, pk := range c.Pickables {
		g.note(pk.Prefab, st)
		if pk.Item != "" {
			g.note("item:"+pk.Item, st)
		}
	}
	for _, r := range c.Resources {
		g.note(r.Prefab, st)
		if res := g.s.DB.Resources[r.Prefab]; res != nil {
			if res.Item != "" {
				g.note("item:"+res.Item, st)
			}
			if res.Drops != nil {
				for _, d := range res.Drops.Drops {
					g.note("item:"+d.Item, st)
				}
			}
		}
	}
	for _, ch := range c.Chests {
		if ch.Loot != nil {
			for _, d := range ch.Loot.Drops {
				g.note("item:"+d.Item, st)
			}
		}
	}
	for _, l := range c.Loot {
		if l.Loot != nil {
			for _, d := range l.Loot.Drops {
				g.note("item:"+d.Item, st)
			}
		}
	}
	for _, cr := range c.Creatures {
		g.note("creature:"+cr.Creature, st)
	}
	for _, sp := range c.Spawners {
		g.note("creature:"+sp.Creature, st)
	}
	if c.Offering != nil && c.Offering.Boss != "" {
		g.note("creature:"+c.Offering.Boss, st)
	}
}

func newStager(s *Site) *stager {
	g := &stager{s: s, items: map[string]int{}, pieces: map[string]int{}, creatures: map[string]int{}, visiting: map[string]bool{},
		grownFrom: map[string][]string{}, growsFrom: map[string][]string{}, found: map[string]int{},
		parentOf: map[string][]string{}, resources: map[string]int{}}
	for _, r := range s.DB.Resources {
		for _, child := range []string{r.Log, r.SubLog, r.Stub, r.SpawnWhenDestroyed} {
			if child != "" {
				g.parentOf[child] = append(g.parentOf[child], r.Prefab)
			}
		}
	}
	for _, p := range s.DB.Pieces {
		if p.Plant != nil {
			for _, grown := range p.Plant.GrownPrefabs {
				g.grownFrom[grown] = append(g.grownFrom[grown], p.Prefab)
			}
		}
	}
	for _, l := range s.DB.Locations {
		st := stageUnknown
		for _, b := range l.Biomes {
			st = minStage(st, biomeStage(b))
		}
		g.contents(l.Contents, st)
	}
	for _, r := range s.DB.Rooms {
		st := stageUnknown
		for _, th := range strings.Split(r.Theme, "|") {
			if v, ok := themeStage[th]; ok {
				st = minStage(st, v)
			}
		}
		g.contents(r.Contents, st)
	}
	// The vegetation rules place the ore deposits and much else whose
	// resource entry carries no biome of its own: the rule names the whole
	// rock, the resource that drops is its fractured form.
	for _, v := range s.DB.Vegetation {
		if !v.Enabled {
			continue
		}
		st := stageUnknown
		for _, b := range v.Biomes {
			st = minStage(st, biomeStage(b))
		}
		if st == stageUnknown {
			continue
		}
		g.note(v.Prefab, st)
		g.note(v.Prefab+"_frac", st)
		for _, p := range []string{v.Prefab, v.Prefab + "_frac"} {
			if r := s.DB.Resources[p]; r != nil {
				if r.Item != "" {
					g.note("item:"+r.Item, st)
				}
				if r.Drops != nil {
					for _, d := range r.Drops.Drops {
						g.note("item:"+d.Item, st)
					}
				}
			}
		}
	}
	for _, c := range s.DB.Creatures {
		if c.Growup != nil && c.Growup.Grown != "" {
			g.growsFrom[c.Growup.Grown] = append(g.growsFrom[c.Growup.Grown], c.Prefab)
		}
	}
	return g
}

// creature is the earliest stage a creature is met at: its first world
// spawn's biome, else the place it stands in, else the egg it hatches from
// or the young it grew from.
func (g *stager) creature(p string) int {
	if v, ok := g.creatures[p]; ok {
		return v
	}
	if g.visiting["c:"+p] {
		return stageUnknown
	}
	g.visiting["c:"+p] = true
	defer delete(g.visiting, "c:"+p)
	ix := g.s.Index
	best := stageUnknown
	if b := ix.Primary[p]; b != "" {
		best = biomeStage(b)
	}
	for _, l := range ix.Found[p] {
		for _, b := range l.Biomes {
			best = minStage(best, biomeStage(b))
		}
	}
	if st, ok := g.found["creature:"+p]; ok {
		best = minStage(best, st)
	}
	if c := g.s.DB.Creatures[p]; c != nil && c.Egg != nil && c.Egg.From != "" {
		best = minStage(best, g.item(c.Egg.From))
	}
	for _, young := range g.growsFrom[p] {
		best = minStage(best, g.creature(young))
	}
	g.creatures[p] = best
	return best
}

func biomeStage(slug string) int {
	for i, b := range model.Biomes {
		if b == slug {
			if b == "ocean" {
				return 1
			}
			return i
		}
	}
	return stageUnknown
}

func minStage(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxStage(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// item is the earliest stage an item can be had at.
func (g *stager) item(p string) int {
	if v, ok := stageOverrides[p]; ok {
		return v
	}
	if v, ok := g.items[p]; ok {
		return v
	}
	if g.visiting["i:"+p] {
		return stageUnknown
	}
	g.visiting["i:"+p] = true
	defer delete(g.visiting, "i:"+p)
	ix := g.s.Index
	best := stageUnknown
	for _, d := range ix.DroppedBy[p] {
		best = minStage(best, g.creature(d.Creature.Prefab))
	}
	for _, r := range ix.Gathered[p] {
		// Where it grows or stands, or what it is cut from; or it is
		// grown from a sapling, which is as early as its seed.
		best = minStage(best, g.resource(r.Prefab))
		for _, sapling := range g.grownFrom[r.Prefab] {
			best = minStage(best, g.piece(sapling))
		}
	}
	if st, ok := g.found["item:"+p]; ok {
		best = minStage(best, st)
	}
	for _, c := range ix.InChests[p] {
		for _, b := range c.Location.Biomes {
			best = minStage(best, biomeStage(b))
		}
	}
	for _, sl := range ix.Sold[p] {
		if l := g.s.DB.Locations[sl.Trader.Location]; l != nil {
			st := stageUnknown
			for _, b := range l.Biomes {
				st = minStage(st, biomeStage(b))
			}
			if k := sl.Item.RequiredKey; k != "" {
				if ks, ok := keyStage[k]; ok {
					st = maxStage(st, ks)
				} else {
					st = stageUnknown
				}
			}
			best = minStage(best, st)
		}
	}
	for _, r := range ix.Recipes[p] {
		st := 0
		if r.Station != "" {
			st = g.piece(r.Station)
		}
		for _, res := range r.Resources {
			if res.Upgrader {
				// The Forge of Potential's idol, listed as an ingredient
				// but not one.
				continue
			}
			st = maxStage(st, g.item(res.Item))
		}
		best = minStage(best, st)
	}
	for _, cv := range ix.Converts[p] {
		st := g.piece(cv.Converter.Prefab)
		if cv.Conversion.From != "" {
			st = maxStage(st, g.item(cv.Conversion.From))
		}
		best = minStage(best, st)
	}
	g.items[p] = best
	return best
}

// piece is the earliest stage a piece can be built at: when its materials
// can be had.
func (g *stager) piece(p string) int {
	if v, ok := g.pieces[p]; ok {
		return v
	}
	if g.visiting["p:"+p] {
		return stageUnknown
	}
	g.visiting["p:"+p] = true
	defer delete(g.visiting, "p:"+p)
	pc := g.s.DB.Pieces[p]
	if pc == nil {
		g.pieces[p] = stageUnknown
		return stageUnknown
	}
	st := 0
	for _, r := range pc.Resources {
		st = maxStage(st, g.item(r.Item))
	}
	g.pieces[p] = st
	return st
}

// foodGuide builds the guide once.
func (s *Site) foodGuide() *FoodGuide {
	if s.food != nil {
		return s.food
	}
	g := s.stagerFor()
	type food struct {
		it    *model.Item
		stage int
	}
	var foods []food
	guide := &FoodGuide{ByBiome: map[string]*FoodStage{}}
	for _, it := range s.Index.Items {
		if it.Food <= 0 && it.FoodStamina <= 0 && it.FoodEitr <= 0 {
			continue
		}
		if !s.visible(it) {
			continue
		}
		st := g.item(it.Prefab)
		if st == stageUnknown {
			guide.Unknown = append(guide.Unknown, it)
			continue
		}
		foods = append(foods, food{it, st})
	}
	sort.SliceStable(foods, func(i, j int) bool {
		a, b := foods[i].it, foods[j].it
		return a.Food+a.FoodStamina+a.FoodEitr > b.Food+b.FoodStamina+b.FoodEitr
	})

	for bi, biome := range model.Biomes {
		if biome == "ocean" {
			continue
		}
		stage := FoodStage{Biome: biome}
		var have []*model.Item
		for _, f := range foods {
			if f.stage <= bi {
				have = append(have, f.it)
			}
			if f.stage == bi {
				stage.New = append(stage.New, f.it)
			}
		}
		stage.Count = len(have)
		if len(have) >= 3 {
			stage.Combos = bestCombos(have)
		}
		guide.Stages = append(guide.Stages, stage)
	}
	for i := range guide.Stages {
		guide.ByBiome[guide.Stages[i].Biome] = &guide.Stages[i]
	}
	s.food = guide
	return guide
}

// bestCombos picks, over every three of the foods, the most health, the
// most stamina, the most eitr (when any food gives it) and the best
// balance of health and stamina.
func bestCombos(foods []*model.Item) []FoodCombo {
	type pick struct {
		id, label string
		score     func(h, st, e float64) float64
		need      func(h, st, e float64) bool
	}
	picks := []pick{
		{"health", "Most health", func(h, st, e float64) float64 { return h*1000 + st }, nil},
		{"stamina", "Most stamina", func(h, st, e float64) float64 { return st*1000 + h }, nil},
		{"balanced", "Balanced", func(h, st, e float64) float64 {
			lo := h
			if st < lo {
				lo = st
			}
			return lo + (h+st)/4
		}, nil},
		{"eitr", "Most eitr", func(h, st, e float64) float64 { return e*1000 + h + st }, func(h, st, e float64) bool { return e > 0 }},
	}
	best := make([]FoodCombo, len(picks))
	bestScore := make([]float64, len(picks))
	found := make([]bool, len(picks))
	n := len(foods)
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			for k := j + 1; k < n; k++ {
				a, b, c := foods[i], foods[j], foods[k]
				h := a.Food + b.Food + c.Food
				st := a.FoodStamina + b.FoodStamina + c.FoodStamina
				e := a.FoodEitr + b.FoodEitr + c.FoodEitr
				for pi, p := range picks {
					if p.need != nil && !p.need(h, st, e) {
						continue
					}
					sc := p.score(h, st, e)
					if !found[pi] || sc > bestScore[pi] {
						found[pi] = true
						bestScore[pi] = sc
						dur := a.FoodBurn
						if b.FoodBurn < dur {
							dur = b.FoodBurn
						}
						if c.FoodBurn < dur {
							dur = c.FoodBurn
						}
						best[pi] = FoodCombo{ID: p.id, Label: p.label, Foods: []*model.Item{a, b, c}, Health: h, Stamina: st, Eitr: e,
							Regen: a.FoodRegen + b.FoodRegen + c.FoodRegen, Duration: dur}
					}
				}
			}
		}
	}
	var out []FoodCombo
	for pi := range picks {
		if found[pi] {
			out = append(out, best[pi])
		}
	}
	return out
}

// pagesGuides writes the guides: the food guide here, the rest in
// guides_gear.go.
func (s *Site) pagesGuides() error {
	guide := s.foodGuide()
	if err := s.write("guides/food/", "food", &Page{Site: s, Title: "Best food by biome",
		Description: "The best three foods to eat together in every biome of Valheim, for the most health, stamina or eitr, worked out from what the game can have handed you by then.",
		Section:     "guides", Path: "guides/food/", Crumbs: []Crumb{{"Guides", "guides/"}, {"Best food", ""}}, Data: guide}); err != nil {
		return err
	}
	return s.pagesGuideSet()
}
