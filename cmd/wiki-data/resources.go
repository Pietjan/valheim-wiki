package main

import (
	"sort"

	"github.com/pietjan/valheim-wiki/internal/assets"
	"github.com/pietjan/valheim-wiki/internal/model"
)

// extractResources reads the world's harvestables: every prefab root
// with a Pickable or PickableItem (berries, mushrooms, flint on the
// ground), a TreeBase (a standing tree), a TreeLog (the felled trunk and
// its halves), a MineRock or MineRock5 (ore veins and boulders), a
// Destructible that drops or spawns something (stubs, bushes, bone
// piles, the whole rock before it shatters into its MineRock5 fragments)
// or a wild Plant. Pieces, items, creatures, ragdolls and projectiles
// are not resources however destructible they are.
func extractResources(x *X) error {
	// Candidate roots, found by component rather than by walking every
	// root: the field pairs are each component's own.
	var roots []assets.Ref
	seen := map[assets.Ref]bool{}
	for _, fields := range [][]string{
		{"m_itemPrefab", "m_respawnTimeMinutes"},       // Pickable
		{"m_itemPrefab", "m_randomItemPrefabs"},        // PickableItem
		{"m_logPrefab", "m_stubPrefab"},                // TreeBase
		{"m_subLogPrefab", "m_subLogPoints"},           // TreeLog
		{"m_dropItems", "m_removeWhenDestroyed"},       // MineRock
		{"m_dropItems", "m_supportCheck"},              // MineRock5
		{"m_destructibleType", "m_spawnWhenDestroyed"}, // Destructible
		{"m_growTime", "m_grownPrefabs"},               // Plant
	} {
		x.EachMono(fields, func(cab string, id int64, t map[string]any) {
			g, ok := x.Deref(cab, t["m_GameObject"])
			if !ok || seen[g] || !x.isPrefabRoot(g) {
				return
			}
			seen[g] = true
			roots = append(roots, g)
		})
	}
	// Load order decides between roots of one name; sort by name only
	// for a stable walk within it.
	sort.SliceStable(roots, func(i, j int) bool { return x.Name(roots[i]) < x.Name(roots[j]) })
	for _, g := range roots {
		name := x.Name(g)
		if _, dup := x.DB.Resources[name]; dup {
			continue
		}
		if r := x.readResource(name, g); r != nil {
			x.DB.Resources[name] = r
		}
	}
	return nil
}

// readResource reads the root g's components into one Resource, or nil
// when the root is not a resource after all.
func (x *X) readResource(prefab string, g assets.Ref) *model.Resource {
	r := &model.Resource{Prefab: prefab}
	var token, hover string
	hasDrops := false
	// A vine grows its berries — the Pickable it carries — only where
	// its Vine component allows any: the ivy allows none, and the
	// vineberry cluster on it is never there to pick.
	if vine, ok := x.Find(g, "m_maxBerriesWithinBlocker", "m_vinePrefab"); ok && I(vine.Tree, "m_maxBerriesWithinBlocker") == 0 {
		return nil
	}
	for _, c := range x.Components(g) {
		if c.ClassID != 114 {
			continue
		}
		t, cab := c.Tree, c.Ref.Cab
		switch {
		case Has(t, "m_resources", "m_icon", "m_category"), // Piece
			Has(t, "m_itemData"),                      // ItemDrop
			Has(t, "m_health", "m_faction"),           // Character
			Has(t, "m_mainModel", "m_lootSpawnJoint"), // Ragdoll
			Has(t, "m_aoe", "m_hitMidFlight"):         // Projectile
			return nil
		case Has(t, "m_itemPrefab", "m_respawnTimeMinutes"): // Pickable
			r.Kind = "pickable"
			r.Item = x.GameObjectName(cab, t["m_itemPrefab"])
			r.Amount = I(t, "m_amount")
			r.Respawn = F(t, "m_respawnTimeMinutes")
			if _, ok := x.Deref(cab, t["m_hideWhenPicked"]); ok {
				r.HideWhenPicked = true
			}
			r.ExtraDrops = x.DropTable(cab, M(t, "m_extraDrops"))
			token = S(t, "m_overrideName")
		case Has(t, "m_itemPrefab", "m_randomItemPrefabs"): // PickableItem
			r.Kind = "pickable"
			r.Item = x.GameObjectName(cab, t["m_itemPrefab"])
			r.Amount = I(t, "m_stack")
			if dt := x.randomItemTable(cab, t["m_randomItemPrefabs"]); dt != nil {
				r.Drops = dt
				hasDrops = true
			}
		case Has(t, "m_logPrefab", "m_stubPrefab"): // TreeBase
			setKind(r, "tree")
			r.Health = F(t, "m_health")
			r.Resist = DamageMods(M(t, "m_damageModifiers"))
			r.MinToolTier = I(t, "m_minToolTier")
			r.Stub = x.GameObjectName(cab, t["m_stubPrefab"])
			r.Log = x.GameObjectName(cab, t["m_logPrefab"])
			r.Drops = x.DropTable(cab, M(t, "m_dropWhenDestroyed"))
		case Has(t, "m_subLogPrefab", "m_subLogPoints"): // TreeLog
			setKind(r, "log")
			r.Health = F(t, "m_health")
			r.Resist = DamageMods(M(t, "m_damages"))
			r.MinToolTier = I(t, "m_minToolTier")
			r.Drops = x.DropTable(cab, M(t, "m_dropWhenDestroyed"))
			r.SubLog = x.GameObjectName(cab, t["m_subLogPrefab"])
			r.SubLogs = len(L(t, "m_subLogPoints"))
		case Has(t, "m_dropItems", "m_removeWhenDestroyed"), Has(t, "m_dropItems", "m_supportCheck"): // MineRock, MineRock5
			setKind(r, "rock")
			r.Health = F(t, "m_health")
			r.Resist = DamageMods(M(t, "m_damageModifiers"))
			r.MinToolTier = I(t, "m_minToolTier")
			r.Drops = x.DropTable(cab, M(t, "m_dropItems"))
			if token == "" {
				token = S(t, "m_name")
			}
		case Has(t, "m_destructibleType", "m_spawnWhenDestroyed"): // Destructible
			setKind(r, "destructible")
			if r.Health == 0 {
				r.Health = F(t, "m_health")
				r.Resist = DamageMods(M(t, "m_damages"))
				r.MinToolTier = I(t, "m_minToolTier")
			}
			r.SpawnWhenDestroyed = x.GameObjectName(cab, t["m_spawnWhenDestroyed"])
		case Has(t, "m_dropWhenDestroyed", "m_spawnYOffset") && !Has(t, "m_logPrefab"): // DropOnDestroyed
			if dt := x.DropTable(cab, M(t, "m_dropWhenDestroyed")); dt != nil {
				r.Drops = dt
				hasDrops = true
			}
		case Has(t, "m_growTime", "m_grownPrefabs"): // Plant
			setKind(r, "plant")
			r.Plant = x.plantInfo(cab, t)
			if token == "" {
				token = S(t, "m_name")
			}
		case Has(t, "m_text", "m_hoverOffset") && len(t) <= 6: // HoverText: those two fields and Unity's own four
			hover = S(t, "m_text")
		}
	}
	switch r.Kind {
	case "":
		return nil
	case "destructible":
		if !hasDrops && r.SpawnWhenDestroyed == "" {
			return nil
		}
	}
	if token == "" && r.Item != "" {
		token = x.itemToken(r.Item)
	}
	if token == "" {
		token = hover
	}
	r.Token = token
	r.Name = x.NameOf(prefab, token)
	return r
}

// setKind keeps the most specific kind: a pickable bush is a pickable
// however destructible, a tree is a tree before a destructible.
func setKind(r *model.Resource, k string) {
	rank := map[string]int{"pickable": 5, "tree": 4, "log": 3, "rock": 2, "destructible": 1, "plant": 0}
	if r.Kind == "" || rank[k] > rank[r.Kind] {
		r.Kind = k
	}
}

// randomItemTable turns PickableItem.m_randomItemPrefabs into a
// DropTable: one even-weighted pick of a stack between min and max.
func (x *X) randomItemTable(cab string, v any) *model.DropTable {
	dt := &model.DropTable{Min: 1, Max: 1, Chance: 1}
	for _, e := range assets.AsList(v) {
		m := assets.AsMap(e)
		item := x.GameObjectName(cab, m["m_itemPrefab"])
		if item == "" {
			continue
		}
		dt.Drops = append(dt.Drops, model.DropEntry{Item: item, Min: I(m, "m_stackMin"), Max: I(m, "m_stackMax"), Weight: 1})
	}
	if len(dt.Drops) == 0 {
		return nil
	}
	return dt
}

// itemToken is an item prefab's $item token, read from its ItemDrop.
func (x *X) itemToken(item string) string {
	for _, r := range x.RootsAll[item] {
		if c, ok := x.Find(r, "m_itemData"); ok {
			return S(M(M(c.Tree, "m_itemData"), "m_shared"), "m_name")
		}
	}
	return ""
}
