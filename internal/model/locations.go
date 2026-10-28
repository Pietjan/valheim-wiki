package model

// Location is a ZoneSystem.ZoneLocation rule with what its prefab holds.
type Location struct {
	Prefab  string `json:"prefab"`
	Rule    string `json:"rule,omitempty"` // ZoneLocation.m_name where it differs from the prefab
	Name    string `json:"name"`           // English, from the game's token when it has one, ours otherwise
	Token   string `json:"token,omitempty"`
	Kind    string `json:"kind,omitempty"` // grouping slug shared by variants: "abandoned-house" for WoodHouse1..13
	List    string `json:"list,omitempty"` // which _LocationList_ it came from
	Enabled bool   `json:"enabled"`
	Icon    bool   `json:"icon,omitempty"`    // assets/icons/locations/<prefab>.png rendered
	MapIcon string `json:"mapIcon,omitempty"` // the game's own sprite (mapicon_trader, ...)

	Biomes                 []string `json:"biomes,omitempty"`
	BiomeArea              string   `json:"biomeArea,omitempty"`
	Quantity               int      `json:"quantity"`
	Prioritized            bool     `json:"prioritized,omitempty"`
	CenterFirst            bool     `json:"centerFirst,omitempty"`
	Unique                 bool     `json:"unique,omitempty"`
	Group                  string   `json:"group,omitempty"`
	MinDistanceFromSimilar float64  `json:"minDistanceFromSimilar,omitempty"`
	GroupMax               string   `json:"groupMax,omitempty"`
	MaxDistanceFromSimilar float64  `json:"maxDistanceFromSimilar,omitempty"`
	IconAlways             bool     `json:"iconAlways,omitempty"`
	IconPlaced             bool     `json:"iconPlaced,omitempty"`
	RandomRotation         bool     `json:"randomRotation,omitempty"`
	SlopeRotation          bool     `json:"slopeRotation,omitempty"`
	SnapToWater            bool     `json:"snapToWater,omitempty"`
	InteriorRadius         float64  `json:"interiorRadius,omitempty"`
	ExteriorRadius         float64  `json:"exteriorRadius,omitempty"`
	ClearArea              bool     `json:"clearArea,omitempty"`
	MinTerrainDelta        float64  `json:"minTerrainDelta,omitempty"`
	MaxTerrainDelta        float64  `json:"maxTerrainDelta,omitempty"`
	MinVegetation          float64  `json:"minVegetation,omitempty"`
	MaxVegetation          float64  `json:"maxVegetation,omitempty"`
	InForest               bool     `json:"inForest,omitempty"`
	ForestMin              float64  `json:"forestMin,omitempty"`
	ForestMax              float64  `json:"forestMax,omitempty"`
	MinDistanceFromCenter  float64  `json:"minDistanceFromCenter,omitempty"`
	MaxDistanceFromCenter  float64  `json:"maxDistanceFromCenter,omitempty"`
	MinDistance            float64  `json:"minDistance,omitempty"`
	MaxDistance            float64  `json:"maxDistance,omitempty"`
	MinAltitude            float64  `json:"minAltitude,omitempty"`
	MaxAltitude            float64  `json:"maxAltitude,omitempty"`
	// The surround check: the placement also samples the vegetation mask
	// in Layers rings Distance apart and wants it below the threshold (or
	// merely better than the average).
	SurroundCheckVegetation   bool    `json:"surroundCheckVegetation,omitempty"`
	SurroundCheckDistance     float64 `json:"surroundCheckDistance,omitempty"`
	SurroundCheckLayers       int     `json:"surroundCheckLayers,omitempty"`
	SurroundBetterThanAverage bool    `json:"surroundBetterThanAverage,omitempty"`
	AltBiome                  string  `json:"altBiome,omitempty"` // added by this alt biome

	// The prefab: Location component and contents.
	HasInterior       bool      `json:"hasInterior,omitempty"`
	InteriorEnv       string    `json:"interiorEnvironment,omitempty"`
	NoBuild           bool      `json:"noBuild,omitempty"`
	NoBuildRadius     float64   `json:"noBuildRadius,omitempty"` // m_noBuildRadiusOverride; 0 = the exterior radius
	ApplyRandomDamage bool      `json:"applyRandomDamage,omitempty"`
	DiscoverLabel     string    `json:"discoverLabel,omitempty"`
	EnemyMinLevel     int       `json:"enemyMinLevel,omitempty"`
	EnemyMaxLevel     int       `json:"enemyMaxLevel,omitempty"`
	EnemyLevelUp      float64   `json:"enemyLevelUpChance,omitempty"` // m_enemyLevelUpOverride, percent; 0 = not overridden
	Dungeon           *Dungeon  `json:"dungeon,omitempty"`
	Contents          *Contents `json:"contents,omitempty"`
}

// Dungeon is a DungeonGenerator on a location: the Dungeon algorithm
// grows a tree of rooms from an entrance by matching connections; the
// camp algorithms scatter rooms on a grid or in rings between the camp
// radii. Themes selects which rooms of the room lists it may use.
type Dungeon struct {
	Prefab            string   `json:"prefab"`    // the generator's GameObject: DG_ForestCrypt, DG_GoblinCamp, ...
	Algorithm         string   `json:"algorithm"` // Dungeon, CampGrid, CampRadial
	Themes            []string `json:"themes,omitempty"`
	MinRooms          int      `json:"minRooms,omitempty"`
	MaxRooms          int      `json:"maxRooms,omitempty"`
	MinRequiredRooms  int      `json:"minRequiredRooms,omitempty"`
	RequiredRooms     []string `json:"requiredRooms,omitempty"`
	CampRadiusMin     float64  `json:"campRadiusMin,omitempty"`
	CampRadiusMax     float64  `json:"campRadiusMax,omitempty"`
	TileWidth         float64  `json:"tileWidth,omitempty"` // CampGrid cell size
	GridSize          int      `json:"gridSize,omitempty"`  // CampGrid cells per side
	SpawnChance       float64  `json:"spawnChance,omitempty"`
	PerimeterSections int      `json:"perimeterSections,omitempty"`
	DoorChance        float64  `json:"doorChance,omitempty"`
	Doors             []Door   `json:"doors,omitempty"`
}

// Door is one DungeonGenerator.DoorDef: a door prefab placed on the
// connections of that type, with its own chance when not zero.
type Door struct {
	Prefab         string  `json:"prefab"`
	ConnectionType string  `json:"connectionType,omitempty"`
	Chance         float64 `json:"chance,omitempty"`
}

// Contents is what stands in a location or room prefab, counted by kind.
type Contents struct {
	Chests     []Chest      `json:"chests,omitempty"`
	Spawners   []Spawner    `json:"spawners,omitempty"`
	Nests      []Nest       `json:"nests,omitempty"`
	Pickables  []PickableIn `json:"pickables,omitempty"`
	Loot       []LootIn     `json:"loot,omitempty"` // LootSpawner
	Runestones []Runestone  `json:"runestones,omitempty"`
	Vegvisirs  []Vegvisir   `json:"vegvisirs,omitempty"`
	Offering   *Offering    `json:"offering,omitempty"`
	Creatures  []CreatureIn `json:"creatures,omitempty"` // creatures standing in the prefab directly
	Pieces     []PieceIn    `json:"pieces,omitempty"`    // notable pieces: beds, portals, stations, ...
	Resources  []ResourceIn `json:"resources,omitempty"` // trees, rocks, destructibles with drops
	Portal     bool         `json:"portal,omitempty"`
}

// Chest is a treasure chest found in a prefab.
type Chest struct {
	Prefab string     `json:"prefab"`
	Count  int        `json:"count"`
	Chance float64    `json:"chance,omitempty"` // RandomSpawn chance when under one
	Loot   *DropTable `json:"loot,omitempty"`
}

// Spawner is a CreatureSpawner.
type Spawner struct {
	Creature    string  `json:"creature"`
	Count       int     `json:"count"`
	MinLevel    int     `json:"minLevel,omitempty"`
	MaxLevel    int     `json:"maxLevel,omitempty"`
	LevelUp     float64 `json:"levelUpChance,omitempty"`
	Respawn     float64 `json:"respawnMinutes,omitempty"`
	Night       bool    `json:"night,omitempty"`
	Day         bool    `json:"day,omitempty"`
	RequiredKey string  `json:"requiredKey,omitempty"`
	Chance      float64 `json:"chance,omitempty"`
}

// Nest is a SpawnArea.
type Nest struct {
	Prefab    string      `json:"prefab"`
	Count     int         `json:"count"`
	Creatures []NestEntry `json:"creatures"`
	Interval  float64     `json:"interval,omitempty"`
	MaxNear   int         `json:"maxNear,omitempty"`
	MaxTotal  int         `json:"maxTotal,omitempty"`
	LevelUp   float64     `json:"levelUpChance,omitempty"`
}

// NestEntry is one SpawnArea.SpawnData.
type NestEntry struct {
	Creature string  `json:"creature"`
	Weight   float64 `json:"weight"`
	MinLevel int     `json:"minLevel,omitempty"`
	MaxLevel int     `json:"maxLevel,omitempty"`
}

// PickableIn is a Pickable inside a prefab.
type PickableIn struct {
	Prefab string  `json:"prefab"`
	Item   string  `json:"item,omitempty"`
	Amount int     `json:"amount,omitempty"`
	Count  int     `json:"count"`
	Chance float64 `json:"chance,omitempty"`
}

// LootIn is a LootSpawner.
type LootIn struct {
	Prefab  string     `json:"prefab"`
	Count   int        `json:"count"`
	Loot    *DropTable `json:"loot,omitempty"`
	Respawn float64    `json:"respawnMinutes,omitempty"`
}

// Runestone is a RuneStone's text.
type Runestone struct {
	Prefab string   `json:"prefab"`
	Label  string   `json:"label,omitempty"`
	Topic  string   `json:"topic,omitempty"`
	Text   string   `json:"text,omitempty"`
	Texts  []string `json:"texts,omitempty"`
	Count  int      `json:"count"`
}

// Vegvisir is a Vegvisir: it marks locations on the map.
type Vegvisir struct {
	Prefab    string   `json:"prefab"`
	Locations []string `json:"locations"`
	PinName   string   `json:"pinName,omitempty"`
	Count     int      `json:"count"`
}

// Offering is an OfferingBowl: the altar's summoning.
type Offering struct {
	Item    string `json:"item"`
	Count   int    `json:"count"`
	Boss    string `json:"boss"`
	SetsKey string `json:"setsKey,omitempty"`
}

// CreatureIn is a creature placed directly in a prefab.
type CreatureIn struct {
	Creature string  `json:"creature"`
	Count    int     `json:"count"`
	Chance   float64 `json:"chance,omitempty"`
}

// PieceIn is a notable piece in a prefab.
type PieceIn struct {
	Prefab string `json:"prefab"`
	Count  int    `json:"count"`
}

// ResourceIn is a destructible with drops in a prefab.
type ResourceIn struct {
	Prefab string `json:"prefab"`
	Count  int    `json:"count"`
}

// Room is a dungeon room prefab, with what it holds. Theme and Enabled
// are the room list's entry (DungeonDB.RoomData), which is what the
// generator filters on; the rest is the Room component on the prefab.
type Room struct {
	Prefab  string `json:"prefab"`
	Theme   string `json:"theme"` // the entry's theme; a mask lists its names joined by "|"
	List    string `json:"list,omitempty"`
	Enabled bool   `json:"enabled"`
	// RoomTheme is the Room component's own theme mask, which can differ
	// from the entry's.
	RoomTheme     string    `json:"roomTheme,omitempty"`
	Weight        float64   `json:"weight,omitempty"`
	Entrance      bool      `json:"entrance,omitempty"`
	EndCap        bool      `json:"endCap,omitempty"`
	Divider       bool      `json:"divider,omitempty"`
	EndCapPrio    int       `json:"endCapPrio,omitempty"`
	MinPlaceOrder int       `json:"minPlaceOrder,omitempty"`
	FaceCenter    bool      `json:"faceCenter,omitempty"` // camp rooms: turned to face the camp centre
	Perimeter     bool      `json:"perimeter,omitempty"`  // camp rooms: placed on the perimeter ring
	Size          [3]int    `json:"size,omitempty"`
	Contents      *Contents `json:"contents,omitempty"`
}
