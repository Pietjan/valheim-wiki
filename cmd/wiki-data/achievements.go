package main

import (
	"log"
	"sort"
	"strings"

	"github.com/pietjan/valheim-wiki/internal/assets"
	"github.com/pietjan/valheim-wiki/internal/model"
)

// extractAchievements reads the client's achievements: the prefab that
// carries an m_achievements list (Achievements1), each entry an asset
// with its name, description, icon and triggers. The triggers name things
// by their localisation token — $enemy_eikthyr, $item_trophy_abomination,
// $piece_woodwall — which the creatures, items and pieces already
// extracted resolve to prefabs.
func extractAchievements(x *X) error {
	x.DB.Achievements = nil
	excluded := map[string][]string{}
	byToken := map[string]string{}
	// A token no thing carries any more — the achievement list keeps a
	// few old ones — is tried as a prefab name with its separators dropped:
	// $item_battleaxe_gold is BattleaxeGold.
	byFlat := map[string]string{}
	flat := func(s string) string {
		return strings.ToLower(strings.NewReplacer("_", "", "-", "", " ", "").Replace(s))
	}
	for _, k := range model.SortedKeys(x.DB.Items) {
		byFlat["item"+flat(k)] = k
	}
	for _, k := range model.SortedKeys(x.DB.Pieces) {
		byFlat["piece"+flat(k)] = k
	}
	resolve := func(token string) string {
		if p := byToken[strings.ToLower(token)]; p != "" {
			return p
		}
		return byFlat[flat(strings.TrimPrefix(token, "$"))]
	}
	// Pieces and items first, creatures last and the plainest prefab of a
	// token winning, so $enemy_draugr is Draugr and not a raid's copy.
	for _, k := range model.SortedKeys(x.DB.Pieces) {
		if t := x.DB.Pieces[k].Token; t != "" {
			if _, ok := byToken[strings.ToLower(t)]; !ok {
				byToken[strings.ToLower(t)] = k
			}
		}
	}
	for _, k := range model.SortedKeys(x.DB.Items) {
		if t := x.DB.Items[k].Token; t != "" {
			if _, ok := byToken[strings.ToLower(t)]; !ok {
				byToken[strings.ToLower(t)] = k
			}
		}
	}
	creatures := model.SortedKeys(x.DB.Creatures)
	sort.SliceStable(creatures, func(i, j int) bool { return len(creatures[i]) < len(creatures[j]) })
	for _, k := range creatures {
		if t := x.DB.Creatures[k].Token; t != "" {
			if _, ok := byToken[strings.ToLower(t)]; !ok {
				byToken[strings.ToLower(t)] = k
			}
		}
	}

	needs := func(cab string, list []any) []model.Need {
		var out []model.Need
		for _, e := range list {
			m := assets.AsMap(e)
			token := S(m, "m_stat")
			if token == "" {
				continue
			}
			n := model.Need{Token: token, Prefab: resolve(token), Name: x.Text(token), Amount: I(m, "m_amount")}
			if n.Name == "" || strings.HasPrefix(n.Name, "$") {
				n.Name = prettyName(strings.TrimPrefix(token, "$"))
			}
			out = append(out, n)
		}
		return out
	}

	for _, name := range model.SortedKeys(x.Roots) {
		g := x.Roots[name]
		for _, c := range x.Components(g) {
			if c.ClassID != 114 || !Has(c.Tree, "m_achievements") {
				continue
			}
			list := L(c.Tree, "m_achievements")
			if len(list) == 0 {
				continue
			}
			for _, field := range []string{"m_itemsExcluded", "m_consumablesExcluded", "m_charactersExcluded"} {
				for _, ref := range L(c.Tree, field) {
					if n := x.GameObjectName(c.Ref.Cab, ref); n != "" {
						excluded[field] = append(excluded[field], n)
					}
				}
			}
			for _, ref := range list {
				t, r := x.Env.TreeOf(c.Ref.Cab, ref)
				if t == nil {
					continue
				}
				a := &model.Achievement{
					ID:          S(t, "m_id"),
					Name:        x.Text(S(t, "m_name")),
					Description: x.Text(S(t, "m_description")),
					Secret:      B(t, "m_isSecret"),
					Difficulty:  I(t, "m_difficultyRequirement"),
					Dynamic:     B(t, "m_isDynamicallyPopulated"),
					Lenient:     B(t, "m_lenientBuildAchievement"),
					Kills:       needs(r.Cab, L(t, "m_enemyStatsTriggers")),
					Crafts:      needs(r.Cab, L(t, "m_itemCraftTriggers")),
					Eaten:       needs(r.Cab, L(t, "m_foodEatenTriggers")),
					PickedUp:    needs(r.Cab, L(t, "m_itemPickupTriggers")),
					Caught:      needs(r.Cab, L(t, "m_pickableTriggers")),
					Pieces:      needs(r.Cab, L(t, "m_piecePlacedTriggers")),
				}
				if a.ID == "" {
					a.ID = assets.AsString(t["m_Name"])
				}
				for _, e := range L(t, "m_statTrigger") {
					m := assets.AsMap(e)
					n := I(m, "m_stat")
					a.Stats = append(a.Stats, model.StatNeed{Stat: n, Key: playerStatName(n), Amount: F(m, "m_amountAboveEquals")})
				}
				for _, oref := range L(t, "m_otherAchievementTriggers") {
					if ot, _ := x.Env.TreeOf(r.Cab, oref); ot != nil {
						if id := S(ot, "m_id"); id != "" {
							a.Others = append(a.Others, id)
						}
					}
				}
				if icon, ok := x.Deref(r.Cab, t["m_icon"]); ok {
					x.WantSprite("achievements", a.ID, icon)
					a.Icon = true
				}
				x.DB.Achievements = append(x.DB.Achievements, a)
			}
		}
	}
	x.populateDynamic(excluded)
	return nil
}

// populateDynamic fills the dynamically populated achievements the way
// Achievements.Initialize does when the game runs, since the lists their
// assets carry are a snapshot the game throws away:
//
//	AllWeaponCraft   every recipe in ObjectDB whose item IsWeapon, less m_itemsExcluded
//	AllFoodCooked    every recipe whose item has food, and every cooking
//	                 station conversion to a consumable with food
//	AllItemCraft     every recipe with a crafting station and a named item,
//	                 less m_itemsExcluded
//	AllFoodEaten     every ObjectDB item that is a consumable with food,
//	                 less m_consumablesExcluded
//	AllBuildPieces   every piece of every tool's table that can remove
//	                 pieces, the repair entry aside, hidden ones included
//	FindAllTrophies  every trophy item, one per name, less m_itemsExcluded
//	KillAllCreatures every ZNetScene prefab whose Character is marked
//	                 Included for achievements, one per name; Hard the same
//
// The game keys a requirement by the thing's name token and never
// dedupes the item lists; a repeated token is one requirement, so each
// list here is deduped by token. ObjectDB.m_items and m_recipes and
// ZNetScene.m_prefabs are read from their components, in the order the
// game holds them; when a component is not found the extracted tables
// stand in, which is logged.
func (x *X) populateDynamic(excluded map[string][]string) {
	byID := map[string]*model.Achievement{}
	for _, a := range x.DB.Achievements {
		if a.Dynamic {
			byID[a.ID] = a
		}
	}
	if len(byID) == 0 {
		return
	}
	set := func(list []string) map[string]bool {
		m := map[string]bool{}
		for _, n := range list {
			m[n] = true
		}
		return m
	}
	itemsExcluded := set(excluded["m_itemsExcluded"])
	foodExcluded := set(excluded["m_consumablesExcluded"])

	// ObjectDB: the items and recipes the game registers, in order.
	var dbItems []string
	type dbRecipe struct {
		item    string
		station bool
	}
	var dbRecipes []dbRecipe
	x.EachMono([]string{"m_items", "m_recipes", "m_StatusEffects"}, func(cab string, id int64, t map[string]any) {
		if dbItems != nil {
			return
		}
		for _, ref := range L(t, "m_items") {
			if n := x.GameObjectName(cab, ref); n != "" {
				dbItems = append(dbItems, n)
			}
		}
		for _, ref := range L(t, "m_recipes") {
			rt, rr := x.Env.TreeOf(cab, ref)
			if rt == nil {
				continue
			}
			item := x.GameObjectName(rr.Cab, rt["m_item"])
			if item == "" {
				continue
			}
			dbRecipes = append(dbRecipes, dbRecipe{item: item, station: x.componentRoot(rr.Cab, rt["m_craftingStation"]) != ""})
		}
	})
	if dbItems == nil {
		log.Printf("achievements: ObjectDB not found; the dynamic lists use the extracted items and recipes")
		dbItems = model.SortedKeys(x.DB.Items)
		for _, r := range x.DB.Recipes {
			dbRecipes = append(dbRecipes, dbRecipe{item: r.Item, station: r.Station != ""})
		}
	}
	// ZNetScene: the prefabs the game registers.
	var scenePrefabs []string
	x.EachMono([]string{"m_prefabs", "m_nonNetViewPrefabs"}, func(cab string, id int64, t map[string]any) {
		if scenePrefabs != nil {
			return
		}
		for _, ref := range L(t, "m_prefabs") {
			if n := x.GameObjectName(cab, ref); n != "" {
				scenePrefabs = append(scenePrefabs, n)
			}
		}
	})
	if scenePrefabs == nil {
		log.Printf("achievements: ZNetScene not found; the dynamic lists use every prefab root")
		scenePrefabs = model.SortedKeys(x.RootsAll)
	}

	need := func(token, prefab string) model.Need {
		n := model.Need{Token: token, Prefab: prefab, Name: x.Text(token), Amount: 1}
		if n.Name == "" || strings.HasPrefix(n.Name, "$") {
			n.Name = prettyName(strings.TrimPrefix(token, "$"))
		}
		return n
	}
	// collect dedupes by token, keeping the first's place and, as the thing
	// the page shows for it, the plainest prefab that carries the token:
	// Boar over Boar_spiritcaller, portal over portal_wood.
	collect := func(entries []model.Need) []model.Need {
		at := map[string]int{}
		var out []model.Need
		for _, n := range entries {
			if n.Token == "" {
				continue
			}
			if i, ok := at[n.Token]; ok {
				if len(n.Prefab) < len(out[i].Prefab) {
					out[i].Prefab = n.Prefab
				}
				continue
			}
			at[n.Token] = len(out)
			out = append(out, n)
		}
		return out
	}
	isWeapon := func(it *model.Item) bool {
		switch it.Type {
		case "OneHandedWeapon", "Bow", "TwoHandedWeapon", "TwoHandedWeaponLeft", "Torch":
			return true
		}
		return false
	}

	if a := byID["AllWeaponCraft"]; a != nil {
		var list []model.Need
		for _, r := range dbRecipes {
			if it := x.DB.Items[r.item]; it != nil && isWeapon(it) && !itemsExcluded[r.item] {
				list = append(list, need(it.Token, it.Prefab))
			}
		}
		a.Crafts = collect(list)
	}
	if a := byID["AllFoodCooked"]; a != nil {
		var list []model.Need
		for _, r := range dbRecipes {
			if it := x.DB.Items[r.item]; it != nil && it.Food > 0 {
				list = append(list, need(it.Token, it.Prefab))
			}
		}
		for _, name := range scenePrefabs {
			c, ok := x.rootWith(name, "m_conversion", "m_overCookedItem")
			if !ok {
				continue
			}
			for _, e := range L(c.Tree, "m_conversion") {
				to := x.GameObjectName(c.Ref.Cab, assets.AsMap(e)["m_to"])
				if it := x.DB.Items[to]; it != nil && it.Food > 0 && it.Type == "Consumable" {
					list = append(list, need(it.Token, it.Prefab))
				}
			}
		}
		a.Crafts = collect(list)
	}
	if a := byID["AllItemCraft"]; a != nil {
		var list []model.Need
		for _, r := range dbRecipes {
			if it := x.DB.Items[r.item]; it != nil && r.station && it.Token != "" && !itemsExcluded[r.item] {
				list = append(list, need(it.Token, it.Prefab))
			}
		}
		a.Crafts = collect(list)
	}
	if a := byID["AllFoodEaten"]; a != nil {
		var list []model.Need
		for _, name := range dbItems {
			if it := x.DB.Items[name]; it != nil && it.Type == "Consumable" && it.Food > 0 && !foodExcluded[name] {
				list = append(list, need(it.Token, it.Prefab))
			}
		}
		a.Eaten = collect(list)
	}
	if a := byID["AllBuildPieces"]; a != nil {
		tables := map[string]*model.PieceTable{}
		for _, pt := range x.DB.PieceTables {
			tables[pt.Prefab] = pt
		}
		var list []model.Need
		for _, name := range dbItems {
			it := x.DB.Items[name]
			if it == nil || it.BuildPieces == "" {
				continue
			}
			pt := tables[it.BuildPieces]
			if pt == nil || !pt.CanRemovePieces {
				continue
			}
			for _, prefab := range pt.Pieces {
				p := x.DB.Pieces[prefab]
				if p == nil {
					log.Printf("achievements: piece %s of %s is not extracted", prefab, pt.Prefab)
					continue
				}
				if p.RepairPiece {
					continue
				}
				list = append(list, need(p.Token, p.Prefab))
			}
		}
		a.Pieces = collect(list)
	}
	if a := byID["FindAllTrophies"]; a != nil {
		var list []model.Need
		for _, name := range dbItems {
			if it := x.DB.Items[name]; it != nil && it.Type == "Trophy" && !itemsExcluded[name] {
				list = append(list, need(it.Token, it.Prefab))
			}
		}
		a.PickedUp = collect(list)
	}
	var kills []model.Need
	for _, name := range scenePrefabs {
		c, ok := x.rootWith(name, "m_health", "m_faction")
		if !ok || I(c.Tree, "m_killedForAchievements") != 1 {
			continue
		}
		kills = append(kills, need(S(c.Tree, "m_name"), name))
	}
	kills = collect(kills)
	for _, id := range []string{"KillAllCreatures", "KillAllCreaturesHard"} {
		if a := byID[id]; a != nil {
			a.Kills = append([]model.Need(nil), kills...)
		}
	}
}

// rootWith is the component with the fields on any of the roots that
// carry a prefab name, since a name may be held by several (the model a
// character was built from often shares it), chosen as the creatures
// extractor chooses: by having the component.
func (x *X) rootWith(name string, fields ...string) (Component, bool) {
	for _, g := range x.RootsAll[name] {
		if c, ok := x.Find(g, fields...); ok {
			return c, true
		}
	}
	return Component{}, false
}
