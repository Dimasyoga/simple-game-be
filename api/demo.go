package api

import "simple-game-be/engine"

// demoGameplay is a minimal built-in gameplay template. Actual game/story
// content authoring is out of scope for this backend (spec.md/tasks.md
// describe engine mechanics, not narrative content) — this exists only so a
// full story can run to completion over the real contract (tasks.md Phase 3
// gate) with the narrator stubbed.
func demoGameplay() engine.Gameplay {
	return engine.Gameplay{
		ID:    "demo",
		Title: "The Locked Door",
		Chapters: []engine.ChapterTemplate{
			{
				Index: 0,
				Title: "The Door",
				Clauses: []engine.ClauseTemplate{
					{ChapterIndex: 0, Order: 0, Type: engine.ClauseConflict, Description: "a locked door blocks the way forward"},
				},
			},
			{
				Index:   1,
				Title:   "What Lies Beyond",
				IsFinal: true,
				Clauses: []engine.ClauseTemplate{
					{ChapterIndex: 1, Order: 0, Type: engine.ClauseConflict, Description: "what lies beyond is revealed"},
				},
			},
		},
	}
}

// demoCheck is a generic DC modified by dexterity — a placeholder until
// real per-scenario content exists.
func demoCheck(_ engine.ParsedAction, c engine.Character, _ engine.ClauseType, _ engine.SceneState) engine.CheckSpec {
	return engine.CheckSpec{DC: 10, StatModifier: c.Stats.Dexterity}
}

// demoEffect records a generic per-character success flag and drops a small
// trophy item on success (or on an unconditional PROCEEDS), so LOOT has
// something to exercise end-to-end.
func demoEffect(a engine.ParsedAction, _ engine.Character, _ engine.ClauseType, outcome engine.Outcome) []engine.StateDelta {
	if outcome == engine.OutcomeFail {
		return nil
	}
	return []engine.StateDelta{
		{Op: "add_flag", Target: "cleared:" + a.CharacterID, Value: true},
		engine.DropItemDelta(engine.Item{ID: "trinket-" + a.CharacterID, Name: "trinket", Type: "curio"}),
	}
}
