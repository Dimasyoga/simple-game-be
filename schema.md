# schema.md — State Model

All state below is **server-authoritative**. Types are language-agnostic; map to
Go structs (or your backend) directly. IDs are opaque strings unless noted.

> UI layout lives in the frontend repo. The client-facing view is defined by
> `PlayerView` / `CharacterSheet(Public)` in `contract.md`; the schema below is
> the backend's internal, authoritative model.

## Character

```
Character {
  id: string
  playerId: string          // owner; single-player => one Character total
  name: string
  class: string             // "warrior" | "knight" | "archer" | ...
  stats: Stats
  personality: Personality
  inventory: Item[]
  status: CharacterStatus
}

Stats {
  strength: int
  dexterity: int
  intelligence: int
  charisma: int
  // add/adjust as needed; used as modifiers in skill checks
  hp: int
  maxHp: int
}

Personality {
  morality: int             // e.g. -100..100; biases outcomes, does NOT veto
  traits: string[]          // e.g. ["cautious", "greedy"]
  alignment: string         // optional label derived from morality/traits
}

CharacterStatus {
  alive: bool
  conditions: string[]      // e.g. ["poisoned", "hidden"]
}
```

## Item

```
Item {
  id: string
  name: string
  type: string              // "weapon" | "consumable" | "key" | ...
  properties: map<string, any>
}
```

Items in the world (not yet owned) live in `SceneState.droppedItems` until a
loot claim assigns them into exactly one `Character.inventory` (atomic).

---

## Story / run

```
Run {
  id: string
  skeleton: SkeletonBeat[]      // fixed spine, ordered, invariant for the run
  beatDeck: BeatType[]          // pre-rolled per skeleton slot, HIDDEN from players
  currentClauseIndex: int
  characters: Character[]
  worldState: WorldState
  proseSummary: string          // rolling compacted narrative (tier 2 memory)
  status: string                // "active" | "ended"
}

SkeletonBeat {
  index: int
  role: string                  // "setup" | "rising" | "climax" | "resolution"
  premise: string               // invariant intent, e.g. "encounter guarding the path"
  climax: bool
}

BeatType = "combat" | "discovery" | "social" | "setback" | "puzzle"
```

## WorldState (tier-1 canonical, LOSSLESS)

```
WorldState {
  flags: map<string, any>       // e.g. {"goblin_dead": true, "goblin_killed_mercifully": false}
  killList: string[]            // entities removed from play (won't reappear at climax)
  relationships: map<string,int>// npcId -> disposition
  // any climax-relevant nuance is PROMOTED here as a flag, never left in prose
}
```

> Promotion example: prose says "the warrior spared the goblin." If sparing can
> matter later, store `flags["goblin_spared"] = true` — do not rely on the
> summary retaining it.

---

## Clause runtime

```
Clause {
  index: int
  beatType: BeatType
  phase: ClausePhase
  sceneState: SceneState
  windowDeadline: timestamp     // server clock; do NOT trust client
  inputs: map<characterId, ActionInput>   // one per acting character
  initiativeOrder: characterId[]           // rolled at RESOLVE
  resolvedActions: ResolvedAction[]        // ordered, post-dice
  droppedItems: Item[]                     // pending loot claims
}

ClausePhase =
  "PRESENT" | "WINDOW" | "GATE" | "INITIATIVE" |
  "RESOLVE" | "LOOT" | "NARRATE" | "COMMIT"
```

## Input & resolution

```
ActionInput {
  characterId: string
  rawText: string               // free-text intent from player
  terminalState: InputTerminal
  submittedAt: timestamp
}

InputTerminal = "SUBMITTED" | "PASSED" | "TIMED_OUT"
// PASSED and TIMED_OUT both => "does nothing", kept distinct for UI + prose tone.

ParsedAction {                  // output of GATE
  characterId: string
  intent: string                // normalized verb/target
  legal: bool                   // false if inventory/legality check fails
  rejectionReason: string?      // e.g. "no gun in inventory"
  referencedItemId: string?
}

ResolvedAction {                // output of RESOLVE, fed to narrator
  characterId: string
  intent: string
  roll: DiceResult
  outcome: Outcome              // SUCCESS | PARTIAL | FAIL
  deltas: StateDelta[]          // mutations to apply on COMMIT
}

DiceResult {
  die: int                      // e.g. d20
  raw: int
  modifier: int                 // from Stats
  dc: int                       // difficulty class (may be modified by prior actions)
  total: int
}

Outcome = "SUCCESS" | "PARTIAL" | "FAIL"

StateDelta {                    // the ONLY way canonical state changes
  op: string                    // "add_flag" | "kill" | "hp" | "add_item" | "morality" | ...
  target: string
  value: any
}
```

## Loot claim (exclusive resource)

```
LootClaim {
  itemId: string
  claimants: characterId[]      // who wants it
  resolvedTo: characterId?      // exactly one, atomically
  method: string                // "roll" | "initiative" | "discussion" (v1: roll/initiative)
}
```

---

## Invariants (enforce in code / tests)

1. `skeleton` and each `Run.beatDeck` entry are immutable after run start.
2. Canonical state (`WorldState`, `Character.inventory`, `Character.status`) only
   ever changes via `StateDelta` applied at COMMIT.
3. A dropped item is in exactly one place at all times: `droppedItems` **or** one
   character's inventory — never both, never neither after LOOT resolves.
4. Every acting character has exactly one `ActionInput` with a terminal state
   before RESOLVE begins.
5. The narrator (LLM) is invoked only in PRESENT and NARRATE, and receives
   already-resolved data in NARRATE. It emits prose only — never StateDeltas.
6. `windowDeadline` is set from the server clock; barrier fires on
   all-`SUBMITTED`/`PASSED` OR `now >= windowDeadline`.
