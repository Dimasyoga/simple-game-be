# contract.md — Backend ⇄ Frontend Contract (source of truth)

This is the **only** coupling point between the two repositories. Owned by the
backend; copied verbatim into the frontend repo. If it changes, version it and
update both sides. The frontend implements **no game logic** — it consumes this.

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
- `runId`, `clauseIndex`, `characterId`, `itemId` as in `schema.md`.
- Auth token identifies the player; the server resolves which character(s) they
  control. Single-player => one character.

---

## REST endpoints

### Create / join
```
POST /runs
  body:  { mode: "single" | "multi", scenario: string }
  200:   { runId, wsUrl }

POST /runs/{runId}/join
  body:  { characterClass: string, name: string }
  200:   { characterId }
```

### State snapshot (authoritative pull; also pushed over WS)
```
GET /runs/{runId}/state
  200: PlayerView            // see "Payloads" below
```

### Actions during the input window
```
POST /runs/{runId}/clauses/{clauseIndex}/action
  body: { characterId, rawText }
  200:  { accepted: true, terminalState: "SUBMITTED" }
  409:  { accepted: false, reason }        // window closed / wrong phase

POST /runs/{runId}/clauses/{clauseIndex}/pass
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
```
clause_presented   { clauseIndex, beatContextLabel?, scenePlain }   // scene prose
window_opened      { clauseIndex, deadline: ISO8601 }               // display-only countdown
input_status       { submitted: int, total: int, per: [{characterId, state}] }
resolving          { clauseIndex }                                  // LOCK input, show spinner
clause_narrated    { clauseIndex, narrationPlain, stateSummary }    // resolved prose
state_updated      { view: PlayerView }                             // push new snapshot
loot_window        { items: [{itemId, name}], deadline: ISO8601 }   // claim window open
loot_resolved      { itemId, winnerCharacterId }
discussion_message { fromCharacterId, name, text, at }              // OOC relay, NOT canonical
run_ended          { outcome: string, summaryPlain }
error              { code, message }
```

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
  clauseIndex
  phase: "PRESENT"|"WINDOW"|"RESOLVING"|"LOOT"|"NARRATE"|"ENDED"   // FE-facing phase
  self: CharacterSheet            // active player's full sheet
  party: CharacterSheetPublic[]   // teammates (peek: stats+status, maybe hidden inventory)
  storyLog: { clauseIndex, kind: "scene"|"narration", textPlain }[]
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

> Note: the FE receives **beatContextLabel only if you choose to reveal it**. The
> rolled beat *type* is hidden from players by design — default: do not send it.

---

## Phase → UI mapping (FE must honor)
- `PRESENT` / `WINDOW` → show scene, open free-input, run display countdown from
  `window.deadline`. Discussion box active.
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
