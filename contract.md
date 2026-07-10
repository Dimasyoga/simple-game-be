# contract.md — Backend ⇄ Frontend Contract (source of truth)

**contractVersion: 2** — bump on any change; both repos check/log a mismatch.

This is the **only** coupling point between the two repositories. Owned by the
backend; copied verbatim into the frontend repo. If it changes, version it and
update both sides. The frontend implements **no game logic** — it consumes this.

> v2 changes vs v1: added `POST /runs/{id}/start` + `lobby`/`LOBBY` status;
> `scenario` → `gameplayId`; clauses addressed by `(chapterIndex, clauseOrder)`;
> events carry chapter+clause; clause `type`/`description` never sent (only
> `requiresInput`); removed the hidden random beat-type concept.

Split of authority:
- **Backend owns:** dice, resolve, loot, barrier clock, all canonical state, LLM.
- **Frontend owns:** rendering, input capture, display-only countdown, WS handling.
- The frontend never computes an outcome, a roll, or a deadline; it only displays
  values the backend sends.

Transport: **REST** for request/response actions, **WebSocket** for real-time
room events (window, resolving, narration, state pushes, discussion).

---

## Conventions
- All timestamps are server-issued ISO-8601 (UTC). The client uses them for
  display-only countdown; it never derives authority from its local clock.
- `runId`, `chapterIndex`, `clauseOrder`, `characterId`, `itemId` as in `schema.md`.
- Auth token identifies the player; the server resolves which character(s) they
  control. Single-player => one character.

---

## REST endpoints

### Create / join / start
```
POST /runs
  body:  { mode: "single" | "multi", gameplayId: string }
  200:   { runId, wsUrl }
  // creates run in status "lobby". Loads the gameplay template.
  // NO LLM call, NO clock, NO WS pushes yet.

POST /runs/{runId}/join
  body:  { characterClass: string, name: string }
  200:   { characterId, isHost: bool }
  // single-player: called once. isHost = true only for the first character to
  // join this run (schema.md Run.hostCharacterId); false for everyone after.

POST /runs/{runId}/start
  body:  { characterId }                      // must be the host
  200:   { started: true }
  409:   { started: false, reason }            // not host / already active / no players
  // transitions lobby -> active and triggers chapter 0, clause 0 (PRESENT).
  // single-player MAY auto-start on first join instead of requiring this call.
```
> `scenario` was renamed to `gameplayId` (the authored template to load).
> Host = the first character to successfully `join` the run (`isHost` in the join
> response). Not renegotiated if the host disconnects (v1); no transfer-of-host
> mechanism yet.

### State snapshot (authoritative pull; also pushed over WS)
```
GET /runs/{runId}/state
  200: PlayerView            // see "Payloads" below
```

### Actions during the input window
A clause is addressed by `(chapterIndex, clauseOrder)`.
```
POST /runs/{runId}/chapters/{chapterIndex}/clauses/{clauseOrder}/action
  body: { characterId, rawText }
  200:  { accepted: true, terminalState: "SUBMITTED" }
  409:  { accepted: false, reason }        // window closed / wrong phase / no-input clause

POST /runs/{runId}/chapters/{chapterIndex}/clauses/{clauseOrder}/pass
  body: { characterId }
  200:  { terminalState: "PASSED" }
```
> There is no "resolve" or "roll" endpoint. Resolution is backend-internal and
> triggered by the barrier (all submitted/passed OR timeout). The client cannot
> initiate it.

### Loot claim (exclusive resource)
```
POST /runs/{runId}/loot/{itemId}/claim
  body: { characterId }
  200:  { claimed: true }                  // registered as claimant, not yet owner
  409:  { claimed: false, reason: "already_resolved" }
```
> The endpoint registers a *claim*. The backend decides the single owner
> (roll/initiative) and pushes `loot_resolved`. The client never assigns items.

---

## WebSocket events

### Server → Client
Each clause-scoped event carries `chapterIndex` + `clauseOrder`.
```
chapter_started    { chapterIndex, title }                          // optional, on chapter advance
clause_presented   { chapterIndex, clauseOrder, scenePlain, requiresInput }
window_opened      { chapterIndex, clauseOrder, deadline: ISO8601 } // only if requiresInput
input_status       { submitted: int, total: int, per: [{characterId, state}] }
resolving          { chapterIndex, clauseOrder }                    // LOCK input, show spinner
clause_narrated    { chapterIndex, clauseOrder, narrationPlain, stateSummary }
state_updated      { view: PlayerView }                             // push new snapshot
loot_window        { items: [{itemId, name}], deadline: ISO8601 }   // claim window open
loot_resolved      { itemId, winnerCharacterId }
discussion_message { fromCharacterId, name, text, at }              // OOC relay, NOT canonical
run_ended          { outcome: string, summaryPlain }
error              { code, message }
```
> The clause `type` (setup/conflict/resolution), `description`, and
> `scriptedDeltas` are backend internals and are **not** sent to the client. The
> FE learns only `requiresInput` (whether to open the input box) via
> `clause_presented`; any item drops from a no-input clause still surface the
> normal way, via `state_updated`/`loot_window`.

### Client → Server (real-time; alternatives to REST for latency)
```
discussion_send    { text }        // OOC only; server broadcasts, never persists to canonical
                                   // and NEVER forwards to the LLM (v1)
```
> `discussion_send` is metagame table-talk. The backend relays it to the room and
> drops it from the canonical path and narrator context. Do not store it in
> WorldState. (Actions and loot claims go over REST above; they may also be
> offered over WS, but REST is the contract of record.)

---

## Payloads

```
PlayerView {
  runId
  chapterIndex
  clauseOrder
  phase: "LOBBY"|"PRESENT"|"WINDOW"|"RESOLVING"|"LOOT"|"NARRATE"|"ENDED"  // FE-facing
  self: CharacterSheet            // active player's full sheet
  party: CharacterSheetPublic[]   // teammates (peek: stats+status, maybe hidden inventory)
  storyLog: { chapterIndex, clauseOrder, kind: "scene"|"narration", textPlain }[]
  window?: { deadline: ISO8601 }  // present only during WINDOW/LOOT
  droppedItems?: { itemId, name }[]
}

CharacterSheet {
  characterId, name, class,
  stats, personality, status,     // shapes per schema.md
  inventory: { itemId, name, type }[]
}

CharacterSheetPublic {            // what teammates are allowed to see
  characterId, name, class, stats, status
  // inventory visibility is a design choice; default: hidden
}
```

---

## Phase → UI mapping (FE must honor)
- `LOBBY` → pre-game: roster + a Start control (host). No scene, no input.
- `PRESENT` → render scene. Open free-input only if `clause_presented.requiresInput`.
- `WINDOW` → free-input open, run display countdown from `window.deadline`.
  Discussion box active.
- `RESOLVING` → **lock free-input**, show "resolving…". No new actions accepted.
- `LOOT` → show claim UI for `droppedItems` with its own countdown.
- `NARRATE` → render `narrationPlain`, then `state_updated` refreshes sheets.
- `ENDED` → show summary.

## Invariants the FE must never break
1. Never compute or display a roll/outcome it wasn't sent.
2. Never trust its own clock for anything but rendering a countdown toward a
   server `deadline`.
3. Never treat discussion text as canonical or expect it to affect the story.
4. Never assign loot locally; only reflect `loot_resolved`.
