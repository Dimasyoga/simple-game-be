package room

import (
	"sort"
	"sync"
	"time"
)

// LootWindow implements the claim-collection half of rules.md R7: an
// exclusive resource with a short window where any number of characters may
// register interest, closing on a timeout goroutine that is guaranteed to
// fire — never on waiting for participants, so two (or more) players
// spam-claiming the same item can never hang it open. engine.ResolveLoot
// (already deadlock-free, Phase 1) decides the single owner afterward.
type LootWindow struct {
	mu        sync.Mutex
	clock     Clock
	deadline  time.Time
	claimants map[string]bool
	fired     bool
	done      chan struct{}
}

// NewLootWindow opens a claim window that closes after duration unless
// Close is called first (e.g. once every known-interested character has
// claimed).
func NewLootWindow(clock Clock, duration time.Duration) *LootWindow {
	w := &LootWindow{
		clock:     clock,
		deadline:  clock.Now().Add(duration),
		claimants: make(map[string]bool),
		done:      make(chan struct{}),
	}

	// clock.After is called synchronously so the timer is armed before
	// NewLootWindow returns (see the matching comment in barrier.go).
	timeoutCh := clock.After(duration)
	go func() {
		<-timeoutCh
		w.mu.Lock()
		defer w.mu.Unlock()
		w.fireLocked()
	}()

	return w
}

// Deadline is the server-issued instant the window closes absent an early Close.
func (w *LootWindow) Deadline() time.Time { return w.deadline }

// Claim registers characterID as wanting the item. Safe for concurrent use;
// repeated claims from the same character are idempotent. Returns false if
// the window already closed.
func (w *LootWindow) Claim(characterID string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.fired {
		return false
	}
	w.claimants[characterID] = true
	return true
}

// Close ends the window immediately (e.g. an early-exit policy), safe to
// call multiple times or concurrently with Claim.
func (w *LootWindow) Close() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.fireLocked()
}

func (w *LootWindow) fireLocked() {
	if w.fired {
		return
	}
	w.fired = true
	close(w.done)
}

// Wait blocks until the window closes and returns the sorted claimant IDs.
func (w *LootWindow) Wait() []string {
	<-w.done
	w.mu.Lock()
	defer w.mu.Unlock()
	ids := make([]string, 0, len(w.claimants))
	for id := range w.claimants {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}
