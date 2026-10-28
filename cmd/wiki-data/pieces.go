package main

import (
	"sort"

	"github.com/pietjan/valheim-wiki/internal/assets"
	"github.com/pietjan/valheim-wiki/internal/model"
)

// extractPieces reads every buildable: a Piece component on a prefab root
// (the hammer's walls and furniture, the hoe's and cultivator's ground
// pieces and saplings, the feaster's food, ships and carts) with what
// the other components on the same root make it: its durability
// (WearNTear), storage (Container), door, fire, bed, portal, ward, ship,
// cart, plant, station, extension, converter, item stand, sign, map
// table, turret, trap, demister, windmill, chair and light. Then the
// four PieceTables, the build menus, which say which tool opens each
// piece and in what order.
func extractPieces(x *X) error {
	x.EachMono([]string{"m_resources", "m_icon", "m_category"}, func(cab string, id int64, t map[string]any) {
		root, g := x.Owner(cab, t)
		if root == "" || !x.isPrefabRoot(g) {
			return
		}
		if _, dup := x.DB.Pieces[root]; dup {
			return
		}
		x.DB.Pieces[root] = x.readPiece(cab, root, g, t)
	})
	x.readPieceTables()
	return nil
}

// isPrefabRoot is whether g is one of the fatherless GameObjects, i.e. a
// prefab and not something nested in one (a ship's chest, a location's
// furniture).
func (x *X) isPrefabRoot(g assets.Ref) bool {
	for _, r := range x.RootsAll[x.Name(g)] {
		if r == g {
			return true
		}
	}
	return false
}

// readPiece reads the Piece component t on the root g, then the rest of
// the root's components.
func (x *X) readPiece(cab, prefab string, g assets.Ref, t map[string]any) *model.Piece {
	p := &model.Piece{
		Prefab:      prefab,
		Token:       S(t, "m_name"),
		Name:        x.NameOf(prefab, S(t, "m_name")),
		Description: x.Text(S(t, "m_description")),
		Category:    pieceCategoryName(I(t, "m_category")),
		Usage:       pieceUsageTags(I(t, "m_usage")),
		Enabled:     B(t, "m_enabled"),
		DLC:         S(t, "m_dlc"),
		Station:     x.GameObjectName(cab, t["m_craftingStation"]),
		Resources:   x.Requirements(cab, t["m_resources"]),
		Comfort:     I(t, "m_comfort"),
		OnlyBiome:   Biomes(I(t, "m_onlyInBiome")),
	}
	if cg := I(t, "m_comfortGroup"); cg > 0 && cg < len(comfortGroupNames) {
		p.ComfortGroup = comfortGroupNames[cg]
	}
	if icon, ok := x.Deref(cab, t["m_icon"]); ok {
		p.Icon = true
		x.WantSprite("pieces", prefab, icon)
	}
	for _, f := range placementFlags {
		if B(t, "m_"+f) {
			p.Placement = append(p.Placement, f)
		}
	}
	for _, c := range x.Components(g) {
		if c.ClassID != 114 {
			continue
		}
		ct := c.Tree
		switch {
		case Has(ct, "m_materialType", "m_supports"): // WearNTear
			p.Health = F(ct, "m_health")
			if m := I(ct, "m_materialType"); m >= 0 && m < len(materialTypeNames) {
				p.Material = materialTypeNames[m]
			}
			p.Resist = DamageMods(M(ct, "m_damages"))
			p.Supports = B(ct, "m_supports")
			p.Burnable = B(ct, "m_burnable")
			p.NoRoofWear = B(ct, "m_noRoofWear")
			p.AshImmune = B(ct, "m_ashDamageImmune")
			p.MinToolTier = I(ct, "m_minToolTier")
		case Has(ct, "m_width", "m_height", "m_privacy", "m_defaultItems"): // Container
			p.Container = x.containerInfo(c.Ref.Cab, ct)
		case Has(ct, "m_keyItem", "m_consumeKey"): // Door
			p.Door = &model.DoorInfo{Key: x.GameObjectName(c.Ref.Cab, ct["m_keyItem"]), ConsumeKey: B(ct, "m_consumeKey")}
		case Has(ct, "m_startFuel", "m_secPerFuel", "m_fuelItem"): // Fireplace
			p.Fireplace = &model.FireplaceInfo{
				Fuel:       x.GameObjectName(c.Ref.Cab, ct["m_fuelItem"]),
				StartFuel:  F(ct, "m_startFuel"),
				MaxFuel:    F(ct, "m_maxFuel"),
				SecPerFuel: F(ct, "m_secPerFuel"),
				Infinite:   B(ct, "m_infiniteFuel"),
			}
		case Has(ct, "m_spawnPoint", "m_monsterCheckRadius"): // Bed
			p.Bed = true
		case Has(ct, "m_allowAllItems", "m_activationRange"): // TeleportWorld
			p.Portal = &model.PortalInfo{AllowAllItems: B(ct, "m_allowAllItems")}
		case Has(ct, "m_radius", "m_enabledByDefault", "m_ownerFaction"): // PrivateArea
			p.Ward = &model.WardInfo{Radius: F(ct, "m_radius")}
		case Has(ct, "m_sailForceFactor", "m_rudderSpeed"): // Ship
			p.Ship = &model.ShipInfo{
				SailForce:     F(ct, "m_sailForceFactor"),
				RudderSpeed:   F(ct, "m_rudderSpeed"),
				SteerForce:    F(ct, "m_stearForce"),
				BackwardForce: F(ct, "m_backwardForce"),
				HasSail:       B(ct, "m_hasSail"),
				AshlandsReady: B(ct, "m_ashlandsReady"),
				Container:     x.childContainer(g),
			}
		case Has(ct, "m_baseMass", "m_itemWeightMassFactor", "m_container"): // Vagon
			p.Cart = &model.CartInfo{BaseMass: F(ct, "m_baseMass")}
			if cont, r := x.Env.TreeOf(c.Ref.Cab, ct["m_container"]); cont != nil {
				p.Cart.Container = x.containerInfo(r.Cab, cont)
			}
		case Has(ct, "m_growTime", "m_grownPrefabs"): // Plant
			p.Plant = x.plantInfo(c.Ref.Cab, ct)
		case Has(ct, "m_upgrader", "m_craftRequireRoof"): // CraftingStation
			p.Station_ = prefab
		case Has(ct, "m_craftingStation", "m_maxStationDistance"): // StationExtension
			p.Extends = x.GameObjectName(c.Ref.Cab, ct["m_craftingStation"])
		case Has(ct, "m_conversion", "m_fuelPerProduct"), // Smelter
			Has(ct, "m_conversion", "m_overCookedItem"),       // CookingStation
			Has(ct, "m_conversion", "m_fermentationDuration"), // Fermenter
			Has(ct, "m_conversions", "m_defaultResult"),       // Incinerator
			Has(ct, "m_maxHoney", "m_honeyItem"),              // Beehive
			Has(ct, "m_spawnItem", "m_secPerUnit"),            // SapCollector
			Has(ct, "m_eatStacks", "m_feastParts"):            // Feast
			p.Converter_ = prefab
		case Has(ct, "m_supportedTypes", "m_guardianPower"): // ItemStand
			is := &model.ItemStandInfo{GuardianPower: x.Env.Name(c.Ref.Cab, ct["m_guardianPower"])}
			for _, v := range L(ct, "m_supportedTypes") {
				if n := standItemType(int(assets.AsInt64(v))); n != "" {
					is.Types = append(is.Types, n)
				}
			}
			p.ItemStand = is
		case Has(ct, "m_characterLimit", "m_defaultText"): // Sign
			p.Sign = true
		case Has(ct, "m_readSwitch", "m_writeSwitch"): // MapTable
			p.MapTable = true
		case Has(ct, "m_allowedAmmo", "m_maxAmmo"): // Turret
			tu := &model.TurretInfo{MaxAmmo: I(ct, "m_maxAmmo"), Range: F(ct, "m_viewDistance"), Cooldown: F(ct, "m_attackCooldown")}
			for _, a := range L(ct, "m_allowedAmmo") {
				if n := x.GameObjectName(c.Ref.Cab, M(assets.AsMap(a), "m_ammo")); n != "" {
					tu.Ammo = append(tu.Ammo, n)
				}
			}
			p.Turret = tu
		case Has(ct, "m_rearmCooldown", "m_visualArmed"): // Trap
			p.Trap = true
		case Has(ct, "m_propeller", "m_grindstone"): // Windmill
			p.Windmill = true
		case Has(ct, "m_attachAnimation", "m_inShip"): // Chair
			p.Chair = true
		}
	}
	// Lights and demisters sit on a child as often as on the root: the
	// flame of a torch, the force field of a wisp lure.
	x.Walk(g, func(_ int, c assets.Ref) bool {
		for _, comp := range x.Components(c) {
			switch {
			case comp.ClassID == 108:
				p.Light = true
			case comp.ClassID == 114 && Has(comp.Tree, "m_forceField", "m_disableForcefieldDelay"):
				p.Demister = true
			}
		}
		return !(p.Light && p.Demister)
	})
	return p
}

// containerInfo reads a Container component; its m_defaultItems is the
// loot a chest starts with (empty for anything the player builds).
func (x *X) containerInfo(cab string, t map[string]any) *model.ContainerInfo {
	return &model.ContainerInfo{
		Width:   I(t, "m_width"),
		Height:  I(t, "m_height"),
		Private: I(t, "m_privacy") == 0, // PrivacySetting.Private
		Loot:    x.DropTable(cab, M(t, "m_defaultItems")),
	}
}

// childContainer is the first Container below g: a ship's hold.
func (x *X) childContainer(g assets.Ref) *model.ContainerInfo {
	var out *model.ContainerInfo
	x.Walk(g, func(d int, c assets.Ref) bool {
		if out != nil {
			return false
		}
		if d == 0 {
			return true
		}
		if comp, ok := x.Find(c, "m_width", "m_height", "m_privacy", "m_defaultItems"); ok {
			out = x.containerInfo(comp.Ref.Cab, comp.Tree)
			return false
		}
		return true
	})
	return out
}

// plantInfo reads a Plant component: the sapling grows into one of
// m_grownPrefabs after a random time between m_growTime and
// m_growTimeMax seconds, if nothing stands within m_growRadius and the
// biome and temperature suit it.
func (x *X) plantInfo(cab string, t map[string]any) *model.PlantInfo {
	pi := &model.PlantInfo{
		GrowTime:       F(t, "m_growTime"),
		GrowTimeMax:    F(t, "m_growTimeMax"),
		GrowRadius:     F(t, "m_growRadius"),
		NeedCultivated: B(t, "m_needCultivatedGround"),
		Biomes:         Biomes(I(t, "m_biome")),
		TolerateHeat:   B(t, "m_tolerateHeat"),
		TolerateCold:   B(t, "m_tolerateCold"),
		DestroyIfCant:  B(t, "m_destroyIfCantGrow"),
	}
	for _, g := range L(t, "m_grownPrefabs") {
		if n := x.GameObjectName(cab, g); n != "" {
			pi.GrownPrefabs = append(pi.GrownPrefabs, n)
		}
	}
	return pi
}

// readPieceTables reads the PieceTable prefabs (_HammerPieceTable and
// its siblings): the build menu of the tool whose SharedData
// m_buildPieces points at it. Pieces take the tool of the first table
// listing them.
func (x *X) readPieceTables() {
	// Tool items name their table; items are extracted later (or not at
	// all with -only pieces), so read the ItemDrops directly.
	tool := map[string]string{}
	x.EachMono([]string{"m_itemData"}, func(cab string, id int64, t map[string]any) {
		root, g := x.Owner(cab, t)
		if root == "" || !x.isPrefabRoot(g) {
			return
		}
		table := x.GameObjectName(cab, M(M(t, "m_itemData"), "m_shared")["m_buildPieces"])
		if table == "" {
			return
		}
		if _, ok := tool[table]; !ok {
			tool[table] = root
		}
	})
	seen := map[string]bool{}
	x.EachMono([]string{"m_pieces", "m_categories", "m_categoryLabels"}, func(cab string, id int64, t map[string]any) {
		root, g := x.Owner(cab, t)
		if root == "" || !x.isPrefabRoot(g) || seen[root] {
			return
		}
		seen[root] = true
		pt := &model.PieceTable{Prefab: root, Tool: tool[root]}
		for _, p := range L(t, "m_pieces") {
			if n := x.GameObjectName(cab, p); n != "" {
				pt.Pieces = append(pt.Pieces, n)
			}
		}
		for _, c := range L(t, "m_categories") {
			pt.Categories = append(pt.Categories, pieceCategoryName(int(assets.AsInt64(c))))
		}
		for _, l := range Strs(t["m_categoryLabels"]) {
			pt.Labels = append(pt.Labels, x.Text(l))
		}
		x.DB.PieceTables = append(x.DB.PieceTables, pt)
	})
	sort.Slice(x.DB.PieceTables, func(i, j int) bool { return x.DB.PieceTables[i].Prefab < x.DB.PieceTables[j].Prefab })
	for _, pt := range x.DB.PieceTables {
		for _, name := range pt.Pieces {
			if p := x.DB.Pieces[name]; p != nil && p.Tool == "" {
				p.Tool = pt.Tool
			}
		}
	}
}

// pieceCategoryName names a Piece.PieceCategory: the tab of the build
// menu the piece sits in.
func pieceCategoryName(v int) string {
	names := []string{"Misc", "Crafting", "BuildingWorkbench", "BuildingStonecutter", "Furniture", "DeepNorth", "Feasts", "Food", "Meads"}
	if v >= 0 && v < len(names) {
		return names[v]
	}
	if v == 100 {
		return "All"
	}
	return ""
}

// pieceUsageTags expands Piece.UsageTagFlags, the filter tags of the
// build menu's search.
func pieceUsageTags(mask int) []string {
	names := []string{"Misc", "Crafting", "Building", "Floor", "Wall", "Roof", "Architecture", "Furniture", "Lighting", "Decor", "Storage", "Transport", "Food", "Meads", "Feasts", "Defense", "Stacks", "Stairs", "Doors", "Seasonal"}
	var out []string
	for i, n := range names {
		if mask&(1<<i) != 0 {
			out = append(out, n)
		}
	}
	return out
}

// comfortGroupNames is Piece.ComfortGroup: only one piece per group
// counts towards the rested bonus.
var comfortGroupNames = []string{"None", "Fire", "Bed", "Banner", "Chair", "Table", "Carpet", "Display", "Decor", "Garland", "Lantern", "Leisure"}

// materialTypeNames is WearNTear.MaterialType: what the piece is made of
// decides how far it can carry load from the ground.
var materialTypeNames = []string{"Wood", "Stone", "Iron", "HardWood", "Marble", "Ashstone", "Ancient", "Ice", "Timberwood"}

// placementFlags are Piece's placement rule booleans, in the order the
// model documents them (without the m_ prefix).
var placementFlags = []string{
	"groundPiece", "groundOnly", "waterPiece", "cultivatedGroundOnly", "allowedInDungeons",
	"noInWater", "notOnWood", "notOnTiltingSurface", "inCeilingOnly", "notOnFloor",
	"onlyInTeleportArea", "allowedInDeepSnow", "requireDeepSnow", "vegetationGroundOnly", "clipGround",
}

// standItemType names an ItemDrop.ItemData.ItemType, as ItemStand's
// m_supportedTypes lists them.
func standItemType(v int) string {
	switch v {
	case 1:
		return "Material"
	case 2:
		return "Consumable"
	case 3:
		return "OneHandedWeapon"
	case 4:
		return "Bow"
	case 5:
		return "Shield"
	case 6:
		return "Helmet"
	case 7:
		return "Chest"
	case 9:
		return "Ammo"
	case 10:
		return "Customization"
	case 11:
		return "Legs"
	case 12:
		return "Hands"
	case 13:
		return "Trophy"
	case 14:
		return "TwoHandedWeapon"
	case 15:
		return "Torch"
	case 16:
		return "Misc"
	case 17:
		return "Shoulder"
	case 18:
		return "Utility"
	case 19:
		return "Tool"
	case 20:
		return "Attach_Atgeir"
	case 21:
		return "Fish"
	case 22:
		return "TwoHandedWeaponLeft"
	case 23:
		return "AmmoNonEquipable"
	case 24:
		return "Trinket"
	}
	return ""
}
