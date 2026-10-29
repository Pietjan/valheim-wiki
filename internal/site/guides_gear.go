package site

import (
	"fmt"
	"sort"
	"strings"

	"github.com/pietjan/valheim-wiki/internal/model"
)

// The gear, boss and comfort guides: what the data says is the best thing
// to wear, swing, build and bring at each stage, with the same notion of a
// stage as the food guide — the biome by which everything in it can be
// had.

// GearPick is one chosen item and the number it was chosen by.
type GearPick struct {
	Slot   string
	Item   *model.Item
	Value  float64
	Detail string
}

// GearStage is one biome's best armour and weapons.
type GearStage struct {
	Biome      string
	Armor      []GearPick // helmet, chest, legs, cape; those that exist by then
	ArmorTotal float64
	Weight     float64
	Movement   float64 // the set's movement penalty, summed, as a fraction
	Shield     *GearPick
	Weapons    []GearPick // the best of each skill
}

// GearGuide is the gear guide, built once.
type GearGuide struct {
	Stages  []GearStage
	ByBiome map[string]*GearStage
}

var armorSlots = []struct{ typ, slot string }{
	{"Helmet", "Head"}, {"Chest", "Chest"}, {"Legs", "Legs"}, {"Shoulder", "Cape"},
}

var weaponSkills = []string{"Swords", "Axes", "Clubs", "Knives", "Spears", "Polearms", "Bows", "Crossbows", "ElementalMagic", "BloodMagic"}

// atMax is a stat at the item's top quality.
func atMax(it *model.Item, base, perLevel float64) float64 {
	q := it.MaxQuality
	if q < 1 {
		q = 1
	}
	return base + perLevel*float64(q-1)
}

// weaponDamage is a weapon's damage at top quality by type, without the
// chop and pickaxe that only fell trees and rock, and the sum of it.
func weaponDamage(it *model.Item) (map[string]float64, float64) {
	out := map[string]float64{}
	total := 0.0
	for typ, v := range it.Damage {
		if typ == "chop" || typ == "pickaxe" {
			continue
		}
		d := atMax(it, v, it.DamagePerLevel[typ])
		if d <= 0 {
			continue
		}
		out[typ] = d
		total += d
	}
	return out, total
}

func damageDetail(d map[string]float64) string {
	types := make([]string, 0, len(d))
	for t := range d {
		types = append(types, t)
	}
	sort.Slice(types, func(i, j int) bool { return d[types[i]] > d[types[j]] })
	parts := make([]string, 0, len(types))
	for _, t := range types {
		parts = append(parts, fmt.Sprintf("%s %s", Num(d[t]), t))
	}
	return strings.Join(parts, " · ")
}

// gearGuide builds the gear guide once.
func (s *Site) gearGuide() *GearGuide {
	if s.gear != nil {
		return s.gear
	}
	g := s.stagerFor()
	guide := &GearGuide{ByBiome: map[string]*GearStage{}}
	type staged struct {
		it    *model.Item
		stage int
	}
	var items []staged
	for _, it := range s.Index.Items {
		if !s.visible(it) || it.TamedOnly || it.DLC != "" {
			continue
		}
		if it.Armor <= 0 && it.BlockPower <= 0 && len(it.Damage) == 0 {
			continue
		}
		if st := g.item(it.Prefab); st != stageUnknown {
			items = append(items, staged{it, st})
		}
	}
	for bi, biome := range model.Biomes {
		if biome == "ocean" {
			continue
		}
		st := GearStage{Biome: biome}
		for _, slot := range armorSlots {
			var best *GearPick
			for _, x := range items {
				if x.stage > bi || x.it.Type != slot.typ || x.it.Armor <= 0 {
					continue
				}
				v := atMax(x.it, x.it.Armor, x.it.ArmorPerLevel)
				if best == nil || v > best.Value || (v == best.Value && x.it.Weight < best.Item.Weight) {
					best = &GearPick{Slot: slot.slot, Item: x.it, Value: v, Detail: Num(v) + " armour"}
				}
			}
			if best != nil {
				st.Armor = append(st.Armor, *best)
				st.ArmorTotal += best.Value
				st.Weight += atMax(best.Item, best.Item.Weight, best.Item.WeightPerLevel)
				st.Movement += best.Item.Modifiers["movement"]
			}
		}
		for _, x := range items {
			if x.stage > bi || x.it.Type != "Shield" || x.it.BlockPower <= 0 {
				continue
			}
			v := atMax(x.it, x.it.BlockPower, x.it.BlockPowerPerLevel)
			if st.Shield == nil || v > st.Shield.Value {
				st.Shield = &GearPick{Slot: "Shield", Item: x.it, Value: v, Detail: Num(v) + " block"}
			}
		}
		for _, skill := range weaponSkills {
			var best *GearPick
			for _, x := range items {
				if x.stage > bi || x.it.Skill != skill || len(x.it.Damage) == 0 {
					continue
				}
				switch x.it.Type {
				case "OneHandedWeapon", "TwoHandedWeapon", "TwoHandedWeaponLeft", "Bow":
				default:
					continue
				}
				d, total := weaponDamage(x.it)
				if total <= 0 {
					continue
				}
				if best == nil || total > best.Value {
					best = &GearPick{Slot: skillName(skill), Item: x.it, Value: total, Detail: damageDetail(d)}
				}
			}
			if best != nil {
				st.Weapons = append(st.Weapons, *best)
			}
		}
		guide.Stages = append(guide.Stages, st)
	}
	for i := range guide.Stages {
		guide.ByBiome[guide.Stages[i].Biome] = &guide.Stages[i]
	}
	s.gear = guide
	return guide
}

func skillName(skill string) string {
	switch skill {
	case "ElementalMagic":
		return "Elemental magic"
	case "BloodMagic":
		return "Blood magic"
	}
	return skill
}

// stagerFor is the one stager a build shares.
func (s *Site) stagerFor() *stager {
	if s.stages == nil {
		s.stages = newStager(s)
	}
	return s.stages
}

// --- bosses -------------------------------------------------------------

// BossPrep is what to know and bring for one Forsaken.
type BossPrep struct {
	Creature *model.Creature
	Altar    *model.Location
	Offering *model.Offering
	Power    *model.Effect
	Biome    string
	Weak     []string // damage types it takes more of
	Resists  []string // damage types it takes less of, or none
	Weapons  []GearPick
	Armor    *GearStage
	Food     []FoodCombo
}

// damageFactor is what a creature's damage modifier does to a hit.
func damageFactor(mod string) float64 {
	switch mod {
	case "VeryWeak":
		return 2
	case "Weak":
		return 1.5
	case "Resistant":
		return 0.5
	case "VeryResistant":
		return 0.25
	case "Immune", "Ignore":
		return 0
	}
	return 1
}

// bossPreps is the boss guide: the Forsaken in order.
func (s *Site) bossPreps() []BossPrep {
	ix := s.Index
	gear := s.gearGuide()
	food := s.foodGuide()
	var bosses []*model.Creature
	for _, c := range ix.Creatures {
		if c.Boss && c.BossOrder > 0 {
			bosses = append(bosses, c)
		}
	}
	sort.Slice(bosses, func(i, j int) bool { return bosses[i].BossOrder < bosses[j].BossOrder })
	var out []BossPrep
	for _, c := range bosses {
		p := BossPrep{Creature: c}
		for _, l := range s.DB.Locations {
			if l.Contents != nil && l.Contents.Offering != nil && l.Contents.Offering.Boss == c.Prefab {
				p.Altar, p.Offering = l, l.Contents.Offering
				break
			}
		}
		switch {
		case p.Altar != nil && len(p.Altar.Biomes) > 0:
			p.Biome = p.Altar.Biomes[0]
		case ix.Primary[c.Prefab] != "":
			p.Biome = ix.Primary[c.Prefab]
		case len(c.Biomes) > 0:
			p.Biome = c.Biomes[0]
		case c.BossOrder >= 1 && c.BossOrder <= 8:
			// The Forsaken come one a biome, in the biomes' order.
			p.Biome = model.Biomes[c.BossOrder-1]
		}
		for _, e := range ix.Effects {
			if e.GuardianOf == c.Prefab {
				p.Power = e
			}
		}
		mods := map[string]float64{}
		for _, r := range c.Resist {
			mods[r.Type] = damageFactor(r.Modifier)
			switch r.Modifier {
			case "Weak", "VeryWeak":
				p.Weak = append(p.Weak, r.Type)
			case "Resistant", "VeryResistant", "Immune", "Ignore":
				if r.Type != "chop" && r.Type != "pickaxe" {
					p.Resists = append(p.Resists, r.Type)
				}
			}
		}
		if st := gear.ByBiome[p.Biome]; st != nil {
			p.Armor = st
			// The weapons by what they do to this boss: each type scaled by
			// its modifier.
			var picks []GearPick
			for _, w := range st.Weapons {
				d, _ := weaponDamage(w.Item)
				eff := 0.0
				for t, v := range d {
					f, ok := mods[t]
					if !ok {
						f = 1
					}
					eff += v * f
				}
				if eff <= 0 {
					continue
				}
				picks = append(picks, GearPick{Slot: w.Slot, Item: w.Item, Value: eff, Detail: Num(eff) + " effective · " + w.Detail})
			}
			sort.SliceStable(picks, func(i, j int) bool { return picks[i].Value > picks[j].Value })
			if len(picks) > 4 {
				picks = picks[:4]
			}
			p.Weapons = picks
		}
		if fs := food.ByBiome[p.Biome]; fs != nil {
			p.Food = fs.Combos
		}
		out = append(out, p)
	}
	return out
}

// --- comfort ------------------------------------------------------------

// ComfortStage is the most comfort a base can reach by a biome, and what
// gives it: one piece per comfort group (pieces of a group do not stack)
// and every ungrouped piece.
type ComfortStage struct {
	Biome  string
	Total  int
	Pieces []*model.Piece
	Rested string // how long the rested bonus lasts at that comfort
}

func (s *Site) comfortGuide() []ComfortStage {
	g := s.stagerFor()
	var out []ComfortStage
	for bi, biome := range model.Biomes {
		if biome == "ocean" {
			continue
		}
		best := map[string]*model.Piece{} // group → piece
		var loose []*model.Piece
		for _, k := range model.SortedKeys(s.DB.Pieces) {
			p := s.DB.Pieces[k]
			if p.Comfort <= 0 || !p.Enabled || g.piece(p.Prefab) > bi {
				continue
			}
			if p.ComfortGroup == "" {
				loose = append(loose, p)
				continue
			}
			if cur := best[p.ComfortGroup]; cur == nil || p.Comfort > cur.Comfort {
				best[p.ComfortGroup] = p
			}
		}
		st := ComfortStage{Biome: biome}
		for _, p := range best {
			st.Pieces = append(st.Pieces, p)
			st.Total += p.Comfort
		}
		for _, p := range loose {
			st.Pieces = append(st.Pieces, p)
			st.Total += p.Comfort
		}
		sort.Slice(st.Pieces, func(i, j int) bool {
			if st.Pieces[i].Comfort != st.Pieces[j].Comfort {
				return st.Pieces[i].Comfort > st.Pieces[j].Comfort
			}
			return st.Pieces[i].Name < st.Pieces[j].Name
		})
		// SE_Rested: eight minutes, and a minute more per comfort level.
		st.Rested = Duration(float64(480 + 60*st.Total))
		out = append(out, st)
	}
	return out
}

// pagesGuideSet writes the gear, boss and comfort guides and the index.
func (s *Site) pagesGuideSet() error {
	if err := s.write("guides/gear/", "gear", &Page{Site: s, Title: "Best gear by biome",
		Description: "The best armour set, shield and weapon of every skill in each biome of Valheim, at top quality, out of what can be made by then.",
		Section:     "guides", Path: "guides/gear/", Crumbs: []Crumb{{"Guides", "guides/"}, {"Best gear", ""}}, Data: s.gearGuide()}); err != nil {
		return err
	}
	if err := s.write("guides/bosses/", "bossprep", &Page{Site: s, Title: "Boss preparation",
		Description: "What to bring to each Forsaken of Valheim: its weaknesses and resistances, the weapons that hit it hardest, the armour and food of its biome, the offering and the power it grants.",
		Section:     "guides", Path: "guides/bosses/", Crumbs: []Crumb{{"Guides", "guides/"}, {"Bosses", ""}}, Data: s.bossPreps()}); err != nil {
		return err
	}
	if err := s.write("guides/comfort/", "comfort", &Page{Site: s, Title: "Comfort and rest",
		Description: "The most comfort a base can reach in each biome of Valheim, the pieces that give it, and how long the rested bonus then lasts.",
		Section:     "guides", Path: "guides/comfort/", Crumbs: []Crumb{{"Guides", "guides/"}, {"Comfort", ""}}, Data: s.comfortGuide()}); err != nil {
		return err
	}
	return s.write("guides/", "guides", &Page{Site: s, Title: "Guides",
		Description: "What the numbers add up to: the best food, gear and comfort in every biome of Valheim, and what to bring to each boss.",
		Section:     "guides", Path: "guides/", Data: nil})
}
