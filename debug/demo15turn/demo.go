package main

import (
	"fmt"
	"strings"

	"simple-game-be/engine"
)

func demoGameplay() engine.Gameplay {
	return engine.Gameplay{
		ID:    "demo15",
		Title: "The Sleepless Forest",
		Chapters: []engine.ChapterTemplate{
			{
				Index: 0,
				Title: "Into the Forest",
				Clauses: []engine.ClauseTemplate{
					{ChapterIndex: 0, Order: 0, Type: engine.ClauseSetup, Description: "the edge of the Sleepless Forest looms ahead"},
					{ChapterIndex: 0, Order: 1, Type: engine.ClauseSetup, Description: "a narrow path winds through dense undergrowth"},
					{ChapterIndex: 0, Order: 2, Type: engine.ClauseSetup, Description: "strange tracks mark the forest floor"},
					{ChapterIndex: 0, Order: 3, Type: engine.ClauseConflict, Description: "a clearing holds the ruins of an old shrine"},
					{ChapterIndex: 0, Order: 4, Type: engine.ClauseConflict, Description: "a pack of shadow wolves circles the clearing"},
				},
			},
			{
				Index: 1,
				Title: "The Inner Grove",
				Clauses: []engine.ClauseTemplate{
					{ChapterIndex: 1, Order: 0, Type: engine.ClauseConflict, Description: "a moss-covered archway blocks the trail"},
					{ChapterIndex: 1, Order: 1, Type: engine.ClauseConflict, Description: "a locked gate guards the inner grove"},
					{ChapterIndex: 1, Order: 2, Type: engine.ClauseSetup, Description: "a hermit in the grove offers cryptic advice"},
					{ChapterIndex: 1, Order: 3, Type: engine.ClauseSetup, Description: "the path collapses into a sinkhole"},
					{ChapterIndex: 1, Order: 4, Type: engine.ClauseSetup, Description: "a hidden spring restores vitality"},
				},
			},
			{
				Index:   2,
				Title:   "The Witch's Heart",
				IsFinal: true,
				Clauses: []engine.ClauseTemplate{
					{ChapterIndex: 2, Order: 0, Type: engine.ClauseSetup, Description: "a bridge of roots spans a ravine"},
					{ChapterIndex: 2, Order: 1, Type: engine.ClauseConflict, Description: "the witch's illusions test your resolve"},
					{ChapterIndex: 2, Order: 2, Type: engine.ClauseConflict, Description: "the heart of the forest reveals itself"},
					{ChapterIndex: 2, Order: 3, Type: engine.ClauseResolution, Description: "the forest's curse lifts"},
					{ChapterIndex: 2, Order: 4, Type: engine.ClauseResolution, Description: "dawn breaks over the clearing"},
				},
			},
		},
	}
}

func demoCheck(_ engine.ParsedAction, c engine.Character, _ engine.ClauseType, _ engine.SceneState) engine.CheckSpec {
	return engine.CheckSpec{DC: 10, StatModifier: c.Stats.Dexterity}
}

func demoEffect(a engine.ParsedAction, c engine.Character, _ engine.ClauseType, outcome engine.Outcome) []engine.StateDelta {
	if outcome == engine.OutcomeFail {
		return nil
	}

	var chapterIdx, clauseOrder int
	for _, cond := range c.Status.Conditions {
		if strings.HasPrefix(cond, "clause_idx:") {
			parts := strings.Split(cond[len("clause_idx:"):], ":")
			if len(parts) == 2 {
				fmt.Sscanf(parts[0], "%d", &chapterIdx)
				fmt.Sscanf(parts[1], "%d", &clauseOrder)
				break
			}
		}
	}

	var item engine.Item
	switch {
	case chapterIdx == 1 && clauseOrder == 1:
		item = engine.Item{ID: "forest-key-" + a.CharacterID, Name: "forest key", Type: "key"}
	case chapterIdx == 2 && clauseOrder == 1:
		item = engine.Item{ID: "witch-robe-" + a.CharacterID, Name: "witch robe", Type: "armor"}
	default:
		item = engine.Item{ID: "trinket-" + a.CharacterID, Name: "trinket", Type: "curio"}
	}

	return []engine.StateDelta{
		{Op: "add_flag", Target: "cleared:" + a.CharacterID, Value: true},
		engine.DropItemDelta(item),
	}
}
