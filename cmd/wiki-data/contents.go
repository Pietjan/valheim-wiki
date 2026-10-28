package main

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/pietjan/valheim-wiki/internal/assets"
	"github.com/pietjan/valheim-wiki/internal/model"
)

// contentsOf counts what a location or room prefab holds. Every child
// of the prefab is a nested prefab instance the game spawns in place:
// the walk recognises each by its component's fields (MonoScripts are
// not loaded) and merges equal entries into counts.
//
// Two placement rules from the game shape the walk. A RandomSpawn
// component rolls m_chanceToSpawn (percent) once when the location is
// placed and destroys its GameObject on a miss, so everything below it
// carries that chance, multiplied through nested rolls. A GameObject
// that is inactive in the prefab and has no RandomSpawn of its own stays
// inactive in the world (the crow's sitting model, disabled variants),
// so it is skipped. A DungeonGenerator child is an interior made of room
// prefabs, which have their own entries: the walk does not descend
// into it.
func (x *X) contentsOf(root assets.Ref) *model.Contents {
	c := &model.Contents{}
	w := &contentsWalk{}
	x.walkContents(root, 0, c, true, w)
	if c.Offering != nil && w.standPrefix != "" {
		x.offeringStands(c.Offering, w)
	}
	if emptyContents(c) {
		return nil
	}
	sortContents(c)
	return c
}

// contentsWalk carries what one walk needs to resolve at its end.
type contentsWalk struct {
	standPrefix string      // OfferingBowl.m_itemStandPrefix when the altar uses item stands
	stands      []itemStand // every ItemStand seen, resolved against the prefix afterwards
}

// itemStand is an ItemStand in the prefab: its name and the single item
// it accepts, when it accepts exactly one.
type itemStand struct {
	name string
	item string
}

// offeringStands fills an item-stand altar's Item and Count from the
// stands whose name starts with the bowl's prefix, as
// OfferingBowl.FindItemStands matches them: Moder's three dragoneggcup
// stands take a DragonEgg each, Yagluth's five goblinking_totemholder a
// GoblinTotem each.
func (x *X) offeringStands(o *model.Offering, w *contentsWalk) {
	o.Stands = true
	n := 0
	item := ""
	for _, s := range w.stands {
		if !strings.HasPrefix(s.name, w.standPrefix) {
			continue
		}
		n++
		if item == "" {
			item = s.item
		}
	}
	if n > 0 {
		o.Count = n
	}
	if o.Item == "" {
		o.Item = item
	}
}

// emptyContents is whether nothing was counted.
func emptyContents(c *model.Contents) bool {
	return len(c.Chests) == 0 && len(c.Spawners) == 0 && len(c.Nests) == 0 && len(c.Pickables) == 0 &&
		len(c.Loot) == 0 && len(c.Runestones) == 0 && len(c.Vegvisirs) == 0 && c.Offering == nil &&
		len(c.Creatures) == 0 && len(c.Traders) == 0 && len(c.Pieces) == 0 && len(c.Resources) == 0 && !c.Portal
}

// walkContents visits g with the RandomSpawn chance inherited from its
// ancestors (0 when none) and recurses. isRoot skips the prefab root's
// own Location/Room components.
func (x *X) walkContents(g assets.Ref, chance float64, c *model.Contents, isRoot bool, w *contentsWalk) {
	comps := x.Components(g)
	mono := func(fields ...string) (map[string]any, bool) {
		for _, cm := range comps {
			if cm.ClassID == 114 && Has(cm.Tree, fields...) {
				return cm.Tree, true
			}
		}
		return nil, false
	}
	if rs, ok := mono("m_chanceToSpawn", "m_requireBiome"); ok {
		p := F(rs, "m_chanceToSpawn") / 100
		if chance == 0 {
			chance = p
		} else {
			chance *= p
		}
	} else if !isRoot && !x.Active(g) {
		return
	}
	if !isRoot {
		if _, ok := mono("m_algorithm", "m_themes"); ok {
			return // an interior: rooms are their own prefabs
		}
		x.countContents(g, comps, mono, chance, c, w)
	}
	for _, ch := range x.Children(g) {
		x.walkContents(ch, chance, c, false, w)
	}
}

// countContents adds what one GameObject's components make it.
func (x *X) countContents(g assets.Ref, comps []Component, mono func(...string) (map[string]any, bool), chance float64, c *model.Contents, w *contentsWalk) {
	cab := g.Cab
	name := baseName(x.Name(g))
	_, isPiece := mono("m_resources", "m_icon", "m_category")

	if t, ok := mono("m_defaultItems", "m_width"); ok { // Container
		if loot := x.DropTable(cab, M(t, "m_defaultItems")); loot != nil {
			addChest(c, model.Chest{Prefab: name, Count: 1, Chance: chance, Loot: loot})
		} else if isPiece {
			addPiece(c, name)
		}
	}
	if t, ok := mono("m_creaturePrefab", "m_respawnTimeMinuts"); ok { // CreatureSpawner
		addSpawner(c, model.Spawner{
			Creature: x.GameObjectName(cab, t["m_creaturePrefab"]), Count: 1,
			MinLevel: I(t, "m_minLevel"), MaxLevel: I(t, "m_maxLevel"), LevelUp: F(t, "m_levelupChance"),
			Respawn: F(t, "m_respawnTimeMinuts"), Night: B(t, "m_spawnAtNight"), Day: B(t, "m_spawnAtDay"),
			RequiredKey: S(t, "m_requiredGlobalKey"), Chance: chance,
		})
	}
	if t, ok := mono("m_prefabs", "m_maxNear"); ok { // SpawnArea
		addNest(c, x.nestOf(cab, name, t))
	}
	if t, ok := mono("m_amount", "m_itemPrefab", "m_respawnTimeMinutes"); ok { // Pickable
		addPickable(c, model.PickableIn{Prefab: name, Item: x.GameObjectName(cab, t["m_itemPrefab"]), Amount: I(t, "m_amount"), Count: 1, Chance: chance})
	} else if t, ok := mono("m_itemPrefab", "m_randomItemPrefabs"); ok { // PickableItem
		p := model.PickableIn{Prefab: name, Item: x.GameObjectName(cab, t["m_itemPrefab"]), Amount: I(t, "m_stack"), Count: 1, Chance: chance}
		if p.Item == "" { // SetupRandomPrefab: one of the list, equally likely
			for _, r := range L(t, "m_randomItemPrefabs") {
				rm := assets.AsMap(r)
				if item := x.GameObjectName(cab, rm["m_itemPrefab"]); item != "" {
					p.Random = append(p.Random, model.RandomPick{Item: item, Min: I(rm, "m_stackMin"), Max: I(rm, "m_stackMax")})
				}
			}
			p.Amount = 0
		}
		addPickable(c, p)
	}
	if t, ok := mono("m_items", "m_respawnTimeMinuts"); ok { // LootSpawner
		addLoot(c, model.LootIn{Prefab: name, Count: 1, Loot: x.DropTable(cab, M(t, "m_items")), Respawn: F(t, "m_respawnTimeMinuts")})
	}
	if t, ok := mono("m_randomTexts", "m_text", "m_topic"); ok { // RuneStone
		r := model.Runestone{Prefab: name, Label: x.Text(S(t, "m_label")), Topic: x.Text(S(t, "m_topic")), Text: x.Text(S(t, "m_text")), Count: 1}
		for _, rt := range L(t, "m_randomTexts") {
			if s := x.Text(S(assets.AsMap(rt), "m_text")); s != "" {
				r.Texts = append(r.Texts, s)
			}
		}
		addRunestone(c, r)
	}
	if t, ok := mono("m_locations", "m_useText"); ok { // Vegvisir
		v := model.Vegvisir{Prefab: name, Count: 1}
		for _, l := range L(t, "m_locations") {
			lm := assets.AsMap(l)
			if n := S(lm, "m_locationName"); n != "" {
				v.Locations = append(v.Locations, n)
			}
			if v.PinName == "" {
				v.PinName = x.Text(S(lm, "m_pinName"))
			}
		}
		addVegvisir(c, v)
	}
	if t, ok := mono("m_bossItem", "m_bossItems"); ok { // OfferingBowl
		c.Offering = &model.Offering{
			Item: x.GameObjectName(cab, t["m_bossItem"]), Count: I(t, "m_bossItems"),
			Boss: x.GameObjectName(cab, t["m_bossPrefab"]), SetsKey: S(t, "m_setGlobalKey"),
		}
		if B(t, "m_useItemStands") {
			w.standPrefix = S(t, "m_itemStandPrefix")
		}
	}
	if t, ok := mono("m_attachOther", "m_autoAttach"); ok { // ItemStand
		st := itemStand{name: x.Name(g)}
		if items := L(t, "m_supportedItems"); len(items) == 1 {
			st.item = x.GameObjectName(cab, items[0])
		}
		w.stands = append(w.stands, st)
	}
	if _, ok := mono("m_health", "m_faction"); ok { // Character
		addCreature(c, model.CreatureIn{Creature: name, Count: 1, Chance: chance})
	}
	if _, ok := mono("m_items", "m_standRange"); ok { // Trader
		addTrader(c, name)
	}
	if _, ok := mono("m_allowAllItems", "m_activationRange"); ok { // TeleportWorld
		c.Portal = true
		if isPiece {
			addPiece(c, name)
		}
	}
	if isPiece {
		_, bed := mono("m_monsterCheckRadius", "m_spawnPoint")
		_, station := mono("m_craftRequireRoof", "m_discoverRange")
		_, stand := mono("m_attachOther", "m_autoAttach")
		if bed || station || stand {
			addPiece(c, name)
		}
	}
	// Destructibles with drops: trees, logs, ore veins and rocks, and a
	// Destructible with a DropOnDestroyed beside it.
	_, tree := mono("m_dropWhenDestroyed", "m_stubPrefab")
	_, log := mono("m_dropWhenDestroyed", "m_subLogPrefab")
	_, rock := mono("m_dropItems", "m_damageModifiers")
	_, destructible := mono("m_destructibleType", "m_spawnWhenDestroyed")
	_, dropOnDestroyed := mono("m_dropWhenDestroyed", "m_spawnYStep")
	if tree || log || rock || (destructible && dropOnDestroyed) {
		addResource(c, name)
	}
}

// nestOf reads a SpawnArea: a nest that keeps spawning from a weighted
// list while fewer than MaxNear of its creatures stand nearby.
func (x *X) nestOf(cab, name string, t map[string]any) model.Nest {
	n := model.Nest{
		Prefab: name, Count: 1, Interval: F(t, "m_spawnIntervalSec"),
		MaxNear: I(t, "m_maxNear"), MaxTotal: I(t, "m_maxTotal"), LevelUp: F(t, "m_levelupChance"),
	}
	for _, e := range L(t, "m_prefabs") {
		em := assets.AsMap(e)
		creature := x.GameObjectName(cab, em["m_prefab"])
		if creature == "" {
			continue
		}
		n.Creatures = append(n.Creatures, model.NestEntry{Creature: creature, Weight: F(em, "m_weight"), MinLevel: I(em, "m_minLevel"), MaxLevel: I(em, "m_maxLevel")})
	}
	return n
}

var instanceSuffix = regexp.MustCompile(`\s*\(\d+\)$`)

// baseName strips Unity's " (3)" instance suffix from a GameObject name.
func baseName(s string) string { return instanceSuffix.ReplaceAllString(s, "") }

// The add* helpers merge an entry into one that equals it in everything
// but Count.

func addChest(c *model.Contents, e model.Chest) {
	for i := range c.Chests {
		if c.Chests[i].Prefab == e.Prefab && c.Chests[i].Chance == e.Chance {
			c.Chests[i].Count++
			return
		}
	}
	c.Chests = append(c.Chests, e)
}

func addSpawner(c *model.Contents, e model.Spawner) {
	for i := range c.Spawners {
		k := c.Spawners[i]
		k.Count = e.Count
		if k == e {
			c.Spawners[i].Count++
			return
		}
	}
	c.Spawners = append(c.Spawners, e)
}

func addNest(c *model.Contents, e model.Nest) {
	for i := range c.Nests {
		if c.Nests[i].Prefab == e.Prefab {
			c.Nests[i].Count++
			return
		}
	}
	c.Nests = append(c.Nests, e)
}

func addPickable(c *model.Contents, e model.PickableIn) {
	for i := range c.Pickables {
		k := c.Pickables[i]
		if k.Prefab == e.Prefab && k.Item == e.Item && k.Amount == e.Amount && k.Chance == e.Chance && fmt.Sprint(k.Random) == fmt.Sprint(e.Random) {
			c.Pickables[i].Count++
			return
		}
	}
	c.Pickables = append(c.Pickables, e)
}

func addLoot(c *model.Contents, e model.LootIn) {
	for i := range c.Loot {
		if c.Loot[i].Prefab == e.Prefab {
			c.Loot[i].Count++
			return
		}
	}
	c.Loot = append(c.Loot, e)
}

func addRunestone(c *model.Contents, e model.Runestone) {
	for i := range c.Runestones {
		k := c.Runestones[i]
		if k.Prefab == e.Prefab && k.Text == e.Text && k.Label == e.Label && strings.Join(k.Texts, "\n") == strings.Join(e.Texts, "\n") {
			c.Runestones[i].Count++
			return
		}
	}
	c.Runestones = append(c.Runestones, e)
}

func addVegvisir(c *model.Contents, e model.Vegvisir) {
	for i := range c.Vegvisirs {
		if c.Vegvisirs[i].Prefab == e.Prefab && strings.Join(c.Vegvisirs[i].Locations, ",") == strings.Join(e.Locations, ",") {
			c.Vegvisirs[i].Count++
			return
		}
	}
	c.Vegvisirs = append(c.Vegvisirs, e)
}

func addCreature(c *model.Contents, e model.CreatureIn) {
	for i := range c.Creatures {
		if c.Creatures[i].Creature == e.Creature && c.Creatures[i].Chance == e.Chance {
			c.Creatures[i].Count++
			return
		}
	}
	c.Creatures = append(c.Creatures, e)
}

func addTrader(c *model.Contents, prefab string) {
	for _, t := range c.Traders {
		if t == prefab {
			return
		}
	}
	c.Traders = append(c.Traders, prefab)
}

func addPiece(c *model.Contents, prefab string) {
	for i := range c.Pieces {
		if c.Pieces[i].Prefab == prefab {
			c.Pieces[i].Count++
			return
		}
	}
	c.Pieces = append(c.Pieces, model.PieceIn{Prefab: prefab, Count: 1})
}

func addResource(c *model.Contents, prefab string) {
	for i := range c.Resources {
		if c.Resources[i].Prefab == prefab {
			c.Resources[i].Count++
			return
		}
	}
	c.Resources = append(c.Resources, model.ResourceIn{Prefab: prefab, Count: 1})
}

// sortContents orders every list by count, most first, then by name, so
// the output does not depend on the prefab's transform order.
func sortContents(c *model.Contents) {
	sort.SliceStable(c.Chests, func(i, j int) bool {
		return byCount(c.Chests[i].Count, c.Chests[j].Count, c.Chests[i].Prefab, c.Chests[j].Prefab)
	})
	sort.SliceStable(c.Spawners, func(i, j int) bool {
		return byCount(c.Spawners[i].Count, c.Spawners[j].Count, c.Spawners[i].Creature, c.Spawners[j].Creature)
	})
	sort.SliceStable(c.Nests, func(i, j int) bool {
		return byCount(c.Nests[i].Count, c.Nests[j].Count, c.Nests[i].Prefab, c.Nests[j].Prefab)
	})
	sort.SliceStable(c.Pickables, func(i, j int) bool {
		return byCount(c.Pickables[i].Count, c.Pickables[j].Count, c.Pickables[i].Prefab, c.Pickables[j].Prefab)
	})
	sort.SliceStable(c.Loot, func(i, j int) bool {
		return byCount(c.Loot[i].Count, c.Loot[j].Count, c.Loot[i].Prefab, c.Loot[j].Prefab)
	})
	sort.Strings(c.Traders)
	sort.SliceStable(c.Creatures, func(i, j int) bool {
		return byCount(c.Creatures[i].Count, c.Creatures[j].Count, c.Creatures[i].Creature, c.Creatures[j].Creature)
	})
	sort.SliceStable(c.Pieces, func(i, j int) bool {
		return byCount(c.Pieces[i].Count, c.Pieces[j].Count, c.Pieces[i].Prefab, c.Pieces[j].Prefab)
	})
	sort.SliceStable(c.Resources, func(i, j int) bool {
		return byCount(c.Resources[i].Count, c.Resources[j].Count, c.Resources[i].Prefab, c.Resources[j].Prefab)
	})
}

func byCount(ci, cj int, ni, nj string) bool {
	if ci != cj {
		return ci > cj
	}
	return ni < nj
}
