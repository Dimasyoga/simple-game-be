package engine

import "math/rand"

// RNG is the sole source of randomness for dice, initiative, and loot rolls.
// Every call site must take one as a parameter (never reach for the global
// math/rand functions) so tests can force outcomes with a seeded instance.
type RNG interface {
	// Roll returns a value in [1, die] inclusive (e.g. Roll(20) for a d20).
	Roll(die int) int
}

type seededRNG struct {
	r *rand.Rand
}

// NewSeededRNG returns an RNG deterministic for a given seed.
func NewSeededRNG(seed int64) RNG {
	return &seededRNG{r: rand.New(rand.NewSource(seed))}
}

func (s *seededRNG) Roll(die int) int {
	if die <= 0 {
		panic("engine: Roll called with non-positive die")
	}
	return s.r.Intn(die) + 1
}
