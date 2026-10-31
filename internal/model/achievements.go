package model

// Achievement is one of the client's achievements: the Achievements
// prefab's list, each an asset with a name, a description and the
// triggers that unlock it. The platforms' own ids are left out.
type Achievement struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"desc,omitempty"`
	Icon        bool   `json:"icon,omitempty"`
	// Secret is hidden until unlocked.
	Secret bool `json:"secret,omitempty"`
	// Difficulty is m_difficultyRequirement: 1 any, 6 normal or harder, 7 hard.
	Difficulty int `json:"difficulty,omitempty"`
	// Dynamic is a list the game fills from its tables (every creature,
	// every food); Lenient a build check that forgives a little.
	Dynamic bool `json:"dynamic,omitempty"`
	Lenient bool `json:"lenient,omitempty"`

	Kills    []Need     `json:"kills,omitempty"`    // creatures to kill
	Crafts   []Need     `json:"crafts,omitempty"`   // items to craft
	Eaten    []Need     `json:"eaten,omitempty"`    // foods to eat
	PickedUp []Need     `json:"pickedUp,omitempty"` // items to pick up
	Caught   []Need     `json:"caught,omitempty"`   // pickables: the fish
	Pieces   []Need     `json:"pieces,omitempty"`   // pieces to place
	Stats    []StatNeed `json:"stats,omitempty"`    // player stats to reach
	Others   []string   `json:"others,omitempty"`   // achievement ids to have first
}

// Need is one thing of a kind, by the token the trigger names it with,
// the prefab that token belongs to when one is known, and how many.
type Need struct {
	Token  string `json:"token"`
	Prefab string `json:"prefab,omitempty"`
	Name   string `json:"name,omitempty"`
	Amount int    `json:"amount,omitempty"`
}

// StatNeed is a PlayerStatType to reach: its number, its enum name as
// the client spells it (BuiltPieces, DeathByTree) and the value the
// stat has to reach or pass.
type StatNeed struct {
	Stat   int     `json:"stat"`
	Key    string  `json:"key,omitempty"`
	Amount float64 `json:"amount"`
}
