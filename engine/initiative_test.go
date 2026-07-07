package engine

import (
	"reflect"
	"testing"
)

func TestRollInitiativeDeterministicTiebreak(t *testing.T) {
	// Same seed twice must give the same order.
	characters := map[string]Character{
		"pc1": {ID: "pc1", Stats: Stats{Dexterity: 3}},
		"pc2": {ID: "pc2", Stats: Stats{Dexterity: 3}},
		"pc3": {ID: "pc3", Stats: Stats{Dexterity: 1}},
	}
	ids := []string{"pc1", "pc2", "pc3"}

	orderA := RollInitiative(NewSeededRNG(7), characters, ids)
	orderB := RollInitiative(NewSeededRNG(7), characters, ids)
	if !reflect.DeepEqual(orderA, orderB) {
		t.Fatalf("same seed produced different orders: %v vs %v", orderA, orderB)
	}
}

func TestRollInitiativeTieBreaksByDexterityThenID(t *testing.T) {
	// constRNG forces every roll identical, so ordering is decided purely by
	// the deterministic tiebreak: higher dexterity, then characterId.
	characters := map[string]Character{
		"pc2": {ID: "pc2", Stats: Stats{Dexterity: 5}},
		"pc1": {ID: "pc1", Stats: Stats{Dexterity: 5}},
		"pc3": {ID: "pc3", Stats: Stats{Dexterity: 1}},
	}
	order := RollInitiative(constRNG{val: 10}, characters, []string{"pc2", "pc1", "pc3"})
	want := []string{"pc1", "pc2", "pc3"} // pc1/pc2 tie on total+dex -> lower id first; pc3 lowest dex last
	if !reflect.DeepEqual(order, want) {
		t.Fatalf("expected %v, got %v", want, order)
	}
}
