package main

import (
	"fmt"
	"sort"

	"github.com/pietjan/valheim-wiki/internal/assets"
	"github.com/pietjan/valheim-wiki/internal/model"
)

// converterKind is one component type that turns items into items, found
// by its distinctive fields. Kinds are tried in order and the first to
// claim a prefab names it; a later component on the same prefab (the
// windmill's Windmill next to its Smelter) only adds to the record.
type converterKind struct {
	kind   string
	fields []string
	read   func(x *X, cab string, t map[string]any, c *model.Converter)
}

var converterKinds = []converterKind{
	// Smelter: smelter, blastfurnace, charcoal_kiln, eitrrefinery,
	// windmill, piece_spinningwheel, piece_bathtub (a sauna is a smelter
	// that burns wood and makes nothing).
	{"smelter", []string{"m_conversion", "m_maxOre", "m_secPerProduct"}, readSmelter},
	// CookingStation: piece_cookingstation, piece_cookingstation_iron,
	// piece_oven.
	{"cooking", []string{"m_conversion", "m_overCookedItem"}, readCooking},
	{"fermenter", []string{"m_conversion", "m_fermentationDuration"}, readFermenter},
	{"incinerator", []string{"m_conversions", "m_defaultResult"}, readIncinerator},
	{"beehive", []string{"m_honeyItem", "m_secPerUnit"}, readBeehive},
	{"sap", []string{"m_spawnItem", "m_maxLevel", "m_secPerUnit"}, readSap},
	{"feast", []string{"m_eatStacks", "m_foodItem"}, readFeast},
	{"shield", []string{"m_fuelItems", "m_maxFuel", "m_maxShieldRadius"}, readShield},
	{"turret", []string{"m_defaultAmmo", "m_allowedAmmo", "m_maxAmmo"}, readTurret},
	{"wisp", []string{"m_wispPrefab", "m_spawnChance", "m_maxSpawned"}, readWisp},
	{"trap", []string{"m_rearmCooldown", "m_triggeredByEnemies"}, readTrap},
	// Fireplace last: a hearth is a fireplace, an oven is a cooking
	// station first.
	{"fireplace", []string{"m_fuelItem", "m_startFuel", "m_secPerFuel"}, readFireplace},
	{"windmill", []string{"m_propellerRotationSpeed", "m_minWindSpeed"}, readWindmill},
}

// extractConverters reads one Converter per piece prefab root that
// carries a converting component. Fireplaces are kept only when the
// prefab is a buildable Piece: the game lights many unbuildable fires
// (camp fires in villages, the trader's hearth) with the same component.
func extractConverters(x *X) error {
	for _, k := range converterKinds {
		x.EachMono(k.fields, func(cab string, id int64, t map[string]any) {
			g, ok := x.Deref(cab, t["m_GameObject"])
			if !ok || !x.isRoot(g) {
				return
			}
			prefab := x.Name(g)
			if prefab == "" {
				return
			}
			piece, isPiece := x.Find(g, "m_resources", "m_icon", "m_category")
			if k.kind == "fireplace" && !isPiece {
				return
			}
			c := x.DB.Converters[prefab]
			if c == nil {
				if k.kind == "windmill" {
					return // the Windmill only drives a Smelter
				}
				token := S(t, "m_name")
				if isPiece {
					token = S(piece.Tree, "m_name")
				}
				c = &model.Converter{Prefab: prefab, Name: x.NameOf(prefab, token), Kind: k.kind}
				x.DB.Converters[prefab] = c
			} else if c.Kind == k.kind {
				return // a copy in a later bundle
			}
			k.read(x, cab, t, c)
		})
	}
	return nil
}

// itemName is the prefab behind a PPtr to an ItemDrop component.
func (x *X) itemName(cab string, ref any) string { return x.GameObjectName(cab, ref) }

// readSmelter: m_maxOre ore queue up, m_maxFuel fuel, m_fuelPerProduct
// fuel and m_secPerProduct seconds per product.
func readSmelter(x *X, cab string, t map[string]any, c *model.Converter) {
	c.Fuel = x.itemName(cab, t["m_fuelItem"])
	c.FuelPerItem = F(t, "m_fuelPerProduct")
	c.SecPerItem = F(t, "m_secPerProduct")
	c.MaxInput = I(t, "m_maxOre")
	c.MaxFuel = I(t, "m_maxFuel")
	c.RequiresRoof = B(t, "m_requiresRoof")
	for _, e := range L(t, "m_conversion") {
		m := assets.AsMap(e)
		c.Conversions = append(c.Conversions, model.Conversion{From: x.itemName(cab, m["m_from"]), To: x.itemName(cab, m["m_to"])})
	}
}

// readCooking: each conversion has its own cook time; an item left
// twice as long burns to m_overCookedItem. m_requireFire wants a lit
// fire under the station; an oven burns m_fuelItem instead, one every
// m_secPerFuel seconds.
func readCooking(x *X, cab string, t map[string]any, c *model.Converter) {
	c.RequiresFire = B(t, "m_requireFire")
	if B(t, "m_canOvercookItems") {
		c.Overcooked = x.itemName(cab, t["m_overCookedItem"])
	}
	if B(t, "m_useFuel") {
		c.Fuel = x.itemName(cab, t["m_fuelItem"])
		c.MaxFuel = I(t, "m_maxFuel")
		c.SecPerFuel = F(t, "m_secPerFuel")
	}
	for _, e := range L(t, "m_conversion") {
		m := assets.AsMap(e)
		c.Conversions = append(c.Conversions, model.Conversion{From: x.itemName(cab, m["m_from"]), To: x.itemName(cab, m["m_to"]), Time: F(m, "m_cookTime")})
	}
}

// readFermenter: one input becomes m_producedItems after
// m_fermentationDuration seconds, under a roof (Fermenter.GetStatus
// reports Exposed otherwise).
func readFermenter(x *X, cab string, t map[string]any, c *model.Converter) {
	c.Duration = F(t, "m_fermentationDuration")
	c.RequiresRoof = true
	for _, e := range L(t, "m_conversion") {
		m := assets.AsMap(e)
		c.Conversions = append(c.Conversions, model.Conversion{From: x.itemName(cab, m["m_from"]), To: x.itemName(cab, m["m_to"]), Amount: I(m, "m_producedItems")})
	}
}

// readIncinerator: the conversions are tried by priority; whatever is
// left burns to m_defaultResult, one per m_defaultCost items.
func readIncinerator(x *X, cab string, t map[string]any, c *model.Converter) {
	c.DefaultResult = x.itemName(cab, t["m_defaultResult"])
	c.DefaultCost = I(t, "m_defaultCost")
	for _, e := range L(t, "m_conversions") {
		m := assets.AsMap(e)
		inc := model.Incinerate{
			Result:   x.itemName(cab, m["m_result"]),
			Amount:   I(m, "m_resultAmount"),
			Priority: I(m, "m_priority"),
			OneOf:    B(m, "m_requireOnlyOneIngredient"),
		}
		for _, r := range L(m, "m_requirements") {
			rm := assets.AsMap(r)
			if item := x.itemName(cab, rm["m_resItem"]); item != "" {
				inc.Requirements = append(inc.Requirements, model.Requirement{Item: item, Amount: I(rm, "m_amount")})
			}
		}
		if inc.Requirements == nil {
			inc.Requirements = []model.Requirement{}
		}
		c.Incinerations = append(c.Incinerations, inc)
	}
	sort.SliceStable(c.Incinerations, func(i, j int) bool { return c.Incinerations[i].Priority > c.Incinerations[j].Priority })
}

// readBeehive: one honey every m_secPerUnit seconds in daylight, up to
// m_maxHoney, in the m_biome biomes and with less than m_maxCover cover.
func readBeehive(x *X, cab string, t map[string]any, c *model.Converter) {
	c.Product = x.itemName(cab, t["m_honeyItem"])
	c.SecPerItem = F(t, "m_secPerUnit")
	c.MaxProduct = I(t, "m_maxHoney")
	c.Biomes = Biomes(I(t, "m_biome"))
	c.Notes = append(c.Notes, fmt.Sprintf("Needs less than %.0f%% cover", F(t, "m_maxCover")*100))
	if B(t, "m_effectOnlyInDaylight") {
		c.Notes = append(c.Notes, "Works in daylight only")
	}
}

// readSap: one unit every m_secPerUnit seconds up to m_maxLevel, while
// attached to a root.
func readSap(x *X, cab string, t map[string]any, c *model.Converter) {
	c.Product = x.itemName(cab, t["m_spawnItem"])
	c.SecPerItem = F(t, "m_secPerUnit")
	c.MaxProduct = I(t, "m_maxLevel")
	if root := x.GameObjectName(cab, t["m_mustConnectTo"]); root != "" {
		c.Notes = append(c.Notes, "Must be placed on "+root)
	}
}

// readFeast: a placed feast serves m_eatStacks helpings of m_foodItem,
// which is the feast's own ItemDrop when left unset (Feast.Awake falls
// back to GetComponent<ItemDrop>).
func readFeast(x *X, cab string, t map[string]any, c *model.Converter) {
	c.Product = x.itemName(cab, t["m_foodItem"])
	if c.Product == "" {
		c.Product = c.Prefab
	}
	c.MaxProduct = I(t, "m_eatStacks")
}

// readShield: a shield generator burns any of m_fuelItems, m_fuelPerDamage
// per point absorbed, and its dome shrinks from m_maxShieldRadius towards
// m_minShieldRadius as fuel runs low.
func readShield(x *X, cab string, t map[string]any, c *model.Converter) {
	for _, f := range L(t, "m_fuelItems") {
		if item := x.itemName(cab, f); item != "" {
			c.Fuels = append(c.Fuels, item)
		}
	}
	c.MaxFuel = I(t, "m_maxFuel")
	c.Notes = append(c.Notes,
		fmt.Sprintf("Radius %.0f-%.0f m", F(t, "m_minShieldRadius"), F(t, "m_maxShieldRadius")),
		fmt.Sprintf("%.2g fuel per point of damage absorbed", F(t, "m_fuelPerDamage")))
}

// readTurret: loads m_defaultAmmo or any of m_allowedAmmo, up to
// m_maxAmmo, and fires every m_attackCooldown seconds at targets within
// m_viewDistance. A turret with m_maxAmmo 0 never runs out (Turret.HasAmmo
// is true and ShootProjectile spends nothing): the Charred ballista.
func readTurret(x *X, cab string, t map[string]any, c *model.Converter) {
	c.Fuel = x.itemName(cab, t["m_defaultAmmo"])
	for _, a := range L(t, "m_allowedAmmo") {
		if item := x.itemName(cab, assets.AsMap(a)["m_ammo"]); item != "" {
			c.Fuels = append(c.Fuels, item)
		}
	}
	c.MaxFuel = I(t, "m_maxAmmo")
	c.Notes = append(c.Notes,
		fmt.Sprintf("Fires every %.1f s, range %.0f m", F(t, "m_attackCooldown"), F(t, "m_viewDistance")))
	if c.MaxFuel == 0 {
		c.Notes = append(c.Notes, "Does not consume ammo")
	}
}

// readWisp: the wisp fountain spawns a wisp with m_spawnChance every
// m_spawnInterval seconds, up to m_maxSpawned nearby.
func readWisp(x *X, cab string, t map[string]any, c *model.Converter) {
	c.Product = x.GameObjectName(cab, t["m_wispPrefab"])
	c.SecPerItem = F(t, "m_spawnInterval")
	c.MaxProduct = I(t, "m_maxSpawned")
	c.Notes = append(c.Notes, fmt.Sprintf("%.0f%% chance per interval", F(t, "m_spawnChance")*100))
	if B(t, "m_onlySpawnAtNight") {
		c.Notes = append(c.Notes, "At night only")
	}
	if B(t, "m_dontSpawnInCover") {
		c.Notes = append(c.Notes, fmt.Sprintf("Needs less than %.0f%% cover", F(t, "m_maxCover")*100))
	}
}

// readTrap: re-arms m_rearmCooldown seconds after triggering.
func readTrap(x *X, cab string, t map[string]any, c *model.Converter) {
	c.SecPerItem = F(t, "m_rearmCooldown")
	switch {
	case B(t, "m_triggeredByEnemies") && B(t, "m_triggeredByPlayers"):
		c.Notes = append(c.Notes, "Triggered by enemies and players")
	case B(t, "m_triggeredByEnemies"):
		c.Notes = append(c.Notes, "Triggered by enemies")
	case B(t, "m_triggeredByPlayers"):
		c.Notes = append(c.Notes, "Triggered by players")
	}
}

// readFireplace: built with m_startFuel of m_fuelItem, holds m_maxFuel,
// burns one every m_secPerFuel seconds unless m_infiniteFuel. Fireplace
// serialises m_maxFuel as a float (unlike Smelter's int), so it is read as
// one.
func readFireplace(x *X, cab string, t map[string]any, c *model.Converter) {
	c.Fuel = x.itemName(cab, t["m_fuelItem"])
	c.StartFuel = F(t, "m_startFuel")
	c.MaxFuel = int(F(t, "m_maxFuel"))
	c.SecPerFuel = F(t, "m_secPerFuel")
	c.InfiniteFuel = B(t, "m_infiniteFuel")
}

// readWindmill: the Smelter on the same prefab grinds only while the
// propeller turns, from m_minWindSpeed up.
func readWindmill(x *X, cab string, t map[string]any, c *model.Converter) {
	c.Notes = append(c.Notes, fmt.Sprintf("Wind driven: runs at wind strength %.0f%% and above, faster in stronger wind", F(t, "m_minWindSpeed")*100))
}
