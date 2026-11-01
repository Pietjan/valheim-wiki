package site

import (
	"fmt"
	"strings"
)

// The hover cards: tips.json holds, for every thing a chip can point to,
// its name, type line, icon, page and two or three lines of what matters
// about it, by the key the chip carries. The page's script fetches it
// once on the first hover and draws the card beside the chip.

// Tip is one row of tips.json.
type Tip struct {
	N string   `json:"n"`           // name
	S string   `json:"s,omitempty"` // sub line
	I string   `json:"i,omitempty"` // icon path under icons/
	U string   `json:"u,omitempty"` // url (site-relative)
	L []string `json:"l,omitempty"` // the lines
}

// writeTips writes tips.json.
func (s *Site) writeTips() error {
	tips := map[string]Tip{}
	for key, t := range s.Index.Things {
		if t.Kind == "item" {
			if it := s.DB.Items[t.Prefab]; it != nil && !s.visible(it) {
				continue
			}
		}
		tips[key] = Tip{N: t.Name, S: t.Sub, I: t.Icon, U: t.URL, L: s.tipLines(key, t)}
	}
	return s.writeJSON("tips.json", tips)
}

// tipLines is what a card says under the name, by kind: the numbers a
// reader would look the page up for, and the game's own words where they
// are short.
func (s *Site) tipLines(key string, t *Thing) []string {
	var lines []string
	add := func(parts ...string) {
		var keep []string
		for _, p := range parts {
			if p != "" {
				keep = append(keep, p)
			}
		}
		if len(keep) > 0 {
			lines = append(lines, strings.Join(keep, " · "))
		}
	}
	db := s.DB
	switch t.Kind {
	case "item":
		it := db.Items[t.Prefab]
		if it == nil {
			break
		}
		if d, total := weaponDamage(it); total > 0 {
			add(Num(total)+" damage at top quality", damageDetail(d))
		}
		if it.Armor > 0 {
			add(Num(atMax(it, it.Armor, it.ArmorPerLevel)) + " armour at top quality")
		}
		if it.BlockPower > 0 && it.Armor == 0 && it.Damage.Total() == 0 {
			add(Num(atMax(it, it.BlockPower, it.BlockPowerPerLevel)) + " block at top quality")
		}
		if it.Food > 0 || it.FoodStamina > 0 || it.FoodEitr > 0 {
			var f []string
			if it.Food > 0 {
				f = append(f, Num(it.Food)+" hp")
			}
			if it.FoodStamina > 0 {
				f = append(f, Num(it.FoodStamina)+" stamina")
			}
			if it.FoodEitr > 0 {
				f = append(f, Num(it.FoodEitr)+" eitr")
			}
			if it.FoodBurn > 0 {
				f = append(f, Duration(it.FoodBurn))
			}
			add(f...)
		}
		var meta []string
		if it.ToolTier > 0 && it.BuildPieces == "" {
			meta = append(meta, fmt.Sprintf("tier %d", it.ToolTier))
		}
		if it.Weight > 0 {
			meta = append(meta, "weight "+Num(it.Weight))
		}
		if it.Stack > 1 {
			meta = append(meta, fmt.Sprintf("stacks to %d", it.Stack))
		}
		if !it.Teleportable {
			meta = append(meta, "no portals")
		}
		add(meta...)
		// How it is made: the first recipe's materials and station, or the
		// cooking station it comes off and what goes on, with that thing's
		// own materials when it has a recipe, so a pie says what its dough
		// took.
		if n := s.needsLine(t.Prefab); n != "" {
			add(n)
		} else if cs := s.Index.Converts[t.Prefab]; len(cs) > 0 {
			c := cs[0]
			if c.Conversion.From != "" {
				add("from " + s.nameOf(c.Conversion.From) + " on " + s.nameOf(c.Converter.Prefab))
				if n := s.needsLine(c.Conversion.From); n != "" {
					add(s.nameOf(c.Conversion.From) + " " + n)
				}
			}
		}
		if s.Unplaced(t.Prefab) {
			add("nothing in the game places it")
		}
		add(short(it.Description))
	case "creature":
		c := db.Creatures[t.Prefab]
		if c == nil {
			break
		}
		hp := ""
		if c.Health > 0 {
			hp = Num(c.Health) + " hp"
		}
		add(hp, strings.Join(biomeNames(c.Biomes), ", "))
		var weak, resist []string
		for _, m := range c.Resist {
			switch m.Modifier {
			case "Weak", "VeryWeak":
				weak = append(weak, m.Type)
			case "Resistant", "VeryResistant", "Immune", "Ignore":
				resist = append(resist, m.Type)
			}
		}
		if len(weak) > 0 {
			add("weak to " + strings.Join(weak, ", "))
		}
		if len(resist) > 0 {
			add("resists " + strings.Join(resist, ", "))
		}
	case "location":
		l := db.Locations[t.Prefab]
		if l == nil {
			break
		}
		if l.Unique {
			add("one per world")
		} else if l.Quantity > 0 {
			add(fmt.Sprintf("up to %d per world", l.Quantity))
		}
	case "piece":
		p := db.Pieces[t.Prefab]
		if p == nil {
			break
		}
		var need []string
		for _, r := range p.Resources {
			need = append(need, fmt.Sprintf("%s ×%d", s.nameOf(r.Item), r.Amount))
		}
		if len(need) > 0 {
			add("needs " + strings.Join(need, ", "))
		}
		var meta []string
		if p.Comfort > 0 {
			meta = append(meta, fmt.Sprintf("comfort %d", p.Comfort))
		}
		if p.Health > 0 {
			meta = append(meta, Num(p.Health)+" hp")
		}
		if p.Station != "" {
			meta = append(meta, "near "+s.nameOf(p.Station))
		}
		add(meta...)
		add(short(p.Description))
	case "resource":
		r := db.Resources[t.Prefab]
		if r == nil {
			break
		}
		if r.Item != "" {
			n := ""
			if r.Amount > 1 {
				n = fmt.Sprintf(" ×%d", r.Amount)
			}
			add("gives " + s.nameOf(r.Item) + n)
		}
		if r.Respawn > 0 {
			add("grows back in " + Duration(r.Respawn*60))
		}
		if r.MinToolTier > 0 {
			add(fmt.Sprintf("needs a tier %d tool", r.MinToolTier))
		}
	case "effect":
		if e := db.Effects[t.Prefab]; e != nil {
			add(short(e.Tooltip))
			// A second or less is a placeholder the game sets in play.
			if e.TTL > 1 {
				add("lasts " + Duration(e.TTL))
			}
		}
	case "skill":
		for _, sk := range db.Skills {
			if sk.ID == t.Prefab {
				add(short(sk.Description))
			}
		}
	case "station":
		if st := db.Stations[t.Prefab]; st != nil {
			var meta []string
			if st.MaxLevel > 1 {
				meta = append(meta, fmt.Sprintf("up to level %d", st.MaxLevel))
			}
			if st.RequireRoof {
				meta = append(meta, "needs a roof")
			}
			if st.RequireFire {
				meta = append(meta, "needs a fire")
			}
			add(meta...)
		}
	case "converter":
		if c := db.Converters[t.Prefab]; c != nil {
			var meta []string
			if c.SecPerItem > 0 {
				meta = append(meta, Duration(c.SecPerItem)+" an item")
			}
			if c.Fuel != "" {
				meta = append(meta, "burns "+s.nameOf(c.Fuel))
			}
			add(meta...)
		}
	case "trader":
		for _, tr := range db.Traders {
			if tr.Prefab == t.Prefab {
				add(fmt.Sprintf("sells %d things", len(tr.Items)))
			}
		}
	case "biome":
		for _, b := range db.Biomes {
			if b.ID == t.Prefab && b.Boss != "" {
				add("boss: " + s.nameOf(b.Boss))
			}
		}
	case "achievement":
		for _, a := range db.Achievements {
			if a.ID == t.Prefab {
				add(short(a.Description))
			}
		}
	}
	return lines
}

// needsLine is "needs A ×n, B ×m at Station level k" from an item's first
// recipe, or "" without one.
func (s *Site) needsLine(prefab string) string {
	rs := s.Index.Recipes[prefab]
	if len(rs) == 0 {
		return ""
	}
	r := rs[0]
	var need []string
	for _, res := range r.Resources {
		if res.Upgrader || res.Amount <= 0 {
			continue
		}
		need = append(need, fmt.Sprintf("%s ×%d", s.nameOf(res.Item), res.Amount))
	}
	if len(need) == 0 {
		return ""
	}
	at := ""
	if r.Station != "" {
		at = " at " + s.nameOf(r.Station)
		if r.MinStationLevel > 1 {
			at += fmt.Sprintf(" level %d", r.MinStationLevel)
		}
	}
	return "needs " + strings.Join(need, ", ") + at
}

// nameOf is a thing's name by key, or the key.
func (s *Site) nameOf(key string) string {
	if t := s.Index.Things[key]; t != nil {
		return t.Name
	}
	return key
}

// short is a text cut to its first sentence or so, for a card.
func short(text string) string {
	text = strings.TrimSpace(strings.ReplaceAll(text, "\n", " "))
	if text == "" {
		return ""
	}
	if i := strings.Index(text, ". "); i > 0 && i < 140 {
		return text[:i+1]
	}
	if len(text) > 140 {
		if i := strings.LastIndex(text[:140], " "); i > 60 {
			return text[:i] + "…"
		}
	}
	return text
}
