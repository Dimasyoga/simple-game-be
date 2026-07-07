package engine

import "testing"

// TestLootNeverDuplicatesOrLosesItem is the Phase 1 guardrail from tasks.md:
// with >=2 claimants, resolution always yields exactly one owner and the
// item is never duplicated or lost, across a spread of seeds.
func TestLootNeverDuplicatesOrLosesItem(t *testing.T) {
	for seed := int64(0); seed < 50; seed++ {
		rng := NewSeededRNG(seed)
		clause := &Clause{DroppedItems: []Item{{ID: "sword1", Name: "sword"}}}
		characters := map[string]Character{
			"pc1": {ID: "pc1"},
			"pc2": {ID: "pc2"},
			"pc3": {ID: "pc3"},
		}
		claimants := []string{"pc1", "pc2", "pc3"}

		claim := ResolveLoot(rng, "sword1", claimants, LootByRoll, nil)
		if claim.ResolvedTo == "" {
			t.Fatalf("seed %d: expected exactly one owner, got none", seed)
		}
		ApplyLootClaim(clause, characters, claim)

		if len(clause.DroppedItems) != 0 {
			t.Fatalf("seed %d: item should be removed from droppedItems after resolution", seed)
		}
		owners := 0
		for id, c := range characters {
			for _, it := range c.Inventory {
				if it.ID == "sword1" {
					owners++
					if id != claim.ResolvedTo {
						t.Fatalf("seed %d: item ended up with %s, not resolved winner %s", seed, id, claim.ResolvedTo)
					}
				}
			}
		}
		if owners != 1 {
			t.Fatalf("seed %d: expected exactly one owner, got %d", seed, owners)
		}
	}
}

func TestLootRollDeterministicTiebreak(t *testing.T) {
	rng := constRNG{val: 10} // every claimant rolls identically -> forces a tie
	claim := ResolveLoot(rng, "item1", []string{"pc3", "pc1", "pc2"}, LootByRoll, nil)
	if claim.ResolvedTo != "pc1" {
		t.Fatalf("expected alphabetically-first claimant to win the tie, got %s", claim.ResolvedTo)
	}
}

func TestLootByInitiative(t *testing.T) {
	claim := ResolveLoot(nil, "item1", []string{"pc3", "pc1"}, LootByInitiative, []string{"pc2", "pc1", "pc3"})
	if claim.ResolvedTo != "pc1" {
		t.Fatalf("expected earliest-in-initiativeOrder claimant to win, got %s", claim.ResolvedTo)
	}
}

func TestLootSingleClaimantIsTrivial(t *testing.T) {
	claim := ResolveLoot(nil, "item1", []string{"pc1"}, LootByRoll, nil)
	if claim.ResolvedTo != "pc1" {
		t.Fatalf("expected sole claimant to win trivially, got %q", claim.ResolvedTo)
	}
}

func TestLootZeroClaimantsLeavesItemUnresolved(t *testing.T) {
	clause := &Clause{DroppedItems: []Item{{ID: "sword1", Name: "sword"}}}
	claim := ResolveLoot(nil, "sword1", nil, LootByRoll, nil)
	if claim.ResolvedTo != "" {
		t.Fatalf("expected no resolution with zero claimants, got %q", claim.ResolvedTo)
	}
	ApplyLootClaim(clause, map[string]Character{}, claim)
	if len(clause.DroppedItems) != 1 {
		t.Fatal("expected item to remain in droppedItems when unclaimed")
	}
}
