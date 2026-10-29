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
	Others     []*Thing
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
