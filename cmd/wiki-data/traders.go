package main

import (
	"sort"

	"github.com/pietjan/valheim-wiki/internal/assets"
	"github.com/pietjan/valheim-wiki/internal/model"
)

// traderCamps is the location each trader prefab is placed in. The game
// wires it the other way round (the location prefab contains the trader),
// so the pairs are the game's: Haldor in Vendor_BlackForest, Hildir in
// Hildir_camp, the Bog Witch in BogWitch_Camp.
var traderCamps = map[string]string{
	"Haldor":   "Vendor_BlackForest",
	"Hildir":   "Hildir_camp",
	"BogWitch": "BogWitch_Camp",
}

// extractTraders reads every Trader component on a prefab root: what is
// for sale (m_items: item, stack, coins, and the global key that must be
// set before it appears) and what the trader takes from you (m_useItems:
// the item, the key it sets, whether it is consumed, and what they say).
func extractTraders(x *X) error {
	seen := map[string]bool{}
	x.EachMono([]string{"m_items", "m_useItems", "m_randomTalk"}, func(cab string, id int64, t map[string]any) {
		g, ok := x.Deref(cab, t["m_GameObject"])
		if !ok || !x.isRoot(g) {
			return
		}
		prefab := x.Name(g)
		if prefab == "" || seen[prefab] {
			return
		}
		seen[prefab] = true
		tr := &model.Trader{
			Prefab:   prefab,
			Name:     x.NameOf(prefab, S(t, "m_name")),
			Location: traderCamps[prefab],
			Items:    []model.TradeItem{},
		}
		for _, e := range L(t, "m_items") {
			m := assets.AsMap(e)
			item := x.GameObjectName(cab, m["m_prefab"])
			if item == "" {
				continue
			}
			tr.Items = append(tr.Items, model.TradeItem{
				Item:        item,
				Stack:       I(m, "m_stack"),
				Price:       I(m, "m_price"),
				RequiredKey: S(m, "m_requiredGlobalKey"),
			})
		}
		for _, e := range L(t, "m_useItems") {
			m := assets.AsMap(e)
			item := x.GameObjectName(cab, m["m_prefab"])
			if item == "" {
				continue
			}
			tr.UseItems = append(tr.UseItems, model.UseItem{
				Item:     item,
				SetsKey:  S(m, "m_setsGlobalKey"),
				Removes:  B(m, "m_removesItem"),
				Dialogue: x.Text(S(m, "m_dialog")),
			})
		}
		x.DB.Traders = append(x.DB.Traders, tr)
	})
	sort.Slice(x.DB.Traders, func(i, j int) bool { return x.DB.Traders[i].Prefab < x.DB.Traders[j].Prefab })
	return nil
}
