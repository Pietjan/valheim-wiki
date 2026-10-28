package main

import (
	"fmt"
	"strings"

	"github.com/pietjan/valheim-wiki/internal/assets"
	"github.com/pietjan/valheim-wiki/internal/model"
)

// extractSkills reads the Skills component on the Player prefab: one
// SkillDef per skill with its icon, description and m_increseStep, the
// experience one use of the skill adds (Skills.Skill.Raise multiplies it
// by the action's factor and the world's skill gain rate).
func extractSkills(x *X) error {
	var skills Component
	found := false
	for _, g := range x.RootsAll["Player"] {
		if c, ok := x.Find(g, "m_skills", "m_DeathLowerFactor"); ok {
			skills, found = c, true
			break
		}
	}
	if !found {
		return fmt.Errorf("no Skills component on a Player prefab")
	}
	for _, e := range L(skills.Tree, "m_skills") {
		m := assets.AsMap(e)
		id := SkillName(I(m, "m_skill"))
		sk := &model.Skill{
			ID:          id,
			Name:        x.Text("$skill_" + strings.ToLower(id)),
			Description: x.Text(S(m, "m_description")),
			Step:        F(m, "m_increseStep"),
		}
		if ref, ok := x.Deref(skills.Ref.Cab, m["m_icon"]); ok {
			sk.Icon = true
			x.WantSprite("skills", id, ref)
		}
		x.DB.Skills = append(x.DB.Skills, sk)
	}
	return nil
}
