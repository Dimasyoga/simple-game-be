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

## Gameplay / run

A **gameplay** is an authored template (see `design.md`): ordered **chapters**,
each containing ordered **clause templates**. It is loaded at run start and is
invariant for the run. (This replaces the earlier "skeleton + hidden random beat
deck" model — clause structure is now authored, not rolled.)

```
Gameplay {                      // the authored template (data, not code)
  id: string
  title: string
  tone: string                  // optional narrator style hint
  chapters: ChapterTemplate[]   // ordered
}

ChapterTemplate {
  index: int
  title: string
  clauses: ClauseTemplate[]     // ordered, ~3
  isFinal: bool                 // last chapter (the climax chapter)
}

ClauseTemplate {
  chapterIndex: int
  order: int                    // position within the chapter (1..N)
  type: ClauseType              // AUTHORED, extensible
  description: string           // DM's intent seed for the narrator (NOT player-facing)
  scriptedDeltas: StateDelta[]? // authored reward/effect applied at COMMIT when
                                 // requires_input = false (no RESOLVE step to emit
                                 // deltas from a player action); null/empty for
                                 // clauses with no unconditional reward
}

ClauseType = "setup" | "conflict" | "resolution"   // v1; extensible (see design.md)
// type drives engine behavior via the type->behavior table in design.md:
//   requires_input?, requires_roll?, reward?

Run {                           // the live run of a Gameplay
  id: string
  gameplayId: string
  mode: "single" | "multi"
  status: RunStatus
  hostCharacterId: string?       // the first character to join; null until first join
  chapterIndex: int             // current chapter
  clauseOrder: int              // current clause within the chapter
  characters: Character[]
  worldState: WorldState
  chapterSummaries: string[]    // one engine-written summary per COMPLETED chapter (tier-2)
}

RunStatus = "lobby" | "active" | "ended"
// lobby : created, awaiting players + start (no LLM, no clock)
// active: started; clause loop running
// ended : final chapter's final clause committed
```

> A clause is identified within a run by `(chapterIndex, order)`. The old flat
> `currentClauseIndex` is gone.

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
Clause {                        // the live run of a ClauseTemplate
  chapterIndex: int
  order: int
  type: ClauseType              // from the template; drives requires_input / requires_roll
  phase: ClausePhase
  sceneState: SceneState
  windowDeadline: timestamp?    // server clock; null if requires_input = false
  inputs: map<characterId, ActionInput>   // one per acting character
  initiativeOrder: characterId[]           // rolled at RESOLVE
  resolvedActions: ResolvedAction[]        // ordered, post-dice
  droppedItems: Item[]                     // pending loot claims
}

ClausePhase =
  "PRESENT" | "WINDOW" | "GATE" | "INITIATIVE" |
  "RESOLVE" | "LOOT" | "NARRATE" | "COMMIT"

// A clause with requires_input = false skips WINDOW/GATE/INITIATIVE/RESOLVE and
// auto-proceeds. A clause with requires_roll = false emits roll = null (below).
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
  roll: DiceResult?             // null when the action was deterministic (no check needed)
  outcome: Outcome              // SUCCESS | PARTIAL | FAIL | PROCEEDS (deterministic)
  deltas: StateDelta[]          // mutations to apply on COMMIT
}

DiceResult {
  die: int                      // e.g. d20
  raw: int
  modifier: int                 // from Stats
  dc: int                       // difficulty class (may be modified by prior actions)
  total: int
}

Outcome = "SUCCESS" | "PARTIAL" | "FAIL"   // when a roll happened
        | "PROCEEDS"                        // deterministic action, no roll (roll = null)

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

1. The loaded `Gameplay` (chapters + clause templates) is immutable after run
   start. Clause `type`/`description` are authored data, never mutated at runtime.
2. Canonical state (`WorldState`, `Character.inventory`, `Character.status`) only
   ever changes via `StateDelta` applied at COMMIT.
3. A dropped item is in exactly one place at all times: `droppedItems` **or** one
   character's inventory — never both, never neither after LOOT resolves.
4. Every acting character has exactly one `ActionInput` with a terminal state
   before RESOLVE begins (for clauses where `requires_input = true`).
5. The narrator (LLM) is invoked only in PRESENT and NARRATE, and receives
   already-resolved data in NARRATE. It emits prose only — never StateDeltas.
6. `windowDeadline` is set from the server clock; barrier fires on
   all-`SUBMITTED`/`PASSED` OR `now >= windowDeadline`.
7. `chapterSummaries` are **engine-written** (never by the LLM) and appended once
   per completed chapter; they are injected into later chapters verbatim.
8. A clause with `requires_roll = false` produces `roll = null` / `outcome =
   PROCEEDS`; the narrator must not invent a success/failure where none was rolled.
9. `hostCharacterId` is set once, to the first character that successfully joins,
   and never changes for the life of the run.
10. A clause with `requires_input = false` applies its `scriptedDeltas` (if any) at
    COMMIT unconditionally — no RESOLVE step runs, so no roll/outcome is produced
    for it.
