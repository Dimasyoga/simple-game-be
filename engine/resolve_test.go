package engine

import "testing"

// TestHeadOnVsSneakResolvesByInitiativeOrder is the Phase 1 guardrail from
// tasks.md: conflicts are emergent from initiative ordering, not a separate
// resolver. Same inputs, same dice, different initiative order -> different
// outcome for the sneak, because the attacker's alert flag lands in the
// working scene state before or after the sneak check depending on order.
func TestHeadOnVsSneakResolvesByInitiativeOrder(t *testing.T) {
	characters := map[string]Character{
		"attacker": {ID: "attacker"},
		"sneaker":  {ID: "sneaker"},
	}
	actions := map[string]ParsedAction{
		"attacker": {CharacterID: "attacker", Intent: "attack head-on", Legal: true},
		"sneaker":  {CharacterID: "sneaker", Intent: "sneak past", Legal: true},
	}

	checkFn := func(a ParsedAction, c Character, bt BeatType, scene SceneState) CheckSpec {
		if a.CharacterID == "sneaker" {
			dc := 10
			if alerted, _ := scene.Facts["alerted"].(bool); alerted {
				dc = 20
			}
			return CheckSpec{DC: dc}
		}
		return CheckSpec{DC: 8}
	}
	effectFn := func(a ParsedAction, c Character, bt BeatType, outcome Outcome) []StateDelta {
		if a.CharacterID == "attacker" {
			return []StateDelta{{Op: "add_flag", Target: "alerted", Value: true}}
		}
		return nil
	}

	rng := constRNG{val: 15} // fixed roll: 15 >= 10 (succeeds unalerted), 15 < 18 (fails alerted, DC 20 - 2)

	resolvedAttackerFirst := Resolve(rng, []string{"attacker", "sneaker"}, actions, characters, BeatCombat, &SceneState{}, checkFn, effectFn)
	if got := outcomeFor(resolvedAttackerFirst, "sneaker"); got != OutcomeFail {
		t.Fatalf("attacker-first: expected sneak to fail once alerted, got %s", got)
	}

	resolvedSneakerFirst := Resolve(rng, []string{"sneaker", "attacker"}, actions, characters, BeatCombat, &SceneState{}, checkFn, effectFn)
	if got := outcomeFor(resolvedSneakerFirst, "sneaker"); got != OutcomeSuccess {
		t.Fatalf("sneaker-first: expected sneak to succeed before alert, got %s", got)
	}
}

// TestPersonalityBiasShiftsOddsQueuesDeltaNeverVetoes is the Phase 1
// guardrail from tasks.md: acting against alignment can succeed at a cost
// and always queues an alignment delta, but the input itself is never
// blocked (rules.md R6.2).
func TestPersonalityBiasShiftsOddsQueuesDeltaNeverVetoes(t *testing.T) {
	characters := map[string]Character{"pc1": {ID: "pc1"}}
	actions := map[string]ParsedAction{
		"pc1": {CharacterID: "pc1", Intent: "betray ally", Legal: true},
	}
	checkFn := func(ParsedAction, Character, BeatType, SceneState) CheckSpec {
		return CheckSpec{DC: 15, PersonalityBias: -5, AgainstMorality: true}
	}
	effectFn := func(ParsedAction, Character, BeatType, Outcome) []StateDelta { return nil }

	rng := constRNG{val: 18} // 18 - 5 = 13 < DC 15: bias is what tips this to a fail
	resolved := Resolve(rng, []string{"pc1"}, actions, characters, BeatSocial, &SceneState{}, checkFn, effectFn)

	if len(resolved) != 1 {
		t.Fatalf("expected 1 resolved action, got %d", len(resolved))
	}
	if resolved[0].Intent != "betray ally" {
		t.Fatal("personality must never block/veto the input itself")
	}
	if resolved[0].Outcome == OutcomeSuccess {
		t.Fatal("expected personality bias to push the total below the success threshold")
	}
	foundAlignmentDelta := false
	for _, d := range resolved[0].Deltas {
		if d.Op == "morality" {
			foundAlignmentDelta = true
		}
	}
	if !foundAlignmentDelta {
		t.Fatal("expected an alignment/morality delta to be queued for acting against morality")
	}
}
