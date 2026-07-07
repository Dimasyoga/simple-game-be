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

// ---- Story / run ----

type Run struct {
	ID                 string
	Skeleton           []SkeletonBeat // fixed spine, ordered, invariant for the run
	BeatDeck           []BeatType     // pre-rolled per skeleton slot, HIDDEN from players
	CurrentClauseIndex int
	Characters         []Character
	WorldState         WorldState
	ProseSummary       string // rolling compacted narrative (tier 2 memory)
	Status             string // "active" | "ended"
}

type SkeletonBeat struct {
	Index   int
	Role    string // "setup" | "rising" | "climax" | "resolution"
	Premise string // invariant intent, e.g. "encounter guarding the path"
	Climax  bool
}

type BeatType string

const (
	BeatCombat    BeatType = "combat"
	BeatDiscovery BeatType = "discovery"
	BeatSocial    BeatType = "social"
	BeatSetback   BeatType = "setback"
	BeatPuzzle    BeatType = "puzzle"
)

// ---- WorldState (tier-1 canonical, LOSSLESS) ----

type WorldState struct {
	Flags         map[string]any // e.g. {"goblin_dead": true}
	KillList      []string       // entities removed from play
	Relationships map[string]int // npcId -> disposition
}

// ---- Clause runtime ----

type Clause struct {
	Index           int
	BeatType        BeatType
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
	Roll        DiceResult
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
	OutcomeSuccess Outcome = "SUCCESS"
	OutcomePartial Outcome = "PARTIAL"
	OutcomeFail    Outcome = "FAIL"
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
