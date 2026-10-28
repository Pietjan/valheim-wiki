package main

import (
	"fmt"
	"strconv"

	"github.com/pietjan/valheim-wiki/internal/assets"
	"github.com/pietjan/valheim-wiki/internal/model"
)

// itemTypes names ItemDrop.ItemData.ItemType (8 is unused).
var itemTypes = map[int]string{
	0: "None", 1: "Material", 2: "Consumable", 3: "OneHandedWeapon", 4: "Bow",
	5: "Shield", 6: "Helmet", 7: "Chest", 9: "Ammo", 10: "Customization",
	11: "Legs", 12: "Hands", 13: "Trophy", 14: "TwoHandedWeapon", 15: "Torch",
	16: "Misc", 17: "Shoulder", 18: "Utility", 19: "Tool", 20: "Attach_Atgeir",
	21: "Fish", 22: "TwoHandedWeaponLeft", 23: "AmmoNonEquipable", 24: "Trinket",
}

// skillTypes names Skills.SkillType.
var skillTypes = map[int]string{
	0: "", 1: "Swords", 2: "Knives", 3: "Clubs", 4: "Polearms", 5: "Spears",
	6: "Blocking", 7: "Axes", 8: "Bows", 9: "ElementalMagic", 10: "BloodMagic",
	11: "Unarmed", 12: "Pickaxes", 13: "WoodCutting", 14: "Crossbows",
	100: "Jump", 101: "Sneak", 102: "Run", 103: "Swim", 104: "Fishing",
	105: "Cooking", 106: "Farming", 107: "Crafting", 108: "Dodge", 110: "Ride",
	999: "All",
}

// attackTypes names Attack.AttackType.
var attackTypes = []string{"Horizontal", "Vertical", "Projectile", "None", "Area", "TriggerProjectile"}

// SkillName is the SkillType enum name for a value.
func SkillName(v int) string {
	if s, ok := skillTypes[v]; ok {
		return s
	}
	return fmt.Sprintf("Skill%d", v)
}

// Which stats mean something for an item type. SharedData carries a
// default for every field (armor 10, block power 10, skill Swords,
// durability 100, knockback 30 or 50, backstab 4, parry 1.5/5, ...), so a
// loaf of bread reads as a sword unless the type gates what is kept. The
// gates follow ItemDrop.ItemData.GetTooltip, which is where the game
// decides what a stat means per type:
//
//   - weapons (OneHandedWeapon, TwoHandedWeapon, TwoHandedWeaponLeft, Bow,
//     Torch, Tool, Attach_Atgeir): skill, damage, knockback, backstab, both
//     attacks, block/parry/deflection (every weapon can block), attack
//     effect, ammo type, tool tier; no armor. Tools keep attacks and skill
//     (the hammer's swing costs stamina, the cultivator trains Farming)
//     but the tooltip shows no block for them, so block is dropped.
//   - Shield: block/parry/deflection and the Blocking skill; no damage, no
//     attack.
//   - Ammo, AmmoNonEquipable: damage and knockback (the arrow's own; the
//     bow's backstab and skill apply on hit), ammo type.
//   - armor (Helmet, Chest, Legs, Shoulder): armor and resistances;
//     Player.GetBodyArmor sums only these four slots.
//   - Hands, Utility, Trinket: equipped but armorless: modifiers, set and
//     equip effects only.
//   - Consumable: food and the consume effect.
//   - Material, Trophy, Misc, Fish, Customization, None: none of the above.
//
// Durability is kept wherever m_useDurability is set, whatever the type,
// which is also the game's rule. The Forge of Potential block
// (m_upgradeChance, m_breakChance, ...) is read by InventoryGui off the
// recipe's upgrader resource, the idol, so it is kept on the items a
// recipe spends that way and dropped everywhere else.
type gate struct {
	weapon, shield, ammo, armor, wearable, food bool
}

func gateFor(typ string) gate {
	switch typ {
	case "OneHandedWeapon", "TwoHandedWeapon", "TwoHandedWeaponLeft", "Bow", "Torch", "Attach_Atgeir":
		return gate{weapon: true, shield: true, wearable: true}
	case "Tool":
		return gate{weapon: true, wearable: true}
	case "Shield":
		return gate{shield: true, wearable: true}
	case "Ammo", "AmmoNonEquipable":
		return gate{ammo: true}
	case "Helmet", "Chest", "Legs", "Shoulder":
		return gate{armor: true, wearable: true}
	case "Hands", "Utility", "Trinket":
		return gate{wearable: true}
	case "Consumable":
		return gate{food: true}
	}
	return gate{}
}

// extractItems reads every ItemDrop prefab: the MonoBehaviour with
// m_itemData whose GameObject is a prefab root. Creature attacks and NPC
// costumes are ItemDrops too; they come in marked Hidden (no icon) and the
// recipes pass clears the mark on anything a recipe makes.
func extractItems(x *X) error {
	idols := upgraderItems(x)
	x.EachMono([]string{"m_itemData"}, func(cab string, id int64, t map[string]any) {
		g, ok := x.Deref(cab, t["m_GameObject"])
		if !ok || !x.isRoot(g) {
			return
		}
		name := x.Name(g)
		if name == "" {
			return
		}
		if _, seen := x.DB.Items[name]; seen {
			return // a copy in a later bundle; the first loaded wins
		}
		x.DB.Items[name] = x.readItem(cab, name, g, assets.AsMap(assets.AsMap(t["m_itemData"])["m_shared"]), idols[name])
	})
	return nil
}

// isRoot is whether a GameObject's Transform has no father.
func (x *X) isRoot(g assets.Ref) bool {
	for _, c := range x.Components(g) {
		if c.ClassID == 4 || c.ClassID == 224 {
			return assets.AsInt64(assets.AsMap(c.Tree["m_Father"])["m_PathID"]) == 0
		}
	}
	return false
}

// readItem reads one SharedData, keeping what the type gives meaning to.
func (x *X) readItem(cab, prefab string, g assets.Ref, s map[string]any, idol bool) *model.Item {
	typ, ok := itemTypes[I(s, "m_itemType")]
	if !ok {
		typ = fmt.Sprintf("Type%d", I(s, "m_itemType"))
	}
	gt := gateFor(typ)
	it := &model.Item{
		Prefab:         prefab,
		Name:           x.NameOf(prefab, S(s, "m_name")),
		Token:          S(s, "m_name"),
		Description:    x.Text(S(s, "m_description")),
		Type:           typ,
		Variants:       I(s, "m_variants"),
		MaxQuality:     I(s, "m_maxQuality"),
		Weight:         F(s, "m_weight"),
		WeightPerLevel: F(s, "m_scaleWeightByQuality"),
		Stack:          I(s, "m_maxStackSize"),
		Value:          I(s, "m_value"),
		Teleportable:   B(s, "m_teleportable"),
		QuestItem:      B(s, "m_questItem"),
		DLC:            S(s, "m_dlc"),
		Resist:         DamageModList(s["m_damageModifiers"]),
	}
	if _, ok := x.Find(g, "m_waterLevelOffset"); ok {
		it.Floating = true
	}
	icons := L(s, "m_icons")
	it.Icons = len(icons)
	it.Hidden = len(icons) == 0
	for i, ic := range icons {
		ref, ok := x.Deref(cab, ic)
		if !ok {
			continue
		}
		name := prefab
		if i > 0 {
			name += "_" + strconv.Itoa(i)
		}
		x.WantSprite("items", name, ref)
	}
	if tbl := x.GameObjectName(cab, s["m_buildPieces"]); tbl != "" {
		it.BuildPieces = tbl
	}
	if B(s, "m_useDurability") {
		it.UseDurability = true
		it.Durability = F(s, "m_maxDurability")
		it.DurabilityPerLevel = F(s, "m_durabilityPerLevel")
		it.DurabilityDrain = F(s, "m_durabilityDrain")
		it.CanRepair = B(s, "m_canBeReparied")
	}
	if gt.wearable {
		it.Modifiers = modifiers(s)
		it.SetName = S(s, "m_setName")
		it.SetSize = I(s, "m_setSize")
		it.SetEffect = x.Env.Name(cab, s["m_setStatusEffect"])
		it.EquipEffect = x.Env.Name(cab, s["m_equipStatusEffect"])
	}
	if gt.weapon || gt.ammo {
		it.Damage = Damage(M(s, "m_damages"))
		it.DamagePerLevel = Damage(M(s, "m_damagesPerLevel"))
		it.Knockback = F(s, "m_attackForce")
		it.AmmoType = x.Text(S(s, "m_ammoType"))
	}
	if gt.weapon {
		it.Skill = SkillName(I(s, "m_skillType"))
		it.ToolTier = I(s, "m_toolTier")
		it.Backstab = F(s, "m_backstabBonus")
		it.TamedOnly = B(s, "m_tamedOnly")
		it.AttackEffect = x.Env.Name(cab, s["m_attackStatusEffect"])
		it.Attack = x.readAttack(cab, M(s, "m_attack"))
		it.Secondary = x.readAttack(cab, M(s, "m_secondaryAttack"))
		if it.Attack != nil {
			it.Projectile = it.Attack.Projectile
		}
	}
	if gt.shield {
		if typ == "Shield" {
			it.Skill = SkillName(I(s, "m_skillType"))
		}
		it.BlockPower = F(s, "m_blockPower")
		it.BlockPowerPerLevel = F(s, "m_blockPowerPerLevel")
		it.Deflection = F(s, "m_deflectionForce")
		it.DeflectionPerLevel = F(s, "m_deflectionForcePerLevel")
		it.ParryBonus = F(s, "m_timedBlockBonus")
		it.ParryAdrenaline = F(s, "m_perfectBlockAdrenaline")
	}
	if gt.armor {
		it.Armor = F(s, "m_armor")
		it.ArmorPerLevel = F(s, "m_armorPerLevel")
	}
	if gt.food {
		it.Food = F(s, "m_food")
		it.FoodStamina = F(s, "m_foodStamina")
		it.FoodEitr = F(s, "m_foodEitr")
		it.FoodBurn = F(s, "m_foodBurnTime")
		it.FoodRegen = F(s, "m_foodRegen")
		it.IsDrink = B(s, "m_isDrink")
		it.ConsumeEffect = x.Env.Name(cab, s["m_consumeStatusEffect"])
	}
	if idol {
		it.Upgrader = &model.Upgrader{
			Chance: F(s, "m_upgradeChance"),
			Break:  F(s, "m_breakChance"),
			Return: F(s, "m_breakReturnIngreientsAmount"),
			Steps:  I(s, "m_successUpgradeSteps"),
		}
	}
	return it
}

// modifiers is the equipped-item stat modifiers, the non-zero ones.
func modifiers(s map[string]any) map[string]float64 {
	fields := map[string]string{
		"movement": "m_movementModifier", "eitrRegen": "m_eitrRegenModifier",
		"jumpStamina": "m_jumpStaminaModifier", "attackStamina": "m_attackStaminaModifier",
		"blockStamina": "m_blockStaminaModifier", "dodgeStamina": "m_dodgeStaminaModifier",
		"swimStamina": "m_swimStaminaModifier", "sneakStamina": "m_sneakStaminaModifier",
		"runStamina": "m_runStaminaModifier", "homeItemsStamina": "m_homeItemsStaminaModifier",
		"heatResistance": "m_heatResistanceModifier",
	}
	out := map[string]float64{}
	for k, f := range fields {
		if v := F(s, f); v != 0 {
			out[k] = v
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// readAttack reads an Attack block. An attack without an animation is
// the unused slot (most items have no secondary attack), so it is nil.
func (x *X) readAttack(cab string, a map[string]any) *model.Attack {
	if a == nil || S(a, "m_attackAnimation") == "" {
		return nil
	}
	typ := I(a, "m_attackType")
	at := &model.Attack{
		Type:              fmt.Sprintf("Type%d", typ),
		Animation:         S(a, "m_attackAnimation"),
		Stamina:           F(a, "m_attackStamina"),
		Eitr:              F(a, "m_attackEitr"),
		Health:            F(a, "m_attackHealth"),
		HealthPct:         F(a, "m_attackHealthPercentage"),
		DrawStamina:       F(a, "m_drawStaminaDrain"),
		Range:             F(a, "m_attackRange"),
		Height:            F(a, "m_attackHeight"),
		Angle:             F(a, "m_attackAngle"),
		RayWidth:          F(a, "m_attackRayWidth"),
		DamageMultiplier:  F(a, "m_damageMultiplier"),
		ForceMultiplier:   F(a, "m_forceMultiplier"),
		StaggerMultiplier: F(a, "m_staggerMultiplier"),
		ChainLevels:       I(a, "m_attackChainLevels"),
		Harvest:           B(a, "m_harvest"),
		ConsumesItem:      B(a, "m_consumeItem"),
	}
	if typ >= 0 && typ < len(attackTypes) {
		at.Type = attackTypes[typ]
	}
	if at.ChainLevels > 0 {
		at.LastChainMult = F(a, "m_lastChainDamageMultiplier")
	}
	if p := x.GameObjectName(cab, a["m_attackProjectile"]); p != "" {
		at.Projectile = p
		at.Projectiles = I(a, "m_projectiles")
		at.Bursts = I(a, "m_projectileBursts")
		at.ProjectileVel = F(a, "m_projectileVel")
		at.Accuracy = F(a, "m_projectileAccuracy")
	}
	if B(a, "m_requiresReload") {
		at.Reload = F(a, "m_reloadTime")
	}
	return at
}
