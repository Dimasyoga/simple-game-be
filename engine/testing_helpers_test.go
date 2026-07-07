package engine

// constRNG always returns the same value, regardless of die size. Used where
// a test needs to pin the "random" roll to isolate the mechanic under test
// (e.g. proving a missing-item action can't succeed no matter what it rolls).
type constRNG struct{ val int }

func (c constRNG) Roll(int) int { return c.val }

func outcomeFor(resolved []ResolvedAction, characterID string) Outcome {
	for _, r := range resolved {
		if r.CharacterID == characterID {
			return r.Outcome
		}
	}
	return ""
}
