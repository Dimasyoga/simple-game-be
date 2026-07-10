# design.md — Gameplay Design (the "why")

Design rationale and authoring model. This is the *why*; `schema.md` and
`rules.md` are the *what to build*. Where they seem to differ, the canonical build
specs win — open an issue rather than diverging.

Supersedes the earlier "fixed skeleton + hidden random beat deck" model: gameplays
are now **authored templates** (a DM decides the structure), not randomly rolled.
Variety across playthroughs comes from player actions, dice outcomes, and LLM
prose — not from randomized beat types.

---

## 1. Story hierarchy

| Term | Definition | ~Size |
|------|------------|-------|
| **Gameplay** | One full run start→end. The template/skeleton the player picks. | ~5 chapters, 15–20 min |
| **Chapter** | One story beat with its own mini-arc. Ends → engine writes a summary carried forward. | ~3 clauses |
| **Clause** | The atomic round: narration → player input → resolve → narration. One player action. | 1 exchange |

A chapter is a mini three-act arc (e.g. setup → conflict → resolution), which is
exactly why it is summarizable — it has a beginning, middle, and end to compress.

### Time budget (a real constraint)
Target **15–20 min per gameplay** (a "medium article" read), for people on the go.
~5 chapters × ~3 clauses = **~15 clauses**, each with two narrations + a typing
pause. Therefore: **keep narrations tight** (a few sentences), setup clauses can be
shorter than conflict clauses. Treat narration length as budgeted, not free.

### Staple dev story — "The Dragon of Emberpeak"
Clause `type` is authored per clause (below); setup/conflict/resolution is a
*typical* shape, not a fixed rule.

```
Ch.1 Introduction        : setup (establish) / conflict (wolves) / resolution (loot)
Ch.2 Back to the Tavern  : setup (intel) / conflict (dragon arrives) / resolution
Ch.3 Start of Adventure  : (authored)
Ch.4 Journey to the Lair : (authored)
Ch.5 Final Battle        : (authored, climax)
```

### Templates now, custom later
- **v1:** all gameplays are **templates** authored by the dev team as data. One
  staple story for development.
- **Later:** a library of predetermined stories the player chooses from.
- **Later still:** a **dashboard** where a user authors their own gameplay —
  becoming the DM — while the engine handles memory/dice/calculation and the LLM
  only narrates.

### Load-bearing discipline: data, not code
The staple story is expressed **entirely as data** the engine interprets — never
hardcoded logic (no `if chapter == 2`, no magic numbers in the resolver). The
engine is a generic interpreter; the gameplay is data it reads. This is what makes
the future dashboard cheap (a dashboard edits data, not source). Free to enforce
now, painful to retrofit.

### Player non-interaction (v1)
Empty input in a clause = **no-op** ("the character does nothing"), story proceeds.
A deliberate "player disengaged → gameplay ends early" is a later authored option.

---

## 2. Clause authoring

The atomic authoring unit (matches the authoring mockup: chapter, type,
description):

```
ClauseTemplate {
  chapterIndex: int
  order:        int          // position within the chapter (1..N)
  type:         ClauseType   // AUTHORED, extensible
  description:  string       // DM's intent seed for the narrator (NOT player-facing)
}
```

**`description` is a seed, not narration.** It's the DM's intent, e.g. *"A locked
door blocks the path; a goblin guards the key."* The LLM elaborates it into the
actual narration at runtime, varying per playthrough. description = authored input;
narration = generated output.

**`ClauseType` is an open, growable list**, not a hardcoded three. Adding a type
later defines its behavior mapping — it must not require rewiring the loop.

```
ClauseType = "setup" | "conflict" | "resolution"   // v1
           | ...                                     // "puzzle", "social", "boss" later
```

### Type → engine behavior (type-driven, with defaults)
`type` tells the engine whether the clause needs input and whether dice roll —
encoding *"dice only when the outcome isn't exact."*

| Type | requires_input | requires_roll | reward | Notes |
|------|:--:|:--:|:--:|-------|
| **setup** | usually (may be flavor) | no | no | atmosphere; action rarely branches |
| **conflict** | yes | **yes** | maybe | uncertain outcome → dice |
| **resolution** | maybe | maybe | often | reward deterministic OR gated by a roll |
| *(future)* | per type | per type | per type | each new type defines its mapping |

**v1: type-driven with defaults** — choosing a type sets input/roll/reward
automatically (one dropdown to author). **Later (optional):** type + explicit
override flags, which can be added without breaking v1.

### Rewards on a no-input clause
A `requires_input=false` clause never runs RESOLVE (no player action to roll or
apply deltas from), so it cannot earn a reward through play. If such a clause is
meant to hand out something unconditionally (e.g. a setup clause that hands the
party a starting item), the author sets `ClauseTemplate.scriptedDeltas` — data,
not code — and the engine applies those deltas at COMMIT with no roll involved.
This is the only reward path available to a no-input clause; `reward` in the
type table above describes only clauses that *do* require input.

---

## 3. Memory carry-forward (the DM model)

**The engine is the DM's memory; the LLM only reads it.** A human DM keeps state in
their head and by the rules, and uses it to narrate the next scene — the
recollection is the source of truth, the storytelling never rewrites it.

### Two levels
| Scope | Narrator sees | Detail |
|-------|---------------|--------|
| Within a chapter | previous clauses of *this* chapter | fresh |
| Across chapters | one **summary per completed chapter** | compressed |
| Always | canonical structured state (inventory, kills, flags, alignment) | lossless |

Context is bounded: `(prior chapter summaries) + (current chapter's live clauses) +
(canonical state)`. It does **not** grow with raw prose, no matter how long the run.

### End-of-chapter summary
When a chapter's last clause commits:
1. **Engine writes the summary deterministically** from that chapter's clause
   outcomes and deltas — **no LLM call at summary time.** e.g. *"Ch.3 — Ambushed by
   bandits; Kael killed two brutally; looted a steel longsword."*
2. Append to `chapterSummaries[]`.
3. Fold structured deltas into canonical state (lossless).

**Engine-written, not LLM-written**, because the summary feeds every later chapter;
an LLM hallucination would propagate through the whole downstream story.

### Context injection (model (a), decided)
At the next chapter's start, the engine injects `chapterSummaries[]` + canonical
state into the narrator context **verbatim**. It is **injection, not rewrite** — the
LLM reads summaries for continuity but never rewrites the stored summary. Nicer
wording affects only the narration the player reads, never memory. This prevents
summary-of-a-summary drift.

### Promotion rule
At commit, ask: *could a later chapter (esp. the climax) mechanically care?* If yes,
promote it into a **flag**, don't leave it in compressible prose. Cheap to store,
expensive to lose — when unsure, promote.

### Same shape over time
The narrator's context is always the same shape; it just fills up. Early: empty
summaries, tiny state, rolls often null. Late: several one-line summaries, a fat
state block, populated rolls. (LLM prompt implementation is owned by the LLM team;
this defines only what the engine hands them.)
