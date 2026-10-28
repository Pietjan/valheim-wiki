package model

// Creature is a Character prefab.
type Creature struct {
	Prefab  string `json:"prefab"`
	Name    string `json:"name"`
	Token   string `json:"token,omitempty"`
	Group   string `json:"group,omitempty"`
	Faction string `json:"faction"`
	// Icon is whether assets/icons/creatures/<prefab>.png was rendered.
	Icon bool `json:"icon,omitempty"`

	Boss      bool `json:"boss,omitempty"`
	BossOrder int  `json:"bossOrder,omitempty"`
	// MiniBoss is a boss without an altar — Hildir's quarry, the
	// Ashlands' lords — whose name the game writes in orange.
	MiniBoss  bool   `json:"miniBoss,omitempty"`
	DefeatKey string `json:"defeatKey,omitempty"`
	BossEvent string `json:"bossEvent,omitempty"`

	Health    float64     `json:"health"`
	Resist    []DamageMod `json:"resist,omitempty"` // the non-Normal entries of m_damageModifiers
	WalkSpeed float64     `json:"walkSpeed,omitempty"`
	RunSpeed  float64     `json:"runSpeed,omitempty"`
	SwimSpeed float64     `json:"swimSpeed,omitempty"`
	FlySpeed  float64     `json:"flySpeed,omitempty"` // m_flyFastSpeed, for flyers
	Flying    bool        `json:"flying,omitempty"`
	CanSwim   bool        `json:"canSwim,omitempty"`
	Tolerates []string    `json:"tolerates,omitempty"` // water, fire, smoke, tar
	Stagger   float64     `json:"staggerDamageFactor,omitempty"`

	Attacks []CreatureAttack `json:"attacks,omitempty"`
	Drops   []Drop           `json:"drops,omitempty"`
	Trophy  string           `json:"trophy,omitempty"` // the Trophy* item among the drops

	AI          *AIInfo          `json:"ai,omitempty"`
	Tameable    *TameInfo        `json:"tameable,omitempty"`
	Procreation *ProcreationInfo `json:"procreation,omitempty"`
	Growup      *GrowupInfo      `json:"growup,omitempty"`
	Egg         *GrowupInfo      `json:"egg,omitempty"` // the EggGrow item this hatches from
	Fish        *FishInfo        `json:"fish,omitempty"`
	Ragdoll     bool             `json:"-"`

	// Where it is met: filled from spawn tables, locations and events.
	Biomes []string `json:"biomes,omitempty"`
	// Summoned by a staff, spawned by a boss, or otherwise not a world spawn.
	Summoned bool `json:"summoned,omitempty"`
	// Humanoid gear, when it has any.
	DefaultItems  []string  `json:"defaultItems,omitempty"`
	RandomWeapons []string  `json:"randomWeapons,omitempty"`
	RandomArmor   []string  `json:"randomArmor,omitempty"`
	RandomShields []string  `json:"randomShields,omitempty"`
	RandomSets    []ItemSet `json:"randomSets,omitempty"`
	// RandomItems are Humanoid.m_randomItems: each rolled on its own.
	RandomItems []ItemChance `json:"randomItems,omitempty"`
	// NPC is reserved for a Character that is a person rather than a
	// monster. No such prefab exists in the current bundles: the traders
	// and Odin's ravens carry no Character, so wiki-data never sets it.
	NPC bool `json:"npc,omitempty"`
}

// ItemChance is one Humanoid.RandomItem: the chance the creature carries
// the item (the game keeps it when Random.value > m_chance, so this is
// 1 - m_chance).
type ItemChance struct {
	Item   string  `json:"item"`
	Chance float64 `json:"chance"`
}

// CreatureAttack is one of the creature's weapons: an ItemDrop prefab's
// damage, read off the prefab the Humanoid is given.
type CreatureAttack struct {
	Item       string  `json:"item"`
	Name       string  `json:"name,omitempty"`
	Damage     Damage  `json:"damage,omitempty"`
	Knockback  float64 `json:"knockback,omitempty"`
	Range      float64 `json:"range,omitempty"` // m_aiAttackRange: how close the AI wants to be
	Reach      float64 `json:"reach,omitempty"` // m_attack.m_attackRange: how far the swing hits
	Interval   float64 `json:"interval,omitempty"`
	Type       string  `json:"type,omitempty"`
	Projectile string  `json:"projectile,omitempty"`
	Effect     string  `json:"effect,omitempty"`
	Chance     float64 `json:"chance,omitempty"` // random item chance
	ToolTier   int     `json:"toolTier,omitempty"`
}

// Drop is one CharacterDrop entry.
type Drop struct {
	Item                     string  `json:"item"`
	Min                      int     `json:"min"`
	Max                      int     `json:"max"`
	Chance                   float64 `json:"chance"`
	OnePerPlayer             bool    `json:"onePerPlayer,omitempty"`
	LevelMultiplier          bool    `json:"levelMultiplier,omitempty"`
	LevelMultiplierNotAmount bool    `json:"levelMultiplierNotAmount,omitempty"`
	DontScale                bool    `json:"dontScale,omitempty"`
}

// AIInfo is BaseAI + MonsterAI/AnimalAI.
type AIInfo struct {
	Kind                string   `json:"kind"` // MonsterAI, AnimalAI
	ViewRange           float64  `json:"viewRange,omitempty"`
	ViewAngle           float64  `json:"viewAngle,omitempty"`
	HearRange           float64  `json:"hearRange,omitempty"`
	AlertRange          float64  `json:"alertRange,omitempty"`
	FleeLowHealth       float64  `json:"fleeLowHealth,omitempty"`
	FleeRange           float64  `json:"fleeRange,omitempty"`
	HuntPlayer          bool     `json:"huntPlayer,omitempty"`
	AttackPlayerObjects bool     `json:"attackPlayerObjects,omitempty"`
	Sleeping            bool     `json:"sleeping,omitempty"`
	WakeupRange         float64  `json:"wakeupRange,omitempty"`
	AvoidFire           bool     `json:"avoidFire,omitempty"`
	AfraidOfFire        bool     `json:"afraidOfFire,omitempty"`
	AvoidWater          bool     `json:"avoidWater,omitempty"`
	AvoidLand           bool     `json:"avoidLand,omitempty"`
	Aggravatable        bool     `json:"aggravatable,omitempty"`
	PassiveAggressive   bool     `json:"passiveAggressive,omitempty"`
	MistVision          bool     `json:"mistVision,omitempty"`
	MaxChase            float64  `json:"maxChase,omitempty"`
	ConsumeItems        []string `json:"consumeItems,omitempty"` // what it eats (taming)
	SpawnMessage        string   `json:"spawnMessage,omitempty"`
	DeathMessage        string   `json:"deathMessage,omitempty"`
	AlertedMessage      string   `json:"alertedMessage,omitempty"`
}

// TameInfo is a Tameable component.
type TameInfo struct {
	TamingTime   float64  `json:"tamingTime"`
	FedDuration  float64  `json:"fedDuration"`
	Commandable  bool     `json:"commandable,omitempty"`
	Saddle       string   `json:"saddle,omitempty"`
	StartsTamed  bool     `json:"startsTamed,omitempty"`
	Names        []string `json:"names,omitempty"`
	LevelUpSkill bool     `json:"levelUpOwnerSkill,omitempty"`
	Skill        string   `json:"skill,omitempty"` // the owner's skill it trains (Ride)
	// Unsummon is how far from its owner a summon may stray (0: not a summon).
	Unsummon float64 `json:"unsummonDistance,omitempty"`
}

// ProcreationInfo is a Procreation component.
type ProcreationInfo struct {
	Offspring          string  `json:"offspring,omitempty"`
	MaxCreatures       int     `json:"maxCreatures,omitempty"`
	PregnancyChance    float64 `json:"pregnancyChance,omitempty"`
	PregnancyDuration  float64 `json:"pregnancyDuration,omitempty"`
	RequiredLove       int     `json:"requiredLovePoints,omitempty"`
	PartnerRange       float64 `json:"partnerRange,omitempty"`
	MinOffspringLevel  int     `json:"minOffspringLevel,omitempty"`
	Partner            string  `json:"partner,omitempty"`            // m_seperatePartner: a different prefab it mates with
	NoPartnerOffspring string  `json:"noPartnerOffspring,omitempty"` // laid without a partner (eggs)
}

// GrowupInfo is Growup (a cub that becomes Grown) or EggGrow (an egg
// item From that hatches into Grown).
type GrowupInfo struct {
	GrowTime    float64  `json:"growTime"`
	Grown       string   `json:"grown,omitempty"`
	From        string   `json:"from,omitempty"`
	AltGrown    []string `json:"altGrown,omitempty"`
	InheritTame bool     `json:"inheritTame,omitempty"`
	NeedsFire   bool     `json:"needsFire,omitempty"`
	NeedsRoof   bool     `json:"needsRoof,omitempty"`
}

// FishInfo is a Fish component.
type FishInfo struct {
	MinDepth       float64    `json:"minDepth"`
	MaxDepth       float64    `json:"maxDepth"`
	BaseHookChance float64    `json:"baseHookChance"`
	PickupItem     string     `json:"pickupItem,omitempty"`
	Baits          []BaitInfo `json:"baits,omitempty"`
	ExtraDrops     *DropTable `json:"extraDrops,omitempty"`
	EscapePerLevel float64    `json:"escapeMaxPerLevel,omitempty"`
}

// BaitInfo is one BaitSetting.
type BaitInfo struct {
	Bait   string  `json:"bait"`
	Chance float64 `json:"chance"`
}

// ItemSet is a Humanoid.ItemSet.
type ItemSet struct {
	Name  string   `json:"name"`
	Items []string `json:"items"`
}

// Spawn is one SpawnSystem.SpawnData: the world's rule for a creature.
type Spawn struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Prefab string `json:"prefab"`
	// Creatures is what a Prefab that is not itself a creature spawns: a
	// CreatureSpawner's creature (Spawner_TrollFrost -> TrollFrost) or a
	// SpawnArea's roster (Spawner_CharredStone_event). Empty when Prefab
	// is the creature.
	Creatures                []string `json:"creatures,omitempty"`
	Source                   string   `json:"source"` // "world:<list>", "event:<name>", "location:<prefab>", "area:<prefab>", "altbiome:<name>"
	Enabled                  bool     `json:"enabled"`
	Biomes                   []string `json:"biomes,omitempty"`
	BiomeArea                string   `json:"biomeArea,omitempty"` // Edge, Median, Everything
	MaxSpawned               int      `json:"maxSpawned,omitempty"`
	Interval                 float64  `json:"interval,omitempty"`
	Chance                   float64  `json:"chance,omitempty"`
	Distance                 float64  `json:"distance,omitempty"`
	RadiusMin                float64  `json:"radiusMin,omitempty"`
	RadiusMax                float64  `json:"radiusMax,omitempty"`
	RequiredKey              string   `json:"requiredKey,omitempty"`
	RequiredEvent            string   `json:"requiredEvent,omitempty"` // m_requiredPersistentEvent
	Environments             []string `json:"environments,omitempty"`
	GroupMin                 int      `json:"groupMin,omitempty"`
	GroupMax                 int      `json:"groupMax,omitempty"`
	GroupRadius              float64  `json:"groupRadius,omitempty"`
	Night                    bool     `json:"night"`
	Day                      bool     `json:"day"`
	MinAltitude              float64  `json:"minAltitude,omitempty"`
	MaxAltitude              float64  `json:"maxAltitude,omitempty"`
	MinTilt                  float64  `json:"minTilt,omitempty"`
	MaxTilt                  float64  `json:"maxTilt,omitempty"`
	InForest                 bool     `json:"inForest"`
	OutsideForest            bool     `json:"outsideForest"`
	InLava                   bool     `json:"inLava,omitempty"`
	OutsideLava              bool     `json:"outsideLava,omitempty"`
	MinOceanDepth            float64  `json:"minOceanDepth,omitempty"`
	MaxOceanDepth            float64  `json:"maxOceanDepth,omitempty"`
	HuntPlayer               bool     `json:"huntPlayer,omitempty"`
	GroundOffset             float64  `json:"groundOffset,omitempty"`
	MinDistanceFromCenter    float64  `json:"minDistanceFromCenter,omitempty"`
	MaxDistanceFromCenter    float64  `json:"maxDistanceFromCenter,omitempty"`
	MinLevel                 int      `json:"minLevel,omitempty"`
	MaxLevel                 int      `json:"maxLevel,omitempty"`
	LevelUpMinCenterDistance float64  `json:"levelUpMinCenterDistance,omitempty"`
	LevelUpChance            float64  `json:"levelUpChance,omitempty"` // override, -1 = default 10%
	CanSpawnCloseToPlayer    bool     `json:"canSpawnCloseToPlayer,omitempty"`
	InsidePlayerBase         bool     `json:"insidePlayerBase,omitempty"`
}

// Event is a RandomEvent (raid).
type Event struct {
	Name          string   `json:"name"`
	Enabled       bool     `json:"enabled"`
	Random        bool     `json:"random"`
	Duration      float64  `json:"duration"`
	NearBaseOnly  bool     `json:"nearBaseOnly,omitempty"`
	Range         float64  `json:"range,omitempty"`
	Biomes        []string `json:"biomes,omitempty"`
	RequiredKeys  []string `json:"requiredKeys,omitempty"`
	NotKeys       []string `json:"notKeys,omitempty"`
	RequiredItems []string `json:"requiredKnownItems,omitempty"`
	// The alternative gates: player keys any/all of which unlock it, keys
	// that block it, items that block it once known.
	PlayerKeysAny []string `json:"playerKeysAny,omitempty"`
	PlayerKeysAll []string `json:"playerKeysAll,omitempty"`
	NotPlayerKeys []string `json:"notPlayerKeys,omitempty"`
	NotKnownItems []string `json:"notKnownItems,omitempty"`
	// Standalone events fire on their own timer instead of the random roll.
	StandaloneInterval float64  `json:"standaloneInterval,omitempty"`
	StandaloneChance   float64  `json:"standaloneChance,omitempty"`
	PauseIfNoPlayer    bool     `json:"pauseIfNoPlayerInArea,omitempty"`
	StartMessage       string   `json:"startMessage,omitempty"`
	EndMessage         string   `json:"endMessage,omitempty"`
	ForceEnv           string   `json:"forceEnvironment,omitempty"`
	ForceMusic         string   `json:"forceMusic,omitempty"`
	Spawns             []string `json:"spawns,omitempty"` // Spawn IDs
}
