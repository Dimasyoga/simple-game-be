package room

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"simple-game-be/engine"
	"simple-game-be/narrator"
)

// Room drives one engine.Run's concurrency: it owns the current clause's
// WINDOW barrier and any open LOOT windows, and is the only thing that may
// mutate Run — callers reach the run only through Room's methods.
//
// engine.Run itself has no internal locking (Phase 1/2 treat it as
// single-threaded, pure state); runMu is what makes concurrent access safe
// once a transport layer (api/) is reading/appending to it from HTTP
// handlers while RunNextClause's goroutine is mutating it. runMu is held
// only around the engine calls that touch Run — never across the barrier
// or loot-window Wait()s, so submissions/claims/joins/reads are never
// blocked by an in-flight clause.
type Room struct {
	mu    sync.Mutex
	runMu sync.Mutex
	Run   *engine.Run
	Clock Clock
	Hooks *Hooks

	WindowDuration     time.Duration
	LootWindowDuration time.Duration

	barrier     *Barrier
	lootWindows map[string]*LootWindow
	actingIDs   []string
}

func NewRoom(run *engine.Run, clock Clock, windowDuration, lootWindowDuration time.Duration) *Room {
	return &Room{
		Run:                run,
		Clock:              clock,
		WindowDuration:     windowDuration,
		LootWindowDuration: lootWindowDuration,
		lootWindows:        make(map[string]*LootWindow),
	}
}

// ActingCharacters returns every living character's ID, sorted. v1 has no
// per-beat targeting of who acts; every alive character either submits,
// passes, or times out each clause.
func ActingCharacters(run *engine.Run) []string {
	ids := make([]string, 0, len(run.Characters))
	for _, c := range run.Characters {
		if c.Status.Alive {
			ids = append(ids, c.ID)
		}
	}
	sort.Strings(ids)
	return ids
}

// RunSnapshot is a point-in-time copy of the Run fields a transport layer
// needs to render state (contract.md's PlayerView) — taken under the same
// lock RunNextClause uses to mutate them, so it never races a clause.
type RunSnapshot struct {
	Status           string
	ChapterIndex     int
	ClauseOrder      int
	HostCharacterID  string
	Characters       []engine.Character
	ChapterSummaries []string
}

// Snapshot returns a race-free copy of Run's transport-relevant fields.
func (r *Room) Snapshot() RunSnapshot {
	r.runMu.Lock()
	defer r.runMu.Unlock()
	return RunSnapshot{
		Status:           r.Run.Status,
		ChapterIndex:     r.Run.ChapterIndex,
		ClauseOrder:      r.Run.ClauseOrder,
		HostCharacterID:  r.Run.HostCharacterID,
		Characters:       append([]engine.Character(nil), r.Run.Characters...),
		ChapterSummaries: append([]string(nil), r.Run.ChapterSummaries...),
	}
}

// AppendCharacter adds a new character to the run (e.g. on join). Safe to
// call concurrently with RunNextClause.
func (r *Room) AppendCharacter(c engine.Character) {
	r.runMu.Lock()
	defer r.runMu.Unlock()
	r.Run.Characters = append(r.Run.Characters, c)
}

// SetHostIfUnset assigns characterID as the run's host if no host has been
// assigned yet (contract.md: the first character to successfully join
// becomes host). Returns whether this call assigned the host.
func (r *Room) SetHostIfUnset(characterID string) bool {
	r.runMu.Lock()
	defer r.runMu.Unlock()
	if r.Run.HostCharacterID != "" {
		return false
	}
	r.Run.HostCharacterID = characterID
	return true
}

// Start transitions the run from lobby to active (engine.StartRun). Safe to
// call concurrently with reads; must not be called concurrently with
// RunNextClause (the caller is responsible for only driving the run after a
// successful Start).
func (r *Room) Start() error {
	r.runMu.Lock()
	defer r.runMu.Unlock()
	return engine.StartRun(r.Run)
}

// Barrier returns the currently open WINDOW barrier, or nil if none is open.
func (r *Room) Barrier() *Barrier {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.barrier
}

// Submit forwards to the open barrier, if any. Returns false if there is no
// open window or characterID isn't acting this clause.
func (r *Room) Submit(characterID, rawText string) bool {
	b := r.Barrier()
	if b == nil {
		return false
	}
	ok := b.Submit(characterID, rawText)
	if ok {
		r.pushInputStatus(b)
	}
	return ok
}

// Pass forwards to the open barrier, if any.
func (r *Room) Pass(characterID string) bool {
	b := r.Barrier()
	if b == nil {
		return false
	}
	ok := b.Pass(characterID)
	if ok {
		r.pushInputStatus(b)
	}
	return ok
}

func (r *Room) pushInputStatus(b *Barrier) {
	r.mu.Lock()
	ids := r.actingIDs
	r.mu.Unlock()
	r.Hooks.inputStatus(ids, b.Status())
}

// ClaimLoot forwards to the open claim window for itemID, if any.
func (r *Room) ClaimLoot(itemID, characterID string) bool {
	r.mu.Lock()
	w := r.lootWindows[itemID]
	r.mu.Unlock()
	if w == nil {
		return false
	}
	return w.Claim(characterID)
}

// RunNextClause drives one full clause end-to-end: PRESENT, then opens the
// WINDOW barrier and waits for it (all-terminal or timeout — never hangs),
// runs GATE..RESOLVE, opens a real claim window per dropped item and waits
// for those (each independently timeout-guaranteed), then finishes
// LOOT..COMMIT. Hooks fire at each transition so a transport layer can push
// contract.md's WS events without Room knowing anything about transport.
func (r *Room) RunNextClause(
	ctx context.Context,
	n narrator.Narrator,
	rng engine.RNG,
	catalog engine.ItemCatalog,
	checkFn engine.CheckResolver,
	effectFn engine.EffectResolver,
	lootMethod engine.LootMethod,
) (engine.ClauseResult, error) {
	r.runMu.Lock()
	if r.Run.ClauseOrder == 0 {
		chapter := r.Run.Gameplay.Chapters[r.Run.ChapterIndex]
		r.Hooks.chapterStarted(r.Run.ChapterIndex, chapter.Title)
	}
	scenePlain, err := engine.PresentClause(ctx, n, r.Run)
	requiresInput, riErr := engine.RequiresInputForCurrent(r.Run)
	r.runMu.Unlock()
	if err != nil {
		return engine.ClauseResult{}, fmt.Errorf("room: %w", err)
	}
	if riErr != nil {
		return engine.ClauseResult{}, fmt.Errorf("room: %w", riErr)
	}
	r.Hooks.scenePresented(scenePlain, requiresInput)

	var inputs map[string]engine.ActionInput
	if requiresInput {
		r.runMu.Lock()
		actingIDs := ActingCharacters(r.Run)
		r.runMu.Unlock()
		r.mu.Lock()
		r.actingIDs = actingIDs
		r.mu.Unlock()

		barrier := NewBarrier(r.Clock, actingIDs, r.WindowDuration)
		r.mu.Lock()
		r.barrier = barrier
		r.mu.Unlock()
		r.Hooks.windowOpened(barrier.Deadline())

		inputs = barrier.Wait()

		r.mu.Lock()
		r.barrier = nil
		r.mu.Unlock()
		r.Hooks.resolving()
	} else {
		inputs = map[string]engine.ActionInput{}
	}

	r.runMu.Lock()
	cip, err := engine.BeginClause(ctx, rng, r.Run, catalog, checkFn, effectFn, scenePlain, inputs)
	r.runMu.Unlock()
	if err != nil {
		return engine.ClauseResult{}, fmt.Errorf("room: %w", err)
	}

	dropped := cip.DroppedItems()
	claimants := make(map[string][]string, len(dropped))
	if len(dropped) > 0 {
		windows := make(map[string]*LootWindow, len(dropped))
		var deadline time.Time
		r.mu.Lock()
		for _, item := range dropped {
			w := NewLootWindow(r.Clock, r.LootWindowDuration)
			deadline = w.Deadline()
			r.lootWindows[item.ID] = w
			windows[item.ID] = w
		}
		r.mu.Unlock()
		r.Hooks.lootWindow(dropped, deadline)

		for itemID, w := range windows {
			claimants[itemID] = w.Wait()
		}

		r.mu.Lock()
		for _, item := range dropped {
			delete(r.lootWindows, item.ID)
		}
		r.mu.Unlock()
	}

	r.runMu.Lock()
	result, err := engine.FinishClause(ctx, n, r.Run, cip, rng, claimants, lootMethod)
	r.runMu.Unlock()
	if err != nil {
		return result, fmt.Errorf("room: %w", err)
	}
	for _, claim := range result.LootClaims {
		r.Hooks.lootResolved(claim)
	}
	return result, nil
}

// HasOpenLootWindow reports whether LOOT is currently collecting claims —
// used by a transport layer to render the FE-facing "LOOT" phase.
func (r *Room) HasOpenLootWindow() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.lootWindows) > 0
}
