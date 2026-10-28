package main

import (
	"fmt"
	"math"
	"sort"

	"github.com/pietjan/valheim-wiki/internal/assets"
	"github.com/pietjan/valheim-wiki/internal/model"
)

// biomeTable is what the bundles do not say about a biome: the wiki's
// flat map colour (the seed viewer's palette), which Minimap field holds
// the game's own palette entry, and which boss and altar belong to it
// (the progression the game only implies through global keys).
var biomeTable = map[string]struct {
	color, minimapField, boss, bossLocation string
}{
	"meadows":      {"#7c9e4a", "m_meadowsColor", "Eikthyr", "Eikthyrnir"},
	"black-forest": {"#3b5233", "m_blackforestColor", "gd_king", "GDKing"},
	"swamp":        {"#5e543d", "m_swampColor", "Bonemass", "Bonemass"},
	"mountain":     {"#e2e7ec", "m_mountainColor", "Dragon", "Dragonqueen"},
	"plains":       {"#baa860", "m_heathColor", "GoblinKing", "GoblinKing"},
	"mistlands":    {"#4a4658", "", "SeekerQueen", "Mistlands_DvergrBossEntrance1"},
	"ashlands":     {"#823e2f", "m_ashlandsColor", "Fader", "FaderLocation"},
	"deep-north":   {"#c6d6e2", "m_deepnorthColor", "FrozenKing", "DN_Bossroom"},
	"ocean":        {"#2e4f7a", "", "", ""},
}

// minimapCodeColors are the palette entries Minimap.GetPixelColor hard
// codes rather than serialises (Minimap.cs: m_mistlandsColor is private,
// Ocean falls through to Color.white).
var minimapCodeColors = map[string]string{"mistlands": "#333333", "ocean": "#ffffff"}

// extractBiomes writes one Biome per slug and every weather (EnvSetup).
// The biome's weather table comes from EnvMan.m_biomes on _GameMain plus
// the LocationList.m_biomeEnvironments the expansions add: a weighted
// list the game rolls every environment period, and the music per time
// of day. The weathers themselves come from EnvMan.m_environments and
// LocationList.m_environments.
func extractBiomes(x *X) error {
	byID := map[string]*model.Biome{}
	for _, slug := range model.Biomes {
		row := biomeTable[slug]
		b := &model.Biome{ID: slug, Name: model.BiomeName[slug], Bit: model.BiomeBit[slug], Color: row.color, MapColor: minimapCodeColors[slug]}
		if row.boss != "" {
			if _, ok := x.RootsAll[row.boss]; ok {
				b.Boss = row.boss
			}
			if _, ok := x.RootsAll[row.bossLocation]; ok {
				b.BossLocation = row.bossLocation
			}
		}
		byID[slug] = b
		x.DB.Biomes = append(x.DB.Biomes, b)
	}
	// The game's minimap palette, off the first Minimap component (the
	// HUD prefab holds one, the map screens duplicate it).
	found := false
	x.EachMono([]string{"m_meadowsColor", "m_heathColor"}, func(cab string, id int64, t map[string]any) {
		if found {
			return
		}
		found = true
		for slug, row := range biomeTable {
			if row.minimapField != "" {
				byID[slug].MapColor = hexColor(M(t, row.minimapField))
			}
		}
	})
	envSetups := func(cab string, v any, source string) {
		for _, e := range assets.AsList(v) {
			env := x.envSetup(cab, assets.AsMap(e), source)
			if env.Name != "" {
				if _, dup := x.DB.Environments[env.Name]; !dup {
					x.DB.Environments[env.Name] = env
				}
			}
		}
	}
	biomeEnvs := func(v any) {
		for _, e := range assets.AsList(v) {
			m := assets.AsMap(e)
			for _, slug := range Biomes(I(m, "m_biome")) {
				b := byID[slug]
				for _, ee := range L(m, "m_environments") {
					em := assets.AsMap(ee)
					b.Environments = append(b.Environments, model.EnvEntry{Env: S(em, "m_environment"), Weight: F(em, "m_weight")})
				}
				if music := musicOf(m); len(music) > 0 && b.Music == nil {
					b.Music = music
				}
			}
		}
	}
	// The bundles hold the EnvMan twice with the same tables: on _GameMain,
	// which the game runs, and on a standalone _Environment prefab. Read
	// the _GameMain one only, or the biome weights would double up.
	type envMan struct {
		cab  string
		root string
		tree map[string]any
	}
	var envMans []envMan
	x.EachMono([]string{"m_dayLengthSec", "m_environments", "m_biomes"}, func(cab string, id int64, t map[string]any) {
		root, _ := x.Owner(cab, t)
		envMans = append(envMans, envMan{cab, root, t})
	})
	sort.SliceStable(envMans, func(i, j int) bool { return envMans[i].root == "_GameMain" && envMans[j].root != "_GameMain" })
	if len(envMans) > 0 {
		em := envMans[0]
		envSetups(em.cab, em.tree["m_environments"], em.root)
		biomeEnvs(em.tree["m_biomes"])
	}
	seen := map[string]bool{}
	x.EachMono([]string{"m_locations", "m_environments", "m_biomeEnvironments"}, func(cab string, id int64, t map[string]any) {
		root, _ := x.Owner(cab, t)
		if seen[root] {
			return
		}
		seen[root] = true
		envSetups(cab, t["m_environments"], root)
		biomeEnvs(t["m_biomeEnvironments"])
	})
	for _, b := range x.DB.Biomes {
		sort.SliceStable(b.Environments, func(i, j int) bool { return b.Environments[i].Weight > b.Environments[j].Weight })
	}
	return nil
}

// envSetup reads one EnvSetup: the flags that matter to a player (wet,
// cold, freezing, dark), the wind, and what it looks like.
func (x *X) envSetup(cab string, m map[string]any, source string) *model.Env {
	env := &model.Env{
		Name:            S(m, "m_name"),
		Default:         B(m, "m_default"),
		Wet:             B(m, "m_isWet"),
		Freezing:        B(m, "m_isFreezing"),
		FreezingAtNight: B(m, "m_isFreezingAtNight"),
		Cold:            B(m, "m_isCold"),
		ColdAtNight:     B(m, "m_isColdAtNight"),
		AlwaysDark:      B(m, "m_alwaysDark"),
		SnowBuildup:     F(m, "m_snowBuildup") > 0,
		WindMin:         F(m, "m_windMin"),
		WindMax:         F(m, "m_windMax"),
		RainCloudAlpha:  F(m, "m_rainCloudAlpha"),
		LightDay:        F(m, "m_lightIntensityDay"),
		LightNight:      F(m, "m_lightIntensityNight"),
		SunAngle:        F(m, "m_sunAngle"),
		AmbientList:     S(m, "m_ambientList"),
		Music:           musicOf(m),
		Source:          source,
	}
	for _, k := range []string{"m_auroraIntensityNight", "m_auroraIntensityMorning", "m_auroraIntensityDay", "m_auroraIntensityEvening"} {
		if F(m, k) > 0 {
			env.Aurora = true
		}
	}
	for _, p := range L(m, "m_psystems") {
		if n := x.GameObjectName(cab, p); n != "" {
			env.Particles = append(env.Particles, n)
		}
	}
	return env
}

// musicOf reads the four m_music* fields shared by EnvSetup and
// BiomeEnvSetup, keeping the set ones.
func musicOf(m map[string]any) map[string]string {
	out := map[string]string{}
	for _, k := range []string{"Morning", "Day", "Evening", "Night"} {
		if v := S(m, "m_music"+k); v != "" {
			out[lowerFirst(k)] = v
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func lowerFirst(s string) string { return string(s[0]+'a'-'A') + s[1:] }

// hexColor turns a Unity Color {r,g,b,a} in 0..1 (HDR values clamped)
// into #rrggbb.
func hexColor(m map[string]any) string {
	if m == nil {
		return ""
	}
	ch := func(k string) int {
		return int(math.Round(math.Max(0, math.Min(1, F(m, k))) * 255))
	}
	return fmt.Sprintf("#%02x%02x%02x", ch("r"), ch("g"), ch("b"))
}
