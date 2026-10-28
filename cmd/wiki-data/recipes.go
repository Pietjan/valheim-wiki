package main

import (
	"sort"

	"github.com/pietjan/valheim-wiki/internal/assets"
	"github.com/pietjan/valheim-wiki/internal/model"
)

// recipeFields identifies a Recipe asset: a ScriptableObject with the
// item it makes, the resources it costs and the station level it needs.
var recipeFields = []string{"m_resources", "m_item", "m_minStationLevel"}

// extractRecipes reads every enabled Recipe asset. Recipe.m_item,
// m_craftingStation and m_repairStation point at components (the ItemDrop,
// the CraftingStation), so each is resolved to the prefab that owns it.
// An item a recipe makes is something a player can hold, so its Hidden
// mark is cleared.
func extractRecipes(x *X) error {
	x.EachMono(recipeFields, func(cab string, id int64, t map[string]any) {
		if !B(t, "m_enabled") {
			return
		}
		item := x.GameObjectName(cab, t["m_item"])
		if item == "" {
			return
		}
		r := &model.Recipe{
			ID:              S(t, "m_Name"),
			Item:            item,
			Amount:          I(t, "m_amount"),
			Station:         x.componentRoot(cab, t["m_craftingStation"]),
			RepairStation:   x.componentRoot(cab, t["m_repairStation"]),
			MinStationLevel: I(t, "m_minStationLevel"),
			OnlyUpgrade:     B(t, "m_noCraftOnlyUpgrade"),
			OneIngredient:   B(t, "m_requireOnlyOneIngredient"),
			QualityMult:     F(t, "m_qualityResultAmountMultiplier"),
			Resources:       x.Requirements(cab, t["m_resources"]),
		}
		if r.Resources == nil {
			r.Resources = []model.Requirement{}
		}
		x.DB.Recipes = append(x.DB.Recipes, r)
		if it := x.DB.Items[item]; it != nil {
			it.Hidden = false
		}
	})
	sort.SliceStable(x.DB.Recipes, func(i, j int) bool {
		a, b := x.DB.Recipes[i], x.DB.Recipes[j]
		if a.Item != b.Item {
			return a.Item < b.Item
		}
		return a.ID < b.ID
	})
	return nil
}

// upgraderItems is the set of items an enabled recipe spends as its
// upgrader resource: the Forge of Potential idols, whose own
// m_upgradeChance and m_breakChance decide the attempt.
func upgraderItems(x *X) map[string]bool {
	out := map[string]bool{}
	x.EachMono(recipeFields, func(cab string, id int64, t map[string]any) {
		if !B(t, "m_enabled") {
			return
		}
		for _, r := range x.Requirements(cab, t["m_resources"]) {
			if r.Upgrader {
				out[r.Item] = true
			}
		}
	})
	return out
}

// componentRoot is the prefab root that owns the component (or
// GameObject) a PPtr points at, or "" for a null reference.
func (x *X) componentRoot(cab string, ref any) string {
	t, r := x.Env.TreeOf(cab, ref)
	if t == nil {
		return ""
	}
	if o := x.Env.Object(r.Cab, r.ID); o != nil && o.ClassID == 1 {
		return x.RootName(r)
	}
	g, ok := x.Deref(r.Cab, t["m_GameObject"])
	if !ok {
		return assets.AsString(t["m_Name"])
	}
	return x.RootName(g)
}
