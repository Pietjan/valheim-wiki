package site

import (
	"sort"
	"strings"

	"github.com/pietjan/valheim-wiki/internal/model"
)

// The food guide: for each biome, in the order a Viking meets them, the
// three foods to eat together for the most health, the most stamina and
// the most eitr, and the menus Vikings eat — two health foods and a
// stamina one, two stamina and a health, two eitr with a health or a
// stamina — out of everything the game can have handed out by then.
//
// When a food becomes available is worked out from the data: a food is as
// late as the latest of its ingredients and the station that cooks it,
// an ingredient as early as the earliest creature that drops it, pickable
// that gives it, trader that sells it or chest that holds it, a station as
// late as the latest material it is built from. The ocean counts as the
// Black Forest, where the first boat and fishing rod come from.

// FoodCombo is three foods eaten together and what they add up to. Alt,
// when a food of the three needs fishing, is the best of the same menu
// without fish, since many never pick up a rod.
type FoodCombo struct {
	ID       string
	Label    string
	Stat     string // the stat the combo is for: health, stamina or eitr
	Alt      *FoodCombo
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
	"OatSeeds":     7, // the Deep North's crops: their seeds' source is not extracted
	"PoteitrSeeds": 7,
	"DvergrNeedle": 5, // dropped by the Dvergr, whose drops are not extracted
	"Fish4_cave":   3, // the mountain caves' fish, whose waters are no biome
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
	// A fish is the creature of the same name, caught where it swims.
	if c := g.s.DB.Creatures[p]; c != nil && c.Fish != nil {
		for _, b := range c.Biomes {
			best = minStage(best, biomeStage(b))
		}
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
					// A key the table does not know: a later boss's.
					st = maxStage(st, len(model.Biomes)-2)
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

// Unplaced reports an item the game's data never hands out: nothing drops,
// sells, crafts or converts it, no chest holds it, and whatever it is
// gathered from is itself never placed. Stricter than an unknown stage,
// which the stager also gives where its reading has a gap, so a page can
// say so outright. The blue mushroom is one: its pickable exists and
// nothing grows it. Iron ore is not: bog iron is vegetation.
func (s *Site) Unplaced(p string) bool {
	ix := s.Index
	if len(ix.DroppedBy[p]) > 0 || len(ix.Sold[p]) > 0 || len(ix.InChests[p]) > 0 || len(ix.Recipes[p]) > 0 || len(ix.Converts[p]) > 0 {
		return false
	}
	if c := s.DB.Creatures[p]; c != nil && c.Fish != nil {
		return false
	}
	if _, ok := s.stagerFor().found["item:"+p]; ok {
		return false
	}
	for _, r := range ix.Gathered[p] {
		if !s.UnplacedResource(r.Prefab) {
			return false
		}
	}
	return true
}

// UnplacedResource reports a resource nothing puts in the world: no
// vegetation rule or clutter grows it, no location or dungeon room holds
// it, no sapling grows it, and nor is anything it is cut or broken from
// placed. The lured pickables, a wisp at a wisp fountain and an ember at
// Fader's, are placed by their piece, which the data does not link.
func (s *Site) UnplacedResource(p string) bool {
	if strings.HasPrefix(p, "Lured") {
		return false
	}
	if s.placedResources()[p] {
		return false
	}
	if r := s.DB.Resources[p]; r != nil && len(r.Biomes) > 0 {
		return false
	}
	g := s.stagerFor()
	if len(g.grownFrom[p]) > 0 {
		return false
	}
	for _, parent := range g.parentOf[p] {
		if !s.UnplacedResource(parent) {
			return false
		}
	}
	return true
}

// placedResources is every resource prefab some rule, clutter, location
// or room puts in the world, built once.
func (s *Site) placedResources() map[string]bool {
	if s.placed != nil {
		return s.placed
	}
	placed := map[string]bool{}
	for _, v := range s.DB.Vegetation {
		if v.Enabled {
			placed[v.Prefab] = true
		}
	}
	for _, c := range s.DB.Clutter {
		if c.Enabled {
			placed[c.Prefab] = true
		}
	}
	note := func(c *model.Contents) {
		if c == nil {
			return
		}
		for _, pk := range c.Pickables {
			placed[pk.Prefab] = true
		}
		for _, r := range c.Resources {
			placed[r.Prefab] = true
		}
	}
	for _, l := range s.DB.Locations {
		if l.Enabled {
			note(l.Contents)
		}
	}
	for _, r := range s.DB.Rooms {
		if r.Enabled {
			note(r.Contents)
		}
	}
	s.placed = placed
	return placed
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
	// And the station it is built beside: a hearth is stone, but it takes
	// a stonecutter, which takes iron.
	if pc.Station != "" && pc.Station != p {
		st = maxStage(st, g.piece(pc.Station))
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
		if !s.visible(it) || s.Unplaced(it.Prefab) {
			// The blue mushroom: in the game, placed nowhere.
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
			stage.Combos = bestCombos(have, s.needsFish)
		}
		guide.Stages = append(guide.Stages, stage)
	}
	for i := range guide.Stages {
		guide.ByBiome[guide.Stages[i].Biome] = &guide.Stages[i]
	}
	s.food = guide
	return guide
}

// foodKind is what a food is for: health, stamina or eitr, whichever it
// gives most of. Raspberries are a stamina food, cooked meat a health food;
// a magecap, as much eitr as health, is an eitr food, and a mushroom, as
// much stamina as health, a stamina food, since that is what it is eaten
// for.
func foodKind(it *model.Item) byte {
	switch {
	case it.FoodEitr > 0 && it.FoodEitr >= it.Food && it.FoodEitr >= it.FoodStamina:
		return 'e'
	case it.FoodStamina >= it.Food:
		return 's'
	default:
		return 'h'
	}
}

// bestCombos picks, over every three of the foods, the most health, the
// most stamina and the most eitr (when any food gives it) with any three,
// and then the menus Vikings eat: two health foods and a stamina one for a
// fight, two stamina and a health for the day's work, and once there is
// eitr, two eitr foods with a health or a stamina one. A menu whose slots
// the foods cannot fill is left out.
//
// The ceilings are strict: the most of their stat, the other only breaking
// ties. The menus weigh the other stat at a half, so a few points of the
// main stat can buy many of the other: three stamina for twenty health is
// a trade worth making, and so is seven health for twenty stamina.
func bestCombos(foods []*model.Item, fishy func(*model.Item) bool) []FoodCombo {
	type pick struct {
		id, label, stat string
		slots           string // the kinds the three must be, in any order; "" for any
		score           func(h, st, e float64) float64
		need            func(h, st, e float64) bool
	}
	// In the order the page lists them: the health menus, then stamina,
	// then eitr, the ceiling of each before its two-and-ones, and the
	// balanced menu last.
	picks := []pick{
		{"health", "Most health", "health", "", func(h, st, e float64) float64 { return h*1000 + st }, nil},
		{"two-health", "Two health, one stamina", "health", "hhs", func(h, st, e float64) float64 { return h + st/2 }, nil},
		{"stamina", "Most stamina", "stamina", "", func(h, st, e float64) float64 { return st*1000 + h }, nil},
		{"two-stamina", "Two stamina, one health", "stamina", "hss", func(h, st, e float64) float64 { return st + h/2 }, nil},
		{"two-stamina-eitr", "Two stamina, one eitr", "stamina", "sse", func(h, st, e float64) float64 { return st + e/2 }, nil},
		{"eitr", "Most eitr", "eitr", "", func(h, st, e float64) float64 { return e*1000 + h + st }, func(h, st, e float64) bool { return e > 0 }},
		{"two-eitr-health", "Two eitr, one health", "eitr", "ehe", func(h, st, e float64) float64 { return e + h/2 }, nil},
		{"two-eitr-stamina", "Two eitr, one stamina", "eitr", "ese", func(h, st, e float64) float64 { return e + st/2 }, nil},
		// Balanced: the best of health and stamina together, any three, the
		// lower of the two counting most.
		{"balanced", "Balanced", "balanced", "", func(h, st, e float64) float64 {
			lo := h
			if st < lo {
				lo = st
			}
			return lo + (h+st)/4
		}, nil},
	}
	kinds := make([]byte, len(foods))
	fish := make([]bool, len(foods))
	for i, f := range foods {
		kinds[i] = foodKind(f)
		fish[i] = fishy != nil && fishy(f)
	}
	// fills is whether three kinds are the slots, in any order.
	fills := func(slots string, a, b, c byte) bool {
		if slots == "" {
			return true
		}
		want := [3]byte{slots[0], slots[1], slots[2]}
		for _, k := range [3]byte{a, b, c} {
			hit := false
			for i, w := range want {
				if w == k {
					want[i] = 0
					hit = true
					break
				}
			}
			if !hit {
				return false
			}
		}
		return true
	}
	// Two bests per pick: with any food, and with no fish among the three.
	best := make([]FoodCombo, len(picks))
	bestScore := make([]float64, len(picks))
	found := make([]bool, len(picks))
	noFish := make([]FoodCombo, len(picks))
	noFishScore := make([]float64, len(picks))
	noFishFound := make([]bool, len(picks))
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
					if !fills(p.slots, kinds[i], kinds[j], kinds[k]) {
						continue
					}
					sc := p.score(h, st, e)
					anyFish := fish[i] || fish[j] || fish[k]
					if (!found[pi] || sc > bestScore[pi]) || (!anyFish && (!noFishFound[pi] || sc > noFishScore[pi])) {
						dur := a.FoodBurn
						if b.FoodBurn < dur {
							dur = b.FoodBurn
						}
						if c.FoodBurn < dur {
							dur = c.FoodBurn
						}
						combo := FoodCombo{ID: p.id, Label: p.label, Stat: p.stat, Foods: []*model.Item{a, b, c}, Health: h, Stamina: st, Eitr: e,
							Regen: a.FoodRegen + b.FoodRegen + c.FoodRegen, Duration: dur}
						if !found[pi] || sc > bestScore[pi] {
							found[pi] = true
							bestScore[pi] = sc
							best[pi] = combo
						}
						if !anyFish && (!noFishFound[pi] || sc > noFishScore[pi]) {
							noFishFound[pi] = true
							noFishScore[pi] = sc
							noFish[pi] = combo
						}
					}
				}
			}
		}
	}
	var out []FoodCombo
	for pi := range picks {
		if !found[pi] {
			continue
		}
		b := best[pi]
		hasFish := false
		for _, f := range b.Foods {
			if fishy != nil && fishy(f) {
				hasFish = true
			}
		}
		if hasFish && noFishFound[pi] {
			alt := noFish[pi]
			b.Alt = &alt
		}
		out = append(out, b)
	}
	return out
}

// needsFish is whether an item is a fish, raw fish, or made from either
// anywhere down its recipes and conversions: what takes a rod to have.
func (s *Site) needsFish(it *model.Item) bool {
	seen := map[string]bool{}
	var walk func(p string, depth int) bool
	walk = func(p string, depth int) bool {
		if seen[p] || depth > 8 {
			return false
		}
		seen[p] = true
		if x := s.DB.Items[p]; x != nil && (x.Type == "Fish" || p == "FishRaw" || p == "FishAnglerRaw") {
			return true
		}
		for _, r := range s.Index.Recipes[p] {
			for _, res := range r.Resources {
				if walk(res.Item, depth+1) {
					return true
				}
			}
		}
		for _, c := range s.Index.Converts[p] {
			if c.Conversion.From != "" && walk(c.Conversion.From, depth+1) {
				return true
			}
		}
		return false
	}
	return walk(it.Prefab, 0)
}

// pagesGuides writes the guides: the food guide here, the rest in
// guides_gear.go.
func (s *Site) pagesGuides() error {
	guide := s.foodGuide()
	if err := s.write("guides/food/", "food", &Page{Site: s, Title: "Best food by biome",
		Description: "The best three foods to eat together in every biome of Valheim: the most health, stamina or eitr, and the two-and-one menus for fighting, working and casting, worked out from what the game can have handed you by then.",
		Section:     "guides", Path: "guides/food/", Crumbs: []Crumb{{"Guides", "guides/"}, {"Best food", ""}}, Data: guide}); err != nil {
		return err
	}
	return s.pagesGuideSet()
}
