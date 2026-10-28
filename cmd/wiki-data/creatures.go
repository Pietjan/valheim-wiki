package main

import (
	"sort"
	"strings"

	"github.com/pietjan/valheim-wiki/internal/assets"
	"github.com/pietjan/valheim-wiki/internal/model"
)

// factionNames in Character.Faction order (Character.cs). The faction
// decides who a creature fights: BaseAI.IsEnemy pairs AnimalsVeg with
// everything, the monster factions with each other and the players,
// PlayerSpawned with the players' enemies.
var factionNames = []string{"Players", "AnimalsVeg", "ForestMonsters", "Undead", "Demon", "MountainMonsters", "SeaMonsters", "PlainsMonsters", "Boss", "MistlandsMonsters", "Dverger", "PlayerSpawned", "TrainingDummy", "DeepNorth"}

// attackTypeNames in Attack.AttackType order (Attack.cs).
var attackTypeNames = []string{"Horizontal", "Vertical", "Projectile", "None", "Area", "TriggerProjectile"}

// tameSkillNames is Skills.SkillType for Tameable.m_levelUpOwnerSkill:
// the owner skill a mount trains.
var tameSkillNames = map[int]string{0: "", 1: "Swords", 2: "Knives", 3: "Clubs", 4: "Polearms", 5: "Spears", 6: "Blocking", 7: "Axes", 8: "Bows", 9: "ElementalMagic", 10: "BloodMagic", 11: "Unarmed", 12: "Pickaxes", 13: "WoodCutting", 14: "Crossbows", 100: "Jump", 101: "Sneak", 102: "Run", 103: "Swim", 104: "Fishing", 105: "Cooking", 106: "Farming", 107: "Crafting", 108: "Dodge", 110: "Ride", 999: "All"}

// extractCreatures reads every prefab root that carries a Character
// (Humanoid and Player are subclasses with the same fields) into a
// model.Creature, plus the Fish prefabs, which have no Character but are
// the bestiary's fish. Ragdolls and the Player prefab are skipped. The
// traders (Haldor, Hildir, BogWitch) and Odin's ravens have no Character
// and so are not creatures; the traders extractor has them.
//
// A prefab name may be held by several roots (the model prefab a
// character was built from often shares its name: Boar, Lox, Hildir), so
// the root is chosen by having the component, not by load order.
func extractCreatures(x *X) error {
	names := make([]string, 0, len(x.RootsAll))
	for name := range x.RootsAll {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if name == "Player" || strings.HasSuffix(strings.ToLower(name), "_ragdoll") {
			continue
		}
		g, ch, ok := characterRoot(x, x.RootsAll[name])
		if ok {
			if _, ragdoll := x.Find(g, "m_mainModel", "m_velMultiplier"); ragdoll {
				continue
			}
			x.DB.Creatures[name] = x.creature(name, g, ch)
			continue
		}
		if g, fish, ok := fishRoot(x, x.RootsAll[name]); ok {
			x.DB.Creatures[name] = x.fishCreature(name, g, fish)
		}
	}
	x.hatchedFrom()
	// The trophy knows its creature. Sorted order makes the shared ones
	// deterministic: Draugr before Draugr_Ranged, Goblin before GoblinArcher.
	for _, name := range model.SortedKeys(x.DB.Creatures) {
		c := x.DB.Creatures[name]
		if c.Trophy == "" {
			continue
		}
		if it := x.DB.Items[c.Trophy]; it != nil && it.TrophyOf == "" {
			it.TrophyOf = name
		}
	}
	return nil
}

// characterRoot is the first of the roots with a Character component.
func characterRoot(x *X, refs []assets.Ref) (assets.Ref, Component, bool) {
	for _, g := range refs {
		if c, ok := x.Find(g, "m_health", "m_faction"); ok {
			return g, c, true
		}
	}
	return assets.Ref{}, Component{}, false
}

// fishRoot is the first of the roots with a Fish component.
func fishRoot(x *X, refs []assets.Ref) (assets.Ref, Component, bool) {
	for _, g := range refs {
		if c, ok := x.Find(g, "m_baseHookChance", "m_baits"); ok {
			return g, c, true
		}
	}
	return assets.Ref{}, Component{}, false
}

// factionName names a Character.Faction value.
func factionName(v int) string {
	if v >= 0 && v < len(factionNames) {
		return factionNames[v]
	}
	return ""
}

// creature reads a Character root: the Character itself, then the
// components beside it.
func (x *X) creature(name string, g assets.Ref, ch Component) *model.Creature {
	t := ch.Tree
	cab := ch.Ref.Cab
	c := &model.Creature{
		Prefab:    name,
		Token:     S(t, "m_name"),
		Name:      x.NameOf(name, S(t, "m_name")),
		Group:     S(t, "m_group"),
		Faction:   factionName(I(t, "m_faction")),
		Boss:      B(t, "m_boss"),
		BossOrder: I(t, "m_bossOrder"),
		MiniBoss:  x.MiniBoss(S(t, "m_name")),
		DefeatKey: S(t, "m_defeatSetGlobalKey"),
		BossEvent: S(t, "m_bossEvent"),
		Health:    F(t, "m_health"),
		Resist:    DamageMods(M(t, "m_damageModifiers")),
		WalkSpeed: F(t, "m_walkSpeed"),
		RunSpeed:  F(t, "m_runSpeed"),
		SwimSpeed: F(t, "m_swimSpeed"),
		Flying:    B(t, "m_flying"),
		CanSwim:   B(t, "m_canSwim"),
		Stagger:   F(t, "m_staggerDamageFactor"),
	}
	if c.Flying {
		c.FlySpeed = F(t, "m_flyFastSpeed")
	}
	for _, tol := range []struct{ field, name string }{{"m_tolerateWater", "water"}, {"m_tolerateFire", "fire"}, {"m_tolerateSmoke", "smoke"}, {"m_tolerateTar", "tar"}} {
		if B(t, tol.field) {
			c.Tolerates = append(c.Tolerates, tol.name)
		}
	}
	if Has(t, "m_defaultItems") {
		x.humanoidGear(c, cab, t)
	}
	if d, ok := x.Find(g, "m_drops", "m_cheated"); ok {
		for _, e := range L(d.Tree, "m_drops") {
			m := assets.AsMap(e)
			item := x.GameObjectName(d.Ref.Cab, m["m_prefab"])
			if item == "" {
				continue
			}
			c.Drops = append(c.Drops, model.Drop{
				Item: item, Min: I(m, "m_amountMin"), Max: I(m, "m_amountMax"), Chance: F(m, "m_chance"),
				OnePerPlayer: B(m, "m_onePerPlayer"), LevelMultiplier: B(m, "m_levelMultiplier"),
				LevelMultiplierNotAmount: B(m, "m_levelMultiplierNotAmount"), DontScale: B(m, "m_dontScale"),
			})
			if c.Trophy == "" && strings.HasPrefix(item, "Trophy") {
				c.Trophy = item
			}
		}
	}
	if ai, ok := x.Find(g, "m_viewRange", "m_hearRange"); ok {
		c.AI = x.aiInfo(ai)
	}
	if tm, ok := x.Find(g, "m_tamingTime", "m_fedDuration"); ok {
		c.Tameable = x.tameInfo(tm)
	}
	if p, ok := x.Find(g, "m_pregnancyChance", "m_offspring"); ok {
		m := p.Tree
		c.Procreation = &model.ProcreationInfo{
			Offspring: x.GameObjectName(p.Ref.Cab, m["m_offspring"]), MaxCreatures: I(m, "m_maxCreatures"),
			PregnancyChance: F(m, "m_pregnancyChance"), PregnancyDuration: F(m, "m_pregnancyDuration"),
			RequiredLove: I(m, "m_requiredLovePoints"), PartnerRange: F(m, "m_partnerCheckRange"),
			MinOffspringLevel:  I(m, "m_minOffspringLevel"),
			Partner:            x.GameObjectName(p.Ref.Cab, m["m_seperatePartner"]),
			NoPartnerOffspring: x.GameObjectName(p.Ref.Cab, m["m_noPartnerOffspring"]),
		}
	}
	if gr, ok := x.Find(g, "m_growTime", "m_inheritTame"); ok {
		m := gr.Tree
		c.Growup = &model.GrowupInfo{GrowTime: F(m, "m_growTime"), Grown: x.GameObjectName(gr.Ref.Cab, m["m_grownPrefab"]), InheritTame: B(m, "m_inheritTame")}
		for _, e := range L(m, "m_altGrownPrefabs") {
			if n := x.GameObjectName(gr.Ref.Cab, assets.AsMap(e)["m_prefab"]); n != "" {
				c.Growup.AltGrown = append(c.Growup.AltGrown, n)
			}
		}
	}
	if f, ok := x.Find(g, "m_baseHookChance", "m_baits"); ok {
		c.Fish = x.fishInfo(f)
	}
	c.Summoned = summoned(c)
	return c
}

// summoned is whether a creature exists only by a player's doing: the
// PlayerSpawned faction (staff summons such as the Skeleton_Friendly
// variants), a Tameable that unsummons when it strays from its owner, or
// a prefab the game names as a summon (Charred_Twitcher_Summoned, which
// the Charred warlocks raise).
func summoned(c *model.Creature) bool {
	if c.Faction == "PlayerSpawned" || (c.Tameable != nil && c.Tameable.Unsummon > 0) {
		return true
	}
	return strings.Contains(strings.ToLower(c.Prefab), "summoned")
}

// humanoidGear reads what a Humanoid is handed when it spawns
// (Humanoid.GiveDefaultItems): every m_defaultItems, one of each random
// list, one of the m_randomSets, and each m_randomItems on its own roll.
// Items that deal damage become the creature's attacks.
func (x *X) humanoidGear(c *model.Creature, cab string, t map[string]any) {
	names := func(v any) []string {
		var out []string
		for _, e := range assets.AsList(v) {
			if n := x.GameObjectName(cab, e); n != "" {
				out = append(out, n)
			}
		}
		return out
	}
	c.DefaultItems = names(t["m_defaultItems"])
	c.RandomWeapons = names(t["m_randomWeapon"])
	c.RandomArmor = names(t["m_randomArmor"])
	c.RandomShields = names(t["m_randomShield"])
	for _, e := range L(t, "m_randomSets") {
		m := assets.AsMap(e)
		c.RandomSets = append(c.RandomSets, model.ItemSet{Name: S(m, "m_name"), Items: names(m["m_items"])})
	}
	chance := map[string]float64{}
	for _, e := range L(t, "m_randomItems") {
		m := assets.AsMap(e)
		if n := x.GameObjectName(cab, m["m_prefab"]); n != "" {
			ch := 1 - F(m, "m_chance")
			chance[n] = ch
			c.RandomItems = append(c.RandomItems, model.ItemChance{Item: n, Chance: ch})
		}
	}
	seen := map[string]bool{}
	var candidates []any
	candidates = append(candidates, L(t, "m_defaultItems")...)
	candidates = append(candidates, L(t, "m_randomWeapon")...)
	for _, e := range L(t, "m_randomSets") {
		candidates = append(candidates, L(assets.AsMap(e), "m_items")...)
	}
	for _, e := range L(t, "m_randomItems") {
		candidates = append(candidates, assets.AsMap(e)["m_prefab"])
	}
	for _, ptr := range candidates {
		g, ok := x.Deref(cab, ptr)
		if !ok {
			continue
		}
		item := x.Name(g)
		if item == "" || seen[item] {
			continue
		}
		seen[item] = true
		if a := x.creatureAttack(g); a != nil {
			a.Chance = chance[item]
			c.Attacks = append(c.Attacks, *a)
		}
	}
}

// creatureAttack reads an item prefab as a monster weapon: nil when it
// deals no damage and launches nothing (a helmet, a cape, a torch held
// for light).
func (x *X) creatureAttack(g assets.Ref) *model.CreatureAttack {
	it, ok := x.Find(g, "m_itemData")
	if !ok {
		return nil
	}
	sh := M(M(it.Tree, "m_itemData"), "m_shared")
	if sh == nil {
		return nil
	}
	at := M(sh, "m_attack")
	a := &model.CreatureAttack{
		Item:       x.Name(g),
		Name:       x.NameOf(x.Name(g), S(sh, "m_name")),
		Damage:     Damage(M(sh, "m_damages")),
		Knockback:  F(sh, "m_attackForce"),
		Range:      F(sh, "m_aiAttackRange"),
		Reach:      F(at, "m_attackRange"),
		Interval:   F(sh, "m_aiAttackInterval"),
		Projectile: x.GameObjectName(it.Ref.Cab, at["m_attackProjectile"]),
		Effect:     x.Env.Name(it.Ref.Cab, sh["m_attackStatusEffect"]),
		ToolTier:   I(sh, "m_toolTier"),
	}
	if v := I(at, "m_attackType"); v >= 0 && v < len(attackTypeNames) {
		a.Type = attackTypeNames[v]
	}
	if len(a.Damage) == 0 && a.Projectile == "" && a.Effect == "" {
		return nil
	}
	return a
}

// aiInfo reads BaseAI and whichever subclass sits on it: MonsterAI (has
// m_alertRange, hunts and sleeps) or AnimalAI (has m_timeToSafe, only
// flees).
func (x *X) aiInfo(c Component) *model.AIInfo {
	t := c.Tree
	ai := &model.AIInfo{
		ViewRange: F(t, "m_viewRange"), ViewAngle: F(t, "m_viewAngle"), HearRange: F(t, "m_hearRange"),
		FleeRange: F(t, "m_fleeRange"), AvoidFire: B(t, "m_avoidFire"), AfraidOfFire: B(t, "m_afraidOfFire"),
		AvoidWater: B(t, "m_avoidWater"), Aggravatable: B(t, "m_aggravatable"), PassiveAggressive: B(t, "m_passiveAggresive"),
		MistVision:   B(t, "m_mistVision"),
		SpawnMessage: x.Text(S(t, "m_spawnMessage")), DeathMessage: x.Text(S(t, "m_deathMessage")), AlertedMessage: x.Text(S(t, "m_alertedMessage")),
	}
	switch {
	case Has(t, "m_alertRange"):
		ai.Kind = "MonsterAI"
		ai.AlertRange = F(t, "m_alertRange")
		ai.FleeLowHealth = F(t, "m_fleeIfLowHealth")
		ai.HuntPlayer = B(t, "m_enableHuntPlayer")
		ai.AttackPlayerObjects = B(t, "m_attackPlayerObjects")
		ai.Sleeping = B(t, "m_sleeping")
		ai.WakeupRange = F(t, "m_wakeupRange")
		ai.AvoidLand = B(t, "m_avoidLand")
		ai.MaxChase = F(t, "m_maxChaseDistance")
		for _, e := range L(t, "m_consumeItems") {
			if n := x.GameObjectName(c.Ref.Cab, e); n != "" {
				ai.ConsumeItems = append(ai.ConsumeItems, n)
			}
		}
	case Has(t, "m_timeToSafe"):
		ai.Kind = "AnimalAI"
	default:
		ai.Kind = "BaseAI"
	}
	return ai
}

// tameInfo reads a Tameable.
func (x *X) tameInfo(c Component) *model.TameInfo {
	t := c.Tree
	tm := &model.TameInfo{
		TamingTime: F(t, "m_tamingTime"), FedDuration: F(t, "m_fedDuration"), Commandable: B(t, "m_commandable"),
		Saddle: x.GameObjectName(c.Ref.Cab, t["m_saddleItem"]), StartsTamed: B(t, "m_startsTamed"),
		Skill: tameSkillNames[I(t, "m_levelUpOwnerSkill")], Unsummon: F(t, "m_unsummonDistance"),
	}
	tm.LevelUpSkill = tm.Skill != ""
	for _, n := range Strs(t["m_randomStartingName"]) {
		tm.Names = append(tm.Names, x.Text(n))
	}
	return tm
}

// fishInfo reads a Fish: where it swims, what hooks it, what it carries.
func (x *X) fishInfo(c Component) *model.FishInfo {
	t := c.Tree
	f := &model.FishInfo{
		MinDepth: F(t, "m_minDepth"), MaxDepth: F(t, "m_maxDepth"), BaseHookChance: F(t, "m_baseHookChance"),
		PickupItem: x.GameObjectName(c.Ref.Cab, t["m_pickupItem"]), ExtraDrops: x.DropTable(c.Ref.Cab, M(t, "m_extraDrops")),
		EscapePerLevel: F(t, "m_escapeMaxPerLevel"),
	}
	for _, e := range L(t, "m_baits") {
		m := assets.AsMap(e)
		if n := x.GameObjectName(c.Ref.Cab, m["m_bait"]); n != "" {
			f.Baits = append(f.Baits, model.BaitInfo{Bait: n, Chance: F(m, "m_chance")})
		}
	}
	return f
}

// fishCreature is a Fish prefab without a Character: the swimming fish
// is also the item it is picked up as, so it has no health of its own.
func (x *X) fishCreature(name string, g assets.Ref, fish Component) *model.Creature {
	token := S(fish.Tree, "m_name")
	return &model.Creature{Prefab: name, Token: token, Name: x.NameOf(name, token), Fish: x.fishInfo(fish)}
}

// hatchedFrom finds every EggGrow item and records it on the creature it
// hatches into: the egg's grow time and what it needs (a fire nearby,
// a roof overhead).
func (x *X) hatchedFrom() {
	x.EachMono([]string{"m_growTime", "m_requireNearbyFire"}, func(cab string, id int64, t map[string]any) {
		egg, _ := x.Owner(cab, t)
		grown := x.GameObjectName(cab, t["m_grownPrefab"])
		c := x.DB.Creatures[grown]
		if c == nil || c.Egg != nil {
			return
		}
		c.Egg = &model.GrowupInfo{GrowTime: F(t, "m_growTime"), Grown: grown, From: egg, NeedsFire: B(t, "m_requireNearbyFire"), NeedsRoof: B(t, "m_requireUnderRoof")}
	})
}
