package model

// Piece is a Piece component: something built with a tool.
type Piece struct {
	Prefab      string   `json:"prefab"`
	Name        string   `json:"name"`
	Token       string   `json:"token,omitempty"`
	Description string   `json:"desc,omitempty"`
	Icon        bool     `json:"icon,omitempty"`
	Category    string   `json:"category"` // the tab of the tool's build menu: Misc, Crafting, BuildingWorkbench, BuildingStonecutter, Furniture, DeepNorth, Feasts, Food, Meads
	Usage       []string `json:"usage,omitempty"`
	Tool        string   `json:"tool,omitempty"` // the item whose PieceTable lists it: Hammer, Hoe, Cultivator, FeasterTable
	Enabled     bool     `json:"enabled"`
	DLC         string   `json:"dlc,omitempty"`

	Station   string        `json:"station,omitempty"`
	Resources []Requirement `json:"resources,omitempty"`

	Comfort      int    `json:"comfort,omitempty"`
	ComfortGroup string `json:"comfortGroup,omitempty"`

	// WearNTear.
	Health      float64     `json:"health,omitempty"`
	Material    string      `json:"material,omitempty"`
	Resist      []DamageMod `json:"resist,omitempty"`
	Supports    bool        `json:"supports,omitempty"`
	Burnable    bool        `json:"burnable,omitempty"`
	NoRoofWear  bool        `json:"noRoofWear,omitempty"`
	AshImmune   bool        `json:"ashImmune,omitempty"`
	MinToolTier int         `json:"minToolTier,omitempty"`

	// Placement rules, as flags: groundPiece, groundOnly, waterPiece,
	// cultivatedGroundOnly, allowedInDungeons, noInWater, notOnWood,
	// notOnTiltingSurface, inCeilingOnly, notOnFloor, onlyInTeleportArea,
	// allowedInDeepSnow, requireDeepSnow, vegetationGroundOnly, clipGround.
	Placement []string `json:"placement,omitempty"`
	OnlyBiome []string `json:"onlyBiome,omitempty"`

	// What else the piece is.
	Container  *ContainerInfo `json:"container,omitempty"`
	Door       *DoorInfo      `json:"door,omitempty"`
	Fireplace  *FireplaceInfo `json:"fireplace,omitempty"`
	Bed        bool           `json:"bed,omitempty"`
	Portal     *PortalInfo    `json:"portal,omitempty"`
	Ward       *WardInfo      `json:"ward,omitempty"`
	Ship       *ShipInfo      `json:"ship,omitempty"`
	Cart       *CartInfo      `json:"cart,omitempty"`
	Plant      *PlantInfo     `json:"plant,omitempty"`
	Station_   string         `json:"isStation,omitempty"`   // the CraftingStation this piece is
	Extends    string         `json:"extends,omitempty"`     // StationExtension target
	Converter_ string         `json:"isConverter,omitempty"` // the Converter this piece is
	ItemStand  *ItemStandInfo `json:"itemStand,omitempty"`
	Light      bool           `json:"light,omitempty"`
	Sign       bool           `json:"sign,omitempty"`
	MapTable   bool           `json:"mapTable,omitempty"`
	Turret     *TurretInfo    `json:"turret,omitempty"`
	Trap       bool           `json:"trap,omitempty"`
	Demister   bool           `json:"demister,omitempty"`
	Windmill   bool           `json:"windmill,omitempty"`
	Chair      bool           `json:"chair,omitempty"`
	Notes      []string       `json:"notes,omitempty"`
}

// ContainerInfo is a Container component.
type ContainerInfo struct {
	Width   int        `json:"width"`
	Height  int        `json:"height"`
	Private bool       `json:"private,omitempty"`
	Loot    *DropTable `json:"loot,omitempty"` // m_defaultItems, for loot chests
}

// DoorInfo is a Door component.
type DoorInfo struct {
	Key        string `json:"key,omitempty"`
	ConsumeKey bool   `json:"consumeKey,omitempty"`
}

// FireplaceInfo is a Fireplace component.
type FireplaceInfo struct {
	Fuel       string  `json:"fuel,omitempty"`
	StartFuel  float64 `json:"startFuel,omitempty"`
	MaxFuel    float64 `json:"maxFuel,omitempty"`
	SecPerFuel float64 `json:"secPerFuel,omitempty"`
	Infinite   bool    `json:"infinite,omitempty"`
}

// PortalInfo is a TeleportWorld.
type PortalInfo struct {
	AllowAllItems bool `json:"allowAllItems,omitempty"`
}

// WardInfo is a PrivateArea.
type WardInfo struct {
	Radius float64 `json:"radius"`
}

// ShipInfo is a Ship component.
type ShipInfo struct {
	SailForce     float64        `json:"sailForce,omitempty"`
	RudderSpeed   float64        `json:"rudderSpeed,omitempty"`
	SteerForce    float64        `json:"steerForce,omitempty"`
	BackwardForce float64        `json:"backwardForce,omitempty"`
	HasSail       bool           `json:"hasSail,omitempty"`
	AshlandsReady bool           `json:"ashlandsReady,omitempty"`
	Container     *ContainerInfo `json:"container,omitempty"`
}

// CartInfo is a Vagon.
type CartInfo struct {
	Container *ContainerInfo `json:"container,omitempty"`
	BaseMass  float64        `json:"baseMass,omitempty"`
}

// PlantInfo is a Plant component on a sapling.
type PlantInfo struct {
	GrowTime       float64  `json:"growTime"`
	GrowTimeMax    float64  `json:"growTimeMax"`
	GrownPrefabs   []string `json:"grown,omitempty"`
	GrowRadius     float64  `json:"growRadius,omitempty"`
	NeedCultivated bool     `json:"needCultivated,omitempty"`
	Biomes         []string `json:"biomes,omitempty"`
	TolerateHeat   bool     `json:"tolerateHeat,omitempty"`
	TolerateCold   bool     `json:"tolerateCold,omitempty"`
	DestroyIfCant  bool     `json:"destroyIfCantGrow,omitempty"`
}

// ItemStandInfo is an ItemStand.
type ItemStandInfo struct {
	Types         []string `json:"types,omitempty"`
	GuardianPower string   `json:"guardianPower,omitempty"`
}

// TurretInfo is a Turret.
type TurretInfo struct {
	Ammo     []string `json:"ammo,omitempty"`
	MaxAmmo  int      `json:"maxAmmo,omitempty"`
	Range    float64  `json:"range,omitempty"`
	Cooldown float64  `json:"cooldown,omitempty"`
}

// PieceTable is a tool's build menu.
type PieceTable struct {
	Prefab     string   `json:"prefab"` // _HammerPieceTable
	Tool       string   `json:"tool"`   // item prefab that opens it
	Pieces     []string `json:"pieces"` // in menu order
	Categories []string `json:"categories,omitempty"`
	Labels     []string `json:"labels,omitempty"`
}
