package engine

import "sort"

type LootMethod string

const (
	LootByRoll       LootMethod = "roll"
	LootByInitiative LootMethod = "initiative"
)

// ResolveLoot implements rules.md R7: resolve a dropped item to exactly one
// owner via a non-deadlocking method. 0 claimants leaves it unresolved
// (still sitting in droppedItems); 1 is trivial; ≥2 is the interesting case.
//
// "roll" rolls each claimant in a fixed (sorted) order so RNG consumption is
// deterministic regardless of map/slice ordering upstream, with ties broken
// by lowest characterId — this and initiativeOrder are both provably
// terminating, so LOOT can never hang.
func ResolveLoot(rng RNG, itemID string, claimants []string, method LootMethod, initiativeOrder []string) LootClaim {
	claim := LootClaim{ItemID: itemID, Claimants: claimants, Method: string(method)}

	switch len(claimants) {
	case 0:
		return claim
	case 1:
		claim.ResolvedTo = claimants[0]
		return claim
	}

	switch method {
	case LootByInitiative:
		claimantSet := make(map[string]bool, len(claimants))
		for _, id := range claimants {
			claimantSet[id] = true
		}
		for _, id := range initiativeOrder {
			if claimantSet[id] {
				claim.ResolvedTo = id
				break
			}
		}
	default: // LootByRoll
		sorted := append([]string(nil), claimants...)
		sort.Strings(sorted)
		bestRoll := -1
		for _, id := range sorted {
			r := rng.Roll(20)
			if r > bestRoll { // strict: ties keep the alphabetically-first claimant
				bestRoll = r
				claim.ResolvedTo = id
			}
		}
	}
	return claim
}

// ApplyLootClaim atomically moves the item out of the clause's droppedItems
// and into the winner's inventory. A dropped item is in exactly one place at
// all times (schema.md invariant 3): this is the only function that may move
// one, and it does so as a single mutation with no intermediate state where
// the item is in neither or both places.
func ApplyLootClaim(clause *Clause, characters map[string]Character, claim LootClaim) {
	if claim.ResolvedTo == "" {
		return
	}

	idx := -1
	for i, it := range clause.DroppedItems {
		if it.ID == claim.ItemID {
			idx = i
			break
		}
	}
	if idx == -1 {
		return
	}

	item := clause.DroppedItems[idx]
	clause.DroppedItems = append(clause.DroppedItems[:idx:idx], clause.DroppedItems[idx+1:]...)

	winner := characters[claim.ResolvedTo]
	winner.Inventory = append(winner.Inventory, item)
	characters[claim.ResolvedTo] = winner
}
