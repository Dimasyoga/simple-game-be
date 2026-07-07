package room

import (
	"sync"
	"testing"
	"time"
)

// TestLootWindowSpamClaimSameItem is the Phase 3 guardrail from tasks.md:
// two (or more) players spam-claiming the same item concurrently must never
// hang the window or corrupt its claimant set. Run with -race to confirm no
// data race, in addition to the deadlock-freedom check.
func TestLootWindowSpamClaimSameItem(t *testing.T) {
	clock := newFakeClock()
	w := NewLootWindow(clock, time.Hour)

	var wg sync.WaitGroup
	spam := func(characterID string) {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			w.Claim(characterID)
		}
	}
	wg.Add(2)
	go spam("pc1")
	go spam("pc2")
	wg.Wait()

	clock.Fire()

	select {
	case claimants := <-waitLootAsync(w):
		if len(claimants) != 2 || claimants[0] != "pc1" || claimants[1] != "pc2" {
			t.Fatalf("expected exactly [pc1 pc2], got %v", claimants)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("loot window hung under spam-claim: not deadlock-free")
	}
}

func TestLootWindowClosesOnTimeoutWithZeroClaimants(t *testing.T) {
	clock := newFakeClock()
	w := NewLootWindow(clock, time.Hour)
	clock.Fire()

	select {
	case claimants := <-waitLootAsync(w):
		if len(claimants) != 0 {
			t.Fatalf("expected no claimants, got %v", claimants)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("loot window did not close on timeout with zero claimants")
	}
}

func TestLootWindowClaimAfterCloseIsRejected(t *testing.T) {
	clock := newFakeClock()
	w := NewLootWindow(clock, time.Hour)
	w.Close()

	if w.Claim("pc1") {
		t.Fatal("expected a claim after Close to be rejected")
	}
}

func TestLootWindowNeverHangsRealClock(t *testing.T) {
	w := NewLootWindow(RealClock(), 20*time.Millisecond)
	w.Claim("pc1") // pc2 never claims; window must still close on time

	select {
	case claimants := <-waitLootAsync(w):
		if len(claimants) != 1 || claimants[0] != "pc1" {
			t.Fatalf("expected [pc1], got %v", claimants)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("loot window hung past its real deadline")
	}
}

func waitLootAsync(w *LootWindow) <-chan []string {
	ch := make(chan []string, 1)
	go func() { ch <- w.Wait() }()
	return ch
}
