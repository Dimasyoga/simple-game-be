// Package engine holds the authoritative game state and deterministic rules
// described in schema.md and rules.md. Nothing here depends on transport
// (api/room) or on a concrete narrator implementation.
package engine

// ---- Character ----

type Character struct {
	ID          string
	PlayerID    string // owner; single-player => one Character total
	Name        string
	Class       string // "warrior" | "knight" | "archer" | ...
	Stats       Stats
	Personality Personality
	Inventory   []Item
	Status      CharacterStatus
}

type Stats struct {
	Strength     int `json:"strength"`
	Dexterity    int `json:"dexterity"`
	Intelligence int `json:"intelligence"`
	Charisma     int `json:"charisma"`
	HP           int `json:"hp"`
	MaxHP        int `json:"maxHp"`
}

type Personality struct {
	Morality  int      `json:"morality"`  // e.g. -100..100; biases outcomes, does NOT veto
	Traits    []string `json:"traits"`    // e.g. ["cautious", "greedy"]
	Alignment string   `json:"alignment"` // optional label derived from morality/traits
}

type CharacterStatus struct {
	Alive      bool     `json:"alive"`
	Conditions []string `json:"conditions"` // e.g. ["poisoned", "hidden"]
}

// ---- Item ----

type Item struct {
	ID         string
	Name       string
	Type       string // "weapon" | "consumable" | "key" | ...
	Properties map[string]any
}

// ---- Gameplay / run ----

// Gameplay is an authored template (data, not code): ordered chapters, each
// containing ordered clause templates. It is loaded at run start and is
// invariant for the run (schema.md, design.md).
type Gameplay struct {
	ID       string
	Title    string
	Tone     string // optional narrator style hint
	Chapters []ChapterTemplate
}

type ChapterTemplate struct {
	Index   int
	Title   string
	Clauses []ClauseTemplate
	IsFinal bool // last chapter (the climax chapter)
}

type ClauseTemplate struct {
	ChapterIndex   int
	Order          int // position within the chapter (1..N)
	Type           ClauseType
	Description    string       // DM's intent seed for the narrator; NOT player-facing
	ScriptedDeltas []StateDelta // authored reward/effect applied at COMMIT when
	// requires_input = false (no RESOLVE step to emit deltas from a player action)
}

type ClauseType string

const (
	ClauseSetup      ClauseType = "setup"
	ClauseConflict   ClauseType = "conflict"
	ClauseResolution ClauseType = "resolution"
)

type Run struct {
	ID               string
	GameplayID       string
	Gameplay         Gameplay
	Mode             string // "single" | "multi"
	Status           string // "lobby" | "active" | "ended"
	HostCharacterID  string
	ChapterIndex     int // current chapter
	ClauseOrder      int // current clause within the chapter
	Characters       []Character
	WorldState       WorldState
	ChapterSummaries []string         // one engine-written summary per COMPLETED chapter (tier-2)
	ChapterLog       []ResolvedAction // this chapter's resolved actions so far; reset each chapter
}

// ---- WorldState (tier-1 canonical, LOSSLESS) ----

type WorldState struct {
	Flags         map[string]any // e.g. {"goblin_dead": true}
	KillList      []string       // entities removed from play
	Relationships map[string]int // npcId -> disposition
}

// ---- Clause runtime ----

type Clause struct {
	ChapterIndex    int
	Order           int
	Type            ClauseType
	Phase           ClausePhase
	SceneState      SceneState
	WindowDeadline  int64                  // server clock (unix ms); do NOT trust client
	Inputs          map[string]ActionInput // characterId -> input
	InitiativeOrder []string               // characterId[]
	ResolvedActions []ResolvedAction       // ordered, post-dice
	DroppedItems    []Item                 // pending loot claims
}

type ClausePhase string

const (
	PhasePresent    ClausePhase = "PRESENT"
	PhaseWindow     ClausePhase = "WINDOW"
	PhaseGate       ClausePhase = "GATE"
	PhaseInitiative ClausePhase = "INITIATIVE"
	PhaseResolve    ClausePhase = "RESOLVE"
	PhaseLoot       ClausePhase = "LOOT"
	PhaseNarrate    ClausePhase = "NARRATE"
	PhaseCommit     ClausePhase = "COMMIT"
)

// SceneState is the working, in-clause mutable view actions resolve against.
// It is intentionally open-ended (v1): a small bag of facts the engine
// consults for DC adjustments (e.g. "target alerted").
type SceneState struct {
	Facts map[string]any
}

// ---- Input & resolution ----

type ActionInput struct {
	CharacterID   string
	RawText       string // free-text intent from player
	TerminalState InputTerminal
	SubmittedAt   int64 // unix ms
}

type InputTerminal string

const (
	InputSubmitted InputTerminal = "SUBMITTED"
	InputPassed    InputTerminal = "PASSED"
	InputTimedOut  InputTerminal = "TIMED_OUT"
)

// ParsedAction is the output of GATE.
type ParsedAction struct {
	CharacterID      string
	Intent           string // normalized verb/target
	Legal            bool   // false if inventory/legality check fails
	RejectionReason  string // e.g. "no gun in inventory"
	ReferencedItemID string
}

// ResolvedAction is the output of RESOLVE, fed to the narrator.
type ResolvedAction struct {
	CharacterID string
	Intent      string
	Roll        *DiceResult // nil when the action was deterministic (no check needed)
	Outcome     Outcome
	Deltas      []StateDelta // mutations to apply on COMMIT
}

type DiceResult struct {
	Die      int // e.g. 20 for d20
	Raw      int
	Modifier int // from Stats
	DC       int // difficulty class (may be modified by prior actions)
	Total    int
}

type Outcome string

const (
	OutcomeSuccess  Outcome = "SUCCESS"
	OutcomePartial  Outcome = "PARTIAL"
	OutcomeFail     Outcome = "FAIL"
	OutcomeProceeds Outcome = "PROCEEDS" // deterministic action, no roll (roll = nil)
)

// StateDelta is the ONLY way canonical state changes.
type StateDelta struct {
	Op     string // "add_flag" | "kill" | "hp" | "add_item" | "morality" | ...
	Target string
	Value  any
}

// ---- Loot claim (exclusive resource) ----

type LootClaim struct {
	ItemID     string
	Claimants  []string // characterId[]
	ResolvedTo string   // exactly one, atomically; "" until resolved
	Method     string   // "roll" | "initiative" | "discussion" (v1: roll/initiative)
}
