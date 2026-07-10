package engine

// CheckSpec is the outcome of deciding what a legal action is checked
// against. It is produced by a CheckResolver, which encodes content-specific
// knowledge (DC tables, what counts as "against morality") that this package
// deliberately does not hardcode — RESOLVE only owns the deterministic
// mechanics around whatever spec it's given (rules.md R6).
type CheckSpec struct {
	DC              int
	StatModifier    int
	PersonalityBias int  // outcome bias from personality; never a veto (R6.2)
	AgainstMorality bool // if true, an alignment StateDelta is always queued
}

// CheckResolver decides the check for a legal action, given the *current*
// working scene state — so it can see earlier actions this clause (e.g. a
// target already alerted raises a stealth DC).
type CheckResolver func(action ParsedAction, character Character, clauseType ClauseType, scene SceneState) CheckSpec

// EffectResolver decides the StateDeltas an outcome produces. Applied to the
// working scene state immediately after the roll (or immediately, for a
// deterministic PROCEEDS action), so later actions in the same clause see it
// (R6.4).
type EffectResolver func(action ParsedAction, character Character, clauseType ClauseType, outcome Outcome) []StateDelta

// Resolve implements rules.md R6: process legal actions in initiativeOrder,
// mutating a working copy of sceneState as it goes. Illegal actions (from
// GATE) never reach a check — they resolve as a no-op FAIL, per the R4.3
// policy choice of "no-op/improvised fallback at RESOLVE" rather than
// blocking the whole clause. They still appear in the ordered output so the
// narrator can render "you reach for a gun you don't have."
//
// Whether a legal action rolls at all is decided once per clause by
// BehaviorFor(clauseType).RequiresRoll (R6.1): if false, the action
// deterministically PROCEEDS with no dice — effectFn still runs to emit any
// deltas, but roll/DC never enter the picture.
func Resolve(
	rng RNG,
	initiativeOrder []string,
	actions map[string]ParsedAction,
	characters map[string]Character,
	clauseType ClauseType,
	scene *SceneState,
	checkFn CheckResolver,
	effectFn EffectResolver,
) []ResolvedAction {
	if scene.Facts == nil {
		scene.Facts = map[string]any{}
	}
	requiresRoll := BehaviorFor(clauseType).RequiresRoll

	resolved := make([]ResolvedAction, 0, len(initiativeOrder))
	for _, id := range initiativeOrder {
		action, ok := actions[id]
		if !ok {
			continue // PASSED/TIMED_OUT: never entered GATE, no-op
		}

		if !action.Legal {
			resolved = append(resolved, ResolvedAction{
				CharacterID: id,
				Intent:      action.Intent,
				Outcome:     OutcomeFail,
			})
			continue
		}

		character := characters[id]

		if !requiresRoll {
			deltas := effectFn(action, character, clauseType, OutcomeProceeds)
			applyToScene(scene, deltas)
			resolved = append(resolved, ResolvedAction{
				CharacterID: id,
				Intent:      action.Intent,
				Outcome:     OutcomeProceeds,
				Deltas:      deltas,
			})
			continue
		}

		spec := checkFn(action, character, clauseType, *scene)

		raw := rng.Roll(20)
		total := raw + spec.StatModifier + spec.PersonalityBias
		outcome := mapOutcome(total, spec.DC)

		deltas := effectFn(action, character, clauseType, outcome)
		if spec.AgainstMorality {
			deltas = append(deltas, StateDelta{Op: "morality", Target: id, Value: spec.PersonalityBias})
		}
		applyToScene(scene, deltas)

		resolved = append(resolved, ResolvedAction{
			CharacterID: id,
			Intent:      action.Intent,
			Roll: &DiceResult{
				Die:      20,
				Raw:      raw,
				Modifier: spec.StatModifier,
				DC:       spec.DC,
				Total:    total,
			},
			Outcome: outcome,
			Deltas:  deltas,
		})
	}
	return resolved
}

// mapOutcome implements the R6.3 thresholds: total>=DC succeeds outright;
// [DC-2, DC) succeeds with a cost/complication; below that fails.
func mapOutcome(total, dc int) Outcome {
	switch {
	case total >= dc:
		return OutcomeSuccess
	case total >= dc-2:
		return OutcomePartial
	default:
		return OutcomeFail
	}
}

// applyToScene folds deltas into the clause-scoped working state so later
// actions this clause see earlier effects (R6.4). This is separate from
// COMMIT, which folds deltas into the canonical, cross-clause WorldState.
func applyToScene(scene *SceneState, deltas []StateDelta) {
	for _, d := range deltas {
		scene.Facts[d.Target] = d.Value
	}
}

// DropItemDelta is the convention an EffectResolver uses to place an item
// into the clause's droppedItems (unowned, pending a loot claim) rather than
// directly into a character's inventory or canonical WorldState. ApplyDelta
// has no case for "drop_item" — RunClause extracts it before COMMIT so it
// never lands in tier-1 by accident (schema.md invariant 3: a dropped item
// is in droppedItems or a character's inventory, never neither/both).
func DropItemDelta(item Item) StateDelta {
	return StateDelta{Op: "drop_item", Target: item.ID, Value: item}
}
