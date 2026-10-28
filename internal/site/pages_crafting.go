package site

import (
	"sort"
	"strings"

	"github.com/pietjan/valheim-wiki/internal/model"
)

// StationPage is a crafting station's page.
type StationPage struct {
	Lede       string
	Station    *model.Station
	Piece      *model.Piece
	Thing      *Thing
	Extensions []*model.Piece
	Levels     []StationLevel
	Repairs    int
}

// StationLevel is the recipes unlocked at one station level.
type StationLevel struct {
	Level  int
	Groups []RecipeGroup
}

// RecipeGroup is recipes by item group.
type RecipeGroup struct {
	Name    string
	Recipes []*model.Recipe
}

// ConverterPage is a converter's page.
type ConverterPage struct {
	Lede      string
	Converter *model.Converter
	Piece     *model.Piece
	Thing     *Thing
	FuelThing *Thing
}

// CraftingIndex is the crafting overview.
type CraftingIndex struct {
	Stations   []*StationPage
	Converters []ConverterGroup
	ByHand     []*model.Recipe
}

// ConverterGroup is converters of one kind.
type ConverterGroup struct {
	Kind       string
	Name       string
	Converters []*model.Converter
}

func (s *Site) pagesCrafting() error {
	ix := s.Index
	var stations []*StationPage
	for _, k := range model.SortedKeys(s.DB.Stations) {
		st := s.DB.Stations[k]
		pg := &StationPage{Station: st, Piece: s.DB.Pieces[k], Thing: ix.Things[k]}
		if pg.Piece != nil {
			pg.Lede = pg.Piece.Description
		}
		for _, e := range st.Extensions {
			if p := s.DB.Pieces[e]; p != nil {
				pg.Extensions = append(pg.Extensions, p)
			}
		}
		byLevel := map[int]map[string][]*model.Recipe{}
		for _, r := range ix.StationRecipes[k] {
			if r.OnlyUpgrade {
				continue
			}
			it := s.DB.Items[r.Item]
			if it == nil {
				continue
			}
			g := groupOf(it)
			if byLevel[r.MinStationLevel] == nil {
				byLevel[r.MinStationLevel] = map[string][]*model.Recipe{}
			}
			byLevel[r.MinStationLevel][g] = append(byLevel[r.MinStationLevel][g], r)
		}
		for _, r := range s.DB.Recipes {
			if r.RepairStation == k {
				pg.Repairs++
			}
		}
		var levels []int
		for l := range byLevel {
			levels = append(levels, l)
		}
		sort.Ints(levels)
		for _, l := range levels {
			sl := StationLevel{Level: l}
			for _, g := range itemGroups {
				if rs := byLevel[l][g.ID]; len(rs) > 0 {
					sort.Slice(rs, func(i, j int) bool { return s.DB.Items[rs[i].Item].Name < s.DB.Items[rs[j].Item].Name })
					sl.Groups = append(sl.Groups, RecipeGroup{g.Name, rs})
				}
			}
			levels := sl
			pg.Levels = append(pg.Levels, levels)
		}
		stations = append(stations, pg)
	}
	sort.Slice(stations, func(i, j int) bool {
		return stationRank(stations[i].Station.Prefab) < stationRank(stations[j].Station.Prefab)
	})

	kinds := []string{"smelter", "cooking", "fermenter", "beehive", "sap", "feast", "incinerator", "fireplace", "turret", "shield", "trap", "wisp"}
	byKind := map[string][]*model.Converter{}
	for _, k := range model.SortedKeys(s.DB.Converters) {
		c := s.DB.Converters[k]
		if s.DB.Stations[k] != nil {
			continue
		}
		byKind[c.Kind] = append(byKind[c.Kind], c)
	}
	var groups []ConverterGroup
	seen := map[string]bool{}
	for _, k := range kinds {
		if cs := byKind[k]; len(cs) > 0 {
			sort.Slice(cs, func(i, j int) bool { return cs[i].Name < cs[j].Name })
			groups = append(groups, ConverterGroup{k, ConverterKindName(k), cs})
			seen[k] = true
		}
	}
	for k, cs := range byKind {
		if !seen[k] {
			groups = append(groups, ConverterGroup{k, ConverterKindName(k), cs})
		}
	}
	var byHand []*model.Recipe
	for _, r := range ix.StationRecipes[""] {
		if it := s.DB.Items[r.Item]; it != nil && !r.OnlyUpgrade {
			byHand = append(byHand, r)
		}
	}
	sort.Slice(byHand, func(i, j int) bool { return s.DB.Items[byHand[i].Item].Name < s.DB.Items[byHand[j].Item].Name })

	if err := s.write("crafting/", "crafting", &Page{Site: s, Title: "Crafting", Description: "Every crafting station in Valheim with the recipes it unlocks per level, and every smelter, kiln, oven, cauldron and fermenter with what it turns into what.", Section: "crafting", Path: "crafting/",
		Data: &CraftingIndex{Stations: stations, Converters: groups, ByHand: byHand}}); err != nil {
		return err
	}
	for _, pg := range stations {
		t := pg.Thing
		if err := s.write(t.URL, "station", &Page{Site: s, Title: pg.Station.Name, Description: pg.Station.Name + " in Valheim: how to build and upgrade it, and every recipe it unlocks at each level.", Section: "crafting", Path: t.URL, Icon: s.absIcon(t.Icon),
			Crumbs: []Crumb{{"Crafting", "crafting/"}, {pg.Station.Name, ""}}, Data: pg}); err != nil {
			return err
		}
	}
	for _, g := range groups {
		for _, c := range g.Converters {
			t := ix.Things[c.Prefab]
			pg := &ConverterPage{Converter: c, Piece: s.DB.Pieces[c.Prefab], Thing: t}
			if pg.Piece != nil {
				pg.Lede = pg.Piece.Description
			}
			if c.Fuel != "" {
				pg.FuelThing = ix.Things[c.Fuel]
			}
			if err := s.write(t.URL, "converter", &Page{Site: s, Title: c.Name, Description: c.Name + " in Valheim: what it takes, what it makes, and how long it takes.", Section: "crafting", Path: t.URL, Icon: s.absIcon(t.Icon),
				Crumbs: []Crumb{{"Crafting", "crafting/"}, {g.Name, "crafting/#" + g.Kind}, {c.Name, ""}}, Data: pg}); err != nil {
				return err
			}
		}
	}
	return nil
}

// stationRank orders the stations by progression.
func stationRank(p string) int {
	order := []string{"piece_workbench", "forge", "piece_cauldron", "piece_stonecutter", "piece_artisanstation", "blackforge", "piece_magetable", "piece_preptable", "piece_MeadCauldron", "UpgradeStation"}
	for i, o := range order {
		if o == p {
			return i
		}
	}
	return len(order) + int(strings.Compare(p, ""))
}

// PiecePage is a building piece's page.
type PiecePage struct {
	Piece     *model.Piece
	Thing     *Thing
	Table     *model.PieceTable
	Station   *model.Station
	Extends   *model.Station
	Converter *model.Converter
	Similar   []*model.Piece // same category, same tool
	Effects   *model.Effect
}

// BuildingIndex is pieces by tool and category.
type BuildingIndex struct {
	Tools []ToolGroup
	Count int
}

// ToolGroup is one tool's menu.
type ToolGroup struct {
	Tool       string
	ToolThing  *Thing
	Categories []PieceCat
	Count      int
}

// PieceCat is one build-menu tab.
type PieceCat struct {
	ID     string
	Name   string
	Pieces []*model.Piece
}

func (s *Site) pagesBuilding() error {
	ix := s.Index
	tableByTool := map[string]*model.PieceTable{}
	var tools []ToolGroup
	for _, pt := range s.DB.PieceTables {
		tableByTool[pt.Tool] = pt
		tg := ToolGroup{Tool: pt.Tool, ToolThing: ix.Things[pt.Tool]}
		byCat := map[string][]*model.Piece{}
		var order []string
		for _, pp := range pt.Pieces {
			p := s.DB.Pieces[pp]
			if p == nil || !p.Enabled {
				continue
			}
			if _, ok := byCat[p.Category]; !ok {
				order = append(order, p.Category)
			}
			byCat[p.Category] = append(byCat[p.Category], p)
			tg.Count++
		}
		for _, c := range order {
			tg.Categories = append(tg.Categories, PieceCat{ID: Slug(c), Name: PieceCategoryName(c), Pieces: byCat[c]})
		}
		tools = append(tools, tg)
	}
	sort.SliceStable(tools, func(i, j int) bool { return toolRank(tools[i].Tool) < toolRank(tools[j].Tool) })
	// Pieces no table lists (placed by the game, not built): their own group.
	var loose []*model.Piece
	for _, p := range ix.Pieces {
		if p.Tool == "" && p.Enabled && p.Name != "" {
			loose = append(loose, p)
		}
	}
	total := 0
	for _, t := range tools {
		total += t.Count
	}
	if err := s.write("building/", "building", &Page{Site: s, Title: "Building", Description: "Every piece in Valheim's hammer, hoe, cultivator and feast menus: what it costs, what it needs, how much it takes before it breaks, and the comfort it adds.", Section: "building", Path: "building/",
		Data: map[string]any{"Tools": tools, "Count": total, "Loose": loose}}); err != nil {
		return err
	}
	for _, p := range ix.Pieces {
		t := ix.Things[p.Prefab]
		if t == nil || t.Kind != "piece" {
			continue
		}
		pg := &PiecePage{Piece: p, Thing: t, Table: tableByTool[p.Tool], Station: s.DB.Stations[p.Station], Extends: ix.Extends[p.Prefab], Converter: s.DB.Converters[p.Prefab]}
		if p.ItemStand != nil && p.ItemStand.GuardianPower != "" {
			pg.Effects = s.DB.Effects[p.ItemStand.GuardianPower]
		}
		desc := p.Description
		if desc == "" {
			desc = p.Name + " in Valheim: build cost, station, durability and placement."
		}
		if err := s.write(t.URL, "piece", &Page{Site: s, Title: p.Name, Description: desc, Section: "building", Path: t.URL, Icon: s.absIcon(t.Icon),
			Crumbs: []Crumb{{"Building", "building/"}, {PieceCategoryName(p.Category), "building/#" + Slug(p.Tool) + "-" + Slug(p.Category)}, {p.Name, ""}}, Data: pg}); err != nil {
			return err
		}
	}
	return nil
}

func toolRank(t string) int {
	switch t {
	case "Hammer":
		return 0
	case "Hoe":
		return 1
	case "Cultivator":
		return 2
	}
	return 3
}
