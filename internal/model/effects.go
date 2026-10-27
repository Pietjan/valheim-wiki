package model

// Effect is a StatusEffect asset.
type Effect struct {
	ID           string             `json:"id"` // asset name: GP_Eikthyr, Rested, Wet, SetEffect_TrollArmor
	Name         string             `json:"name"`
	Token        string             `json:"token,omitempty"`
	Kind         string             `json:"kind"` // SE_Stats, SE_Shield, SE_Rested, ...
	Category     string             `json:"category,omitempty"`
	Tooltip      string             `json:"tooltip,omitempty"`
	Icon         bool               `json:"icon,omitempty"`
	StartMessage string             `json:"startMessage,omitempty"`
	StopMessage  string             `json:"stopMessage,omitempty"`
	TTL          float64            `json:"ttl,omitempty"`
	Cooldown     float64            `json:"cooldown,omitempty"`
	Attributes   []string           `json:"attributes,omitempty"` // ColdResistance, DoubleImpactDamage, SailingPower, TamingBoost
	Stats        map[string]float64 `json:"stats,omitempty"`      // every numeric field that is not at its default
	Mods         []DamageMod        `json:"mods,omitempty"`
	GuardianOf   string             `json:"guardianOf,omitempty"` // boss prefab for GP_* powers
}

// Skill is one Skills.SkillDef.
type Skill struct {
	Name        string  `json:"name"`
	ID          string  `json:"id"` // SkillType name
	Description string  `json:"desc,omitempty"`
	Icon        bool    `json:"icon,omitempty"`
	Step        float64 `json:"step,omitempty"`
}
