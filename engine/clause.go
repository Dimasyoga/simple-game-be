package engine

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"simple-game-be/narrator"
)

// ClauseInput is what a caller must have already collected before driving a
// clause: WINDOW's barrier (server clock, real concurrency, per-player
// timeouts) is a room/ concern (Phase 3). Here, Inputs is assumed final —
// every acting character already has a terminal ActionInput.
type ClauseInput struct {
	Inputs        map[string]ActionInput
	LootClaimants map[string][]string // itemId -> claimant characterIds
	LootMethod    LootMethod          // defaults to LootByRoll if unset
}

// ClauseResult is everything a transport layer (Phase 3) would push over the
// wire for this clause: the narrated scene/outcome text and which items got
// claimed by whom.
type ClauseResult struct {
	Clause         Clause
	ScenePlain     string
	NarrationPlain string
	LootClaims     []LootClaim
}

// ClauseInProgress is a clause paused between RESOLVE and LOOT. Room (Phase
// 3) needs to see DroppedItems() before LOOT so it can open a real,
// server-clocked claim window per item and collect claimants interactively,
// rather than requiring them all pre-supplied like ClauseInput does.
type ClauseInProgress struct {
	clause     Clause
	scenePlain string
	characters map[string]Character
}

// DroppedItems is what LOOT needs claimants for. Returns a copy; the
// original is only mutated by FinishClause.
func (c *ClauseInProgress) DroppedItems() []Item {
	return append([]Item(nil), c.clause.DroppedItems...)
}

// PresentClause runs PRESENT (R2) alone, before any input is collected.
// Splitting it out of BeginClause matters for Room (Phase 3): the contract
// shows players the scene, *then* opens the input window — the narrator
// call can't be deferred until after WINDOW the way BeginClause used to.
func PresentClause(ctx context.Context, n narrator.Narrator, run *Run) (string, error) {
	if run.CurrentClauseIndex >= len(run.Skeleton) {
		return "", fmt.Errorf("engine: clause index %d beyond skeleton length %d", run.CurrentClauseIndex, len(run.Skeleton))
	}
	beat := run.Skeleton[run.CurrentClauseIndex]
	beatType := run.BeatDeck[run.CurrentClauseIndex]
	scenePlain, err := n.Present(ctx, narrator.PresentContext{
		ProseSummary:  run.ProseSummary,
		RelevantState: worldStateAsMap(run.WorldState),
		BeatPremise:   beat.Premise,
		BeatType:      string(beatType),
	})
	if err != nil {
		return "", fmt.Errorf("engine: PRESENT: %w", err)
	}
	return scenePlain, nil
}

// BeginClause runs WINDOW -> GATE -> INITIATIVE -> RESOLVE (R3-R6) and stops
// before LOOT. scenePlain comes from a prior PresentClause call. inputs must
// already carry a terminal ActionInput per acting character — collecting
// them (the WINDOW barrier) is a room/ concern (R3).
func BeginClause(
	ctx context.Context,
	rng RNG,
	run *Run,
	catalog ItemCatalog,
	checkFn CheckResolver,
	effectFn EffectResolver,
	scenePlain string,
	inputs map[string]ActionInput,
) (*ClauseInProgress, error) {
	if run.CurrentClauseIndex >= len(run.Skeleton) {
		return nil, fmt.Errorf("engine: clause index %d beyond skeleton length %d", run.CurrentClauseIndex, len(run.Skeleton))
	}

	beatType := run.BeatDeck[run.CurrentClauseIndex]
	clause := Clause{Index: run.CurrentClauseIndex, BeatType: beatType}

	// WINDOW (R3) — barrier itself is Phase 3; inputs arrive pre-collected.
	clause.Phase = PhaseWindow
	clause.Inputs = inputs

	// GATE (R4)
	clause.Phase = PhaseGate
	characters := charactersByID(run)
	parsed := Gate(clause.Inputs, characters, catalog)

	// INITIATIVE (R5)
	clause.Phase = PhaseInitiative
	actingIDs := make([]string, 0, len(parsed))
	for id := range parsed {
		actingIDs = append(actingIDs, id)
	}
	sort.Strings(actingIDs) // deterministic RNG consumption order
	clause.InitiativeOrder = RollInitiative(rng, characters, actingIDs)

	// RESOLVE (R6) — scene is seeded from tier-1 so checks this clause (and
	// the climax, R10) can branch on accumulated canonical state, not just
	// what happened earlier in this same clause.
	clause.Phase = PhaseResolve
	clause.SceneState = SceneState{}
	seedSceneFromWorld(&clause.SceneState, run.WorldState)
	// Inject clause index into character conditions so EffectResolver can
	// read it for context-aware loot drops (demo use; harmless for production).
	idxTag := fmt.Sprintf("clause_idx:%d", run.CurrentClauseIndex)
	for id, c := range characters {
		c.Status.Conditions = append(c.Status.Conditions, idxTag)
		characters[id] = c
	}
	clause.ResolvedActions = Resolve(rng, clause.InitiativeOrder, parsed, characters, beatType, &clause.SceneState, checkFn, effectFn)
	// Clean up injected condition.
	for id, c := range characters {
		var filtered []string
		for _, cond := range c.Status.Conditions {
			if cond != idxTag {
				filtered = append(filtered, cond)
			}
		}
		c.Status.Conditions = filtered
		characters[id] = c
	}
	clause.DroppedItems = extractDroppedItems(clause.ResolvedActions)

	return &ClauseInProgress{clause: clause, scenePlain: scenePlain, characters: characters}, nil
}

// FinishClause runs LOOT -> NARRATE -> COMMIT (R7-R9) against a clause
// BeginClause paused. lootClaimants maps itemId -> claimant characterIds,
// however they were collected (pre-supplied by RunClause, or gathered
// interactively by a room.LootWindow per item).
func FinishClause(
	ctx context.Context,
	n narrator.Narrator,
	run *Run,
	cip *ClauseInProgress,
	rng RNG,
	lootClaimants map[string][]string,
	lootMethod LootMethod,
) (ClauseResult, error) {
	clause := cip.clause
	characters := cip.characters

	// LOOT (R7)
	clause.Phase = PhaseLoot
	lootClaims := make([]LootClaim, 0, len(clause.DroppedItems))
	for _, item := range append([]Item(nil), clause.DroppedItems...) {
		method := lootMethod
		if method == "" {
			method = LootByRoll
		}
		claim := ResolveLoot(rng, item.ID, lootClaimants[item.ID], method, clause.InitiativeOrder)
		ApplyLootClaim(&clause, characters, claim)
		lootClaims = append(lootClaims, claim)
	}
	syncCharacters(run, characters)

	// NARRATE (R8)
	clause.Phase = PhaseNarrate
	narrationPlain, err := n.Narrate(ctx, narrator.NarrateContext{
		ProseSummary:    run.ProseSummary,
		RelevantState:   worldStateAsMap(run.WorldState),
		ResolvedActions: summarizeResolved(clause.ResolvedActions, characters),
	})
	if err != nil {
		return ClauseResult{}, fmt.Errorf("engine: NARRATE: %w", err)
	}

	// COMMIT (R9)
	clause.Phase = PhaseCommit
	Commit(run, clause.ResolvedActions)
	run.ProseSummary = narrationPlain
	if run.CurrentClauseIndex >= len(run.Skeleton) {
		run.Status = "ended"
	}

	return ClauseResult{
		Clause:         clause,
		ScenePlain:     cip.scenePlain,
		NarrationPlain: narrationPlain,
		LootClaims:     lootClaims,
	}, nil
}

// RunClause drives one full clause through PRESENT -> WINDOW -> GATE ->
// INITIATIVE -> RESOLVE -> LOOT -> NARRATE -> COMMIT (spec.md §6, rules.md)
// in one call, with loot claimants already known. It is the only place the
// narrator is invoked, and only in PRESENT/NARRATE. Kept as a thin wrapper
// over BeginClause/FinishClause so non-interactive callers (tests, batch
// simulation) don't need to care about the split; room.Room uses the two
// halves directly to open a real claim window in between (Phase 3).
func RunClause(
	ctx context.Context,
	n narrator.Narrator,
	rng RNG,
	run *Run,
	catalog ItemCatalog,
	checkFn CheckResolver,
	effectFn EffectResolver,
	in ClauseInput,
) (ClauseResult, error) {
	scenePlain, err := PresentClause(ctx, n, run)
	if err != nil {
		return ClauseResult{}, err
	}
	cip, err := BeginClause(ctx, rng, run, catalog, checkFn, effectFn, scenePlain, in.Inputs)
	if err != nil {
		return ClauseResult{}, err
	}
	return FinishClause(ctx, n, run, cip, rng, in.LootClaimants, in.LootMethod)
}

func charactersByID(run *Run) map[string]Character {
	m := make(map[string]Character, len(run.Characters))
	for _, c := range run.Characters {
		m[c.ID] = c
	}
	return m
}

// syncCharacters writes LOOT's inventory mutations (made against the map
// copy) back into run.Characters before COMMIT applies its own deltas.
func syncCharacters(run *Run, m map[string]Character) {
	for i, c := range run.Characters {
		if updated, ok := m[c.ID]; ok {
			run.Characters[i] = updated
		}
	}
}

// seedSceneFromWorld gives RESOLVE (and the climax's CheckResolver, R10)
// visibility into accumulated tier-1 state without changing the
// CheckResolver/EffectResolver signatures: flags are copied as-is, and each
// killList entry becomes a "killed:<id>" fact.
func seedSceneFromWorld(scene *SceneState, world WorldState) {
	if scene.Facts == nil {
		scene.Facts = map[string]any{}
	}
	for k, v := range world.Flags {
		scene.Facts[k] = v
	}
	for _, id := range world.KillList {
		scene.Facts["killed:"+id] = true
	}
}

func worldStateAsMap(w WorldState) map[string]any {
	m := make(map[string]any, len(w.Flags)+2)
	for k, v := range w.Flags {
		m[k] = v
	}
	m["killList"] = append([]string(nil), w.KillList...)
	m["relationships"] = w.Relationships
	return m
}

func extractDroppedItems(resolved []ResolvedAction) []Item {
	var items []Item
	for _, ra := range resolved {
		for _, d := range ra.Deltas {
			if d.Op != "drop_item" {
				continue
			}
			if item, ok := d.Value.(Item); ok {
				items = append(items, item)
			}
		}
	}
	return items
}

func summarizeResolved(resolved []ResolvedAction, characters map[string]Character) []narrator.ResolvedActionSummary {
	out := make([]narrator.ResolvedActionSummary, 0, len(resolved))
	for _, ra := range resolved {
		c := characters[ra.CharacterID]
		out = append(out, narrator.ResolvedActionSummary{
			CharacterID:   ra.CharacterID,
			CharacterName: c.Name,
			Intent:        ra.Intent,
			Outcome:       string(ra.Outcome),
			Summary:       summarizeDeltas(ra.Deltas),
		})
	}
	return out
}

func summarizeDeltas(deltas []StateDelta) string {
	if len(deltas) == 0 {
		return ""
	}
	parts := make([]string, 0, len(deltas))
	for _, d := range deltas {
		parts = append(parts, fmt.Sprintf("%s:%s=%v", d.Op, d.Target, d.Value))
	}
	return strings.Join(parts, ", ")
}

// proseSummaryCap bounds tier-2 memory. Lossy compaction is fine by design
// (spec.md §5) — tier-1 (WorldState/Character, folded in Commit) is the
// lossless record; this is only ever a rolling narrative aid for PRESENT.
const proseSummaryCap = 4000

func compactProse(existing, addition string) string {
	combined := existing
	if combined != "" && addition != "" {
		combined += " "
	}
	combined += addition
	if len(combined) <= proseSummaryCap {
		return combined
	}
	return combined[len(combined)-proseSummaryCap:]
}
