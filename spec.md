# spec.md — Backend (Authoritative Game Server)

## 1. Responsibility

This repository is the **single source of truth** for the game. It owns:
- all deterministic logic: dice, initiative, resolve, loot, barriers;
- all canonical state: characters, inventory, world flags, story memory;
- the timed input barrier and its server-authoritative clock;
- the LLM connection (narration);
- the REST + WebSocket API defined in `contract.md`.

The frontend renders and captures input only. If a decision affects another
player or must not be tamperable, it lives here.

## 2. Non-negotiable principles

1. **Engine owns truth; the LLM only narrates.** The model is a stateless
   generator called **last**; it never decides outcomes, memory, or success.
2. **Fixed skeleton, variable realization.** The arc is invariant engine data;
   only content varies per run.
3. **Server-authoritative.** Clock, state, dice, ordering — all here. The client
   is never trusted for timing or outcomes.
4. **Single-player = N=1.** One system; solo is one character.
5. **Deterministic before generative.** All concurrency must be provably
   deadlock-free with the narrator stubbed.

## 3. What this repo exposes

Exactly the surface in `contract.md` (REST + WS). Nothing else leaks. Internals
(rolls, DCs, beat-deck contents, resolution order) are private; the client sees
only results it is entitled to.

## 4. Story model (engine-owned)

- **Skeleton (spine):** fixed ordered `SkeletonBeat`s; invariant per run.
- **Beat deck:** a `BeatType` pre-rolled per slot at run start, **hidden** from
  players; not sent over the contract by default.
- **Clause:** skeleton beat + rolled type + accumulated state → narrated.
- **Climax:** must branch on accumulated tier-1 state or it's a bug.

Details in `schema.md`.

## 5. Memory (two tiers)

- **Tier 1 — structured canonical state:** kills, inventory, status, flags,
  alignment, relationships. **Lossless, never summarized.**
- **Tier 2 — narrative prose:** compacted into a rolling summary.
- **Promotion rule:** climax-relevant nuance is promoted into tier-1 flags, never
  left in prose that will be summarized away.

The narrator receives (tier-2 summary + relevant tier-1 state) per call and never
relies on its own context to remember facts.

## 6. Clause pipeline (all backend)

```
PRESENT   assemble context; [NARRATOR] scene prose; push clause_presented
WINDOW    open barrier (deadline = serverNow + windowDuration); accept actions;
          fire on all-SUBMITTED/PASSED OR timeout; push window_opened/input_status
GATE      parse + hard inventory/legality check (pre-generation)
INITIATIVE roll; deterministic tiebreak
RESOLVE   apply actions in initiative order vs working scene state; dice → outcome;
          emit StateDeltas; conflicts emerge from ordering
LOOT      if items dropped: exclusive-claim barrier + non-deadlocking tiebreak;
          assign atomically
NARRATE   [NARRATOR] renders ordered resolved actions; validate vs structured state
COMMIT    fold deltas into tier-1 (lossless); promote flags; compact prose; advance
```

Full semantics in `rules.md`. Steps GATE→LOOT and COMMIT are pure engine logic and
MUST be testable with the narrator stubbed.

## 7. Concurrency

- **Barrier with timeout** per clause; terminal states `SUBMITTED` / `PASSED` /
  `TIMED_OUT` (pass and timeout both no-op, stored distinctly).
- **Conflicts resolved by initiative order** against shared scene state — no
  separate conflict resolver; the LLM never blends contradictory actions.
- **Discussion is OOC**: relayed to the room, never persisted, never sent to the
  LLM.
- **Loot** is a race on an exclusive resource; one writer wins; provably
  deadlock-free.
- `RESOLVING` phase locks input; pushed to clients.

## 8. Free-text handling

- **Inventory = hard pre-generation gate.** Reject/negate missing-item actions in
  the backend before any generation; never trust the model to police non-possession.
- **Personality = outcome bias, not veto.** Acting against alignment can succeed at
  a cost + reaction; never blocks input. Doubles as content moderation-in-world.
- **Dice/skill checks** are the anti-"narrate-your-own-win" layer: player narrates
  intent, dice+stats decide result, LLM narrates result.

## 9. LLM adapter

- A single `Narrator` interface, invoked only in PRESENT and NARRATE, emitting
  prose only.
- **Hosted first, local last.** Implement against a fast hosted API to validate
  cadence, then swap a local model (Mistral-7B class) behind the same interface.
  The engine must not depend on which model is behind it.
- Post-generation validation: narrated key facts checked against tier-1 state; on
  contradiction, prefer structured state (optionally re-ask).

## 10. Suggested stack
Go server; WebSocket rooms; injectable seedable RNG; in-memory room state for v1
(persistence later). REST + WS per `contract.md`.

## 11. Out of scope (v1)
In-character/diegetic discussion; branching skeletons; accounts/persistence;
local model as the first integration.

## 12. Definition of done (core)
2–3 simulated players complete a multi-clause run with the **narrator stubbed**,
deadlock-free under conflicting/idle inputs; tier-1 state provably lossless; a
climax beat correctly references earlier structured state. Only then wire a real
narrator.
