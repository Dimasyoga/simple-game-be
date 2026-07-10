package engine

import "fmt"

// NewRun implements rules.md R1: load the authored Gameplay template (data,
// invariant for the run — no random beat rolling) and initialize empty
// canonical state. The run starts in "lobby": no clause runs and no
// narrator/clock is touched until StartRun (R1.3).
func NewRun(id, gameplayID string, mode string, gameplay Gameplay, characters []Character) *Run {
	return &Run{
		ID:         id,
		GameplayID: gameplayID,
		Gameplay:   gameplay,
		Mode:       mode,
		Status:     "lobby",
		Characters: characters,
		WorldState: WorldState{
			Flags:         map[string]any{},
			Relationships: map[string]int{},
		},
	}
}

// StartRun transitions a run from "lobby" to "active" (rules.md R1b/R3
// entry point). Host authorization (who is allowed to call this) is an
// API-layer concern per contract.md's 409 semantics — engine only enforces
// the state machine itself.
func StartRun(run *Run) error {
	if run.Status != "lobby" {
		return fmt.Errorf("engine: cannot start run in status %q", run.Status)
	}
	run.Status = "active"
	return nil
}
