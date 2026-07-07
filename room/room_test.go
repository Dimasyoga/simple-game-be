package room

import (
	"context"
	"testing"
	"time"

	"simple-game-be/engine"
	"simple-game-be/narrator"
)

// TestRoomRunNextClauseCompletesDespiteStalledPlayer is the Phase 3 gate
// from tasks.md: a player that stalls forever must not block a clause from
// completing — the timeout must fire and the pipeline must run to COMMIT.
func TestRoomRunNextClauseCompletesDespiteStalledPlayer(t *testing.T) {
	skeleton := []engine.SkeletonBeat{{Index: 0, Role: "setup", Premise: "a chest appears"}}
	run := engine.NewRun(
		"run1", skeleton,
		[]engine.Character{
			{ID: "pc1", Status: engine.CharacterStatus{Alive: true}},
			{ID: "pc2", Status: engine.CharacterStatus{Alive: true}},
		},
		engine.NewSeededRNG(1), []engine.BeatType{engine.BeatDiscovery}, nil,
	)

	r := NewRoom(run, RealClock(), 30*time.Millisecond, 30*time.Millisecond)

	checkFn := func(engine.ParsedAction, engine.Character, engine.BeatType, engine.SceneState) engine.CheckSpec {
		return engine.CheckSpec{DC: 1, StatModifier: 99} // always succeeds if attempted
	}
	effectFn := func(a engine.ParsedAction, c engine.Character, bt engine.BeatType, outcome engine.Outcome) []engine.StateDelta {
		if outcome == engine.OutcomeSuccess {
			return []engine.StateDelta{{Op: "add_flag", Target: "chest_opened", Value: true}}
		}
		return nil
	}

	type outcome struct {
		res engine.ClauseResult
		err error
	}
	done := make(chan outcome, 1)
	go func() {
		res, err := r.RunNextClause(context.Background(), narrator.NewStub(), engine.NewSeededRNG(2), engine.NewItemCatalog(), checkFn, effectFn, engine.LootByRoll)
		done <- outcome{res, err}
	}()

	// Give the window a moment to open, then have exactly one of the two
	// acting characters respond; pc2 stalls forever.
	time.Sleep(5 * time.Millisecond)
	if !r.Submit("pc1", "open chest") {
		t.Fatal("expected pc1's submission to be accepted while the window is open")
	}

	select {
	case out := <-done:
		if out.err != nil {
			t.Fatalf("RunNextClause failed: %v", out.err)
		}
		if run.CurrentClauseIndex != 1 {
			t.Fatalf("expected the clause to complete and advance, got index %d", run.CurrentClauseIndex)
		}
		if run.WorldState.Flags["chest_opened"] != true {
			t.Fatal("expected pc1's successful action to be committed despite pc2 stalling")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("RunNextClause hung: a stalled player must not block clause completion")
	}
}

// TestRoomLootRaceResolvesToExactlyOneOwner is the Phase 3 guardrail from
// tasks.md at the room level: two players racing to claim the same dropped
// item must never hang LOOT and must never duplicate/lose the item.
func TestRoomLootRaceResolvesToExactlyOneOwner(t *testing.T) {
	skeleton := []engine.SkeletonBeat{{Index: 0, Role: "setup", Premise: "a gem glitters"}}
	run := engine.NewRun(
		"run1", skeleton,
		[]engine.Character{
			{ID: "pc1", Status: engine.CharacterStatus{Alive: true}},
			{ID: "pc2", Status: engine.CharacterStatus{Alive: true}},
		},
		engine.NewSeededRNG(1), []engine.BeatType{engine.BeatDiscovery}, nil,
	)

	r := NewRoom(run, RealClock(), 30*time.Millisecond, 40*time.Millisecond)

	checkFn := func(engine.ParsedAction, engine.Character, engine.BeatType, engine.SceneState) engine.CheckSpec {
		return engine.CheckSpec{DC: 1, StatModifier: 99}
	}
	effectFn := func(a engine.ParsedAction, c engine.Character, bt engine.BeatType, outcome engine.Outcome) []engine.StateDelta {
		if a.CharacterID != "pc1" || outcome != engine.OutcomeSuccess {
			return nil
		}
		return []engine.StateDelta{engine.DropItemDelta(engine.Item{ID: "gem1", Name: "gem"})}
	}

	type outcome struct {
		res engine.ClauseResult
		err error
	}
	done := make(chan outcome, 1)
	go func() {
		res, err := r.RunNextClause(context.Background(), narrator.NewStub(), engine.NewSeededRNG(2), engine.NewItemCatalog(), checkFn, effectFn, engine.LootByRoll)
		done <- outcome{res, err}
	}()

	time.Sleep(5 * time.Millisecond)
	r.Submit("pc1", "grab gem")
	r.Pass("pc2")

	// Both players race-claim the gem once it drops; poll ClaimLoot until
	// the window opens (it isn't open until after RESOLVE completes).
	claimDeadline := time.Now().Add(1 * time.Second)
	for time.Now().Before(claimDeadline) {
		gotPc1 := r.ClaimLoot("gem1", "pc1")
		gotPc2 := r.ClaimLoot("gem1", "pc2")
		if gotPc1 || gotPc2 {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}

	select {
	case out := <-done:
		if out.err != nil {
			t.Fatalf("RunNextClause failed: %v", out.err)
		}
		owners := 0
		for _, c := range run.Characters {
			for _, it := range c.Inventory {
				if it.ID == "gem1" {
					owners++
				}
			}
		}
		if owners > 1 {
			t.Fatalf("gem must never be duplicated, found %d owners", owners)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("RunNextClause hung during a loot race: not deadlock-free")
	}
}
