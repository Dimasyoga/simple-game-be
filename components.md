# components.md — Engine vs LLM Reference

Every component in the clause pipeline, in execution order. **Type** = who runs
it. The three LLM rows are the *only* places a model is called; everything else is
deterministic backend logic (seed-reproducible, testable with the LLM stubbed).

| # | Component | Type | Use (what it does) | When (phase) | How | Input | Output |
|---|-----------|------|--------------------|--------------|-----|-------|--------|
| 1 | **Run setup** | Engine | Establishes the run: loads the authored gameplay template (chapters + clause templates) | Once, at `POST /runs` (status→lobby) | Load `Gameplay` data; init sheets + empty state; NO LLM/clock until `start` | `gameplayId`, character choices | `Run` (lobby) with loaded chapters, characters, empty `WorldState`, empty `chapterSummaries` |
| 2 | **Context assembler** | Engine | Builds the prompt payload for the scene narrator | Start of each clause (PRESENT) | Concatenate `chapterSummaries[]` + this chapter's clauses so far + relevant tier-1 state + clause `description`/`type` | `Run` state, `(chapterIndex, clauseOrder)` | A context bundle (text) for the narrator |
| 3 | **Scene narrator** | **LLM** | Writes the opening prose of the clause | PRESENT | One generation call; prose only, introduces no canonical facts | Context bundle from #2 | Scene prose (`scenePlain`) — display only |
| 4 | **Input barrier / window** | Engine (timer) | Collects player actions under a deadline; the core concurrency gate | WINDOW | Open window `deadline = serverNow + 300s`; accept submit/pass; fire on all-SUBMITTED/PASSED **OR** timeout; assign `TIMED_OUT` | Player actions/passes over REST; server clock | Map of `ActionInput` (each `SUBMITTED`/`PASSED`/`TIMED_OUT`) |
| 5 | **Discussion relay** | Engine | OOC table-talk between players | During WINDOW | Broadcast to room over WS; **dropped** from canonical + narrator paths | `discussion_send` text | `discussion_message` to room (not stored) |
| 6 | **Intent classifier** | **LLM** | Turns free text into a structured intent — **categories, not outcomes** | GATE (per non-noop action) | Constrained/structured-output call; returns JSON only | Raw action text (+ optional scene context) | `{type, target, item, skill, confidence}` |
| 7 | **Gate** | Engine | Hard legality check before any roll | GATE (per action) | Check `item` field vs inventory; check target alive/present; mark legal/illegal | Classified intent + character inventory + scene | `ParsedAction{legal, rejectionReason?}` |
| 8 | **Initiative roller** | Engine | Sets the order actions resolve in | INITIATIVE | `d20 + DEX mod` per acting char (seeded RNG); deterministic tiebreak | Acting characters + stats | Ordered `initiativeOrder[]` |
| 9 | **Resolver** | Engine | The heart: decides success/failure and state changes, in order | RESOLVE | Per action in order: **roll only if** clause `type.requires_roll` AND outcome uncertain (else `roll=null`, `PROCEEDS`); look up `type→check config`; compute DC from target + **current** scene; roll `d20 + stat + personality bias`; map SUCCESS/PARTIAL/FAIL; emit deltas; mutate working scene | Ordered legal actions + scene state + stats/personality + clause type | `ResolvedAction[]` (ordered) + `StateDelta[]` |
| 10 | **Loot barrier + assign** | Engine | Resolves who gets a dropped item (exclusive resource) | LOOT (only if items dropped) | Claim window; tiebreak by roll or initiative; move item **atomically** into one inventory | `droppedItems` + claimants | `loot_resolved` per item; updated inventories |
| 11 | **Outcome narrator** | **LLM** | Writes prose for what already happened | NARRATE | One generation call over already-resolved data; validate key facts vs structured state, prefer state on conflict; honor `roll=null` (no invented outcome) | Ordered `ResolvedAction[]` + delta summary + loot results | Narration prose (`narrationPlain`) — display only |
| 12 | **Commit** | Engine | Makes changes canonical (tier-1) and advances | COMMIT | Fold deltas into tier-1 (lossless); **promote** climax-relevant nuance to flags; advance clause | `StateDelta[]`, narration | Updated `WorldState`/sheets, next `(chapterIndex, clauseOrder)` |
| 12b | **Chapter summarizer** | Engine | Writes the chapter's carry-forward memory (tier-2) | On chapter's last clause commit | Deterministically stitch the chapter's outcomes/deltas into one summary line — **no LLM** | The chapter's `ResolvedAction[]` + deltas | Appended `chapterSummaries[]` entry |
| 13 | **State push** | Engine | Keeps clients in sync | Throughout | REST snapshot (`GET /state`) + WS events per `contract.md` | Current authoritative state | `PlayerView` + WS events to clients |

## The three LLM boundaries (summary)

| Role | Called | Emits | Never does |
|------|--------|-------|-----------|
| **Scene narrator** (#3) | PRESENT | scene prose | decide facts/outcomes |
| **Intent classifier** (#6) | GATE | structured intent (categories) | decide success/failure |
| **Outcome narrator** (#11) | NARRATE | outcome prose | change state; contradict resolved facts |

Everything else — rolls, DCs, ordering, gating, loot, memory, timing — is
deterministic engine logic. In Phase 1–3 tests, replace #3/#6/#11 with stubs
(canned prose, hand-fed intents) and the entire pipeline stays reproducible.

> Note: **chapter summaries (#12b) are engine-written, not an LLM role.** The LLM
> only ever *reads* them at #2 (context assembly). Memory is written by the engine,
> consumed by the model.
