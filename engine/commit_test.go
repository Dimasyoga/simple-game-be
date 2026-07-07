package engine

import "testing"

func TestCommitFoldsDeltasLosslesslyAndAdvancesClause(t *testing.T) {
	run := &Run{
		Characters: []Character{
			{ID: "pc1", Stats: Stats{HP: 10}},
		},
	}
	resolved := []ResolvedAction{
		{
			CharacterID: "pc1",
			Deltas: []StateDelta{
				{Op: "hp", Target: "pc1", Value: -3},
				{Op: "add_flag", Target: "goblin_spared", Value: true},
				{Op: "morality", Target: "pc1", Value: 2},
				{Op: "kill", Target: "goblin1"},
			},
		},
	}

	Commit(run, resolved)

	if run.CurrentClauseIndex != 1 {
		t.Fatalf("expected clause index to advance to 1, got %d", run.CurrentClauseIndex)
	}
	if run.Characters[0].Stats.HP != 7 {
		t.Fatalf("expected hp 7 after -3 delta, got %d", run.Characters[0].Stats.HP)
	}
	if run.Characters[0].Personality.Morality != 2 {
		t.Fatalf("expected morality 2, got %d", run.Characters[0].Personality.Morality)
	}
	if run.WorldState.Flags["goblin_spared"] != true {
		t.Fatal("expected goblin_spared flag promoted into WorldState.Flags")
	}
	found := false
	for _, k := range run.WorldState.KillList {
		if k == "goblin1" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected goblin1 to be present in the kill list")
	}
}

func TestApplyDeltaIgnoresUnknownTarget(t *testing.T) {
	run := &Run{}
	// Should not panic even though "ghost" is not a known character.
	ApplyDelta(run, StateDelta{Op: "hp", Target: "ghost", Value: -5})
}
