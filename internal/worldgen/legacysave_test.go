package worldgen

import (
	"os"
	"testing"
)

// The pre-upgrade save itself is 20 MB and not in the repository; the test
// runs where it is, and its counts are the ones the fixture under
// testdata/npcdev-upgrade was extracted with.
func TestReadLegacySave(t *testing.T) {
	const path = "../../docker/config/worlds_local/npcdev_backup_20260909-153420.db"
	save, err := os.ReadFile(path)
	if err != nil {
		t.Skip(err)
	}
	ex, err := ReadLegacySave(save, DefaultLocationRules())
	if err != nil {
		t.Fatal(err)
	}
	if len(ex.Built) != 1151 {
		t.Errorf("%d built sites, want 1151", len(ex.Built))
	}
	if len(ex.Generated) != 4627 {
		t.Errorf("%d generated zones, want 4627", len(ex.Generated))
	}
	if !ex.Generated[Zone{0, 0}] || !ex.Generated[Zone{38, 26}] {
		t.Errorf("the first and last zones of the list are missing")
	}
	if ex.Built[0].Rule.Prefab != "StartTemple" || ex.Built[0].Zone != (Zone{-3, 3}) {
		t.Errorf("first built site = %s at %v, want StartTemple at (-3,3)", ex.Built[0].Rule.Prefab, ex.Built[0].Zone)
	}
}
