package engine

import "testing"

func TestSeededRNGDeterministic(t *testing.T) {
	a := NewSeededRNG(42)
	b := NewSeededRNG(42)

	for i := 0; i < 20; i++ {
		ra, rb := a.Roll(20), b.Roll(20)
		if ra != rb {
			t.Fatalf("roll %d diverged: %d != %d", i, ra, rb)
		}
		if ra < 1 || ra > 20 {
			t.Fatalf("roll %d out of range: %d", i, ra)
		}
	}
}

func TestSeededRNGDifferentSeeds(t *testing.T) {
	a := NewSeededRNG(1)
	b := NewSeededRNG(2)

	same := true
	for i := 0; i < 20; i++ {
		if a.Roll(20) != b.Roll(20) {
			same = false
			break
		}
	}
	if same {
		t.Fatal("expected different seeds to diverge within 20 rolls")
	}
}
