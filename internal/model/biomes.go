package model

// Biome is one biome's page data.
type Biome struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	Bit          int               `json:"bit"`
	Color        string            `json:"color"`    // the map's flat colour, #rrggbb
	MapColor     string            `json:"mapColor"` // the game's minimap palette entry, #rrggbb
	Environments []EnvEntry        `json:"environments,omitempty"`
	Music        map[string]string `json:"music,omitempty"`
	Boss         string            `json:"boss,omitempty"`
	BossLocation string            `json:"bossLocation,omitempty"`
	Description  string            `json:"desc,omitempty"`
}

// EnvEntry is one weighted environment of a biome.
type EnvEntry struct {
	Env    string  `json:"env"`
	Weight float64 `json:"weight"`
}

// Env is an EnvSetup: weather.
type Env struct {
	Name            string   `json:"name"`
	Default         bool     `json:"default,omitempty"`
	Wet             bool     `json:"wet,omitempty"`
	Freezing        bool     `json:"freezing,omitempty"`
	FreezingAtNight bool     `json:"freezingAtNight,omitempty"`
	Cold            bool     `json:"cold,omitempty"`
	ColdAtNight     bool     `json:"coldAtNight,omitempty"`
	AlwaysDark      bool     `json:"alwaysDark,omitempty"`
	SnowBuildup     bool     `json:"snowBuildup,omitempty"`
	WindMin         float64  `json:"windMin,omitempty"`
	WindMax         float64  `json:"windMax,omitempty"`
	RainCloudAlpha  float64  `json:"rainCloudAlpha,omitempty"`
	Aurora          bool     `json:"aurora,omitempty"`
	Particles       []string `json:"particles,omitempty"`
}

// AltBiome is an AltBiome: a themed patch rolled onto a biome sector.
type AltBiome struct {
	Name                  string   `json:"name"`
	Enabled               bool     `json:"enabled"`
	Biome                 string   `json:"biome"`
	Label                 string   `json:"label,omitempty"` // prefix/suffix/override shown on the map
	Chance                float64  `json:"chance"`
	MinAmount             int      `json:"minAmount"`
	MaxAmount             int      `json:"maxAmount"`
	MinDistanceFromCenter float64  `json:"minDistanceFromCenter,omitempty"`
	RequireNeighbor       []string `json:"requireNeighbor,omitempty"`
	NotNeighbor           []string `json:"notNeighbor,omitempty"`
	Incompatible          []string `json:"incompatible,omitempty"`
	MinEdge               int      `json:"minEdge,omitempty"`
	MaxEdge               int      `json:"maxEdge,omitempty"`
	MinAvgHeight          float64  `json:"minAvgHeight,omitempty"`
	MaxAvgHeight          float64  `json:"maxAvgHeight,omitempty"`
	LevelUpMultiplier     float64  `json:"levelUpMultiplier,omitempty"`
	ForceEnv              string   `json:"forceEnvironment,omitempty"`
	AddEnvs               []string `json:"addEnvironments,omitempty"`
	Spawns                []string `json:"spawns,omitempty"` // Spawn IDs
	BlockSpawns           []string `json:"blockSpawns,omitempty"`
	AddVegetation         []string `json:"addVegetation,omitempty"`
	BlockVegetation       []string `json:"blockVegetation,omitempty"`
	AddLocations          []string `json:"addLocations,omitempty"`
	BlockLocations        []string `json:"blockLocations,omitempty"`
}
