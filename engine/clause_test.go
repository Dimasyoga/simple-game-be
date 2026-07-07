package engine

import (
	"context"
	"testing"

	"simple-game-be/narrator"
)

// TestClimaxBranchesOnAccumulatedTierOneState is the Phase 2 guardrail from
// tasks.md: a full multi-clause run where the climax's outcome depends on
// what happened in earlier clauses (a kill recorded in WorldState.KillList,
// an item sitting in a character's inventory) — not on anything the LLM
// remembers, since the narrator here is the stub and never sees dice/DCs.
func TestClimaxBranchesOnAccumulatedTierOneState(t *testing.T) {
	ctx := context.Background()
	skeleton := []SkeletonBeat{
		{Index: 0, Role: "setup", Premise: "a goblin blocks the path"},
		{Index: 1, Role: "rising", Premise: "a fire cloak glints in the goblin's lair"},
		{Index: 2, Role: "climax", Premise: "the dragon descends", Climax: true},
	}
	overrides := map[int]BeatType{0: BeatCombat, 1: BeatDiscovery, 2: BeatCombat}
	catalog := NewItemCatalog(Item{ID: "fire_cloak1", Name: "fire cloak"})

	checkFn := func(a ParsedAction, c Character, bt BeatType, scene SceneState) CheckSpec {
		switch a.Intent {
		case "attack goblin", "grab fire cloak":
			return CheckSpec{DC: 5, StatModifier: c.Stats.Dexterity}
		case "fight dragon":
			dc := 25
			if killed, _ := scene.Facts["killed:goblin1"].(bool); killed {
				dc -= 10 // the goblin can't warn the dragon if it's dead
			}
			if characterHasItem(c, "fire_cloak1") {
				dc -= 5 // fireproof cloak blunts the dragon's breath
			}
			return CheckSpec{DC: dc, StatModifier: c.Stats.Dexterity}
		}
		return CheckSpec{DC: 10, StatModifier: c.Stats.Dexterity}
	}
	effectFn := func(a ParsedAction, c Character, bt BeatType, outcome Outcome) []StateDelta {
		if outcome == OutcomeFail {
			return nil
		}
		switch a.Intent {
		case "attack goblin":
			return []StateDelta{{Op: "kill", Target: "goblin1"}}
		case "grab fire cloak":
			return []StateDelta{DropItemDelta(Item{ID: "fire_cloak1", Name: "fire cloak", Type: "armor"})}
		case "fight dragon":
			if outcome == OutcomeSuccess {
				return []StateDelta{{Op: "add_flag", Target: "dragon_slain", Value: true}}
			}
		}
		return nil
	}

	newHero := func() Character {
		return Character{ID: "hero", Name: "Hero", Stats: Stats{Dexterity: 5, HP: 10, MaxHP: 10}, Status: CharacterStatus{Alive: true}}
	}

	t.Run("prepared hero slays the dragon", func(t *testing.T) {
		run := NewRun("run1", skeleton, []Character{newHero()}, NewSeededRNG(1), []BeatType{BeatCombat}, overrides)
		rng := constRNG{val: 15}
		n := narrator.NewStub()

		mustRunClause(t, ctx, n, rng, run, catalog, checkFn, effectFn, ClauseInput{
			Inputs: map[string]ActionInput{"hero": {CharacterID: "hero", RawText: "attack goblin", TerminalState: InputSubmitted}},
		})
		mustRunClause(t, ctx, n, rng, run, catalog, checkFn, effectFn, ClauseInput{
			Inputs:        map[string]ActionInput{"hero": {CharacterID: "hero", RawText: "grab fire cloak", TerminalState: InputSubmitted}},
			LootClaimants: map[string][]string{"fire_cloak1": {"hero"}},
		})
		result := mustRunClause(t, ctx, n, rng, run, catalog, checkFn, effectFn, ClauseInput{
			Inputs: map[string]ActionInput{"hero": {CharacterID: "hero", RawText: "fight dragon", TerminalState: InputSubmitted}},
		})

		if run.WorldState.Flags["dragon_slain"] != true {
			t.Fatal("expected a prepared hero (dead goblin ally + fire cloak) to slay the dragon")
		}
		if run.Status != "ended" {
			t.Fatalf("expected run to end after the climax, got status %q", run.Status)
		}
		if run.CurrentClauseIndex != 3 {
			t.Fatalf("expected clause index 3 after 3 clauses, got %d", run.CurrentClauseIndex)
		}
		found := false
		for _, id := range run.WorldState.KillList {
			if id == "goblin1" {
				found = true
			}
		}
		if !found {
			t.Fatal("expected goblin1 to remain in the lossless kill list after the climax")
		}
		if result.ScenePlain == "" || result.NarrationPlain == "" {
			t.Fatal("expected the stub narrator to produce non-empty scene/narration text")
		}
	})

	t.Run("unprepared hero fails the dragon", func(t *testing.T) {
		run := NewRun("run2", skeleton, []Character{newHero()}, NewSeededRNG(1), []BeatType{BeatCombat}, overrides)
		rng := constRNG{val: 15}
		n := narrator.NewStub()

		mustRunClause(t, ctx, n, rng, run, catalog, checkFn, effectFn, ClauseInput{
			Inputs: map[string]ActionInput{"hero": {CharacterID: "hero", TerminalState: InputPassed}},
		})
		mustRunClause(t, ctx, n, rng, run, catalog, checkFn, effectFn, ClauseInput{
			Inputs: map[string]ActionInput{"hero": {CharacterID: "hero", TerminalState: InputPassed}},
		})
		mustRunClause(t, ctx, n, rng, run, catalog, checkFn, effectFn, ClauseInput{
			Inputs: map[string]ActionInput{"hero": {CharacterID: "hero", RawText: "fight dragon", TerminalState: InputSubmitted}},
		})

		if run.WorldState.Flags["dragon_slain"] == true {
			t.Fatal("expected an unprepared hero to fail the unbuffed dragon fight")
		}
	})
}

func mustRunClause(
	t *testing.T,
	ctx context.Context,
	n narrator.Narrator,
	rng RNG,
	run *Run,
	catalog ItemCatalog,
	checkFn CheckResolver,
	effectFn EffectResolver,
	in ClauseInput,
) ClauseResult {
	t.Helper()
	res, err := RunClause(ctx, n, rng, run, catalog, checkFn, effectFn, in)
	if err != nil {
		t.Fatalf("RunClause failed: %v", err)
	}
	return res
}
