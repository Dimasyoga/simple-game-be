package api

import "simple-game-be/engine"

// demoSkeleton is a minimal built-in scenario. Actual game/story content
// authoring is out of scope for this backend (spec.md/tasks.md describe
// engine mechanics, not narrative content) — this exists only so a full
// story can run to completion over the real contract (tasks.md Phase 3
// gate) with the narrator stubbed.
func demoSkeleton() []engine.SkeletonBeat {
	return []engine.SkeletonBeat{
		{Index: 0, Role: "setup", Premise: "a locked door blocks the way forward"},
		{Index: 1, Role: "climax", Premise: "what lies beyond is revealed", Climax: true},
	}
}

func demoBeatOverrides() map[int]engine.BeatType {
	return map[int]engine.BeatType{0: engine.BeatPuzzle, 1: engine.BeatDiscovery}
}

func demoBeatPool() []engine.BeatType {
	return []engine.BeatType{engine.BeatDiscovery, engine.BeatPuzzle, engine.BeatCombat, engine.BeatSocial, engine.BeatSetback}
}

// demoCheck is a generic DC modified by dexterity — a placeholder until
// real per-scenario content exists.
func demoCheck(_ engine.ParsedAction, c engine.Character, _ engine.BeatType, _ engine.SceneState) engine.CheckSpec {
	return engine.CheckSpec{DC: 10, StatModifier: c.Stats.Dexterity}
}

// demoEffect records a generic per-character success flag and drops a small
// trophy item on success, so LOOT has something to exercise end-to-end.
func demoEffect(a engine.ParsedAction, _ engine.Character, _ engine.BeatType, outcome engine.Outcome) []engine.StateDelta {
	if outcome == engine.OutcomeFail {
		return nil
	}
	return []engine.StateDelta{
		{Op: "add_flag", Target: "cleared:" + a.CharacterID, Value: true},
		engine.DropItemDelta(engine.Item{ID: "trinket-" + a.CharacterID, Name: "trinket", Type: "curio"}),
	}
}
