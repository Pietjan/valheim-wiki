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
	BiomeArea             string   `json:"biomeArea,omitempty"` // Edge, Median or Everything
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
	RandTilt              float64  `json:"randTilt,omitempty"`
	GroundTiltChance      float64  `json:"groundTiltChance,omitempty"`
	BlockCheck            bool     `json:"blockCheck,omitempty"`
	ForcePlacement        bool     `json:"forcePlacement,omitempty"`
	SnapToWater           bool     `json:"snapToWater,omitempty"`
	SnapToStaticSolid     bool     `json:"snapToStaticSolid,omitempty"`
	GroundOffset          float64  `json:"groundOffset,omitempty"`
	TerrainDeltaRadius    float64  `json:"terrainDeltaRadius,omitempty"`
	MinTerrainDelta       float64  `json:"minTerrainDelta,omitempty"`
	MaxTerrainDelta       float64  `json:"maxTerrainDelta,omitempty"`
	SurroundCheck         bool     `json:"surroundCheck,omitempty"`
	SurroundDistance      float64  `json:"surroundDistance,omitempty"`
	SurroundLayers        int      `json:"surroundLayers,omitempty"`
	SurroundBetter        float64  `json:"surroundBetter,omitempty"`
}

// Clutter is a ClutterSystem.Clutter rule (grass and small things):
// Amount instances per 8 m grass patch where the terrain allows.
type Clutter struct {
	Name          string   `json:"name"`
	Prefab        string   `json:"prefab"`
	Source        string   `json:"source,omitempty"` // the system or location list it came from
	Enabled       bool     `json:"enabled"`
	Biomes        []string `json:"biomes,omitempty"`
	Amount        int      `json:"amount,omitempty"`
	Instanced     bool     `json:"instanced,omitempty"`
	InForest      bool     `json:"inForest,omitempty"`
	ForestMin     float64  `json:"forestMin,omitempty"`
	ForestMax     float64  `json:"forestMax,omitempty"`
	MinAlt        float64  `json:"minAlt,omitempty"`
	MaxAlt        float64  `json:"maxAlt,omitempty"`
	MinTilt       float64  `json:"minTilt,omitempty"`
	MaxTilt       float64  `json:"maxTilt,omitempty"`
	MinVegetation float64  `json:"minVegetation,omitempty"`
	MaxVegetation float64  `json:"maxVegetation,omitempty"`
	MinOceanDepth float64  `json:"minOceanDepth,omitempty"`
	MaxOceanDepth float64  `json:"maxOceanDepth,omitempty"`
	ScaleMin      float64  `json:"scaleMin,omitempty"`
	ScaleMax      float64  `json:"scaleMax,omitempty"`
	OnUncleared   bool     `json:"onUncleared,omitempty"`
	OnCleared     bool     `json:"onCleared,omitempty"`
	SnapToWater   bool     `json:"snapToWater,omitempty"`
	TerrainTilt   bool     `json:"terrainTilt,omitempty"`
}
