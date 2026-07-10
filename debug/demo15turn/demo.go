package main

import (
	"fmt"

	"simple-game-be/engine"
)

func demoSkeleton() []engine.SkeletonBeat {
	return []engine.SkeletonBeat{
		{Index: 0, Role: "setup", Premise: "the edge of the Sleepless Forest looms ahead"},
		{Index: 1, Role: "rising", Premise: "a narrow path winds through dense undergrowth"},
		{Index: 2, Role: "rising", Premise: "strange tracks mark the forest floor"},
		{Index: 3, Role: "rising", Premise: "a clearing holds the ruins of an old shrine"},
		{Index: 4, Role: "rising", Premise: "a pack of shadow wolves circles the clearing"},
		{Index: 5, Role: "rising", Premise: "a moss-covered archway blocks the trail"},
		{Index: 6, Role: "rising", Premise: "a locked gate guards the inner grove"},
		{Index: 7, Role: "rising", Premise: "a hermit in the grove offers cryptic advice"},
		{Index: 8, Role: "rising", Premise: "the path collapses into a sinkhole"},
		{Index: 9, Role: "rising", Premise: "a hidden spring restores vitality"},
		{Index: 10, Role: "rising", Premise: "a bridge of roots spans a ravine"},
		{Index: 11, Role: "climax", Premise: "the witch's illusions test your resolve"},
		{Index: 12, Role: "climax", Premise: "the heart of the forest reveals itself"},
		{Index: 13, Role: "resolution", Premise: "the forest's curse lifts"},
		{Index: 14, Role: "resolution", Premise: "dawn breaks over the clearing"},
	}
}

func demoBeatOverrides() map[int]engine.BeatType {
	return map[int]engine.BeatType{
		0:  engine.BeatDiscovery,
		1:  engine.BeatDiscovery,
		2:  engine.BeatDiscovery,
		3:  engine.BeatPuzzle,
		4:  engine.BeatCombat,
		5:  engine.BeatPuzzle,
		6:  engine.BeatPuzzle,
		7:  engine.BeatSocial,
		8:  engine.BeatSetback,
		9:  engine.BeatDiscovery,
		10: engine.BeatDiscovery,
		11: engine.BeatCombat,
		12: engine.BeatDiscovery,
		13: engine.BeatDiscovery,
		14: engine.BeatDiscovery,
	}
}

func demoBeatPool() []engine.BeatType {
	return []engine.BeatType{
		engine.BeatDiscovery,
		engine.BeatPuzzle,
		engine.BeatCombat,
		engine.BeatSocial,
		engine.BeatSetback,
	}
}

func demoCheck(_ engine.ParsedAction, c engine.Character, _ engine.BeatType, _ engine.SceneState) engine.CheckSpec {
	return engine.CheckSpec{DC: 10, StatModifier: c.Stats.Dexterity}
}

func demoEffect(a engine.ParsedAction, c engine.Character, _ engine.BeatType, outcome engine.Outcome) []engine.StateDelta {
	if outcome == engine.OutcomeFail {
		return nil
	}

	clauseIdx := -1
	for _, cond := range c.Status.Conditions {
		if len(cond) > 10 && cond[:10] == "clause_idx:" {
			var n int
			_, _ = fmt.Sscanf(cond, "clause_idx:%d", &n)
			clauseIdx = n
			break
		}
	}

	var item engine.Item
	switch clauseIdx {
	case 6:
		item = engine.Item{ID: "forest-key-" + a.CharacterID, Name: "forest key", Type: "key"}
	case 11:
		item = engine.Item{ID: "witch-robe-" + a.CharacterID, Name: "witch robe", Type: "armor"}
	default:
		item = engine.Item{ID: "trinket-" + a.CharacterID, Name: "trinket", Type: "curio"}
	}

	return []engine.StateDelta{
		{Op: "add_flag", Target: "cleared:" + a.CharacterID, Value: true},
		engine.DropItemDelta(item),
	}
}
