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
	Name            string            `json:"name"`
	Default         bool              `json:"default,omitempty"`
	Wet             bool              `json:"wet,omitempty"`
	Freezing        bool              `json:"freezing,omitempty"`
	FreezingAtNight bool              `json:"freezingAtNight,omitempty"`
	Cold            bool              `json:"cold,omitempty"`
	ColdAtNight     bool              `json:"coldAtNight,omitempty"`
	AlwaysDark      bool              `json:"alwaysDark,omitempty"`
	SnowBuildup     bool              `json:"snowBuildup,omitempty"`
	WindMin         float64           `json:"windMin,omitempty"`
	WindMax         float64           `json:"windMax,omitempty"`
	RainCloudAlpha  float64           `json:"rainCloudAlpha,omitempty"`
	Aurora          bool              `json:"aurora,omitempty"`
	Particles       []string          `json:"particles,omitempty"`
	LightDay        float64           `json:"lightIntensityDay,omitempty"`
	LightNight      float64           `json:"lightIntensityNight,omitempty"`
	SunAngle        float64           `json:"sunAngle,omitempty"`
	AmbientList     string            `json:"ambientList,omitempty"`
	Music           map[string]string `json:"music,omitempty"`  // morning, day, evening, night
	Source          string            `json:"source,omitempty"` // _GameMain or the _LocationList_ that defines it
}

// WorldConstants are the EnvMan and RandEventSystem numbers that are not
// per biome: how long a day is, how often weather and raids roll. Read
// off _GameMain by the biomes and events extractors; the DB has no field
// for them yet (model.go is not this package's to extend from the
// extractor side), so they are reported, not written.
type WorldConstants struct {
	DayLengthSec        float64 `json:"dayLengthSec"`        // EnvMan.m_dayLengthSec: one in-game day in real seconds
	EnvironmentDuration float64 `json:"environmentDuration"` // EnvMan.m_environmentDuration: minutes a weather lasts
	WindPeriodDuration  float64 `json:"windPeriodDuration"`  // EnvMan.m_windPeriodDuration
	EdgeOfWorldWidth    float64 `json:"edgeOfWorldWidth"`    // EnvMan.m_edgeOfWorldWidth
	IntroEnvironment    string  `json:"introEnvironment"`    // EnvMan.m_introEnvironment
	EventIntervalMin    float64 `json:"eventIntervalMin"`    // RandEventSystem.m_eventIntervalMin: minutes between raid rolls
	EventChance         float64 `json:"eventChance"`         // RandEventSystem.m_eventChance: percent per roll
}

// AltBiome is an AltBiome: a themed patch rolled onto a biome sector.
type AltBiome struct {
	Name                  string     `json:"name"`
	Enabled               bool       `json:"enabled"`
	Biome                 string     `json:"biome"`
	Label                 string     `json:"label,omitempty"` // prefix/suffix/override shown on the map
	Chance                float64    `json:"chance"`
	MinAmount             int        `json:"minAmount"`
	MaxAmount             int        `json:"maxAmount"`
	MinDistanceFromCenter float64    `json:"minDistanceFromCenter,omitempty"`
	RequireNeighbor       []string   `json:"requireNeighbor,omitempty"`
	NotNeighbor           []string   `json:"notNeighbor,omitempty"`
	Incompatible          []string   `json:"incompatible,omitempty"`
	MinEdge               int        `json:"minEdge,omitempty"`
	MaxEdge               int        `json:"maxEdge,omitempty"`
	MinAvgHeight          float64    `json:"minAvgHeight,omitempty"`
	MaxAvgHeight          float64    `json:"maxAvgHeight,omitempty"`
	LevelUpMultiplier     float64    `json:"levelUpMultiplier,omitempty"`
	ForceEnv              string     `json:"forceEnvironment,omitempty"`
	ForceMusic            string     `json:"forceMusic,omitempty"`
	AddEnvs               []EnvEntry `json:"addEnvironments,omitempty"`
	BlockEnvs             []string   `json:"blockEnvironments,omitempty"`
	Spawns                []string   `json:"spawns,omitempty"` // Spawn IDs
	BlockSpawns           []string   `json:"blockSpawns,omitempty"`
	AddVegetation         []string   `json:"addVegetation,omitempty"`
	BlockVegetation       []string   `json:"blockVegetation,omitempty"`
	AddLocations          []string   `json:"addLocations,omitempty"`
	BlockLocations        []string   `json:"blockLocations,omitempty"`
	// Terrain: which biome's texture and height curve the patch borrows,
	// and how the height is rescaled.
	TerrainTexture       string  `json:"terrainTexture,omitempty"`
	HeightMapBiome       string  `json:"heightMapBiome,omitempty"`
	CustomGenerator      string  `json:"customGenerator,omitempty"` // Menu, Flat
	BaseHeightMultiplier float64 `json:"baseHeightMultiplier,omitempty"`
	BaseHeightOffset     float64 `json:"baseHeightOffset,omitempty"`
	HeightMapMultiplier  float64 `json:"heightMapMultiplier,omitempty"`
	HeightMapOffset      float64 `json:"heightMapOffset,omitempty"`
	// World-position gates: only where x/y is below/above these (0: off).
	BelowWorldX float64 `json:"belowWorldX,omitempty"`
	AboveWorldX float64 `json:"aboveWorldX,omitempty"`
	BelowWorldY float64 `json:"belowWorldY,omitempty"`
	AboveWorldY float64 `json:"aboveWorldY,omitempty"`
}
