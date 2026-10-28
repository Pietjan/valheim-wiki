package main

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/pietjan/valheim-wiki/internal/assets"
	"github.com/pietjan/valheim-wiki/internal/model"
)

// guardianPowers maps each forsaken power (the GP_* StatusEffect assets)
// to the boss whose trophy grants it at the sacrificial stones. A GP_*
// asset missing here is reported so the table can be extended.
var guardianPowers = map[string]string{
	"GP_Eikthyr":  "Eikthyr",
	"GP_TheElder": "gd_king",
	"GP_Bonemass": "Bonemass",
	"GP_Moder":    "Dragon",
	"GP_Yagluth":  "GoblinKing",
	"GP_Queen":    "SeekerQueen",
	"GP_Fader":    "Fader",
}

// effectAttributes names StatusEffect.StatusAttribute's bits.
var effectAttributes = []string{"ColdResistance", "DoubleImpactDamage", "SailingPower", "TamingBoost"}

// effectKinds tells the StatusEffect subclasses apart by a field only
// that subclass serialises (MonoScripts are not loaded). Order matters
// where one subclass extends another: SE_Smoke and SE_Wet both have
// m_damageInterval, so they come before SE_Burning.
var effectKinds = []struct {
	kind   string
	fields []string
}{
	{"SE_Shield", []string{"m_absorbDamage"}},
	{"SE_Rested", []string{"m_TTLPerComfortLevel"}},
	{"SE_Frost", []string{"m_freezeTimeEnemy"}},
	{"SE_Wet", []string{"m_waterDamage"}},
	{"SE_Poison", []string{"m_TTLPerDamagePlayer"}},
	{"SE_Smoke", []string{"m_damageInterval", "m_damage"}},
	{"SE_Burning", []string{"m_damageInterval"}},
	{"SE_Harpooned", []string{"m_pullForce"}},
	{"SE_Puke", []string{"m_removeInterval"}},
	{"SE_Demister", []string{"m_ballPrefab"}},
	{"SE_Finder", []string{"m_pingEffectNear"}},
	{"SE_HealthUpgrade", []string{"m_moreHealth"}},
	{"SE_React", []string{"m_spawnObj"}},
	{"SE_Spawn", []string{"m_prefab", "m_delay"}},
	{"SE_Cozy", []string{"m_statusEffect", "m_delay"}},
	{"SE_Stats", []string{"m_healthPerTick"}},
}

// effectSkip is every serialised field that is not a stat: the base
// StatusEffect's presentation and timing (kept in their own fields), the
// bools, the enums paired with a modifier below, and references.
var effectSkip = map[string]bool{
	"m_Name": true, "m_name": true, "m_category": true, "m_icon": true, "m_flashIcon": true,
	"m_cooldownIcon": true, "m_hidden": true, "m_hitVariant": true, "m_tooltip": true,
	"m_attributes": true, "m_startMessageType": true, "m_startMessage": true,
	"m_stopMessageType": true, "m_stopMessage": true, "m_repeatMessageType": true,
	"m_repeatMessage": true, "m_repeatInterval": true, "m_ttl": true,
	"m_effectsOnlyOnPlayer": true, "m_startEffects": true, "m_stopEffects": true,
	"m_cooldown": true, "m_activationAnimation": true, "m_isNew": true, "m_character": true,
	"m_GameObject": true, "m_Enabled": true, "m_Script": true,
	// SE_Stats
	"m_hitType": true, "m_tickEffect": true, "m_staminaOverTimeIsFraction": true,
	"m_mods": true, "m_raiseSkill": true, "m_raiseSkillModifier": true,
	"m_skillLevel": true, "m_skillLevelModifier": true, "m_skillLevel2": true,
	"m_skillLevelModifier2": true, "m_modifyAttackSkill": true, "m_damageModifier": true,
	"m_pheromoneTarget": true, "m_pheromoneFlee": true,
	// SE_Shield
	"m_levelUpSkillOnBreak": true, "m_levelUpSkillFactor": true, "m_breakEffects": true, "m_hitEffects": true,
	// others
	"m_frostSlowMultipliers": true, "m_spawnEffect": true, "m_spawnEffects": true, "m_spawnOffset": true,
	"m_prefab": true, "m_spawnObj": true, "m_ballPrefab": true, "m_statusEffect": true,
	"m_pingEffectNear": true, "m_pingEffectMed": true, "m_pingEffectFar": true, "m_upgradeEffect": true,
	"m_offset": true, "m_offsetInterior": true,
	// SE_Demister: how the wisp ball flies, not what the effect does (the
	// demisting radius is on the ball prefab).
	"m_ballAcceleration": true, "m_ballMaxSpeed": true, "m_ballFriction": true, "m_noiseDistance": true,
	"m_noiseDistanceInterior": true, "m_noiseDistanceYScale": true, "m_noiseSpeed": true,
	"m_characterVelocityFactor": true, "m_rotationSpeed": true,
}

// effectDefaults are the numeric fields whose default is not zero and
// means "no effect": the multipliers (SE_Stats.cs), the health-over-time
// interval that only matters with a health-over-time amount, and the
// pheromone level-up multiplier. Every other field counts when non-zero;
// SE_Shield's 100 absorb and SE_Rested's 300 base TTL are the stat
// itself even when left at the class default.
var effectDefaults = map[string]float64{
	"m_healthRegenMultiplier":      1,
	"m_staminaRegenMultiplier":     1,
	"m_eitrRegenMultiplier":        1,
	"m_healthOverTimeInterval":     5,
	"m_pheromoneLevelUpMultiplier": 1,
}

// skillPairs are the (skill enum, modifier) field pairs on SE_Stats and
// SE_Shield, written as one stat keyed by the skill: raiseSkill.All 0.5
// is Rested's +50% skill gain, skillLevel.Sneak 15 the troll set's bonus.
var skillPairs = [][3]string{
	{"m_raiseSkill", "m_raiseSkillModifier", "raiseSkill"},
	{"m_skillLevel", "m_skillLevelModifier", "skillLevel"},
	{"m_skillLevel2", "m_skillLevelModifier2", "skillLevel"},
	{"m_modifyAttackSkill", "m_damageModifier", "damageModifier"},
	{"m_levelUpSkillOnBreak", "m_levelUpSkillFactor", "levelUpSkillOnBreak"},
}

// extractEffects reads every StatusEffect asset: the ScriptableObject
// MonoBehaviours (no GameObject) with a name, a tooltip and a time to
// live. Stats are every numeric field the asset sets away from its
// default, keyed by the field name without m_ (nested DamageTypes and
// Vector3 fields as percentDamage.fire, jumpModifier.y).
func extractEffects(x *X) error {
	x.EachMono([]string{"m_ttl", "m_tooltip", "m_name"}, func(cab string, id int64, t map[string]any) {
		name := S(t, "m_Name")
		if name == "" {
			return
		}
		if _, onObject := x.Deref(cab, t["m_GameObject"]); onObject {
			return // a component on a prefab, not an asset
		}
		if _, seen := x.DB.Effects[name]; seen {
			return
		}
		e := &model.Effect{
			ID:           name,
			Name:         x.NameOf(name, S(t, "m_name")),
			Token:        S(t, "m_name"),
			Kind:         effectKind(t),
			Category:     S(t, "m_category"),
			Tooltip:      x.Text(S(t, "m_tooltip")),
			StartMessage: x.Text(S(t, "m_startMessage")),
			StopMessage:  x.Text(S(t, "m_stopMessage")),
			TTL:          F(t, "m_ttl"),
			Cooldown:     F(t, "m_cooldown"),
			Mods:         DamageModList(t["m_mods"]),
			Stats:        effectStats(t),
		}
		for i, a := range effectAttributes {
			if I(t, "m_attributes")&(1<<i) != 0 {
				e.Attributes = append(e.Attributes, a)
			}
		}
		if ref, ok := x.Deref(cab, t["m_icon"]); ok {
			e.Icon = true
			x.WantSprite("effects", name, ref)
		}
		if boss, ok := guardianPowers[name]; ok {
			if _, exists := x.Roots[boss]; exists {
				e.GuardianOf = boss
			} else {
				fmt.Fprintf(os.Stderr, "    effects: %s: boss prefab %s not loaded\n", name, boss)
			}
		} else if strings.HasPrefix(name, "GP_") {
			fmt.Fprintf(os.Stderr, "    effects: %s: forsaken power with no boss in guardianPowers\n", name)
		}
		x.DB.Effects[name] = e
	})
	return nil
}

// effectKind is the StatusEffect subclass, by its fields.
func effectKind(t map[string]any) string {
	for _, k := range effectKinds {
		if Has(t, k.fields...) {
			return k.kind
		}
	}
	return "StatusEffect"
}

// effectStats collects the numeric fields set away from their defaults.
func effectStats(t map[string]any) map[string]float64 {
	out := map[string]float64{}
	keys := make([]string, 0, len(t))
	for k := range t {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if effectSkip[k] || !strings.HasPrefix(k, "m_") {
			continue
		}
		name := strings.TrimPrefix(k, "m_")
		switch v := t[k].(type) {
		case map[string]any:
			if _, isPtr := v["m_PathID"]; isPtr {
				continue
			}
			if _, isVec := v["x"]; isVec {
				for _, axis := range []string{"x", "y", "z"} {
					if f := F(v, axis); f != 0 {
						out[name+"."+axis] = f
					}
				}
				continue
			}
			// HitData.DamageTypes: percentigeDamageModifiers, damage, damagePerLevel.
			for dt, f := range Damage(v) {
				out[name+"."+dt] = f
			}
		case []any, string, bool:
			continue
		default:
			f := assets.AsFloat(v)
			if f == 0 {
				f = float64(assets.AsInt64(v))
			}
			if f != effectDefaults[k] {
				out[name] = f
			}
		}
	}
	if F(t, "m_healthOverTime") == 0 {
		delete(out, "healthOverTimeInterval")
	}
	// A pair does nothing with the skill left at None (SE_Stats.ModifySkillLevel
	// and ModifyRaiseSkill, SE_Shield.OnDamaged all test for None first) or
	// with a zero modifier; a damage modifier of 1 is the default no-op.
	for _, p := range skillPairs {
		skill, mod := I(t, p[0]), F(t, p[1])
		if skill == 0 || mod == 0 {
			continue
		}
		if p[0] == "m_modifyAttackSkill" && mod == 1 {
			continue
		}
		if _, ok := t[p[1]]; ok {
			out[p[2]+"."+SkillName(skill)] = mod
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
