package engine

// NewRun implements rules.md R1: load the fixed skeleton, roll a BeatType
// per slot into a hidden beat deck, and initialize empty canonical state.
//
// overrides forces specific slots to a given BeatType (e.g. the climax slot
// always "combat" for the dragon) — R1.2 explicitly allows this. pool is the
// set of types available for un-overridden slots; rng picks among them.
//
// The deck is hidden from players by construction: nothing in this package
// serializes BeatDeck out over the contract surface (that boundary lives in
// api/, which must not expose it — see contract.md).
func NewRun(id string, skeleton []SkeletonBeat, characters []Character, rng RNG, pool []BeatType, overrides map[int]BeatType) *Run {
	deck := make([]BeatType, len(skeleton))
	for i, beat := range skeleton {
		if bt, ok := overrides[beat.Index]; ok {
			deck[i] = bt
			continue
		}
		deck[i] = pool[rng.Roll(len(pool))-1]
	}

	return &Run{
		ID:         id,
		Skeleton:   skeleton,
		BeatDeck:   deck,
		Characters: characters,
		WorldState: WorldState{
			Flags:         map[string]any{},
			Relationships: map[string]int{},
		},
		Status: "active",
	}
}
