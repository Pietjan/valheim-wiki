package site

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io/fs"
	"math"
	"sort"
	"strings"

	"github.com/pietjan/valheim-wiki/internal/model"
)

// Page is what every template gets.
type Page struct {
	Site        *Site
	Title       string
	Description string
	Section     string // nav highlight: items, crafting, building, creatures, biomes, locations, world
	Path        string // site-relative URL path of this page
	Icon        string // icon URL for og:image / header
	Crumbs      []Crumb
	// YourWorld tells the client what this page is about, so the seed
	// layer can say where it is in the reader's world.
	YourWorld *YourWorld
	Data      any
}

// Crumb is one breadcrumb.
type Crumb struct {
	Name string
	URL  string
}

// YourWorld is the seed layer's hook for a page.
type YourWorld struct {
	Kind    string   `json:"kind"`              // location, biome, creature, item, boss, trader
	Prefabs []string `json:"prefabs,omitempty"` // location prefabs to count/find
	Biomes  []string `json:"biomes,omitempty"`  // biomes to find
	Label   string   `json:"label,omitempty"`
}

// loadTemplates parses the layout and partials once, then each page
// template on a clone, so every page shares the partials but defines
// its own "content".
func (s *Site) loadTemplates() error {
	base := template.New("layout").Funcs(s.funcs())
	base, err := base.ParseFS(s.Web, "templates/layout.html", "templates/partials/*.html")
	if err != nil {
		return err
	}
	pages, err := fs.Glob(s.Web, "templates/pages/*.html")
	if err != nil {
		return err
	}
	s.pageT = map[string]*template.Template{}
	for _, p := range pages {
		name := strings.TrimSuffix(p[strings.LastIndex(p, "/")+1:], ".html")
		t, err := base.Clone()
		if err != nil {
			return err
		}
		if _, err := t.ParseFS(s.Web, p); err != nil {
			return fmt.Errorf("%s: %w", p, err)
		}
		s.pageT[name] = t
	}
	return nil
}

// funcs are the template helpers.
func (s *Site) funcs() template.FuncMap {
	return template.FuncMap{
		"url":  s.URL,
		"icon": s.Icon,
		"thing": func(prefab string) *Thing {
			if s.Index == nil {
				return nil
			}
			return s.Index.Things[prefab]
		},
		"thingURL": func(prefab string) string {
			if t := s.Index.Things[prefab]; t != nil {
				return s.URL(t.URL)
			}
			return ""
		},
		"nameOf": func(prefab string) string {
			if t := s.Index.Things[prefab]; t != nil {
				return t.Name
			}
			if it := s.DB.Items[prefab]; it != nil {
				return it.Name
			}
			return prefab
		},
		"iconOf": func(prefab string) string {
			if t := s.Index.Things[prefab]; t != nil && t.Icon != "" {
				return s.URL("icons/" + t.Icon + ".png")
			}
			return ""
		},
		"biomeName": func(b string) string {
			if n, ok := model.BiomeName[b]; ok {
				return n
			}
			return b
		},
		"biomeColor": BiomeColor,
		"biomeURL":   func(b string) string { return s.URL("biomes/" + b + "/") },
		"num":        Num,
		"num1":       func(f float64) string { return Num1(f) },
		"pct":        func(f float64) string { return Num(math.Round(f*1000)/10) + "%" },
		"secs":       Duration,
		"mins":       func(m float64) string { return Duration(m * 60) },
		"metres":     Metres,
		"join":       strings.Join,
		"lower":      strings.ToLower,
		"title":      Title,
		"add":        func(a, b int) int { return a + b },
		"sub":        func(a, b int) int { return a - b },
		"mulf":       func(a, b float64) float64 { return a * b },
		"addf":       func(a, b float64) float64 { return a + b },
		"atLevel":    func(base, step float64, q int) float64 { return base + float64(q-1)*step },
		"seq": func(n int) []int {
			out := make([]int, n)
			for i := range out {
				out[i] = i + 1
			}
			return out
		},
		"dict": func(kv ...any) map[string]any {
			m := map[string]any{}
			for i := 0; i+1 < len(kv); i += 2 {
				m[fmt.Sprint(kv[i])] = kv[i+1]
			}
			return m
		},
		"damageTypes":   func() []string { return model.DamageTypes },
		"damageName":    DamageName,
		"itemType":      ItemTypeName,
		"category":      PieceCategoryName,
		"faction":       FactionName,
		"resourceKind":  ResourceKindName,
		"converterKind": ConverterKindName,
		"modName":       ModifierName,
		"sortedKeys":    func(m map[string]float64) []string { return sortedF(m) },
		"hasPrefix":     strings.HasPrefix,
		"contains":      strings.Contains,
		"replace":       strings.ReplaceAll,
		"nl2br": func(s string) template.HTML {
			return template.HTML(strings.ReplaceAll(template.HTMLEscapeString(s), "\n", "<br>"))
		},
		"safe": func(s string) template.HTML { return template.HTML(s) },
		// summonLinks is the linked names of summoned creatures.
		"summonLinks": func(prefabs []string) []string {
			out := make([]string, 0, len(prefabs))
			for _, p := range prefabs {
				if t := s.Index.Things[p]; t != nil && t.URL != "" {
					out = append(out, `<a href="`+template.HTMLEscapeString(s.URL(t.URL))+`">`+template.HTMLEscapeString(t.Name)+`</a>`)
				} else {
					out = append(out, template.HTMLEscapeString(p))
				}
			}
			return out
		},
		"slug":  Slug,
		"stars": func(q int) string { return strings.Repeat("★", q) },
		"json": func(v any) template.JS {
			b, _ := json.Marshal(v)
			return template.JS(b)
		},
		"yes": func(b bool) string {
			if b {
				return "Yes"
			}
			return "No"
		},
		"damageHTML": func(d model.Damage) template.HTML {
			var parts []string
			for _, k := range model.DamageTypes {
				if v, ok := d[k]; ok {
					parts = append(parts, template.HTMLEscapeString(Num(v))+" <b class=\"muted\">"+strings.ToLower(DamageName(k))+"</b>")
				}
			}
			return template.HTML(strings.Join(parts, "<br>"))
		},
		"resistHTML": func(mods []model.DamageMod) template.HTML {
			var parts []string
			for _, m := range mods {
				parts = append(parts, "<span class=\"r-"+m.Modifier+"\">"+template.HTMLEscapeString(Title(m.Type)+" "+modifierWord(m.Modifier))+"</span>")
			}
			return template.HTML("<span class=\"resist\">" + strings.Join(parts, "") + "</span>")
		},
		"perLevel": func(base, step float64) template.HTML {
			if step == 0 {
				return template.HTML(template.HTMLEscapeString(Num(base)))
			}
			return template.HTML(template.HTMLEscapeString(Num(base)) + "<span class=\"per\">+" + template.HTMLEscapeString(Num(step)) + "/★</span>")
		},
		"signedPct": func(f float64) string {
			s := Num(math.Round(f*1000) / 10)
			if f > 0 {
				s = "+" + s
			}
			return s + "%"
		},
		"keyName": KeyName,
		"when": func(c any, a, b any) any {
			switch v := c.(type) {
			case bool:
				if v {
					return a
				}
			case string:
				if v != "" {
					return a
				}
			case nil:
			default:
				return a
			}
			return b
		},
		"biomeNames":     biomeNames,
		"statName":       StatName,
		"statValue":      StatValue,
		"pieceTableName": func(p string) string { return strings.TrimSuffix(strings.TrimPrefix(p, "_"), "PieceTable") },
		"modWord":        modifierWord,
		"first": func(n int, s []string) []string {
			if len(s) > n {
				return s[:n]
			}
			return s
		},
	}
}

func sortedF(m map[string]float64) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// Num formats a number the way a wiki reads it: no trailing zeros.
func Num(f float64) string {
	if f == math.Trunc(f) && math.Abs(f) < 1e12 {
		return fmt.Sprintf("%d", int64(f))
	}
	s := fmt.Sprintf("%.2f", f)
	s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	return s
}

// Num1 is one decimal at most.
func Num1(f float64) string {
	if f == math.Trunc(f) {
		return fmt.Sprintf("%d", int64(f))
	}
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.1f", f), "0"), ".")
}

// Duration writes seconds as "20 min", "1 h 30 min", "45 s".
func Duration(sec float64) string {
	if sec <= 0 {
		return "—"
	}
	if sec < 60 {
		return Num(sec) + " s"
	}
	m := int(math.Round(sec / 60))
	if m < 60 {
		if sec/60 != float64(m) {
			return Num1(sec/60) + " min"
		}
		return fmt.Sprintf("%d min", m)
	}
	h, m := m/60, m%60
	if m == 0 {
		return fmt.Sprintf("%d h", h)
	}
	return fmt.Sprintf("%d h %d min", h, m)
}

// Metres writes a distance.
func Metres(m float64) string {
	if m >= 1000 {
		return Num1(m/1000) + " km"
	}
	return fmt.Sprintf("%d m", int(math.Round(m)))
}

// Title capitalizes the first letter.
func Title(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// DamageName is the reader's word for a damage type.
func DamageName(t string) string {
	switch t {
	case "damage":
		return "True"
	}
	return Title(t)
}

// ModifierName is the reader's word for an equip modifier key.
func ModifierName(k string) string {
	switch k {
	case "movement":
		return "Movement speed"
	case "eitrRegen":
		return "Eitr regeneration"
	case "jumpStamina":
		return "Jump stamina"
	case "attackStamina":
		return "Attack stamina"
	case "blockStamina":
		return "Block stamina"
	case "dodgeStamina":
		return "Dodge stamina"
	case "swimStamina":
		return "Swim stamina"
	case "sneakStamina":
		return "Sneak stamina"
	case "runStamina":
		return "Run stamina"
	case "homeItemsStamina":
		return "Tool stamina"
	case "heatResistance":
		return "Heat resistance"
	}
	return Title(k)
}

// BiomeColor is a biome's flat colour.
func BiomeColor(b string) string {
	switch b {
	case "meadows":
		return "#7c9e4a"
	case "black-forest":
		return "#3b5233"
	case "swamp":
		return "#5e543d"
	case "mountain":
		return "#c9d3dc"
	case "plains":
		return "#baa860"
	case "mistlands":
		return "#4a4658"
	case "ashlands":
		return "#823e2f"
	case "deep-north":
		return "#9fb7c9"
	case "ocean":
		return "#2e4f7a"
	}
	return "#888"
}

// modifierWord is the reader's phrase for a damage modifier.
func modifierWord(m string) string {
	switch m {
	case "Resistant":
		return "resistant"
	case "VeryResistant":
		return "very resistant"
	case "SlightlyResistant":
		return "slightly resistant"
	case "Weak":
		return "weak"
	case "VeryWeak":
		return "very weak"
	case "SlightlyWeak":
		return "slightly weak"
	case "Immune":
		return "immune"
	case "Ignore":
		return "ignored"
	}
	return strings.ToLower(m)
}

// KeyName is the reader's phrase for a global key such as
// "defeated_eikthyr".
func KeyName(k string) string {
	switch {
	case strings.HasPrefix(k, "defeated_"):
		b := strings.TrimPrefix(k, "defeated_")
		names := map[string]string{"eikthyr": "Eikthyr", "gdking": "the Elder", "bonemass": "Bonemass", "dragon": "Moder", "goblinking": "Yagluth", "queen": "the Queen", "fader": "Fader", "frozenking": "Kall Fimbulbringer"}
		if n, ok := names[b]; ok {
			return "defeating " + n
		}
		return "defeating " + b
	case k == "KilledTroll":
		return "killing a troll"
	case k == "killed_surtling":
		return "killing a surtling"
	case k == "KilledBat":
		return "killing a bat"
	}
	return k
}

// StatName is the reader's label for an SE_Stats field.
func StatName(k string) string {
	names := map[string]string{
		"healthPerTick": "Health per tick", "healthOverTime": "Health over time", "healthOverTimeDuration": "Over",
		"staminaOverTime": "Stamina over time", "staminaDrainPerSec": "Stamina drain per second", "eitrOverTime": "Eitr over time",
		"runStaminaDrainModifier": "Run stamina", "jumpStaminaUseModifier": "Jump stamina", "attackStaminaUseModifier": "Attack stamina",
		"blockStaminaUseModifier": "Block stamina", "dodgeStaminaUseModifier": "Dodge stamina", "swimStaminaUseModifier": "Swim stamina",
		"homeItemStaminaUseModifier": "Tool stamina", "sneakStaminaUseModifier": "Sneak stamina", "runStaminaUseModifier": "Run stamina use",
		"healthRegenMultiplier": "Health regeneration", "staminaRegenMultiplier": "Stamina regeneration", "eitrRegenMultiplier": "Eitr regeneration",
		"addArmor": "Armor", "armorMultiplier": "Armor multiplier", "damageModifier": "Damage dealt", "noiseModifier": "Noise", "stealthModifier": "Stealth",
		"addMaxCarryWeight": "Carry weight", "speedModifier": "Movement speed", "swimSpeedModifier": "Swim speed", "fallDamageModifier": "Fall damage",
		"windMovementModifier": "Movement with wind", "windRunStaminaModifier": "Run stamina with wind", "raiseSkillModifier": "Skill gain",
		"skillLevelModifier": "Skill level", "absorbDamage": "Absorbs damage", "absorbDamagePerSkillLevel": "Absorb per skill level",
		"ttlPerItemLevel": "Duration per item level", "baseTTL": "Base duration", "ttlPerComfortLevel": "Per comfort level",
		"tickInterval": "Tick interval", "cooldown": "Cooldown", "damageInterval": "Damage interval", "freezeTimeEnemy": "Freezes enemies for",
		"minSpeedFactor": "Min speed", "waterDamage": "Water damage", "damage": "Damage", "moreHealth": "Max health", "moreStamina": "Max stamina",
		"moreEitr": "Max eitr", "jumpModifier": "Jump", "jumpModifierX": "Jump height", "jumpModifierY": "Jump height", "jumpModifierZ": "Jump distance",
	}
	if n, ok := names[k]; ok {
		return n
	}
	// camelCase -> words
	var b strings.Builder
	for i, r := range k {
		if i > 0 && r >= 'A' && r <= 'Z' {
			b.WriteByte(' ')
			b.WriteRune(r + 32)
		} else if i == 0 {
			b.WriteRune(r - 32*boolToRune(r >= 'a' && r <= 'z'))
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func boolToRune(b bool) rune {
	if b {
		return 1
	}
	return 0
}

// StatValue formats an SE_Stats field: modifiers as signed percentages,
// multipliers as ×, times as durations, the rest as numbers.
func StatValue(k string, v float64) string {
	switch {
	case strings.HasSuffix(k, "Modifier") || strings.HasSuffix(k, "ModifierX") || strings.HasSuffix(k, "ModifierY") || strings.HasSuffix(k, "ModifierZ"):
		s := Num(math.Round(v*1000) / 10)
		if v > 0 {
			s = "+" + s
		}
		return s + "%"
	case strings.HasSuffix(k, "Multiplier"):
		return "×" + Num(v)
	case strings.HasSuffix(k, "TTL") || strings.HasSuffix(k, "Duration") || strings.HasSuffix(k, "Interval") || k == "cooldown" || k == "freezeTimeEnemy" || strings.HasSuffix(k, "ItemLevel") || strings.HasSuffix(k, "ComfortLevel"):
		return Duration(v)
	}
	return Num(v)
}
