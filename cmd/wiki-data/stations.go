package main

import (
	"sort"

	"github.com/pietjan/valheim-wiki/internal/model"
)

// extractStations reads every CraftingStation (the component with
// m_upgrader and m_craftRequireRoof on a piece prefab root) and every
// StationExtension (m_craftingStation plus m_maxStationDistance), which
// raises its station one level when built within that distance: the
// workbench's chopping block, the forge's bellows. A station's level is
// one plus the number of distinct extensions, so MaxLevel is what a
// player can reach with one of each.
func extractStations(x *X) error {
	x.EachMono([]string{"m_upgrader", "m_craftRequireRoof"}, func(cab string, id int64, t map[string]any) {
		g, ok := x.Deref(cab, t["m_GameObject"])
		if !ok || !x.isRoot(g) {
			return
		}
		prefab := x.Name(g)
		if prefab == "" {
			return
		}
		if _, seen := x.DB.Stations[prefab]; seen {
			return
		}
		st := &model.Station{
			Prefab:      prefab,
			Name:        x.NameOf(prefab, S(t, "m_name")),
			Upgrader:    B(t, "m_upgrader"),
			RequireRoof: B(t, "m_craftRequireRoof"),
			RequireFire: B(t, "m_craftRequireFire"),
			CanRepair:   B(t, "m_canRepair"),
			Skill:       SkillName(I(t, "m_craftingSkill")),
			MaxLevel:    1,
		}
		if ref, ok := x.Deref(cab, t["m_icon"]); ok {
			st.Icon = true
			x.WantSprite("stations", prefab, ref)
		}
		x.DB.Stations[prefab] = st
	})
	seen := map[string]bool{}
	x.EachMono([]string{"m_craftingStation", "m_maxStationDistance"}, func(cab string, id int64, t map[string]any) {
		g, ok := x.Deref(cab, t["m_GameObject"])
		if !ok || !x.isRoot(g) {
			return
		}
		prefab := x.Name(g)
		if prefab == "" || seen[prefab] {
			return
		}
		seen[prefab] = true
		station := x.componentRoot(cab, t["m_craftingStation"])
		st := x.DB.Stations[station]
		if st == nil {
			return
		}
		st.Extensions = append(st.Extensions, prefab)
	})
	for _, st := range x.DB.Stations {
		sort.Strings(st.Extensions)
		st.MaxLevel = 1 + len(st.Extensions)
	}
	return nil
}
