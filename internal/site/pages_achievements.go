package site

import (
	"github.com/pietjan/valheim-wiki/internal/model"
)

// AchievementView is one achievement with its needs resolved to things.
type AchievementView struct {
	*model.Achievement
	Slug       string
	Icon       string // icon URL or ""
	Difficulty string
	Kills      []NeedView
	Crafts     []NeedView
	Eaten      []NeedView
	PickedUp   []NeedView
	Caught     []NeedView
	Pieces     []NeedView
	Stats      []StatLine
	Others     []*Thing
	// DynamicNote says, for an achievement the game fills in itself when
	// it runs, what the list shown is and is not.
	DynamicNote string
}

// StatLine is one stat trigger said in a sentence, with the stat and
// threshold kept for the page's progress script.
type StatLine struct {
	Text   string
	Key    string
	Stat   int
	Amount float64
}

// NeedView is one need with the thing it names, when the wiki has it.
type NeedView struct {
	Need  model.Need
	Thing *Thing
}

// resolveNeeds finds the thing each need names: the prefab the extractor
// matched, or a creature by token where the prefab is a raid's copy.
func (s *Site) resolveNeeds(needs []model.Need) []NeedView {
	ix := s.Index
	out := make([]NeedView, 0, len(needs))
	for _, n := range needs {
		v := NeedView{Need: n}
		if n.Prefab != "" {
			v.Thing = ix.Things[n.Prefab]
			if v.Thing == nil {
				v.Thing = ix.Things["loc:"+n.Prefab]
			}
			// A boss's later form answers to the boss's page.
			if base, ok := phaseOf(n.Prefab); ok && v.Thing == nil {
				v.Thing = ix.Things[base]
			}
		}
		// A trigger naming the prefab outright rather than a token.
		if v.Thing == nil {
			v.Thing = ix.Things[n.Token]
		}
		if v.Thing == nil {
			for _, c := range ix.Creatures {
				if c.Token == n.Token {
					v.Thing = ix.Things[c.Prefab]
					break
				}
			}
		}
		out = append(out, v)
	}
	return out
}

func (s *Site) pagesAchievements() error {
	ix := s.Index
	var views []AchievementView
	for _, a := range s.DB.Achievements {
		v := AchievementView{Achievement: a, Slug: Slug(a.ID)}
		if s.hasIcon("achievements", a.ID) {
			v.Icon = s.URL("icons/achievements/" + a.ID + ".png")
		}
		switch a.Difficulty {
		case 6:
			v.Difficulty = "normal difficulty or harder"
		case 7:
			v.Difficulty = "hard difficulty"
		}
		v.Kills = s.resolveNeeds(a.Kills)
		v.Crafts = s.resolveNeeds(a.Crafts)
		v.Eaten = s.resolveNeeds(a.Eaten)
		v.PickedUp = s.resolveNeeds(a.PickedUp)
		v.Caught = s.resolveNeeds(a.Caught)
		v.Pieces = s.resolveNeeds(a.Pieces)
		if a.Dynamic {
			// The game rebuilds a dynamic list from its tables when it runs,
			// so an entry naming nothing that exists is not a requirement.
			v.Crafts = known(v.Crafts)
			v.Eaten = known(v.Eaten)
			v.PickedUp = known(v.PickedUp)
			v.Pieces = known(v.Pieces)
			v.Kills = known(v.Kills)
			// The lists are filled as the game fills them when it runs, by
			// the extractor (see populateDynamic in cmd/wiki-data); the
			// snapshot the asset carries is thrown away as the game does.
			v.DynamicNote = dynamicNotes[a.ID]
		}
		for _, st := range a.Stats {
			v.Stats = append(v.Stats, StatLine{Text: statLine(st), Key: st.Key, Stat: st.Stat, Amount: st.Amount})
		}
		for _, id := range a.Others {
			if t := ix.Things["ach:"+id]; t != nil {
				v.Others = append(v.Others, t)
			}
		}
		views = append(views, v)
	}
	return s.write("achievements/", "achievements", &Page{Site: s, Title: "Achievements",
		Description: "Every achievement in Valheim and exactly what unlocks it, from the client's own definitions: the bosses, creatures, dishes, items and pieces each one counts.",
		Section:     "achievements", Path: "achievements/", Data: views})
}

// known keeps the needs that name something the wiki has.
// dynamicNotes say, per dynamically populated achievement, what the game
// fills its list with when it runs, which is what the page lists.
var dynamicNotes = map[string]string{
	"AllWeaponCraft":       "Filled when the game runs: every weapon any recipe makes.",
	"AllFoodCooked":        "Filled when the game runs: every food a recipe makes, and every cooked dish a cooking station or oven turns out.",
	"AllItemCraft":         "Filled when the game runs: every item a recipe makes at a crafting station. Things made by hand, like the club and the stone axe, are not counted.",
	"AllFoodEaten":         "Filled when the game runs: every edible item in the game's tables.",
	"AllBuildPieces":       "Filled when the game runs: every piece in the hammer's menu, hidden ones included, the repair entry aside. The hoe's and the cultivator's are not counted.",
	"FindAllTrophies":      "Filled when the game runs: every trophy in the game's tables, one per name.",
	"KillAllCreatures":     "Filled when the game runs: every creature the game marks as counting for achievements, one per name.",
	"KillAllCreaturesHard": "Filled when the game runs: every creature the game marks as counting for achievements, one per name.",
}

func known(list []NeedView) []NeedView {
	out := list[:0:0]
	for _, v := range list {
		if v.Thing != nil || v.Need.Prefab != "" {
			out = append(out, v)
		}
	}
	return out
}
