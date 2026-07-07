package engine

import "sort"

// RollInitiative implements rules.md R5: initiative = d20 + dexterityModifier
// for every acting character, sorted descending, with a fully deterministic
// tiebreak (higher dexterity, then stable characterId order) — no ties are
// ever left unresolved.
//
// actingIDs must contain each acting character's ID exactly once; passing it
// explicitly (rather than deriving it by ranging a map) keeps RNG consumption
// order deterministic across runs.
func RollInitiative(rng RNG, characters map[string]Character, actingIDs []string) []string {
	type entry struct {
		characterID string
		total       int
		dexterity   int
	}

	entries := make([]entry, 0, len(actingIDs))
	for _, id := range actingIDs {
		c := characters[id]
		roll := rng.Roll(20)
		entries = append(entries, entry{
			characterID: id,
			total:       roll + c.Stats.Dexterity,
			dexterity:   c.Stats.Dexterity,
		})
	}

	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].total != entries[j].total {
			return entries[i].total > entries[j].total
		}
		if entries[i].dexterity != entries[j].dexterity {
			return entries[i].dexterity > entries[j].dexterity
		}
		return entries[i].characterID < entries[j].characterID
	})

	order := make([]string, len(entries))
	for i, e := range entries {
		order[i] = e.characterID
	}
	return order
}
