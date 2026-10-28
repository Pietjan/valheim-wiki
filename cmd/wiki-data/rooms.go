package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/pietjan/valheim-wiki/internal/assets"
	"github.com/pietjan/valheim-wiki/internal/model"
)

// extractRooms reads the dungeon room lists: DungeonDB.m_roomLists names
// the _RoomList_* prefabs, each RoomList.m_rooms holds RoomData entries
// (a soft-referenced room prefab, enabled, theme). A DungeonGenerator
// picks among the enabled entries whose theme overlaps its own, so the
// entry's theme and flag are kept as the game's filter, and the Room
// component on the prefab gives the geometry and placement flags.
func extractRooms(x *X) error {
	guids, err := softGUIDs(x.SoftRef)
	if err != nil {
		return err
	}
	var lists []string
	x.EachMono([]string{"m_roomLists", "m_roomScenes"}, func(cab string, id int64, t map[string]any) {
		for _, l := range L(t, "m_roomLists") {
			if name := x.GameObjectName(cab, l); name != "" {
				lists = append(lists, name)
			}
		}
	})
	if len(lists) == 0 {
		// Without a DungeonDB, every _RoomList_* root.
		for name := range x.Roots {
			if strings.HasPrefix(name, "_RoomList") {
				lists = append(lists, name)
			}
		}
		sort.Strings(lists)
	}
	for _, list := range lists {
		root, ok := x.Roots[list]
		if !ok {
			fmt.Printf("    room list %s not loaded\n", list)
			continue
		}
		rl, ok := x.Find(root, "m_rooms")
		if !ok {
			continue
		}
		for _, e := range L(rl.Tree, "m_rooms") {
			em := assets.AsMap(e)
			prefab := x.softPrefab(guids, M(em, "m_prefab"), "")
			if prefab == "" {
				fmt.Printf("    %s: an entry without a prefab\n", list)
				continue
			}
			room := &model.Room{
				Prefab:  prefab,
				Theme:   strings.Join(x.themeNames(I(em, "m_theme")), "|"),
				List:    list,
				Enabled: B(em, "m_enabled"),
			}
			if g, ok := x.Roots[prefab]; ok {
				x.readRoomPrefab(g, room)
			} else {
				fmt.Printf("    room %s not loaded\n", prefab)
			}
			x.DB.Rooms = append(x.DB.Rooms, room)
		}
	}
	return nil
}

// readRoomPrefab fills the Room component and the contents.
func (x *X) readRoomPrefab(g assets.Ref, room *model.Room) {
	if t, ok := x.Find(g, "m_size", "m_theme", "m_endCap"); ok {
		size := M(t.Tree, "m_size")
		room.Size = [3]int{I(size, "x"), I(size, "y"), I(size, "z")}
		room.RoomTheme = strings.Join(x.themeNames(I(t.Tree, "m_theme")), "|")
		room.Weight = F(t.Tree, "m_weight")
		room.Entrance = B(t.Tree, "m_entrance")
		room.EndCap = B(t.Tree, "m_endCap")
		room.Divider = B(t.Tree, "m_divider")
		room.EndCapPrio = I(t.Tree, "m_endCapPrio")
		room.MinPlaceOrder = I(t.Tree, "m_minPlaceOrder")
		room.FaceCenter = B(t.Tree, "m_faceCenter")
		room.Perimeter = B(t.Tree, "m_perimeter")
	}
	room.Contents = x.contentsOf(g)
}

// roomThemes is Room.Theme: a bitmask that a DungeonGenerator and the
// room list entries share.
var roomThemes = map[int]string{
	1: "Crypt", 2: "SunkenCrypt", 4: "Cave", 8: "ForestCrypt", 0x10: "GoblinCamp",
	0x20: "MeadowsVillage", 0x40: "MeadowsFarm", 0x80: "DvergerTown", 0x100: "DvergerBoss",
	0x200: "ForestCryptHildir", 0x400: "CaveHildir", 0x800: "PlainsFortHildir",
	0x1000: "AshlandRuins", 0x2000: "FortressRuins", 0x4000: "Hole",
	0x10000: "NorthVillage", 0x20000: "MorkHalla",
}

// extraThemes names theme bits the decompiled enum does not know, after
// the room list whose entries use them (_RoomList_HalfBurried ->
// "HalfBurried"); built on first use.
var extraThemes map[int]string

// themeNames expands a Room.Theme mask.
func (x *X) themeNames(mask int) []string {
	if extraThemes == nil {
		extraThemes = map[int]string{}
		for _, name := range model.SortedKeys(x.Roots) {
			if !strings.HasPrefix(name, "_RoomList") {
				continue
			}
			rl, ok := x.Find(x.Roots[name], "m_rooms")
			if !ok {
				continue
			}
			for _, e := range L(rl.Tree, "m_rooms") {
				theme := I(assets.AsMap(e), "m_theme")
				for bit := 1; bit > 0 && bit <= theme; bit <<= 1 {
					if theme&bit != 0 && roomThemes[bit] == "" && extraThemes[bit] == "" {
						extraThemes[bit] = strings.TrimPrefix(name, "_RoomList_")
					}
				}
			}
		}
	}
	var out []string
	for bit := 1; bit > 0 && bit <= mask; bit <<= 1 {
		if mask&bit == 0 {
			continue
		}
		switch {
		case roomThemes[bit] != "":
			out = append(out, roomThemes[bit])
		case extraThemes[bit] != "":
			out = append(out, extraThemes[bit])
		default:
			out = append(out, fmt.Sprintf("Theme0x%x", bit))
		}
	}
	return out
}
