package model

// Resource is something in the world that gives items: a pickable, a
// bush, a tree, an ore vein, a rock, a destructible.
type Resource struct {
	Prefab string `json:"prefab"`
	Name   string `json:"name"`
	Token  string `json:"token,omitempty"`
	Kind   string `json:"kind"` // pickable, tree, log, rock, destructible, plant
	Icon   bool   `json:"icon,omitempty"`

	// Pickable.
	Item           string     `json:"item,omitempty"`
	Amount         int        `json:"amount,omitempty"`
	Respawn        float64    `json:"respawnMinutes,omitempty"` // 0 = never
	HideWhenPicked bool       `json:"hideWhenPicked,omitempty"`
	ExtraDrops     *DropTable `json:"extraDrops,omitempty"`

	// Destructibles.
	Health      float64     `json:"health,omitempty"`
	MinToolTier int         `json:"minToolTier,omitempty"`
	Resist      []DamageMod `json:"resist,omitempty"`
	Drops       *DropTable  `json:"drops,omitempty"`
	// Trees: felling chain.
	Stub               string `json:"stub,omitempty"`
	Log                string `json:"log,omitempty"`
	SubLog             string `json:"subLog,omitempty"`
	SubLogs            int    `json:"subLogs,omitempty"`
	SpawnWhenDestroyed string `json:"spawnWhenDestroyed,omitempty"`

	// Plant (growing in the world rather than planted).
	Plant *PlantInfo `json:"plant,omitempty"`

	// Where it grows: filled from the vegetation rules and locations.
	Biomes []string `json:"biomes,omitempty"`
}

// VegRule is one ZoneSystem.ZoneVegetation rule.
type VegRule struct {
	Name                  string   `json:"name"`
	Prefab                string   `json:"prefab"`
	Source                string   `json:"source,omitempty"` // list or altbiome
	Enabled               bool     `json:"enabled"`
	Biomes                []string `json:"biomes,omitempty"`
	BiomeArea             string   `json:"biomeArea,omitempty"`
	Min                   float64  `json:"min"`
	Max                   float64  `json:"max"`
	GroupMin              int      `json:"groupMin,omitempty"`
	GroupMax              int      `json:"groupMax,omitempty"`
	GroupRadius           float64  `json:"groupRadius,omitempty"`
	MinAltitude           float64  `json:"minAltitude,omitempty"`
	MaxAltitude           float64  `json:"maxAltitude,omitempty"`
	MinVegetation         float64  `json:"minVegetation,omitempty"`
	MaxVegetation         float64  `json:"maxVegetation,omitempty"`
	MinOceanDepth         float64  `json:"minOceanDepth,omitempty"`
	MaxOceanDepth         float64  `json:"maxOceanDepth,omitempty"`
	MinTilt               float64  `json:"minTilt,omitempty"`
	MaxTilt               float64  `json:"maxTilt,omitempty"`
	InForest              bool     `json:"inForest,omitempty"`
	ForestMin             float64  `json:"forestMin,omitempty"`
	ForestMax             float64  `json:"forestMax,omitempty"`
	MinDistanceFromCenter float64  `json:"minDistanceFromCenter,omitempty"`
	MaxDistanceFromCenter float64  `json:"maxDistanceFromCenter,omitempty"`
	ScaleMin              float64  `json:"scaleMin,omitempty"`
	ScaleMax              float64  `json:"scaleMax,omitempty"`
	BlockCheck            bool     `json:"blockCheck,omitempty"`
	SnapToWater           bool     `json:"snapToWater,omitempty"`
}

// Clutter is a ClutterSystem.Clutter rule (grass and small things).
type Clutter struct {
	Name        string   `json:"name"`
	Prefab      string   `json:"prefab"`
	Enabled     bool     `json:"enabled"`
	Biomes      []string `json:"biomes,omitempty"`
	Amount      int      `json:"amount,omitempty"`
	InForest    bool     `json:"inForest,omitempty"`
	MinAlt      float64  `json:"minAlt,omitempty"`
	MaxAlt      float64  `json:"maxAlt,omitempty"`
	MaxTilt     float64  `json:"maxTilt,omitempty"`
	OnUncleared bool     `json:"onUncleared,omitempty"`
}
