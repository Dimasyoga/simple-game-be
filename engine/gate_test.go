package engine

import "testing"

func TestGateRejectsMissingItem(t *testing.T) {
	catalog := NewItemCatalog(Item{ID: "gun1", Name: "gun"})
	characters := map[string]Character{"pc1": {ID: "pc1"}}
	inputs := map[string]ActionInput{
		"pc1": {CharacterID: "pc1", RawText: "shoot with gun", TerminalState: InputSubmitted},
	}

	parsed := Gate(inputs, characters, catalog)
	pa, ok := parsed["pc1"]
	if !ok {
		t.Fatal("expected a parsed action for pc1")
	}
	if pa.Legal {
		t.Fatal("expected illegal action: character has no gun")
	}
	if pa.RejectionReason != "no gun" {
		t.Fatalf("unexpected rejection reason: %q", pa.RejectionReason)
	}
}

func TestGateAcceptsOwnedItem(t *testing.T) {
	catalog := NewItemCatalog(Item{ID: "gun1", Name: "gun"})
	characters := map[string]Character{
		"pc1": {ID: "pc1", Inventory: []Item{{ID: "gun1", Name: "gun"}}},
	}
	inputs := map[string]ActionInput{
		"pc1": {CharacterID: "pc1", RawText: "shoot with gun", TerminalState: InputSubmitted},
	}

	parsed := Gate(inputs, characters, catalog)
	if !parsed["pc1"].Legal {
		t.Fatal("expected legal action: character owns the gun")
	}
}

func TestGateSkipsNoopInputs(t *testing.T) {
	inputs := map[string]ActionInput{
		"pc1": {CharacterID: "pc1", TerminalState: InputPassed},
		"pc2": {CharacterID: "pc2", TerminalState: InputTimedOut},
	}
	parsed := Gate(inputs, map[string]Character{}, NewItemCatalog())
	if len(parsed) != 0 {
		t.Fatalf("expected no parsed actions for PASSED/TIMED_OUT, got %d", len(parsed))
	}
}

// TestMissingItemActionNeverResolvesSuccess is the Phase 1 guardrail from
// tasks.md: even with an RNG rigged to always roll max and a check that
// would trivially succeed, an action GATE marked illegal must never reach
// RESOLVE as a SUCCESS.
func TestMissingItemActionNeverResolvesSuccess(t *testing.T) {
	catalog := NewItemCatalog(Item{ID: "gun1", Name: "gun"})
	characters := map[string]Character{"pc1": {ID: "pc1"}}
	inputs := map[string]ActionInput{
		"pc1": {CharacterID: "pc1", RawText: "shoot with gun", TerminalState: InputSubmitted},
	}
	parsed := Gate(inputs, characters, catalog)

	rng := constRNG{val: 20}
	checkFn := func(ParsedAction, Character, BeatType, SceneState) CheckSpec {
		return CheckSpec{DC: 1, StatModifier: 99} // would trivially succeed if ever reached
	}
	effectFn := func(ParsedAction, Character, BeatType, Outcome) []StateDelta { return nil }

	resolved := Resolve(rng, []string{"pc1"}, parsed, characters, BeatCombat, &SceneState{}, checkFn, effectFn)
	if len(resolved) != 1 {
		t.Fatalf("expected 1 resolved action, got %d", len(resolved))
	}
	if resolved[0].Outcome == OutcomeSuccess {
		t.Fatal("missing-item action must never resolve as SUCCESS")
	}
}
