package room

import (
	"testing"
	"time"

	"simple-game-be/engine"
)

func TestBarrierFiresOnAllTerminalBeforeTimeout(t *testing.T) {
	clock := newFakeClock()
	b := NewBarrier(clock, []string{"pc1", "pc2"}, time.Hour)

	resultCh := make(chan map[string]engine.ActionInput, 1)
	go func() { resultCh <- b.Wait() }()

	if !b.Submit("pc1", "attack") {
		t.Fatal("expected pc1's submission to be accepted")
	}
	if !b.Pass("pc2") {
		t.Fatal("expected pc2's pass to be accepted")
	}

	select {
	case inputs := <-resultCh:
		if inputs["pc1"].TerminalState != engine.InputSubmitted {
			t.Fatalf("expected pc1 SUBMITTED, got %s", inputs["pc1"].TerminalState)
		}
		if inputs["pc2"].TerminalState != engine.InputPassed {
			t.Fatalf("expected pc2 PASSED, got %s", inputs["pc2"].TerminalState)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("barrier did not fire once all acting characters reached a terminal state")
	}
}

// TestBarrierMixedSubmitPassTimeout is the Phase 3 guardrail from
// tasks.md: mixed submitted/passed/idle-to-timeout must resolve cleanly,
// with PASSED and TIMED_OUT stored distinctly (rules.md R3.4).
func TestBarrierMixedSubmitPassTimeout(t *testing.T) {
	clock := newFakeClock()
	b := NewBarrier(clock, []string{"pc1", "pc2", "pc3"}, time.Hour)

	resultCh := make(chan map[string]engine.ActionInput, 1)
	go func() { resultCh <- b.Wait() }()

	b.Submit("pc1", "attack")
	b.Pass("pc2")
	// pc3 never responds — the timeout goroutine must still fire.
	clock.Fire()

	select {
	case inputs := <-resultCh:
		if inputs["pc1"].TerminalState != engine.InputSubmitted {
			t.Fatalf("expected pc1 SUBMITTED, got %s", inputs["pc1"].TerminalState)
		}
		if inputs["pc2"].TerminalState != engine.InputPassed {
			t.Fatalf("expected pc2 PASSED, got %s", inputs["pc2"].TerminalState)
		}
		if inputs["pc3"].TerminalState != engine.InputTimedOut {
			t.Fatalf("expected pc3 TIMED_OUT, got %s", inputs["pc3"].TerminalState)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("barrier did not fire on timeout: a stalled player must not hang the clause")
	}
}

// TestBarrierNeverHangsOnStalledPlayerRealClock is the Phase 3 guardrail
// from tasks.md using a real clock: a player stalling forever must not
// block the barrier past its deadline.
func TestBarrierNeverHangsOnStalledPlayerRealClock(t *testing.T) {
	b := NewBarrier(RealClock(), []string{"pc1", "pc2"}, 20*time.Millisecond)

	resultCh := make(chan map[string]engine.ActionInput, 1)
	go func() { resultCh <- b.Wait() }()

	b.Submit("pc1", "attack") // pc2 stalls forever

	select {
	case inputs := <-resultCh:
		if inputs["pc2"].TerminalState != engine.InputTimedOut {
			t.Fatalf("expected stalled pc2 to be TIMED_OUT, got %s", inputs["pc2"].TerminalState)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("barrier hung past its real deadline")
	}
}

func TestBarrierRejectsSubmissionAfterFiring(t *testing.T) {
	clock := newFakeClock()
	b := NewBarrier(clock, []string{"pc1"}, time.Hour)
	b.Submit("pc1", "attack") // fires immediately: sole acting character done
	b.Wait()

	if b.Submit("pc1", "attack again") {
		t.Fatal("expected submission after firing to be rejected")
	}
}

func TestBarrierRejectsNonActingCharacter(t *testing.T) {
	clock := newFakeClock()
	b := NewBarrier(clock, []string{"pc1"}, time.Hour)
	if b.Pass("ghost") {
		t.Fatal("expected pass from a non-acting character to be rejected")
	}
}

func TestBarrierEmptyActingFiresImmediately(t *testing.T) {
	clock := newFakeClock()
	b := NewBarrier(clock, nil, time.Hour)

	select {
	case inputs := <-waitAsync(b):
		if len(inputs) != 0 {
			t.Fatalf("expected no inputs, got %v", inputs)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("barrier with zero acting characters must fire immediately")
	}
}

func waitAsync(b *Barrier) <-chan map[string]engine.ActionInput {
	ch := make(chan map[string]engine.ActionInput, 1)
	go func() { ch <- b.Wait() }()
	return ch
}
