package site

import (
	"sort"
	"strings"

	"github.com/pietjan/valheim-wiki/internal/model"
)

// ItemGroup is one heading of the items index.
type ItemGroup struct {
	ID    string
	Name  string
	Items []*model.Item
	Types []string
}

// itemGroups is the index order and which types land where.
var itemGroups = []ItemGroup{
	{ID: "weapons", Name: "Weapons", Types: []string{"OneHandedWeapon", "TwoHandedWeapon", "TwoHandedWeaponLeft", "Bow", "Attach_Atgeir"}},
	{ID: "shields", Name: "Shields", Types: []string{"Shield"}},
	{ID: "tools", Name: "Tools", Types: []string{"Tool", "Torch"}},
	{ID: "ammo", Name: "Ammo", Types: []string{"Ammo", "AmmoNonEquipable"}},
	{ID: "armor", Name: "Armor", Types: []string{"Helmet", "Chest", "Legs", "Shoulder", "Hands", "Utility", "Trinket", "Customization"}},
	{ID: "food", Name: "Food", Types: []string{"food"}},
	{ID: "meads", Name: "Meads and potions", Types: []string{"mead"}},
	{ID: "materials", Name: "Materials", Types: []string{"Material"}},
	{ID: "trophies", Name: "Trophies", Types: []string{"Trophy"}},
	{ID: "fish", Name: "Fish", Types: []string{"Fish"}},
	{ID: "misc", Name: "Miscellaneous", Types: []string{"Misc", "Consumable", "None"}},
}

// groupOf is the index group an item belongs to.
func groupOf(it *model.Item) string {
	if it.Type == "Consumable" {
		if it.Food > 0 || it.FoodStamina > 0 || it.FoodEitr > 0 {
			return "food"
		}
		if it.ConsumeEffect != "" || it.IsDrink {
			return "meads"
		}
	}
	for _, g := range itemGroups {
		for _, t := range g.Types {
			if t == it.Type {
				return g.ID
			}
		}
	}
	return "misc"
}

// visible is whether an item gets a page: the game shows it somewhere
// (it has an icon) or something makes it.
func (s *Site) visible(it *model.Item) bool {
	if it.Hidden {
		return len(s.Index.Recipes[it.Prefab]) > 0
	}
	return true
}

// ItemPage is one item's page data.
type ItemPage struct {
	Item      *model.Item
	Thing     *Thing
	Group     string
	Recipes   []*model.Recipe
	Stations  map[string]*model.Station
	Qualities []QualityRow
	UsedIn    []*model.Recipe
	Builds    []*model.Piece
	Dropped   []DropLink
	Gathered  []*model.Resource
	Sold      []SoldLink
	Chests    []ChestLink
	Converts  []ConvertLink
	Consumed  []ConvertLink
	Tamers    []*model.Creature
	Set       []*model.Item
	SetEffect *model.Effect
	Equip     *model.Effect
	Consume   *model.Effect
	AttackFx  *model.Effect
	TrophyOf  *model.Creature
	Sources   []string // location prefabs where it is found, for your-world
	Biomes    []string // where its sources are
	IsWeapon  bool
	IsArmor   bool
	IsShield  bool
	IsFood    bool
	IsTool    bool
	Upgrade   *model.Recipe // the recipe whose per-level costs apply
}

// QualityRow is one quality level's numbers.
type QualityRow struct {
	Q          int
	Damage     model.Damage
	Armor      float64
	Block      float64
	Deflection float64
	Durability float64
	Weight     float64
	Cost       []model.Requirement // what this level costs on top of the previous
}

func isWeapon(t string) bool {
	switch t {
	case "OneHandedWeapon", "TwoHandedWeapon", "TwoHandedWeaponLeft", "Bow", "Torch", "Attach_Atgeir":
		return true
	}
	return false
}
func isArmor(t string) bool {
	switch t {
	case "Helmet", "Chest", "Legs", "Shoulder", "Hands", "Utility", "Trinket":
		return true
	}
	return false
}

func (s *Site) pagesItems() error {
	ix := s.Index
	var groups []ItemGroup
	byGroup := map[string]*ItemGroup{}
	for _, g := range itemGroups {
		gg := g
		groups = append(groups, gg)
	}
	for i := range groups {
		byGroup[groups[i].ID] = &groups[i]
	}
	var visible []*model.Item
	for _, it := range ix.Items {
		if !s.visible(it) {
			continue
		}
		visible = append(visible, it)
		g := byGroup[groupOf(it)]
		g.Items = append(g.Items, it)
	}
	if err := s.write("items/", "items", &Page{Site: s, Title: "Items", Description: "Every item in Valheim: weapons, armor, tools, food, meads, materials and trophies, with stats per quality level and how to get them.", Section: "items", Path: "items/", Data: map[string]any{"Groups": groups, "Count": len(visible)}}); err != nil {
		return err
	}
	for _, it := range visible {
		pg := s.itemPage(it)
		t := ix.Things[it.Prefab]
		desc := it.Description
		if desc == "" {
			desc = it.Name + " in Valheim: stats, recipe and where to find it."
		}
		page := &Page{Site: s, Title: it.Name, Description: strings.TrimSpace(desc), Section: "items", Path: t.URL, Icon: s.absIcon(t.Icon),
			Crumbs: []Crumb{{"Items", "items/"}, {byGroup[pg.Group].Name, "items/#" + pg.Group}, {it.Name, ""}}, Data: pg}
		if len(pg.Sources) > 0 || len(pg.Biomes) > 0 {
			page.YourWorld = &YourWorld{Kind: "item", Prefabs: pg.Sources, Biomes: pg.Biomes, Label: it.Name}
		}
		if err := s.write(t.URL, "item", page); err != nil {
			return err
		}
	}
	return nil
}

func (s *Site) absIcon(icon string) string {
	if icon == "" {
		return ""
	}
	return strings.TrimSuffix(s.Origin, "/") + s.URL("icons/"+icon+".png")
}

func (s *Site) itemPage(it *model.Item) *ItemPage {
	ix := s.Index
	pg := &ItemPage{Item: it, Thing: ix.Things[it.Prefab], Group: groupOf(it), Stations: s.DB.Stations,
		Recipes: ix.Recipes[it.Prefab], UsedIn: ix.UsedIn[it.Prefab], Builds: ix.BuildsWith[it.Prefab],
		Dropped: ix.DroppedBy[it.Prefab], Gathered: ix.Gathered[it.Prefab], Sold: ix.Sold[it.Prefab],
		Chests: ix.InChests[it.Prefab], Converts: ix.Converts[it.Prefab], Consumed: ix.Consumed[it.Prefab],
		Tamers: ix.Tamers[it.Prefab], IsWeapon: isWeapon(it.Type), IsArmor: isArmor(it.Type), IsShield: it.Type == "Shield",
		IsFood: it.Food > 0 || it.FoodStamina > 0 || it.FoodEitr > 0, IsTool: it.Type == "Tool"}
	if it.SetName != "" {
		pg.Set = ix.Sets[it.SetName]
	}
	pg.SetEffect = s.DB.Effects[it.SetEffect]
	pg.Equip = s.DB.Effects[it.EquipEffect]
	pg.Consume = s.DB.Effects[it.ConsumeEffect]
	pg.AttackFx = s.DB.Effects[it.AttackEffect]
	if c := ix.TrophyOf[it.Prefab]; c != "" {
		pg.TrophyOf = s.DB.Creatures[c]
	}
	// Quality table.
	for _, r := range pg.Recipes {
		for _, res := range r.Resources {
			if res.PerLevel > 0 {
				pg.Upgrade = r
				break
			}
		}
		if pg.Upgrade != nil {
			break
		}
	}
	if it.MaxQuality > 1 {
		for q := 1; q <= it.MaxQuality; q++ {
			row := QualityRow{Q: q, Armor: it.Armor + float64(q-1)*it.ArmorPerLevel, Block: it.BlockPower + float64(q-1)*it.BlockPowerPerLevel,
				Deflection: it.Deflection + float64(q-1)*it.DeflectionPerLevel, Durability: it.Durability + float64(q-1)*it.DurabilityPerLevel,
				Weight: it.Weight + float64(q-1)*it.WeightPerLevel}
			if len(it.Damage) > 0 {
				row.Damage = model.Damage{}
				for k, v := range it.Damage {
					row.Damage[k] = v + float64(q-1)*it.DamagePerLevel[k]
				}
				for k, v := range it.DamagePerLevel {
					if _, ok := row.Damage[k]; !ok {
						row.Damage[k] = float64(q-1) * v
					}
				}
			}
			if pg.Upgrade != nil {
				for _, res := range pg.Upgrade.Resources {
					if res.Upgrader {
						continue
					}
					if q == 1 {
						if res.Amount > 0 {
							row.Cost = append(row.Cost, model.Requirement{Item: res.Item, Amount: res.Amount})
						}
					} else if res.PerLevel > 0 {
						row.Cost = append(row.Cost, model.Requirement{Item: res.Item, Amount: res.PerLevel * (q - 1)})
					}
				}
			}
			pg.Qualities = append(pg.Qualities, row)
		}
	}
	// Where it comes from, for the seed layer: biomes of its droppers and
	// gatherables, and the locations whose chests hold it.
	seenB := map[string]bool{}
	for _, d := range pg.Dropped {
		for _, b := range d.Creature.Biomes {
			seenB[b] = true
		}
	}
	for _, g := range pg.Gathered {
		for _, b := range g.Biomes {
			seenB[b] = true
		}
	}
	for b := range seenB {
		pg.Biomes = append(pg.Biomes, b)
	}
	sortBiomes(pg.Biomes)
	seenL := map[string]bool{}
	for _, c := range pg.Chests {
		if !seenL[c.Location.Prefab] {
			seenL[c.Location.Prefab] = true
			pg.Sources = append(pg.Sources, c.Location.Prefab)
		}
	}
	for _, sl := range pg.Sold {
		if sl.Trader.Location != "" && !seenL[sl.Trader.Location] {
			seenL[sl.Trader.Location] = true
			pg.Sources = append(pg.Sources, sl.Trader.Location)
		}
	}
	sort.Strings(pg.Sources)
	return pg
}
