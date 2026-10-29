package main

import (
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
					a.Stats = append(a.Stats, model.StatNeed{Stat: I(m, "m_stat"), Amount: I(m, "m_amountAboveEquals")})
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
	return nil
}
