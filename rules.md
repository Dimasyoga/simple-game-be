# rules.md — Resolution Rules

Exact, deterministic semantics for the clause pipeline. Everything here must be
implementable and testable **with the LLM stubbed**. The LLM appears only where
explicitly marked `[NARRATOR]`.

## R0. Golden rule

The engine decides *what happens*. The LLM decides *how it reads*. If a rule
below is ambiguous, resolve it in the engine deterministically; do not defer the
decision to the narrator.

---

## R1. Run setup

1. Load the authored `Gameplay` template (chapters → clause templates). It is
   **data**, invariant for the run. No random beat rolling — clause `type` and
   `description` are authored (see `design.md`).
2. Initialize characters, empty `WorldState`, empty `chapterSummaries[]`.
3. Set `status = lobby`, `chapterIndex = 0`, `clauseOrder = 0`. **Do not** run any
   clause or call the LLM until `start` (see lifecycle). `POST /runs` only reaches
   this point.

## R1b. Chapter loop

Run chapters in order. For each chapter, run its clauses in order (R2–R9). When a
chapter's final clause commits, run **R9b (chapter summary)** before advancing to
the next chapter. After the final chapter (`isFinal`) commits, end the run (R10).

## R2. Clause: PRESENT

1. Assemble narrator context: `chapterSummaries[]` (prior chapters) + current
   chapter's clauses so far + relevant `WorldState` + this clause's authored
   `description` + `type`.
2. `[NARRATOR]` produce scene prose. Output is display-only; it introduces no
   canonical state. If the scene implies new facts (an NPC appears), the **engine**
   records them as structured `SceneState`, not the model.
3. Read `type → requires_input` (design.md table). If `requires_input = false`,
   skip R3–R6 and proceed straight to NARRATE/COMMIT (an atmosphere/setup beat the
   player doesn't act on).

## R3. Clause: WINDOW (timed input barrier)

1. Open window; set `windowDeadline = serverNow + windowDuration` (default 300s).
2. Players may submit / update / pass via free input. Discussion box traffic is
   OOC and ignored by the engine's canonical path.
3. **Barrier fires when** every acting character is `SUBMITTED` or `PASSED`, **OR**
   `serverNow >= windowDeadline`.
4. On fire, any character with no submission → `TIMED_OUT`. `PASSED` and
   `TIMED_OUT` both map to a no-op action but are stored distinctly.
5. Transition to `RESOLVING` phase: **lock all input**, signal UI.

> Never trust client timers. The server owns `windowDeadline` and the fire check.

## R4. Clause: GATE (pre-generation legality)

For each non-noop `ActionInput`:
1. Parse `rawText` → `ParsedAction{ intent, referencedItemId }`.
2. **Hard inventory/legality check** against the character's current inventory
   and world facts. If the action references an item the character does not own
   (e.g. "shoot with gun" but no gun): set `legal=false`,
   `rejectionReason="no <item>"`.
3. Illegal actions become either: (a) a nudge back to the player if the window
   still allows, or (b) a no-op / improvised fallback at RESOLVE. Choose one policy
   and keep it consistent. **Do not** let an illegal action reach the narrator as
   if it succeeded.

> Rationale: a small local model will hallucinate the missing item into
> existence. Gate in the backend, before any generation.

## R5. Clause: INITIATIVE

1. Roll initiative for every acting (non-noop) character:
   `initiative = d20 + dexterityModifier` (tune formula as desired).
2. Sort descending into `initiativeOrder`. Deterministic tiebreak (e.g. higher
   dexterity, then stable characterId order) — **no ties left unresolved**.

## R6. Clause: RESOLVE (ordered, stateful)

Process actions **in `initiativeOrder`**, mutating a working copy of
`sceneState` as you go so later actions see earlier effects:

For each action:
1. **Decide if a roll is needed.** A roll happens only when the clause
   `type → requires_roll` allows it **and** the outcome is genuinely uncertain.
   Deterministic actions (open an unlocked door, walk forward, take an offered
   item) → **no roll**: `roll = null`, `outcome = PROCEEDS`, emit any deltas, done.
   Otherwise continue:
2. Determine check type + `DC` from `intent`, clause `type`, and **current**
   `sceneState` (which reflects earlier actions this clause — e.g. target already
   alerted raises a stealth DC or converts a "sneak" into a "failed sneak").
3. Roll: `total = d20 + statModifier`. Apply **personality bias** as a modifier,
   not a veto: acting against morality is allowed but may lower success odds
   and always queues an alignment/reaction delta.
4. Map to `Outcome`:
   - `total >= DC` → SUCCESS
   - `DC-2 <= total < DC` → PARTIAL (succeeds with cost/complication)
   - else → FAIL
5. Emit `StateDelta[]` (hp, kill, flag, morality, add dropped item, etc.).
   Apply to the working `sceneState` immediately (so ordering matters).
6. Append to `resolvedActions` preserving order.

**Conflict handling is emergent from ordering** — there is no separate "conflict
resolver." A attacks head-on (goes first, alerts goblin) → B's sneak is
recomputed against an alert target. The engine owns this; the LLM never blends
contradictory actions itself.

## R7. Clause: LOOT (exclusive-claim barrier)

Only if RESOLVE produced `droppedItems`.

For each dropped item:
1. Collect `claimants` (characters who indicated they want it). Open a short
   claim window if needed.
2. Resolve to exactly one owner using a **non-deadlocking** method:
   - **roll** (default): each claimant rolls; highest takes it. Deterministic
     tiebreak.
   - **initiative**: earliest in `initiativeOrder` takes it (free, reuses R5).
   - (discussion-based assignment is post-v1; if added, it MUST have a
     roll/initiative timeout-tiebreak underneath so it can never hang.)
3. Move the item **atomically**: remove from `droppedItems`, append to the winner's
   inventory. Losers receive "someone else took it." No item is ever in two places.

> Loot is a race on an exclusive resource. One writer wins; this barrier must be
> provably deadlock-free (see tasks.md tests). 0 or 1 claimant is trivial; the
> interesting case is ≥2.

## R8. Clause: NARRATE

1. `[NARRATOR]` receives: the **ordered** `resolvedActions` (with outcomes),
   applied deltas summary, and loot results — all already decided.
2. It writes prose describing what happened, in order. It must not introduce
   outcomes that contradict `resolvedActions` (constrain via prompt + validate
   key facts against structured state; on contradiction, prefer structured state
   and optionally re-ask).

## R9. Clause: COMMIT (memory)

1. Fold all `StateDelta`s into canonical `WorldState` / character sheets
   (tier-1, lossless).
2. **Promote** any climax-relevant nuance into `WorldState.flags` explicitly.
3. Advance: `clauseOrder++`. If more clauses remain in this chapter → next clause
   (R2). If this was the chapter's last clause → **R9b**, then next chapter.

> Within-chapter memory is the chapter's own clauses (still live in context). There
> is no per-clause rolling prose summary anymore — compression happens per chapter
> at R9b.

## R9b. Chapter end: summary (memory carry-forward)

Runs once, when a chapter's final clause commits.

1. **Engine writes** a summary of the chapter deterministically from its clause
   outcomes + deltas — **no LLM call.** e.g. "Ch.3 — Ambushed by bandits; Kael
   killed two brutally; looted a steel longsword."
2. Append to `chapterSummaries[]`.
3. Advance: `chapterIndex++`, `clauseOrder = 0`. The next chapter's PRESENT (R2)
   injects `chapterSummaries[]` **verbatim** (context injection, not rewrite — the
   LLM reads it, never rewrites it; see `design.md`).

## R10. Final chapter / climax payoff (correctness, not flavor)

The final chapter (`isFinal`) is the climax. Its clause contexts MUST include
accumulated tier-1 state (killList, inventory, alignment, key flags, and all prior
`chapterSummaries`), and resolution MUST branch on them (the dead goblin cannot
reappear; a collected fire-cloak changes the dragon fight). A climax that ignores
accumulated state is a bug. After its final clause commits: `status = ended`, push
`run_ended`.

---

## Determinism / testability contract

- All of R3–R7, R9, and R9b are pure engine logic and MUST be unit-testable with a
  **stubbed narrator** (canned prose). This includes the deterministic-action path
  (R6.1: `roll = null`, `outcome = PROCEEDS`) and engine-written chapter summaries.
- Dice use an injectable RNG so tests can force outcomes.
- Given identical inputs + seeded RNG, resolution is fully reproducible.
