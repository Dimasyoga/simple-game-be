# tasks.md — Backend Build Order

Build in this order. **Do not wire a real LLM until Phase 3 passes.** The hard
problems here are concurrency and state, not generation.

## Phase 0 — Scaffolding
- [x] Go server skeleton; module layout (engine / api / narrator / room).
- [x] All types from `schema.md`.
- [x] Injectable **seedable RNG** for every dice/initiative call.
- [x] `Narrator` interface with a stub returning canned prose. Everything
      downstream depends on the interface, never a concrete model.

## Phase 1 — Deterministic single-clause engine (no LLM, no network)
- [x] GATE: parse action + hard inventory/legality check (rules R4).
- [x] INITIATIVE: roll + deterministic tiebreak, no unresolved ties (R5).
- [x] RESOLVE: ordered, stateful; later actions see earlier mutations; Outcome
      mapping; StateDelta emission; personality-as-modifier (R6).
- [x] LOOT: exclusive-claim resolution (roll/initiative), atomic assign (R7).
- [x] COMMIT: fold deltas into lossless tier-1; flag promotion (R9).
- [x] Unit tests (seeded RNG):
  - head-on vs sneak resolves by initiative order;
  - missing-item action never resolves as success;
  - ≥2 loot claimants → exactly one owner, item never duplicated/lost;
  - personality bias shifts odds + queues alignment delta, never vetoes input.

## Phase 2 — Full lifecycle + memory (still stubbed narrator)
- [x] Clause phase machine PRESENT→…→COMMIT.
- [x] Two-tier memory: lossless tier-1 + rolling compacted prose.
- [x] Skeleton + hidden pre-rolled beat deck (R1); climax reads tier-1 (R10).
- [x] Test: full multi-clause run; climax reflects earlier kills/inventory/alignment.

## Phase 3 — Concurrency, rooms, and the contract (still stubbed narrator)
- [x] WebSocket rooms; server-authoritative clock; `windowDeadline` (R3).
- [x] Timed barrier: fires on all-SUBMITTED/PASSED OR timeout; assigns TIMED_OUT;
      PASSED vs TIMED_OUT distinct.
- [x] `RESOLVING` phase locks input; push `resolving`.
- [x] Implement the full REST + WS surface in `contract.md`, including
      `discussion_send` relay that is dropped from canonical + narrator paths.
- [x] Loot claim endpoint + `loot_window`/`loot_resolved`.
- [x] **Deadlock-freedom tests**, 2–3 simulated players:
  - mixed submitted/passed/idle-to-timeout;
  - two players spam-claim the same item;
  - a player stalls forever → timeout fires, clause completes.
- [x] **GATE:** a full story runs to completion over the real contract with the
      stubbed narrator, deadlock-free, before Phase 4.

## Phase 4 — Narrator (hosted first) — SKIPPED
No hosted API billing available; going straight to the local model path (was
Phase 5). Revisit only if a hosted narrator is wanted later — the `Narrator`
interface makes it a drop-in addition, not a rework.

## Phase 5 — Local model narrator
- [x] Runtime-agnostic HTTP `Narrator` (`narrator/local.go`) against the
      OpenAI-compatible chat completions shape — the common surface across
      Ollama, llama.cpp `llama-server`, vLLM, LM Studio, text-generation-webui.
- [ ] PRESENT + NARRATE prompts: pass compacted summary + relevant tier-1 state;
      NARRATE gets already-resolved ordered actions.
- [ ] Post-gen validation vs structured state; on contradiction prefer structured.
- [ ] Wire the actual local model once chosen: base URL + model name only —
      no code change expected if it speaks the same chat-completions shape.
- [ ] Measure real cadence (discuss→submit→resolve→read) in a multiplayer room;
      tune window duration + resolving UX signals.

## Cross-cutting guardrails
- Server authoritative for clock, dice, state, ordering; client is display+input.
- Canonical state changes ONLY via StateDelta at COMMIT.
- Narrator invoked only in PRESENT/NARRATE; emits prose only.
- Single-player = N=1; no separate solo path.
- Every deterministic rule unit-tested with seeded RNG + stubbed narrator.
- Only the `contract.md` surface is public; internals stay private.
