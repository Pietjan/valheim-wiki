package site

import (
	"sort"
	"strings"

	"github.com/pietjan/valheim-wiki/internal/model"
)

// Index is everything derived from the DB that pages share: slugs and
// URLs for every thing, and the reverse links (what drops an item, what
// a recipe is used in, where a creature spawns, what a location holds).
type Index struct {
	DB *model.DB

	// Things by prefab: what kind it is and where its page is.
	Things map[string]*Thing
	// Lists in display order.
	Items     []*model.Item
	Pieces    []*model.Piece
	Creatures []*model.Creature
	Locations []*model.Location
	Resources []*model.Resource
	Effects   []*model.Effect

	// Location kinds: slug -> the variants (WoodHouse1..13 -> abandoned-house).
	Kinds          map[string]*Kind
	KindList       []*Kind
	KindOf         map[string]*Kind // location prefab -> kind
	ByBiome        map[string]*BiomeIndex
	Recipes        map[string][]*model.Recipe   // item -> recipes producing it
	UsedIn         map[string][]*model.Recipe   // item -> recipes consuming it
	BuildsWith     map[string][]*model.Piece    // item -> pieces costing it
	Converts       map[string][]ConvertLink     // item -> conversions producing it
	Consumed       map[string][]ConvertLink     // item -> conversions consuming it
	DroppedBy      map[string][]DropLink        // item -> creatures dropping it
	Gathered       map[string][]*model.Resource // item -> resources giving it
	Sold           map[string][]SoldLink        // item -> trader offers
	InChests       map[string][]ChestLink       // item -> location chests holding it
	SpawnsOf       map[string][]*model.Spawn    // creature -> world spawns
	Found          map[string][]*model.Location // creature -> locations with it
	EventsOf       map[string][]*model.Event    // creature -> events
	Tamers         map[string][]*model.Creature // item -> creatures that eat it (taming)
	Effects_       map[string][]EffectLink      // effect id -> what grants it
	Sets           map[string][]*model.Item     // set name -> items
	StationRecipes map[string][]*model.Recipe   // station -> recipes
	Extends        map[string]*model.Station    // extension piece -> station
	TrophyOf       map[string]string            // trophy item -> creature
	// Phases are a boss's later forms (FrozenKing_p2, _p3), folded into
	// its page rather than listed as creatures of their own.
	Phases map[string][]*model.Creature
	// Primary is the biome a creature is filed under: that of its first
	// ungated world spawn rule.
	Primary map[string]string
}

// Thing is one page-worthy entity.
type Thing struct {
	Kind   string // item, piece, creature, location, biome, effect, skill, trader, station, resource
	Prefab string
	Name   string
	Slug   string
	URL    string
	Icon   string // icon URL or ""
	Sub    string // a short type line
	// Hint tells two things of one name apart: the prefab, shown small
	// beside the name when the slug had to carry it.
	Hint string
}

// Kind groups location variants under one page.
type Kind struct {
	Slug     string
	Name     string
	Variants []*model.Location
	Biomes   []string
	Icon     string // prefab whose icon stands for the kind
	Quantity int
}

// BiomeIndex is what a biome page lists.
type BiomeIndex struct {
	Biome     *model.Biome
	Creatures []*model.Creature
	Kinds     []*Kind
	Resources []*model.Resource
	Items     []*model.Item // items first obtainable here (materials gathered/dropped)
	Spawns    []*model.Spawn
	Events    []*model.Event
	AltBiomes []*model.AltBiome
}

type ConvertLink struct {
	Converter  *model.Converter
	Conversion model.Conversion
}

type DropLink struct {
	Creature *model.Creature
	Drop     model.Drop
}

type SoldLink struct {
	Trader *model.Trader
	Item   model.TradeItem
}

type ChestLink struct {
	Location *model.Location
	Chest    model.Chest
	Entry    model.DropEntry
}

type EffectLink struct {
	Kind   string // item-set, item-equip, item-consume, item-attack, boss-power
	Prefab string
	Name   string
}

// BuildIndex derives everything.
func BuildIndex(db *model.DB, hasIcon func(kind, name string) bool) *Index {
	ix := &Index{DB: db, Things: map[string]*Thing{}, Kinds: map[string]*Kind{}, KindOf: map[string]*Kind{},
		ByBiome: map[string]*BiomeIndex{}, Recipes: map[string][]*model.Recipe{}, UsedIn: map[string][]*model.Recipe{},
		BuildsWith: map[string][]*model.Piece{}, Converts: map[string][]ConvertLink{}, Consumed: map[string][]ConvertLink{},
		DroppedBy: map[string][]DropLink{}, Gathered: map[string][]*model.Resource{}, Sold: map[string][]SoldLink{},
		InChests: map[string][]ChestLink{}, SpawnsOf: map[string][]*model.Spawn{}, Found: map[string][]*model.Location{},
		EventsOf: map[string][]*model.Event{}, Tamers: map[string][]*model.Creature{}, Effects_: map[string][]EffectLink{},
		Sets: map[string][]*model.Item{}, StationRecipes: map[string][]*model.Recipe{}, Extends: map[string]*model.Station{},
		TrophyOf: map[string]string{}, Phases: map[string][]*model.Creature{}, Primary: map[string]string{}}

	// Items.
	sl := newSlugger()
	for _, k := range model.SortedKeys(db.Items) {
		it := db.Items[k]
		if it.Prefab == "" {
			it.Prefab = k
		}
		ix.Items = append(ix.Items, it)
	}
	sort.SliceStable(ix.Items, func(i, j int) bool { return ix.Items[i].Name < ix.Items[j].Name })
	for _, it := range ix.Items {
		slug := sl.take(it.Name, it.Prefab)
		icon := ""
		if hasIcon("items", it.Prefab) {
			icon = "items/" + it.Prefab
		}
		ix.Things[it.Prefab] = &Thing{Kind: "item", Prefab: it.Prefab, Name: it.Name, Slug: slug, URL: "items/" + slug + "/", Icon: icon, Sub: ItemTypeName(it.Type)}
		if it.SetName != "" {
			ix.Sets[it.SetName] = append(ix.Sets[it.SetName], it)
		}
		if it.TrophyOf != "" {
			ix.TrophyOf[it.Prefab] = it.TrophyOf
		}
	}
	// Pieces.
	sl = newSlugger()
	for _, k := range model.SortedKeys(db.Pieces) {
		p := db.Pieces[k]
		if p.Prefab == "" {
			p.Prefab = k
		}
		ix.Pieces = append(ix.Pieces, p)
	}
	sort.SliceStable(ix.Pieces, func(i, j int) bool { return ix.Pieces[i].Name < ix.Pieces[j].Name })
	for _, p := range ix.Pieces {
		if _, taken := ix.Things[p.Prefab]; taken || p.Category == "All" {
			continue // "All" holds the hammer's repair and remove modes
		}
		slug := sl.take(p.Name, p.Prefab)
		icon := ""
		if hasIcon("pieces", p.Prefab) {
			icon = "pieces/" + p.Prefab
		}
		ix.Things[p.Prefab] = &Thing{Kind: "piece", Prefab: p.Prefab, Name: p.Name, Slug: slug, URL: "building/" + slug + "/", Icon: icon, Sub: PieceCategoryName(p.Category)}
		for _, r := range p.Resources {
			ix.BuildsWith[r.Item] = append(ix.BuildsWith[r.Item], p)
		}
	}
	// Stations and converters are pieces; give them crafting pages too.
	for _, k := range model.SortedKeys(db.Stations) {
		st := db.Stations[k]
		if st.Prefab == "" {
			st.Prefab = k
		}
		for _, e := range st.Extensions {
			ix.Extends[e] = st
		}
		if t := ix.Things[k]; t != nil {
			t.Kind = "station"
			t.URL = "crafting/" + t.Slug + "/"
			t.Sub = "Crafting station"
		} else {
			icon := ""
			if hasIcon("stations", k) {
				icon = "stations/" + k
			}
			slug := Slug(st.Name)
			ix.Things[k] = &Thing{Kind: "station", Prefab: k, Name: st.Name, Slug: slug, URL: "crafting/" + slug + "/", Icon: icon, Sub: "Crafting station"}
		}
	}
	for _, k := range model.SortedKeys(db.Converters) {
		c := db.Converters[k]
		if c.Prefab == "" {
			c.Prefab = k
		}
		if t := ix.Things[k]; t != nil {
			if t.Kind != "station" {
				t.Kind = "converter"
				t.URL = "crafting/" + t.Slug + "/"
				t.Sub = ConverterKindName(c.Kind)
			}
		} else {
			slug := Slug(c.Name)
			ix.Things[k] = &Thing{Kind: "converter", Prefab: k, Name: c.Name, Slug: slug, URL: "crafting/" + slug + "/", Sub: ConverterKindName(c.Kind)}
		}
		for _, cv := range c.Conversions {
			ix.Converts[cv.To] = append(ix.Converts[cv.To], ConvertLink{c, cv})
			ix.Consumed[cv.From] = append(ix.Consumed[cv.From], ConvertLink{c, cv})
		}
		for _, inc := range c.Incinerations {
			ix.Converts[inc.Result] = append(ix.Converts[inc.Result], ConvertLink{c, model.Conversion{From: "", To: inc.Result, Amount: inc.Amount}})
			for _, r := range inc.Requirements {
				ix.Consumed[r.Item] = append(ix.Consumed[r.Item], ConvertLink{c, model.Conversion{From: r.Item, To: inc.Result, Amount: inc.Amount}})
			}
		}
		if c.Product != "" {
			ix.Converts[c.Product] = append(ix.Converts[c.Product], ConvertLink{c, model.Conversion{To: c.Product}})
		}
	}
	// Recipes.
	for _, r := range db.Recipes {
		ix.Recipes[r.Item] = append(ix.Recipes[r.Item], r)
		for _, res := range r.Resources {
			ix.UsedIn[res.Item] = append(ix.UsedIn[res.Item], r)
		}
		ix.StationRecipes[r.Station] = append(ix.StationRecipes[r.Station], r)
	}
	// Creatures.
	sl = newSlugger()
	for _, k := range model.SortedKeys(db.Creatures) {
		c := db.Creatures[k]
		if c.Prefab == "" {
			c.Prefab = k
		}
		if base, ok := phaseOf(k); ok && db.Creatures[base] != nil {
			ix.Phases[base] = append(ix.Phases[base], c)
			if c.BossOrder > db.Creatures[base].BossOrder {
				db.Creatures[base].BossOrder = c.BossOrder
			}
			continue
		}
		ix.Creatures = append(ix.Creatures, c)
	}
	// The biome a creature is filed under: its first world spawn rule
	// without a global-key gate, else its first rule, else the
	// extractor's union.
	for _, sp := range db.Spawns {
		if !strings.HasPrefix(sp.Source, "world:") || !sp.Enabled || len(sp.Biomes) == 0 {
			continue
		}
		cur, seen := ix.Primary[sp.Prefab]
		if !seen || (strings.HasPrefix(cur, "?") && sp.RequiredKey == "") {
			if sp.RequiredKey != "" {
				ix.Primary[sp.Prefab] = "?" + sp.Biomes[0]
			} else {
				ix.Primary[sp.Prefab] = sp.Biomes[0]
			}
		}
	}
	for p, b := range ix.Primary {
		ix.Primary[p] = strings.TrimPrefix(b, "?")
	}
	for _, c := range ix.Creatures {
		if _, ok := ix.Primary[c.Prefab]; !ok && len(c.Biomes) > 0 {
			ix.Primary[c.Prefab] = c.Biomes[0]
		}
	}
	// A creature met only inside places (Bonemass, Surtling in a fire
	// hole) takes the biomes of those places.
	locBiomes := func(prefab string) []string {
		var out []string
		for _, l := range db.Locations {
			if l.Contents == nil || !l.Enabled {
				continue
			}
			hit := false
			for _, sp := range l.Contents.Spawners {
				hit = hit || sp.Creature == prefab
			}
			for _, cr := range l.Contents.Creatures {
				hit = hit || cr.Creature == prefab
			}
			for _, n := range l.Contents.Nests {
				for _, e := range n.Creatures {
					hit = hit || e.Creature == prefab
				}
			}
			if hit {
				for _, b := range l.Biomes {
					if !contains(out, b) {
						out = append(out, b)
					}
				}
			}
		}
		sortBiomes(out)
		return out
	}
	for _, c := range ix.Creatures {
		if len(c.Biomes) == 0 {
			c.Biomes = locBiomes(c.Prefab)
			if _, ok := ix.Primary[c.Prefab]; !ok && len(c.Biomes) > 0 {
				ix.Primary[c.Prefab] = c.Biomes[0]
			}
		}
	}
	sort.SliceStable(ix.Creatures, func(i, j int) bool { return ix.Creatures[i].Name < ix.Creatures[j].Name })
	for _, c := range ix.Creatures {
		slug := sl.take(c.Name, c.Prefab)
		icon := ""
		if hasIcon("creatures", c.Prefab) {
			icon = "creatures/" + c.Prefab
		} else if c.Trophy != "" && hasIcon("items", c.Trophy) {
			icon = "items/" + c.Trophy
		}
		sub := FactionName(c.Faction)
		if c.Boss {
			sub = "Boss"
		}
		ix.Things[c.Prefab] = &Thing{Kind: "creature", Prefab: c.Prefab, Name: c.Name, Slug: slug, URL: "creatures/" + slug + "/", Icon: icon, Sub: sub}
		for _, d := range c.Drops {
			ix.DroppedBy[d.Item] = append(ix.DroppedBy[d.Item], DropLink{c, d})
		}
		if c.Trophy != "" {
			ix.TrophyOf[c.Trophy] = c.Prefab
		}
		if c.AI != nil {
			for _, f := range c.AI.ConsumeItems {
				ix.Tamers[f] = append(ix.Tamers[f], c)
			}
		}
	}
	for _, sp := range db.Spawns {
		ix.SpawnsOf[sp.Prefab] = append(ix.SpawnsOf[sp.Prefab], sp)
	}
	spawnByID := map[string]*model.Spawn{}
	for _, sp := range db.Spawns {
		spawnByID[sp.ID] = sp
	}
	for _, ev := range db.Events {
		seen := map[string]bool{}
		for _, id := range ev.Spawns {
			if sp := spawnByID[id]; sp != nil && !seen[sp.Prefab] {
				seen[sp.Prefab] = true
				ix.EventsOf[sp.Prefab] = append(ix.EventsOf[sp.Prefab], ev)
			}
		}
	}
	// Resources.
	sl = newSlugger()
	for _, k := range model.SortedKeys(db.Resources) {
		r := db.Resources[k]
		if r.Prefab == "" {
			r.Prefab = k
		}
		ix.Resources = append(ix.Resources, r)
	}
	sort.SliceStable(ix.Resources, func(i, j int) bool { return ix.Resources[i].Name < ix.Resources[j].Name })
	for _, r := range ix.Resources {
		if _, taken := ix.Things[r.Prefab]; !taken {
			slug := sl.take(r.Name, r.Prefab)
			icon := ""
			if r.Item != "" && hasIcon("items", r.Item) {
				icon = "items/" + r.Item
			}
			ix.Things[r.Prefab] = &Thing{Kind: "resource", Prefab: r.Prefab, Name: r.Name, Slug: slug, URL: "resources/" + slug + "/", Icon: icon, Sub: ResourceKindName(r.Kind)}
		}
		if r.Item != "" {
			ix.Gathered[r.Item] = append(ix.Gathered[r.Item], r)
		}
		for _, t := range []*model.DropTable{r.Drops, r.ExtraDrops} {
			if t == nil {
				continue
			}
			for _, d := range t.Drops {
				if d.Item != r.Item {
					ix.Gathered[d.Item] = append(ix.Gathered[d.Item], r)
				}
			}
		}
	}
	dedupeResources(ix.Gathered)
	// Traders.
	for _, t := range db.Traders {
		slug := Slug(t.Name)
		ix.Things[t.Prefab] = &Thing{Kind: "trader", Prefab: t.Prefab, Name: t.Name, Slug: slug, URL: "traders/" + slug + "/", Icon: creatureIcon(hasIcon, t.Prefab), Sub: "Trader"}
		for _, ti := range t.Items {
			ix.Sold[ti.Item] = append(ix.Sold[ti.Item], SoldLink{t, ti})
		}
	}
	// Effects and skills.
	for _, k := range model.SortedKeys(db.Effects) {
		e := db.Effects[k]
		if e.ID == "" {
			e.ID = k
		}
		ix.Effects = append(ix.Effects, e)
	}
	sort.SliceStable(ix.Effects, func(i, j int) bool { return ix.Effects[i].Name < ix.Effects[j].Name })
	sl = newSlugger()
	for _, e := range ix.Effects {
		slug := sl.take(e.Name, e.ID)
		icon := ""
		if hasIcon("effects", e.ID) {
			icon = "effects/" + e.ID
		}
		ix.Things["effect:"+e.ID] = &Thing{Kind: "effect", Prefab: e.ID, Name: e.Name, Slug: slug, URL: "effects/" + slug + "/", Icon: icon, Sub: "Status effect"}
	}
	for _, it := range ix.Items {
		if it.SetEffect != "" {
			ix.Effects_[it.SetEffect] = append(ix.Effects_[it.SetEffect], EffectLink{"set", it.Prefab, it.Name})
		}
		if it.EquipEffect != "" {
			ix.Effects_[it.EquipEffect] = append(ix.Effects_[it.EquipEffect], EffectLink{"equip", it.Prefab, it.Name})
		}
		if it.ConsumeEffect != "" {
			ix.Effects_[it.ConsumeEffect] = append(ix.Effects_[it.ConsumeEffect], EffectLink{"consume", it.Prefab, it.Name})
		}
		if it.AttackEffect != "" {
			ix.Effects_[it.AttackEffect] = append(ix.Effects_[it.AttackEffect], EffectLink{"attack", it.Prefab, it.Name})
		}
	}
	for _, e := range ix.Effects {
		if e.GuardianOf != "" {
			name := e.GuardianOf
			if c := db.Creatures[e.GuardianOf]; c != nil {
				name = c.Name
			}
			ix.Effects_[e.ID] = append(ix.Effects_[e.ID], EffectLink{"boss", e.GuardianOf, name})
		}
	}
	for _, sk := range db.Skills {
		icon := ""
		if hasIcon("skills", sk.ID) {
			icon = "skills/" + sk.ID
		}
		ix.Things["skill:"+sk.ID] = &Thing{Kind: "skill", Prefab: sk.ID, Name: sk.Name, Slug: Slug(sk.Name), URL: "skills/#" + Slug(sk.Name), Icon: icon, Sub: "Skill"}
	}
	// Locations and kinds.
	for _, k := range model.SortedKeys(db.Locations) {
		l := db.Locations[k]
		if l.Prefab == "" {
			l.Prefab = k
		}
		ix.Locations = append(ix.Locations, l)
	}
	sort.SliceStable(ix.Locations, func(i, j int) bool {
		if ix.Locations[i].Name != ix.Locations[j].Name {
			return ix.Locations[i].Name < ix.Locations[j].Name
		}
		return ix.Locations[i].Prefab < ix.Locations[j].Prefab
	})
	for _, l := range ix.Locations {
		if !l.Enabled || strings.HasPrefix(l.Prefab, "Dev") {
			continue
		}
		ks := l.Kind
		if ks == "" {
			ks = Slug(l.Name)
		}
		kd := ix.Kinds[ks]
		if kd == nil {
			kd = &Kind{Slug: ks, Name: l.Name}
			ix.Kinds[ks] = kd
			ix.KindList = append(ix.KindList, kd)
		}
		kd.Variants = append(kd.Variants, l)
		kd.Quantity += l.Quantity
		for _, b := range l.Biomes {
			if !contains(kd.Biomes, b) {
				kd.Biomes = append(kd.Biomes, b)
			}
		}
		if kd.Icon == "" && hasIcon("locations", l.Prefab) {
			kd.Icon = l.Prefab
		}
		ix.KindOf[l.Prefab] = kd
		// A boss altar shares its prefab name with the boss (Bonemass,
		// GoblinKing): the location is always reachable as "loc:<prefab>",
		// and under the bare name only when nothing else holds it.
		lt := &Thing{Kind: "location", Prefab: l.Prefab, Name: l.Name, Slug: ks, URL: "locations/" + ks + "/", Icon: locIcon(hasIcon, l.Prefab), Sub: strings.Join(biomeNames(l.Biomes), ", ")}
		ix.Things["loc:"+l.Prefab] = lt
		if _, taken := ix.Things[l.Prefab]; !taken {
			ix.Things[l.Prefab] = lt
		}
		if l.Contents != nil {
			for _, ch := range l.Contents.Chests {
				if ch.Loot == nil {
					continue
				}
				for _, d := range ch.Loot.Drops {
					ix.InChests[d.Item] = append(ix.InChests[d.Item], ChestLink{l, ch, d})
				}
			}
			for _, sp := range l.Contents.Spawners {
				ix.Found[sp.Creature] = appendLoc(ix.Found[sp.Creature], l)
			}
			for _, n := range l.Contents.Nests {
				for _, e := range n.Creatures {
					ix.Found[e.Creature] = appendLoc(ix.Found[e.Creature], l)
				}
			}
			for _, c := range l.Contents.Creatures {
				ix.Found[c.Creature] = appendLoc(ix.Found[c.Creature], l)
			}
		}
	}
	sort.Slice(ix.KindList, func(i, j int) bool { return ix.KindList[i].Name < ix.KindList[j].Name })
	for _, kd := range ix.KindList {
		sortBiomes(kd.Biomes)
	}
	// Biomes.
	for _, b := range db.Biomes {
		bi := &BiomeIndex{Biome: b}
		ix.ByBiome[b.ID] = bi
		ix.Things["biome:"+b.ID] = &Thing{Kind: "biome", Prefab: b.ID, Name: b.Name, Slug: b.ID, URL: "biomes/" + b.ID + "/", Sub: "Biome"}
	}
	for _, c := range ix.Creatures {
		for _, b := range c.Biomes {
			if bi := ix.ByBiome[b]; bi != nil {
				bi.Creatures = append(bi.Creatures, c)
			}
		}
	}
	for _, kd := range ix.KindList {
		for _, b := range kd.Biomes {
			if bi := ix.ByBiome[b]; bi != nil {
				bi.Kinds = append(bi.Kinds, kd)
			}
		}
	}
	for _, r := range ix.Resources {
		for _, b := range r.Biomes {
			if bi := ix.ByBiome[b]; bi != nil {
				bi.Resources = append(bi.Resources, r)
			}
		}
	}
	for _, sp := range db.Spawns {
		if !strings.HasPrefix(sp.Source, "world:") {
			continue
		}
		for _, b := range sp.Biomes {
			if bi := ix.ByBiome[b]; bi != nil {
				bi.Spawns = append(bi.Spawns, sp)
			}
		}
	}
	for _, ev := range db.Events {
		for _, b := range ev.Biomes {
			if bi := ix.ByBiome[b]; bi != nil {
				bi.Events = append(bi.Events, ev)
			}
		}
	}
	for _, ab := range db.AltBiomes {
		if bi := ix.ByBiome[ab.Biome]; bi != nil {
			bi.AltBiomes = append(bi.AltBiomes, ab)
		}
	}
	for _, t := range ix.Things {
		if t.Slug != Slug(t.Name) && t.Slug != "" && Slug(t.Name) != "" {
			t.Hint = t.Prefab
		}
	}
	return ix
}

func appendLoc(list []*model.Location, l *model.Location) []*model.Location {
	for _, x := range list {
		if x == l {
			return list
		}
	}
	return append(list, l)
}

func dedupeResources(m map[string][]*model.Resource) {
	for k, list := range m {
		seen := map[*model.Resource]bool{}
		var out []*model.Resource
		for _, r := range list {
			if !seen[r] {
				seen[r] = true
				out = append(out, r)
			}
		}
		m[k] = out
	}
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func creatureIcon(hasIcon func(string, string) bool, prefab string) string {
	if hasIcon("creatures", prefab) {
		return "creatures/" + prefab
	}
	return ""
}

func locIcon(hasIcon func(string, string) bool, prefab string) string {
	if hasIcon("locations", prefab) {
		return "locations/" + prefab
	}
	return ""
}

// biomeOrder sorts biome slugs in progression order.
func sortBiomes(b []string) {
	rank := map[string]int{}
	for i, s := range model.Biomes {
		rank[s] = i
	}
	sort.Slice(b, func(i, j int) bool { return rank[b[i]] < rank[b[j]] })
}

func biomeNames(b []string) []string {
	out := make([]string, 0, len(b))
	for _, s := range b {
		out = append(out, model.BiomeName[s])
	}
	return out
}

// Thing looks a prefab up, or nil.
func (ix *Index) Thing(prefab string) *Thing { return ix.Things[prefab] }

// Display names for enums.

func ItemTypeName(t string) string {
	switch t {
	case "OneHandedWeapon":
		return "One-handed weapon"
	case "TwoHandedWeapon":
		return "Two-handed weapon"
	case "TwoHandedWeaponLeft":
		return "Two-handed weapon"
	case "AmmoNonEquipable":
		return "Ammo"
	case "Attach_Atgeir":
		return "Attachment"
	case "Helmet":
		return "Helmet"
	case "Chest":
		return "Chest armor"
	case "Legs":
		return "Leg armor"
	case "Shoulder":
		return "Cape"
	case "Hands":
		return "Gloves"
	case "Customization":
		return "Customization"
	case "Misc":
		return "Miscellaneous"
	}
	return t
}

func PieceCategoryName(c string) string {
	switch c {
	case "BuildingWorkbench":
		return "Building"
	case "BuildingStonecutter":
		return "Stone building"
	case "DeepNorth":
		return "Deep North"
	}
	return c
}

func ConverterKindName(k string) string {
	switch k {
	case "smelter":
		return "Smelter"
	case "cooking":
		return "Cooking station"
	case "fermenter":
		return "Fermenter"
	case "incinerator":
		return "Incinerator"
	case "beehive":
		return "Beehive"
	case "sap":
		return "Sap collector"
	case "feast":
		return "Feast"
	case "fireplace":
		return "Fire"
	case "turret":
		return "Turret"
	case "shield":
		return "Shield generator"
	case "trap":
		return "Trap"
	case "wisp":
		return "Wisp fountain"
	}
	return strings.ToUpper(k[:1]) + k[1:]
}

func FactionName(f string) string {
	switch f {
	case "AnimalsVeg":
		return "Animal"
	case "ForestMonsters":
		return "Forest monster"
	case "MountainMonsters":
		return "Mountain monster"
	case "SeaMonsters":
		return "Sea monster"
	case "PlainsMonsters":
		return "Plains monster"
	case "MistlandsMonsters":
		return "Mistlands monster"
	case "DeepNorth":
		return "Deep North monster"
	case "PlayerSpawned":
		return "Summoned"
	case "TrainingDummy":
		return "Training dummy"
	}
	return f
}

func ResourceKindName(k string) string {
	switch k {
	case "pickable":
		return "Pickable"
	case "tree":
		return "Tree"
	case "log":
		return "Log"
	case "rock":
		return "Ore and rock"
	case "destructible":
		return "Destructible"
	case "plant":
		return "Plant"
	}
	return k
}

// phaseOf is the base prefab of a boss phase: "FrozenKing_p2" -> "FrozenKing".
func phaseOf(prefab string) (string, bool) {
	for _, suf := range []string{"_p2", "_p3", "_p4"} {
		if strings.HasSuffix(prefab, suf) {
			return strings.TrimSuffix(prefab, suf), true
		}
	}
	return "", false
}
