package engine

// ClauseBehavior is the type->behavior mapping from design.md/components.md:
// a clause's authored Type alone decides whether it needs player input and
// whether an uncertain outcome should be rolled. v1 is type-driven with
// fixed defaults per type — no per-clause override flags yet.
type ClauseBehavior struct {
	RequiresInput bool
	RequiresRoll  bool
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
		return ClauseBehavior{RequiresInput: false, RequiresRoll: false}
	case ClauseConflict:
		return ClauseBehavior{RequiresInput: true, RequiresRoll: true}
	case ClauseResolution:
		return ClauseBehavior{RequiresInput: true, RequiresRoll: false}
	default:
		return ClauseBehavior{RequiresInput: true, RequiresRoll: false}
	}
}
