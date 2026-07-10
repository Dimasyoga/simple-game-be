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
	gameplay := Gameplay{
		ID: "dragon",
		Chapters: []ChapterTemplate{
			{Index: 0, Title: "The Goblin", Clauses: []ClauseTemplate{
				{ChapterIndex: 0, Order: 0, Type: ClauseConflict, Description: "a goblin blocks the path"},
			}},
			{Index: 1, Title: "The Lair", Clauses: []ClauseTemplate{
				{ChapterIndex: 1, Order: 0, Type: ClauseConflict, Description: "a fire cloak glints in the goblin's lair"},
			}},
			{Index: 2, Title: "The Dragon", IsFinal: true, Clauses: []ClauseTemplate{
				{ChapterIndex: 2, Order: 0, Type: ClauseConflict, Description: "the dragon descends"},
			}},
		},
	}
	catalog := NewItemCatalog(Item{ID: "fire_cloak1", Name: "fire cloak"})

	checkFn := func(a ParsedAction, c Character, bt ClauseType, scene SceneState) CheckSpec {
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
	effectFn := func(a ParsedAction, c Character, bt ClauseType, outcome Outcome) []StateDelta {
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
		run := NewRun("run1", "dragon", "single", gameplay, []Character{newHero()})
		if err := StartRun(run); err != nil {
			t.Fatalf("StartRun failed: %v", err)
		}
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
		if run.ChapterIndex != 2 {
			t.Fatalf("expected the final chapter (index 2) after 3 chapters, got %d", run.ChapterIndex)
		}
		if len(run.ChapterSummaries) != 3 {
			t.Fatalf("expected one chapter summary per completed chapter (3), got %d", len(run.ChapterSummaries))
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
		run := NewRun("run2", "dragon", "single", gameplay, []Character{newHero()})
		if err := StartRun(run); err != nil {
			t.Fatalf("StartRun failed: %v", err)
		}
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

// TestNoInputClauseAppliesScriptedDeltasWithoutARoll is the design.md/
// schema.md guardrail for a requires_input=false clause (setup, by v1's
// BehaviorFor default): it must apply its authored ScriptedDeltas
// unconditionally, produce a PROCEEDS outcome with no dice, and still let a
// scripted item drop flow through LOOT — all with no ActionInput supplied at
// all, since there's no window to collect one from.
func TestNoInputClauseAppliesScriptedDeltasWithoutARoll(t *testing.T) {
	ctx := context.Background()
	gameplay := Gameplay{
		ID: "starting-gift",
		Chapters: []ChapterTemplate{
			{Index: 0, Title: "The Gift", IsFinal: true, Clauses: []ClauseTemplate{
				{
					ChapterIndex: 0, Order: 0, Type: ClauseSetup,
					Description: "the party is handed a satchel before setting out",
					ScriptedDeltas: []StateDelta{
						DropItemDelta(Item{ID: "satchel1", Name: "satchel", Type: "gear"}),
					},
				},
			}},
		},
	}

	run := NewRun("run1", "starting-gift", "single", gameplay, []Character{
		{ID: "hero", Stats: Stats{Dexterity: 5, HP: 10, MaxHP: 10}, Status: CharacterStatus{Alive: true}},
	})
	if err := StartRun(run); err != nil {
		t.Fatalf("StartRun failed: %v", err)
	}

	rng := constRNG{val: 20} // if RESOLVE ever rolled, this would trivially succeed — it must not roll at all
	checkFn := func(ParsedAction, Character, ClauseType, SceneState) CheckSpec {
		t.Fatal("checkFn must never be called for a requires_roll=false clause")
		return CheckSpec{}
	}
	effectFn := func(ParsedAction, Character, ClauseType, Outcome) []StateDelta {
		t.Fatal("effectFn must never be called for a requires_input=false clause (no ActionInput exists to drive it)")
		return nil
	}

	result := mustRunClause(t, ctx, narrator.NewStub(), rng, run, NewItemCatalog(), checkFn, effectFn, ClauseInput{
		LootClaimants: map[string][]string{"satchel1": {"hero"}},
	})

	if run.Status != "ended" {
		t.Fatalf("expected the run to end after its only (final) chapter, got status %q", run.Status)
	}
	if len(run.ChapterSummaries) != 1 {
		t.Fatalf("expected exactly one chapter summary, got %d", len(run.ChapterSummaries))
	}
	if len(result.LootClaims) != 1 || result.LootClaims[0].ResolvedTo != "hero" {
		t.Fatalf("expected the scripted item drop to be claimed by hero, got %+v", result.LootClaims)
	}
	found := false
	for _, it := range run.Characters[0].Inventory {
		if it.ID == "satchel1" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected the scripted satchel to land in hero's inventory with no roll and no player input")
	}
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
