// Package narrator defines the single seam through which an LLM touches the
// game. Per spec.md §9: invoked only in PRESENT and NARRATE, emits prose
// only, never decides outcomes. Everything downstream of engine must depend
// on the Narrator interface, never a concrete model implementation.
package narrator

import "context"

// PresentContext is assembled by the engine for the PRESENT step: the
// rolling tier-2 summary, relevant tier-1 state, and the current beat.
type PresentContext struct {
	ProseSummary  string
	RelevantState map[string]any
	BeatPremise   string
	BeatType      string
}

// NarrateContext is assembled by the engine for the NARRATE step: already
// resolved, ordered actions the narrator must render faithfully.
type NarrateContext struct {
	ProseSummary    string
	RelevantState   map[string]any
	ResolvedActions []ResolvedActionSummary
}

// ResolvedActionSummary is the narrator-facing view of an engine.ResolvedAction.
// Kept separate from engine.ResolvedAction so this package has no dependency
// on the engine package (the seam stays one-directional: engine -> narrator).
type ResolvedActionSummary struct {
	CharacterID string
	Intent      string
	Outcome     string
	Summary     string // engine-provided plain-language delta summary
}

// Narrator produces prose only. It never returns state, dice, or deltas.
type Narrator interface {
	Present(ctx context.Context, pc PresentContext) (scenePlain string, err error)
	Narrate(ctx context.Context, nc NarrateContext) (narrationPlain string, err error)
}

// Stub is a canned-prose Narrator with no external calls. Phases 0-3 build
// and test the entire engine against this implementation; a real model is
// wired in behind the same interface starting Phase 4 (spec.md §9, tasks.md).
type Stub struct{}

func NewStub() *Stub { return &Stub{} }

func (Stub) Present(_ context.Context, pc PresentContext) (string, error) {
	return "[stub scene] " + pc.BeatPremise, nil
}

func (Stub) Narrate(_ context.Context, nc NarrateContext) (string, error) {
	out := "[stub narration]"
	for _, a := range nc.ResolvedActions {
		out += " " + a.CharacterID + ":" + a.Intent + "=" + a.Outcome
	}
	return out, nil
}
