package engine

// Commit implements rules.md R9: fold every StateDelta from an already-
// resolved clause into canonical, cross-clause state (tier-1, lossless),
// then advance the clause index. Flag promotion (R9.2) is not a separate
// step here — it's just an "add_flag" delta among the others; whoever builds
// the ResolvedAction (the EffectResolver, in Phase 1) decides which nuance
// gets promoted. Prose compaction (tier-2) is out of scope until Phase 2
// wires memory alongside a real narrator.
func Commit(run *Run, resolved []ResolvedAction) {
	for _, ra := range resolved {
		for _, d := range ra.Deltas {
			ApplyDelta(run, d)
		}
	}
	run.CurrentClauseIndex++
}

// ApplyDelta is the ONLY function that mutates canonical WorldState or
// Character fields (schema.md invariant 2). Every StateDelta.Target that
// names a character must be a characterId; deltas for unknown Ops or
// characterIds are dropped rather than panicking, since malformed deltas are
// a producer bug to catch in tests, not a reason to crash the run.
func ApplyDelta(run *Run, d StateDelta) {
	switch d.Op {
	case "add_flag":
		if run.WorldState.Flags == nil {
			run.WorldState.Flags = map[string]any{}
		}
		run.WorldState.Flags[d.Target] = d.Value

	case "kill":
		run.WorldState.KillList = append(run.WorldState.KillList, d.Target)
		if idx := findCharacter(run.Characters, d.Target); idx != -1 {
			run.Characters[idx].Status.Alive = false
		}

	case "hp":
		if idx := findCharacter(run.Characters, d.Target); idx != -1 {
			if delta, ok := d.Value.(int); ok {
				run.Characters[idx].Stats.HP += delta
			}
		}

	case "add_item":
		if idx := findCharacter(run.Characters, d.Target); idx != -1 {
			if item, ok := d.Value.(Item); ok {
				run.Characters[idx].Inventory = append(run.Characters[idx].Inventory, item)
			}
		}

	case "morality":
		if idx := findCharacter(run.Characters, d.Target); idx != -1 {
			if delta, ok := d.Value.(int); ok {
				run.Characters[idx].Personality.Morality += delta
			}
		}

	case "relationship":
		if run.WorldState.Relationships == nil {
			run.WorldState.Relationships = map[string]int{}
		}
		if delta, ok := d.Value.(int); ok {
			run.WorldState.Relationships[d.Target] += delta
		}
	}
}

func findCharacter(characters []Character, id string) int {
	for i, c := range characters {
		if c.ID == id {
			return i
		}
	}
	return -1
}
