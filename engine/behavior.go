package engine

// ClauseBehavior is the type->behavior mapping from design.md/components.md:
// a clause's authored Type alone decides whether it needs player input and
// whether an uncertain outcome should be rolled. v1 is type-driven with
// fixed defaults per type — no per-clause override flags yet.
type ClauseBehavior struct {
	RequiresInput bool
	RequiresRoll  bool
	// Narrates reports whether the clause runs the NARRATE step at all. A
	// setup clause is pure atmosphere (design.md): PRESENT sets the scene and
	// that is the whole beat — there is no resolved action worth narrating, so
	// NARRATE (narrate mode) is skipped entirely, even if authored
	// ScriptedDeltas silently mutate state at COMMIT.
	Narrates bool
}

// BehaviorFor returns the v1 default behavior for a clause type. An unknown
// type (e.g. a future type not yet wired into this table) defaults to
// requiring input but never rolling: it always blocks on real player input
// rather than silently auto-proceeding, but the engine never invents dice
// against a mapping it doesn't recognize.
func BehaviorFor(t ClauseType) ClauseBehavior {
	switch t {
	case ClauseSetup:
		// Pure atmosphere: no action to take, per design.md's own example of
		// a setup clause handing the party a starting item unconditionally.
		// Describe-only — the scene IS the beat, so NARRATE is skipped.
		return ClauseBehavior{RequiresInput: false, RequiresRoll: false, Narrates: false}
	case ClauseConflict:
		return ClauseBehavior{RequiresInput: true, RequiresRoll: true, Narrates: true}
	case ClauseResolution:
		return ClauseBehavior{RequiresInput: true, RequiresRoll: false, Narrates: true}
	default:
		return ClauseBehavior{RequiresInput: true, RequiresRoll: false, Narrates: true}
	}
}
