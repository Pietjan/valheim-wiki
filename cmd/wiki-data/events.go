package main

import (
	"github.com/pietjan/valheim-wiki/internal/assets"
	"github.com/pietjan/valheim-wiki/internal/model"
)

// extractEvents reads the raids: RandEventSystem.m_events on _GameMain
// and the LocationList.m_events the expansions add. Every so often
// (m_eventIntervalMin minutes, m_eventChance percent) the game picks a
// random enabled event whose global keys the players have earned, near a
// player base in one of its biomes, and runs its spawn rules for
// Duration seconds. Those rules go into the DB's spawns with Source
// "event:<name>" and are listed on the event by id.
func extractEvents(x *X) error {
	seen := map[string]bool{}
	read := func(cab string, v any) {
		for _, e := range assets.AsList(v) {
			m := assets.AsMap(e)
			name := S(m, "m_name")
			if name == "" || seen[name] {
				continue
			}
			seen[name] = true
			x.DB.Events = append(x.DB.Events, x.event(cab, m))
		}
	}
	x.EachMono([]string{"m_events", "m_eventChance"}, func(cab string, id int64, t map[string]any) { read(cab, t["m_events"]) })
	x.EachMono([]string{"m_events", "m_locations", "m_sortOrder"}, func(cab string, id int64, t map[string]any) { read(cab, t["m_events"]) })
	return nil
}

// event reads one RandomEvent. m_devDisabled is an editor flag the game
// never checks, so Enabled is m_enabled alone.
func (x *X) event(cab string, m map[string]any) *model.Event {
	name := S(m, "m_name")
	ev := &model.Event{
		Name:               name,
		Enabled:            B(m, "m_enabled"),
		Random:             B(m, "m_random"),
		Duration:           F(m, "m_duration"),
		NearBaseOnly:       B(m, "m_nearBaseOnly"),
		Range:              F(m, "m_eventRange"),
		Biomes:             Biomes(I(m, "m_biome")),
		RequiredKeys:       Strs(m["m_requiredGlobalKeys"]),
		NotKeys:            Strs(m["m_notRequiredGlobalKeys"]),
		PlayerKeysAny:      Strs(m["m_altRequiredPlayerKeysAny"]),
		PlayerKeysAll:      Strs(m["m_altRequiredPlayerKeysAll"]),
		NotPlayerKeys:      Strs(m["m_altNotRequiredPlayerKeys"]),
		StandaloneInterval: F(m, "m_standaloneInterval"),
		StandaloneChance:   F(m, "m_standaloneChance"),
		PauseIfNoPlayer:    B(m, "m_pauseIfNoPlayerInArea"),
		StartMessage:       x.Text(S(m, "m_startMessage")),
		EndMessage:         x.Text(S(m, "m_endMessage")),
		ForceEnv:           S(m, "m_forceEnvironment"),
		ForceMusic:         S(m, "m_forceMusic"),
	}
	for _, e := range L(m, "m_altRequiredKnownItems") {
		if n := x.GameObjectName(cab, e); n != "" {
			ev.RequiredItems = append(ev.RequiredItems, n)
		}
	}
	for _, e := range L(m, "m_altRequiredNotKnownItems") {
		if n := x.GameObjectName(cab, e); n != "" {
			ev.NotKnownItems = append(ev.NotKnownItems, n)
		}
	}
	ev.Spawns = x.spawnList(cab, m["m_spawn"], "event:"+name)
	return ev
}
