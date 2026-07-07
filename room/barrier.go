// Package room owns per-run concurrency: the server-authoritative clock,
// the timed input barrier, and the loot claim window (spec.md §7, rules.md
// R3/R7). Every primitive here is built so a stalled or absent player can
// never hang a clause — a timeout goroutine always guarantees the barrier's
// done channel eventually closes.
package room

import (
	"sync"
	"time"

	"simple-game-be/engine"
)

// Barrier implements rules.md R3: it fires when every acting character has
// reached a terminal state (SUBMITTED/PASSED) or when the deadline passes,
// whichever comes first. Submit/Pass never block; Wait blocks only on a
// channel a timeout goroutine is guaranteed to close.
type Barrier struct {
	mu       sync.Mutex
	clock    Clock
	deadline time.Time
	acting   map[string]bool
	inputs   map[string]engine.ActionInput
	fired    bool
	done     chan struct{}
}

// NewBarrier opens a window for exactly actingIDs, closing after duration
// unless everyone submits/passes first. An empty actingIDs fires immediately
// (nothing to wait for).
func NewBarrier(clock Clock, actingIDs []string, duration time.Duration) *Barrier {
	b := &Barrier{
		clock:    clock,
		deadline: clock.Now().Add(duration),
		acting:   make(map[string]bool, len(actingIDs)),
		inputs:   make(map[string]engine.ActionInput, len(actingIDs)),
		done:     make(chan struct{}),
	}
	for _, id := range actingIDs {
		b.acting[id] = true
	}

	if len(actingIDs) == 0 {
		b.fire()
		return b
	}

	// clock.After is called synchronously (not inside the goroutine below)
	// so the timer is registered before NewBarrier returns — callers using a
	// fake clock can rely on it being armed immediately, with no race
	// against a test driving the clock forward.
	timeoutCh := clock.After(duration)
	go func() {
		<-timeoutCh
		b.mu.Lock()
		defer b.mu.Unlock()
		if b.fired {
			return
		}
		for id := range b.acting {
			if _, ok := b.inputs[id]; !ok {
				b.inputs[id] = engine.ActionInput{
					CharacterID:   id,
					TerminalState: engine.InputTimedOut,
					SubmittedAt:   b.clock.Now().UnixMilli(),
				}
			}
		}
		b.fireLocked()
	}()

	return b
}

// Deadline is the server-issued instant the window closes absent full
// participation — this, not any client clock, is authoritative (contract.md).
func (b *Barrier) Deadline() time.Time { return b.deadline }

// Submit registers a SUBMITTED action for characterID. Returns false if the
// window already closed or characterID isn't an acting character this
// clause.
func (b *Barrier) Submit(characterID, rawText string) bool {
	return b.set(characterID, engine.InputSubmitted, rawText)
}

// Pass registers a PASSED (no-op, but distinct from TIMED_OUT) action.
func (b *Barrier) Pass(characterID string) bool {
	return b.set(characterID, engine.InputPassed, "")
}

func (b *Barrier) set(characterID string, state engine.InputTerminal, rawText string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.fired || !b.acting[characterID] {
		return false
	}

	b.inputs[characterID] = engine.ActionInput{
		CharacterID:   characterID,
		RawText:       rawText,
		TerminalState: state,
		SubmittedAt:   b.clock.Now().UnixMilli(),
	}

	if b.allTerminalLocked() {
		b.fireLocked()
	}
	return true
}

func (b *Barrier) allTerminalLocked() bool {
	for id := range b.acting {
		if _, ok := b.inputs[id]; !ok {
			return false
		}
	}
	return true
}

func (b *Barrier) fire() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.fireLocked()
}

func (b *Barrier) fireLocked() {
	if b.fired {
		return
	}
	b.fired = true
	close(b.done)
}

// Status reports each acting character's current terminal state, if any —
// used for the contract's input_status push. Characters with no entry yet
// are still pending.
func (b *Barrier) Status() map[string]engine.InputTerminal {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make(map[string]engine.InputTerminal, len(b.inputs))
	for id, in := range b.inputs {
		out[id] = in.TerminalState
	}
	return out
}

// Wait blocks until the barrier fires (all-terminal or timeout) and returns
// the final terminal ActionInput for every acting character.
func (b *Barrier) Wait() map[string]engine.ActionInput {
	<-b.done
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make(map[string]engine.ActionInput, len(b.inputs))
	for id, in := range b.inputs {
		out[id] = in
	}
	return out
}
