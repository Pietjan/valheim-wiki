package site

import (
	"fmt"
	"strings"

	"github.com/pietjan/valheim-wiki/internal/model"
)

// statLine says in a sentence what one stat trigger asks of the player,
// from the PlayerStatType's name and the value it has to reach, as the
// client counts it: a death stat ticks once per death of that kind, a
// build cluster is the most of one's own pieces of a tag standing at
// once, an explore stat is seconds spent past 10,350 m from the centre.
func statLine(n model.StatNeed) string {
	k, amt := n.Key, n.Amount
	times := func(verb string) string {
		switch {
		case amt <= 1:
			return verb
		case amt == 2:
			return verb + " twice"
		}
		return verb + " " + count(amt) + " times"
	}
	switch k {
	case "Deaths":
		return times("Die")
	case "PlayerSpawn":
		return times("Spawn into a world")
	case "MaxBuildingHeight":
		return "Build a piece " + count(amt) + " m above the ground beneath it"
	case "MaxBuildingHeightWorld":
		return "Build a piece at a height of " + count(amt) + " m, about " + count(amt-30) + " m above the sea"
	case "MaxComfort":
		return "Reach comfort " + count(amt)
	case "CraftFood":
		return "Cook " + count(amt) + " dishes"
	case "ConsecutiveDaysSurvivedMax":
		return "Survive " + count(amt) + " days in a row"
	case "FoodEaten":
		return "Eat " + count(amt) + " items of food"
	case "HarvestBerry":
		return "Pick " + count(amt) + " berries"
	case "HarvestMushroom":
		return "Pick " + count(amt) + " mushrooms"
	case "HarvestCrop":
		return "Harvest " + count(amt) + " crops"
	case "BeesHarvested":
		return "Take " + count(amt) + " honey from beehives"
	case "LeviathanSink":
		return times("Make a leviathan dive")
	case "DistanceSail":
		return "Sail " + km(amt) + " in all"
	case "DistanceSailHelm":
		return "Sail " + km(amt) + " at the helm"
	case "DistanceWalk":
		return "Walk " + km(amt)
	case "DistanceRun":
		return "Run " + km(amt)
	case "DistanceTraveled":
		return "Travel " + km(amt)
	case "TreasureBuriedFound":
		return "Dig up " + count(amt) + " buried treasures"
	case "Tree":
		return "Fell " + count(amt) + " trees"
	case "TreeChops":
		return "Swing at trees " + count(amt) + " times"
	case "BuiltPieces":
		return "Build " + count(amt) + " pieces"
	case "BuiltPiecesNoDebt":
		return "Build " + count(amt) + " pieces beyond the ones you took down"
	case "BossKillSolo":
		return times("Kill a boss alone")
	case "BossKillMultiplayer":
		return times("Kill a boss with other players about")
	case "CreatureTamed":
		return "Tame " + count(amt) + " creatures"
	case "PortalsUsed":
		return "Go through a portal " + count(amt) + " times"
	}
	if d, ok := strings.CutPrefix(k, "DeathByTreeTier"); ok {
		return times("Die to a tier " + d + " tree")
	}
	if d, ok := strings.CutPrefix(k, "DeathBy"); ok {
		return times("Die " + deathCause(d))
	}
	if t, ok := strings.CutPrefix(k, "Tree"); ok && treeKind[t] != "" {
		return times("Die under " + treeKind[t])
	}
	// A fish's tier is its quality, one more than the stars it shows.
	if t, ok := strings.CutPrefix(k, "FishCaughtTier"); ok {
		switch t {
		case "0", "1":
			return times("Catch a fish without a star")
		case "2":
			return times("Catch a 1-star fish")
		}
		return times("Catch a " + string(t[0]-1) + "-star fish")
	}
	if dir, ok := strings.CutPrefix(k, "Explore"); ok {
		noMap := strings.HasSuffix(dir, "NoMap")
		dir = strings.ToLower(strings.TrimSuffix(dir, "NoMap"))
		s := "Go more than 10,350 m " + dir + " of the world's centre"
		if amt > 1 {
			s = "Spend " + count(amt) + " seconds more than 10,350 m " + dir + " of the world's centre"
		}
		if noMap {
			s += ", in a world with the map turned off"
		}
		return s
	}
	if tag, ok := strings.CutPrefix(k, "BuildCluster"); ok {
		if amt <= 1 {
			return "Have a " + clusterTagOne(tag) + " of your own standing"
		}
		return "Have " + count(amt) + " of your own " + clusterTag(tag) + " standing at once"
	}
	return prettyKey(k) + " of at least " + count(amt)
}

// deathCauses finish "Die ..." for each DeathBy stat.
var deathCauses = map[string]string{
	"Undefined": "of an unknown cause", "EnemyHit": "to an enemy", "PlayerHit": "to another player", "Fall": "from a fall",
	"Drowning": "by drowning", "Burning": "in a fire", "Freezing": "of the cold", "Poisoned": "of poison", "Smoke": "of smoke",
	"Water": "in the water", "EdgeOfWorld": "by sailing off the edge of the world", "Impact": "from an impact", "Cart": "under a cart",
	"Tree": "under a falling tree", "Self": "by your own hand", "Structural": "under a collapsing build", "Turret": "to a ballista",
	"Boat": "under a boat", "Stalagtite": "under a falling stalactite", "Catapult": "to a catapult", "CinderFire": "in cinder fire",
	"AshlandsOcean": "in the boiling sea of the Ashlands", "Incinerator": "in the Obliterator", "DrawBridge": "under a drawbridge",
	"AshlandsLava": "in lava",
}

func deathCause(d string) string {
	if s := deathCauses[d]; s != "" {
		return s
	}
	return "of " + strings.ToLower(prettyKey(d))
}

var treeKind = map[string]string{
	"Fir": "a fir", "Oak": "an oak", "Pine": "a pine", "Ashlands": "an Ashlands tree", "YggdrasilShoot": "a Yggdrasil shoot",
	"Swamp": "an ancient tree", "Beech": "a beech", "Birch": "a birch", "SnowFir": "a snow-covered fir", "SnowPine": "a snow-covered pine",
}

// clusterTag is the plural of a piece usage tag.
func clusterTag(tag string) string {
	switch tag {
	case "Misc":
		return "miscellaneous pieces"
	case "Building":
		return "building pieces"
	case "Stairs":
		return "stairs"
	case "Doors":
		return "doors"
	case "Stacks":
		return "stacks"
	case "Meads":
		return "mead pieces"
	case "Feasts":
		return "feast pieces"
	case "Defense":
		return "defences"
	}
	return strings.ToLower(tag) + " pieces"
}

// clusterTagOne is the singular of a piece usage tag.
func clusterTagOne(tag string) string {
	switch tag {
	case "Misc":
		return "miscellaneous piece"
	case "Building":
		return "building piece"
	case "Stairs":
		return "flight of stairs"
	case "Doors":
		return "door"
	case "Stacks":
		return "stack"
	case "Meads":
		return "mead piece"
	case "Feasts":
		return "feast piece"
	case "Defense":
		return "defence"
	}
	return strings.ToLower(tag) + " piece"
}

// prettyKey spaces an enum name: BuiltPiecesNoDebt is Built pieces no debt.
func prettyKey(k string) string {
	var b strings.Builder
	for i, r := range k {
		if i > 0 && r >= 'A' && r <= 'Z' {
			b.WriteByte(' ')
			b.WriteRune(r - 'A' + 'a')
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// count writes a whole amount with thousands separated, 131,946.
func count(f float64) string {
	s := fmt.Sprintf("%.0f", f)
	if f != float64(int64(f)) {
		s = fmt.Sprintf("%.1f", f)
	}
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	whole, frac, _ := strings.Cut(s, ".")
	var b strings.Builder
	for i, c := range whole {
		if i > 0 && (len(whole)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(c)
	}
	out := b.String()
	if frac != "" {
		out += "." + frac
	}
	if neg {
		out = "-" + out
	}
	return out
}

// km writes metres as kilometres, 131.9 km.
func km(m float64) string {
	return fmt.Sprintf("%.1f km", m/1000)
}
