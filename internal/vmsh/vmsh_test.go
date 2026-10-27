package vmsh

import (
	"os"
	"strings"
	"testing"
)

// The foliage rule is the scene's own, and stays so: the two patterns must
// appear verbatim in scene.js, so whoever changes one finds the other.
func TestFoliageRuleIsTheScenes(t *testing.T) {
	js, err := os.ReadFile("../../cmd/npc-ui/static/scene.js")
	if err != nil {
		t.Skip("scene.js not beside this checkout")
	}
	for _, want := range []string{
		`/ashlandstree|ashlandsbranch|ashlandslog|charred|ember|burnt/i`,
		`/leaf|leaves|needle|foliage|branch|shrub|bush|fern|grass|vine|ivy|moss/i`,
	} {
		if !strings.Contains(string(js), want) {
			t.Errorf("scene.js no longer carries %s; the rule has two copies now", want)
		}
	}
	for material, want := range map[string]bool{
		"beech_leaf": true, "PineTree_01#foliage": true, "PineTree_01#trunk": false,
		"beech_bark": false, "AshlandsTree_branch_mat": false, "rock1_copper": false, "stump": false,
	} {
		if got := Foliage(material); got != want {
			t.Errorf("Foliage(%q) = %v, want %v", material, got, want)
		}
	}
}

// A real file, when the checkout has the meshes beside it.
func TestLoadsTheSceneMesh(t *testing.T) {
	lib := NewLibrary("../../meshes")
	m, ok := lib.Get("Beech1")
	if !ok {
		t.Skip("meshes/ not beside this checkout")
	}
	if len(m.Verts)%3 != 0 || len(m.Idx)%3 != 0 || len(m.Runs) == 0 {
		t.Fatalf("Beech1: %d floats, %d indices, %d runs", len(m.Verts), len(m.Idx), len(m.Runs))
	}
	if _, ok := lib.Get("no_such_prefab"); ok {
		t.Error("a missing file read as a mesh")
	}
}
