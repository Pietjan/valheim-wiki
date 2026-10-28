package model

// Item is an ItemDrop prefab: ItemDrop.ItemData.SharedData with the
// defaults the game leaks onto every item left in place; the site gates
// what it shows by Type.
type Item struct {
	Prefab      string `json:"prefab"`
	Name        string `json:"name"`
	Token       string `json:"token,omitempty"`
	Description string `json:"desc,omitempty"`
	Type        string `json:"type"` // ItemType name: Material, Consumable, OneHandedWeapon, ...
	// Icon is the number of icons written as assets/icons/items/<prefab>.png,
	// <prefab>_1.png, ...: one per variant, or per quality where they differ.
	Icons    int `json:"icons,omitempty"`
	Variants int `json:"variants,omitempty"`
	// Hidden marks the ItemDrop prefabs a player never holds: creature
	// attacks, NPC costumes and the like, which have no icon and no
	// enabled recipe. They stay in the table because creatures name them.
	Hidden bool `json:"hidden,omitempty"`

	MaxQuality     int     `json:"maxQuality"`
	Weight         float64 `json:"weight"`
	WeightPerLevel float64 `json:"weightPerLevel,omitempty"`
	Stack          int     `json:"stack,omitempty"`
	Value          int     `json:"value,omitempty"`
	Teleportable   bool    `json:"teleportable"`
	QuestItem      bool    `json:"questItem,omitempty"`
	Floating       bool    `json:"floating,omitempty"`
	DLC            string  `json:"dlc,omitempty"`

	// Weapons and tools.
	Skill          string  `json:"skill,omitempty"`
	ToolTier       int     `json:"toolTier,omitempty"`
	AmmoType       string  `json:"ammoType,omitempty"` // "$ammo_arrows" localized: "Arrows"
	Damage         Damage  `json:"damage,omitempty"`
	DamagePerLevel Damage  `json:"damagePerLevel,omitempty"`
	Knockback      float64 `json:"knockback,omitempty"`
	Backstab       float64 `json:"backstab,omitempty"`
	Attack         *Attack `json:"attack,omitempty"`
	Secondary      *Attack `json:"secondary,omitempty"`
	TamedOnly      bool    `json:"tamedOnly,omitempty"`
	AttackEffect   string  `json:"attackEffect,omitempty"` // status effect id applied on hit
	BuildPieces    string  `json:"buildPieces,omitempty"`  // the PieceTable a tool opens (Hammer, Hoe, Cultivator, ...)
	Projectile     string  `json:"projectile,omitempty"`

	// Armor and blocking.
	Armor              float64     `json:"armor,omitempty"`
	ArmorPerLevel      float64     `json:"armorPerLevel,omitempty"`
	Resist             []DamageMod `json:"resist,omitempty"`
	BlockPower         float64     `json:"blockPower,omitempty"`
	BlockPowerPerLevel float64     `json:"blockPowerPerLevel,omitempty"`
	Deflection         float64     `json:"deflection,omitempty"`
	DeflectionPerLevel float64     `json:"deflectionPerLevel,omitempty"`
	ParryBonus         float64     `json:"parryBonus,omitempty"`
	ParryAdrenaline    float64     `json:"parryAdrenaline,omitempty"`

	// Durability.
	UseDurability      bool    `json:"useDurability,omitempty"`
	Durability         float64 `json:"durability,omitempty"`
	DurabilityPerLevel float64 `json:"durabilityPerLevel,omitempty"`
	DurabilityDrain    float64 `json:"durabilityDrain,omitempty"`
	CanRepair          bool    `json:"canRepair,omitempty"`

	// Modifiers while equipped: movement, eitrRegen, jumpStamina,
	// attackStamina, blockStamina, dodgeStamina, swimStamina,
	// sneakStamina, runStamina, homeItemsStamina, heatResistance.
	Modifiers   map[string]float64 `json:"modifiers,omitempty"`
	SetName     string             `json:"setName,omitempty"`
	SetSize     int                `json:"setSize,omitempty"`
	SetEffect   string             `json:"setEffect,omitempty"`   // effect id
	EquipEffect string             `json:"equipEffect,omitempty"` // effect id

	// Food and drink.
	Food          float64 `json:"food,omitempty"`
	FoodStamina   float64 `json:"foodStamina,omitempty"`
	FoodEitr      float64 `json:"foodEitr,omitempty"`
	FoodBurn      float64 `json:"foodBurn,omitempty"` // seconds
	FoodRegen     float64 `json:"foodRegen,omitempty"`
	IsDrink       bool    `json:"isDrink,omitempty"`
	ConsumeEffect string  `json:"consumeEffect,omitempty"` // effect id (meads)

	// Forge of Potential.
	Upgrader *Upgrader `json:"upgrader,omitempty"`

	// Trophy: the creature it is cut from, when known.
	TrophyOf string `json:"trophyOf,omitempty"`
}

// Attack is one of SharedData's two Attack blocks.
type Attack struct {
	Type              string  `json:"type"` // Horizontal, Vertical, Projectile, None, Area, TriggerProjectile
	Animation         string  `json:"animation,omitempty"`
	Stamina           float64 `json:"stamina,omitempty"`
	Eitr              float64 `json:"eitr,omitempty"`
	Health            float64 `json:"health,omitempty"`
	HealthPct         float64 `json:"healthPct,omitempty"`
	DrawStamina       float64 `json:"drawStamina,omitempty"`
	Range             float64 `json:"range,omitempty"`
	Height            float64 `json:"height,omitempty"`
	Angle             float64 `json:"angle,omitempty"`
	RayWidth          float64 `json:"rayWidth,omitempty"`
	DamageMultiplier  float64 `json:"damageMultiplier,omitempty"`
	ForceMultiplier   float64 `json:"forceMultiplier,omitempty"`
	StaggerMultiplier float64 `json:"staggerMultiplier,omitempty"`
	ChainLevels       int     `json:"chainLevels,omitempty"`
	LastChainMult     float64 `json:"lastChainMult,omitempty"`
	Projectile        string  `json:"projectile,omitempty"`
	Projectiles       int     `json:"projectiles,omitempty"`
	Bursts            int     `json:"bursts,omitempty"`
	ProjectileVel     float64 `json:"projectileVel,omitempty"`
	Accuracy          float64 `json:"accuracy,omitempty"`
	Reload            float64 `json:"reload,omitempty"`
	Harvest           bool    `json:"harvest,omitempty"`
	ConsumesItem      bool    `json:"consumesItem,omitempty"`
}

// Upgrader is what a Forge of Potential attempt paid with this idol rolls
// against.
type Upgrader struct {
	Chance float64 `json:"chance"`
	Break  float64 `json:"break"`
	Return float64 `json:"return"`
	Steps  int     `json:"steps"`
}

// Recipe is a Recipe asset.
type Recipe struct {
	ID              string        `json:"id"`
	Item            string        `json:"item"`
	Amount          int           `json:"amount"`
	Station         string        `json:"station,omitempty"` // piece prefab, "" = by hand
	RepairStation   string        `json:"repairStation,omitempty"`
	MinStationLevel int           `json:"minStationLevel"`
	OnlyUpgrade     bool          `json:"onlyUpgrade,omitempty"`
	OneIngredient   bool          `json:"oneIngredient,omitempty"`
	QualityMult     float64       `json:"qualityMult,omitempty"`
	Resources       []Requirement `json:"resources"`
}

// Station is a CraftingStation component on a piece.
type Station struct {
	Prefab      string   `json:"prefab"`
	Name        string   `json:"name"`
	Icon        bool     `json:"icon,omitempty"`
	Upgrader    bool     `json:"upgrader,omitempty"`
	RequireRoof bool     `json:"requireRoof,omitempty"`
	RequireFire bool     `json:"requireFire,omitempty"`
	CanRepair   bool     `json:"canRepair,omitempty"`
	Skill       string   `json:"skill,omitempty"`
	Extensions  []string `json:"extensions,omitempty"` // piece prefabs with a StationExtension for it
	MaxLevel    int      `json:"maxLevel,omitempty"`   // 1 + len(Extensions)
}

// Converter is a piece that turns items into other items: Smelter,
// CookingStation, Fermenter, Incinerator, Beehive, SapCollector, Feast,
// and the like.
type Converter struct {
	Prefab        string       `json:"prefab"`
	Name          string       `json:"name"`
	Kind          string       `json:"kind"`            // smelter, cooking, fermenter, incinerator, beehive, sap, feast, fireplace, turret, shield, trap, wisp
	Fuel          string       `json:"fuel,omitempty"`  // the fuel item; a turret's default ammo
	Fuels         []string     `json:"fuels,omitempty"` // a shield generator's fuels; a turret's allowed ammo
	FuelPerItem   float64      `json:"fuelPerItem,omitempty"`
	SecPerItem    float64      `json:"secPerItem,omitempty"` // seconds per product (smelter), per unit (beehive, sap), per spawn (wisp)
	SecPerFuel    float64      `json:"secPerFuel,omitempty"` // seconds one fuel burns (fireplace, oven)
	StartFuel     float64      `json:"startFuel,omitempty"`  // fuel a fireplace is built with
	InfiniteFuel  bool         `json:"infiniteFuel,omitempty"`
	MaxInput      int          `json:"maxInput,omitempty"`
	MaxFuel       int          `json:"maxFuel,omitempty"` // a turret's max ammo
	RequiresRoof  bool         `json:"requiresRoof,omitempty"`
	RequiresFire  bool         `json:"requiresFire,omitempty"`
	Overcooked    string       `json:"overcooked,omitempty"`
	Duration      float64      `json:"duration,omitempty"` // fermenter
	Biomes        []string     `json:"biomes,omitempty"`   // beehive
	Product       string       `json:"product,omitempty"`  // beehive/sap item
	MaxProduct    int          `json:"maxProduct,omitempty"`
	Conversions   []Conversion `json:"conversions,omitempty"`
	Incinerations []Incinerate `json:"incinerations,omitempty"`
	// DefaultResult is what an incinerator makes of anything no
	// Incinerations row claims: one per DefaultCost items.
	DefaultResult string   `json:"defaultResult,omitempty"`
	DefaultCost   int      `json:"defaultCost,omitempty"`
	Notes         []string `json:"notes,omitempty"`
}

// Conversion is one ItemConversion.
type Conversion struct {
	From   string  `json:"from"`
	To     string  `json:"to"`
	Amount int     `json:"amount,omitempty"`
	Time   float64 `json:"time,omitempty"`
}

// Incinerate is one IncineratorConversion.
type Incinerate struct {
	Requirements []Requirement `json:"requirements"`
	Result       string        `json:"result"`
	Amount       int           `json:"amount"`
	Priority     int           `json:"priority,omitempty"`
	OneOf        bool          `json:"oneOf,omitempty"`
}

// Trader is Haldor, Hildir or the Bog Witch.
type Trader struct {
	Prefab   string      `json:"prefab"`
	Name     string      `json:"name"`
	Location string      `json:"location,omitempty"` // location prefab of the camp
	Items    []TradeItem `json:"items"`
	UseItems []UseItem   `json:"useItems,omitempty"`
}

// TradeItem is one thing for sale.
type TradeItem struct {
	Item        string `json:"item"`
	Stack       int    `json:"stack"`
	Price       int    `json:"price"`
	RequiredKey string `json:"requiredKey,omitempty"`
}

// UseItem is a thing the trader takes from you.
type UseItem struct {
	Item     string `json:"item"`
	SetsKey  string `json:"setsKey,omitempty"`
	Removes  bool   `json:"removes,omitempty"`
	Dialogue string `json:"dialogue,omitempty"`
}
