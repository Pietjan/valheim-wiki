package site

import (
	"sort"
	"strings"

	"github.com/pietjan/valheim-wiki/internal/model"
)

// CreaturePage is a creature's page.
type CreaturePage struct {
	Lede      string
	Creature  *model.Creature
	Thing     *Thing
	Portrait  string // flat portrait URL
	Stars     []StarRow
	Spawns    []*model.Spawn
	Found     []*model.Location
	Events    []*model.Event
	Trophy    *model.Item
	Offspring *model.Creature
	Grown     *model.Creature
	Parent    *model.Creature // what grows into this one / is born as it
	Altar     *model.Location
	Offering  *model.Offering
	Power     *model.Effect
	Summoner  []*model.Item // staffs or items that summon it
	Kinds     []*Kind       // location kinds it is found in
	Phases    []*model.Creature
	RaidForm  *model.Creature // the copy a raid sends, at half health
	Keys      []*KeyUse       // what its defeat sets and unlocks, its phases' keys included
}

// StarRow is health and drop chance at one star level.
type StarRow struct {
	Level  int
	Stars  string
	Health float64
	Mult   int
}

// BestiaryGroup is one heading of the creature index.
type BestiaryGroup struct {
	ID        string
	Name      string
	Creatures []*model.Creature
}

func (s *Site) pagesCreatures() error {
	ix := s.Index
	// Groups: bosses first, then by first biome, then the rest.
	var groups []BestiaryGroup
	byBiome := map[string][]*model.Creature{}
	var bosses, minis, npcs, rest []*model.Creature
	for _, c := range ix.Creatures {
		switch {
		case c.Boss && c.BossOrder > 0:
			bosses = append(bosses, c)
		case c.MiniBoss:
			minis = append(minis, c)
		case c.NPC:
			npcs = append(npcs, c)
		case ix.Primary[c.Prefab] != "":
			b := ix.Primary[c.Prefab]
			byBiome[b] = append(byBiome[b], c)
		default:
			rest = append(rest, c)
		}
	}
	sort.Slice(bosses, func(i, j int) bool { return bosses[i].BossOrder < bosses[j].BossOrder })
	groups = append(groups, BestiaryGroup{"bosses", "Forsaken", bosses})
	if len(minis) > 0 {
		groups = append(groups, BestiaryGroup{"minibosses", "Mini bosses", minis})
	}
	for _, b := range model.Biomes {
		if cs := byBiome[b]; len(cs) > 0 {
			groups = append(groups, BestiaryGroup{b, model.BiomeName[b], cs})
		}
	}
	if len(npcs) > 0 {
		groups = append(groups, BestiaryGroup{"npcs", "People", npcs})
	}
	if len(rest) > 0 {
		groups = append(groups, BestiaryGroup{"other", "Summoned, tamed and others", rest})
	}
	if err := s.write("creatures/", "creatures", &Page{Site: s, Title: "Creatures", Description: "The bestiary of Valheim: every creature and boss with health per star, damage, resistances, drops, taming and where it spawns.", Section: "creatures", Path: "creatures/",
		Data: map[string]any{"Groups": groups, "Count": len(ix.Creatures)}}); err != nil {
		return err
	}
	// Bosses page.
	var bossPages []*CreaturePage
	for _, c := range bosses {
		bossPages = append(bossPages, s.creaturePage(c))
	}
	if err := s.write("bosses/", "bosses", &Page{Site: s, Title: "Bosses", Description: "The Forsaken of Valheim in order: where each altar stands, what summons it, what it drops and the power it grants.", Section: "creatures", Path: "bosses/",
		Crumbs: []Crumb{{"Creatures", "creatures/"}, {"Bosses", ""}}, Data: bossPages}); err != nil {
		return err
	}
	for _, c := range ix.Creatures {
		pg := s.creaturePage(c)
		t := pg.Thing
		desc := c.Name + " in Valheim: health, damage, resistances, drops and where to find it."
		if c.Boss {
			desc = c.Name + ", a Forsaken of Valheim: how to summon it, its attacks and drops, and the power it grants."
		}
		page := &Page{Site: s, Title: c.Name, Description: desc, Section: "creatures", Path: t.URL, Icon: s.absIcon(t.Icon),
			Crumbs: []Crumb{{"Creatures", "creatures/"}, {c.Name, ""}}, Data: pg}
		if c.Boss {
			page.Crumbs = []Crumb{{"Creatures", "creatures/"}, {"Bosses", "bosses/"}, {c.Name, ""}}
		}
		var prefabs []string
		for _, l := range pg.Found {
			prefabs = append(prefabs, l.Prefab)
		}
		if pg.Altar != nil {
			prefabs = append([]string{pg.Altar.Prefab}, prefabs...)
		}
		if len(prefabs) > 0 || len(c.Biomes) > 0 {
			kind := "creature"
			if c.Boss {
				kind = "boss"
			}
			page.YourWorld = &YourWorld{Kind: kind, Prefabs: prefabs, Biomes: c.Biomes, Label: c.Name}
		}
		if err := s.write(t.URL, "creature", page); err != nil {
			return err
		}
	}
	return nil
}

func (s *Site) creaturePage(c *model.Creature) *CreaturePage {
	ix := s.Index
	pg := &CreaturePage{Creature: c, Thing: ix.Things[c.Prefab], Spawns: ix.SpawnsOf[c.Prefab], Found: ix.Found[c.Prefab], Events: ix.EventsOf[c.Prefab], Phases: ix.Phases[c.Prefab], RaidForm: ix.RaidForm[c.Prefab]}
	if c.AI != nil {
		pg.Lede = c.AI.SpawnMessage
	}
	if s.hasIcon("creatures", c.Prefab+".flat") {
		pg.Portrait = s.URL("icons/creatures/" + c.Prefab + ".flat.png")
	}
	maxStars := 2
	if c.Boss {
		maxStars = 0
	}
	for l := 1; l <= maxStars+1; l++ {
		mult := 1 << (l - 1)
		pg.Stars = append(pg.Stars, StarRow{Level: l, Stars: strings.Repeat("★", l-1), Health: c.Health * float64(l), Mult: mult})
	}
	if c.Trophy != "" {
		pg.Trophy = s.DB.Items[c.Trophy]
	}
	if c.Procreation != nil && c.Procreation.Offspring != "" {
		pg.Offspring = s.DB.Creatures[c.Procreation.Offspring]
	}
	if c.Growup != nil && c.Growup.Grown != "" {
		pg.Grown = s.DB.Creatures[c.Growup.Grown]
	}
	for _, o := range ix.Creatures {
		if o.Growup != nil && o.Growup.Grown == c.Prefab {
			pg.Parent = o
		}
	}
	if c.Boss {
		for _, l := range ix.Locations {
			if l.Contents != nil && l.Contents.Offering != nil && l.Contents.Offering.Boss == c.Prefab {
				pg.Altar = l
				pg.Offering = l.Contents.Offering
				break
			}
		}
		for _, e := range ix.Effects {
			if e.GuardianOf == c.Prefab {
				pg.Power = e
			}
		}
	}
	seenKey := map[string]bool{}
	for _, k := range append([]string{c.DefeatKey}, func() []string {
		var ks []string
		for _, ph := range pg.Phases {
			ks = append(ks, ph.DefeatKey)
		}
		return ks
	}()...) {
		if k == "" || seenKey[strings.ToLower(k)] {
			continue
		}
		seenKey[strings.ToLower(k)] = true
		if u := ix.KeyUses[strings.ToLower(k)]; u != nil {
			pg.Keys = append(pg.Keys, u)
		} else {
			pg.Keys = append(pg.Keys, &KeyUse{Key: k})
		}
	}
	for _, it := range ix.Items {
		if it.Hidden || it.Attack == nil {
			continue
		}
		for _, p := range it.Attack.Summons {
			if p == c.Prefab {
				pg.Summoner = append(pg.Summoner, it)
				break
			}
		}
	}
	seen := map[*Kind]bool{}
	for _, l := range pg.Found {
		if k := ix.KindOf[l.Prefab]; k != nil && !seen[k] {
			seen[k] = true
			pg.Kinds = append(pg.Kinds, k)
		}
	}
	return pg
}
