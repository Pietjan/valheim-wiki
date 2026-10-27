// Package model is the wiki's data: what wiki-data reads out of the game's
// asset bundles and what the site generator renders. One JSON file per
// domain under data/, written by DB.Write and read back by Read.
//
// Conventions: things reference each other by prefab name (the game's own
// id: "SwordBronze", "Troll", "Crypt2"); every name shown to a reader is
// already English; biomes are the slugs in Biomes; per-level stats are a
// base and a step, value at quality q = base + (q-1)*step, as the game
// stores them.
package model

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// DB is everything the wiki knows.
type DB struct {
	Meta Meta `json:"meta"`

	Items      map[string]*Item      `json:"items"`
	Recipes    []*Recipe             `json:"recipes"`
	Stations   map[string]*Station   `json:"stations"`
	Converters map[string]*Converter `json:"converters"`
	Traders    []*Trader             `json:"traders"`

	Pieces      map[string]*Piece `json:"pieces"`
	PieceTables []*PieceTable     `json:"pieceTables"`

	Creatures map[string]*Creature `json:"creatures"`
	Spawns    []*Spawn             `json:"spawns"`
	Events    []*Event             `json:"events"`

	Effects map[string]*Effect `json:"effects"`
	Skills  []*Skill           `json:"skills"`

	Locations    map[string]*Location `json:"locations"`
	Rooms        []*Room              `json:"rooms"`
	Biomes       []*Biome             `json:"biomes"`
	Environments map[string]*Env      `json:"environments"`
	AltBiomes    []*AltBiome          `json:"altBiomes"`

	Resources  map[string]*Resource `json:"resources"`
	Vegetation []*VegRule           `json:"vegetation"`
	Clutter    []*Clutter           `json:"clutter"`

	// Icons lists every sprite written under assets/icons/<kind>/<name>.png.
	Icons map[string][]string `json:"icons"`
}

// Meta records where the data came from.
type Meta struct {
	Game      string `json:"game"`
	Extracted string `json:"extracted"`
	Source    string `json:"source"`
	Unity     string `json:"unity,omitempty"`
}

// New is an empty DB with every map made.
func New() *DB {
	return &DB{
		Items: map[string]*Item{}, Stations: map[string]*Station{}, Converters: map[string]*Converter{},
		Pieces: map[string]*Piece{}, Creatures: map[string]*Creature{}, Effects: map[string]*Effect{},
		Locations: map[string]*Location{}, Environments: map[string]*Env{}, Resources: map[string]*Resource{},
		Icons: map[string][]string{},
	}
}

// files is the DB split into files, by field.
func (db *DB) files() map[string]any {
	return map[string]any{
		"meta.json":  &db.Meta,
		"items.json": &db.Items, "recipes.json": &db.Recipes, "stations.json": &db.Stations,
		"converters.json": &db.Converters, "traders.json": &db.Traders,
		"pieces.json": &db.Pieces, "piecetables.json": &db.PieceTables,
		"creatures.json": &db.Creatures, "spawns.json": &db.Spawns, "events.json": &db.Events,
		"effects.json": &db.Effects, "skills.json": &db.Skills,
		"locations.json": &db.Locations, "rooms.json": &db.Rooms, "biomes.json": &db.Biomes,
		"environments.json": &db.Environments, "altbiomes.json": &db.AltBiomes,
		"resources.json": &db.Resources, "vegetation.json": &db.Vegetation, "clutter.json": &db.Clutter,
		"icons.json": &db.Icons,
	}
}

// Write writes one indented JSON file per domain into dir.
func (db *DB) Write(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for name, v := range db.files() {
		b, err := json.MarshalIndent(v, "", " ")
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), append(b, '\n'), 0o644); err != nil {
			return err
		}
	}
	return nil
}

// Read loads what Write wrote. A missing file leaves its field empty.
func Read(dir string) (*DB, error) {
	db := New()
	for name, v := range db.files() {
		b, err := os.ReadFile(filepath.Join(dir, name))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal(b, v); err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
	}
	return db, nil
}

// SortedKeys is a map's keys in order, for deterministic output.
func SortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// Biome slugs, in the game's progression order. Ocean last.
var Biomes = []string{"meadows", "black-forest", "swamp", "mountain", "plains", "mistlands", "ashlands", "deep-north", "ocean"}

// BiomeName is the English name of a biome slug.
var BiomeName = map[string]string{
	"meadows": "Meadows", "black-forest": "Black Forest", "swamp": "Swamp", "mountain": "Mountain",
	"plains": "Plains", "mistlands": "Mistlands", "ashlands": "Ashlands", "deep-north": "Deep North", "ocean": "Ocean",
}

// BiomeBit is Heightmap.Biome's flag per slug.
var BiomeBit = map[string]int{
	"meadows": 1, "swamp": 2, "mountain": 4, "black-forest": 8, "plains": 16,
	"ashlands": 32, "deep-north": 64, "ocean": 256, "mistlands": 512,
}

// BiomesOf expands a Heightmap.Biome mask into slugs, in Biomes order.
func BiomesOf(mask int) []string {
	var out []string
	for _, b := range Biomes {
		if mask&BiomeBit[b] != 0 {
			out = append(out, b)
		}
	}
	return out
}

// Damage is damage by type: blunt, slash, pierce, chop, pickaxe, fire,
// frost, lightning, poison, spirit, and "damage" for the untyped kind.
// Only the non-zero entries are kept.
type Damage map[string]float64

// DamageTypes is the order damage is listed in.
var DamageTypes = []string{"damage", "blunt", "slash", "pierce", "chop", "pickaxe", "fire", "frost", "lightning", "poison", "spirit"}

// Total is the sum over every type.
func (d Damage) Total() float64 {
	t := 0.0
	for _, v := range d {
		t += v
	}
	return t
}

// DamageMod is one resistance or weakness: a damage type and the
// game's modifier name (Normal, Resistant, Weak, Immune, Ignore,
// VeryResistant, VeryWeak, SlightlyResistant, SlightlyWeak).
type DamageMod struct {
	Type     string `json:"type"`
	Modifier string `json:"mod"`
}

// DamageModifierNames in HitData.DamageModifier order.
var DamageModifierNames = []string{"Normal", "Resistant", "Weak", "Immune", "Ignore", "VeryResistant", "VeryWeak", "SlightlyResistant", "SlightlyWeak"}

// Requirement is one Piece.Requirement: an item, how many, and how many
// more each quality level costs.
type Requirement struct {
	Item     string `json:"item"`
	Amount   int    `json:"amount"`
	PerLevel int    `json:"perLevel,omitempty"`
	Recover  bool   `json:"recover"`
	Upgrader bool   `json:"upgrader,omitempty"`
}

// DropTable is the game's DropTable: between Min and Max picks (with
// Chance of dropping anything) from the weighted Drops.
type DropTable struct {
	Min       int         `json:"min"`
	Max       int         `json:"max"`
	Chance    float64     `json:"chance"`
	OneOfEach bool        `json:"oneOfEach,omitempty"`
	Drops     []DropEntry `json:"drops"`
}

// DropEntry is one row of a DropTable.
type DropEntry struct {
	Item      string  `json:"item"`
	Min       int     `json:"min"`
	Max       int     `json:"max"`
	Weight    float64 `json:"weight"`
	DontScale bool    `json:"dontScale,omitempty"`
}

// Empty is whether the table can drop nothing.
func (t *DropTable) Empty() bool { return t == nil || len(t.Drops) == 0 || t.Max == 0 }
