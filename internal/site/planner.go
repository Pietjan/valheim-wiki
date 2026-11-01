package site

import "github.com/pietjan/valheim-wiki/internal/model"

// The planner: a page where a reader lists what they mean to build and
// make — the pieces of a new settlement, the gear of the next biome — and
// the browser adds up what it costs, down to the ore and the wood, the
// stations and the levels it takes, and what of it a portal will carry.
// The arithmetic runs in planner.js on planner.json, which this file
// writes: every piece a tool's menu lists and every item with a recipe,
// with their costs, plus the materials they name, the converters that
// turn one material into another, and the stations with their upgrades.

// PlanThing is one row of planner.json: a piece, an item with a recipe,
// or a material something costs.
type PlanThing struct {
	N  string  `json:"n"`
	H  string  `json:"h,omitempty"` // hint, when two things share a name
	I  string  `json:"i,omitempty"`
	U  string  `json:"u,omitempty"`
	S  string  `json:"s,omitempty"`  // type line
	K  string  `json:"k"`            // item or piece
	W  float64 `json:"w,omitempty"`  // weight
	WL float64 `json:"wl,omitempty"` // weight per level
	St int     `json:"st,omitempty"` // stack size
	TP bool    `json:"tp,omitempty"` // goes through a portal
	Q  int     `json:"q,omitempty"`  // max quality

	R []PlanRecipe `json:"r,omitempty"` // an item's recipes, the first preferred
	P *PlanPiece   `json:"p,omitempty"` // a piece's cost
}

// PlanRecipe is a recipe without what the planner does not count: the
// Forge of Potential's idols, which only that station asks for.
type PlanRecipe struct {
	At   string    `json:"at,omitempty"` // station piece, "" by hand
	Lv   int       `json:"lv,omitempty"` // min station level
	N    int       `json:"n"`            // items a batch makes
	Rows []PlanReq `json:"res"`
}

// PlanReq is one requirement.
type PlanReq struct {
	Item     string `json:"i"`
	Amount   int    `json:"n"`
	PerLevel int    `json:"l,omitempty"`
}

// PlanPiece is what a piece costs and what it is.
type PlanPiece struct {
	At      string    `json:"at,omitempty"` // station piece
	Tool    string    `json:"tool,omitempty"`
	Rows    []PlanReq `json:"res"`
	Station string    `json:"station,omitempty"` // the station this piece is
	Extends string    `json:"ext,omitempty"`     // the station this piece upgrades
	Conv    string    `json:"conv,omitempty"`    // the converter this piece is
}

// PlanConv is a converter: what it turns into what, and the fuel it burns.
type PlanConv struct {
	Fuel string         `json:"fuel,omitempty"`
	FPI  float64        `json:"fpi,omitempty"` // fuel per product
	SPI  float64        `json:"spi,omitempty"` // seconds per product
	C    []PlanConvRule `json:"c"`
}

// PlanConvRule is one conversion: n of To for one From.
type PlanConvRule struct {
	From string `json:"from"`
	To   string `json:"to"`
	N    int    `json:"n,omitempty"`
}

// PlanStation is a station's top level and the pieces that raise it.
type PlanStation struct {
	Max int      `json:"max"`
	Ext []string `json:"ext,omitempty"`
}

// PlanData is planner.json.
type PlanData struct {
	Things   map[string]*PlanThing   `json:"things"`
	Conv     map[string]*PlanConv    `json:"conv"`
	Stations map[string]*PlanStation `json:"stations"`
}

func (s *Site) plannerData() *PlanData {
	ix := s.Index
	d := &PlanData{Things: map[string]*PlanThing{}, Conv: map[string]*PlanConv{}, Stations: map[string]*PlanStation{}}
	thing := func(prefab, kind string) *PlanThing {
		if t := d.Things[prefab]; t != nil {
			return t
		}
		pt := &PlanThing{K: kind}
		if t := ix.Things[prefab]; t != nil {
			pt.N, pt.H, pt.I, pt.U, pt.S = t.Name, t.Hint, t.Icon, t.URL, t.Sub
		} else {
			pt.N = Pretty(prefab)
		}
		if it := s.DB.Items[prefab]; it != nil && kind == "item" {
			pt.W, pt.WL, pt.St, pt.TP, pt.Q = it.Weight, it.WeightPerLevel, it.Stack, it.Teleportable, it.MaxQuality
			if pt.N == "" {
				pt.N = it.Name
			}
		}
		d.Things[prefab] = pt
		return pt
	}
	reqs := func(rs []model.Requirement) []PlanReq {
		var out []PlanReq
		for _, r := range rs {
			if r.Upgrader || r.Item == "" || r.Amount <= 0 && r.PerLevel <= 0 {
				continue
			}
			thing(r.Item, "item")
			out = append(out, PlanReq{Item: r.Item, Amount: r.Amount, PerLevel: r.PerLevel})
		}
		return out
	}
	// The pieces a tool's menu lists, as the building index has them.
	for _, pt := range s.DB.PieceTables {
		for _, pp := range pt.Pieces {
			p := s.DB.Pieces[pp]
			if p == nil || !p.Enabled || p.Category == "All" || p.RepairPiece || len(p.Resources) == 0 {
				continue
			}
			// The feast table's entries are the foods themselves, set
			// down: the item is the thing to plan, not its placing.
			if s.DB.Items[p.Prefab] != nil {
				continue
			}
			t := thing(p.Prefab, "piece")
			if t.P != nil {
				continue
			}
			t.P = &PlanPiece{At: p.Station, Tool: p.Tool, Rows: reqs(p.Resources), Station: p.Station_, Extends: p.Extends, Conv: p.Converter_}
			if p.Station != "" {
				thing(p.Station, "piece")
			}
		}
	}
	// Every item with a recipe that is shown on the site.
	for _, it := range ix.Items {
		rs := ix.Recipes[it.Prefab]
		if len(rs) == 0 || !s.visible(it) {
			continue
		}
		t := thing(it.Prefab, "item")
		for _, r := range rs {
			if r.OnlyUpgrade {
				continue
			}
			pr := PlanRecipe{At: r.Station, Lv: r.MinStationLevel, N: max(1, r.Amount), Rows: reqs(r.Resources)}
			if len(pr.Rows) == 0 {
				continue
			}
			if r.Station != "" {
				thing(r.Station, "piece")
			}
			t.R = append(t.R, pr)
		}
	}
	// Converters: one material to another. The piece that holds one is
	// named by the converter's prefab, which is the piece's.
	for prefab, c := range s.DB.Converters {
		if len(c.Conversions) == 0 {
			continue
		}
		pc := &PlanConv{Fuel: c.Fuel, FPI: c.FuelPerItem, SPI: c.SecPerItem}
		for _, cv := range c.Conversions {
			if cv.From == "" || cv.To == "" {
				continue
			}
			thing(cv.From, "item")
			thing(cv.To, "item")
			pc.C = append(pc.C, PlanConvRule{From: cv.From, To: cv.To, N: cv.Amount})
		}
		if len(pc.C) == 0 {
			continue
		}
		if c.Fuel != "" {
			thing(c.Fuel, "item")
		}
		thing(prefab, "piece")
		d.Conv[prefab] = pc
	}
	for prefab, st := range s.DB.Stations {
		d.Stations[prefab] = &PlanStation{Max: max(1, st.MaxLevel), Ext: st.Extensions}
		for _, e := range st.Extensions {
			thing(e, "piece")
		}
	}
	// A piece or an item the tables name but no menu or recipe gives is
	// still there to be drawn; stations with no piece of their own too.
	for prefab, t := range d.Things {
		if t.K == "piece" && t.P == nil {
			if p := s.DB.Pieces[prefab]; p != nil && len(p.Resources) > 0 {
				t.P = &PlanPiece{At: p.Station, Tool: p.Tool, Rows: reqs(p.Resources), Station: p.Station_, Extends: p.Extends, Conv: p.Converter_}
			}
		}
	}
	return d
}

// pagesPlanner writes the planner page and its data.
func (s *Site) pagesPlanner() error {
	d := s.plannerData()
	if err := s.writeJSON("planner.json", d); err != nil {
		return err
	}
	return s.write("guides/planner/", "planner", &Page{Site: s, Title: "Planner",
		Description: "List what you mean to build and make in Valheim — the pieces of a new settlement, the gear of the next biome — and see what it costs down to the ore and the wood, the stations and levels it takes, and what of it a portal will carry.",
		Section:     "guides", Path: "guides/planner/", Crumbs: []Crumb{{"Guides", "guides/"}, {"Planner", ""}}, Data: map[string]any{"Things": len(d.Things)}})
}
