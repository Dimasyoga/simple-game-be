# Manual endpoint testing (contract v2)

All requests assume the server is running locally with defaults:

```
go run .
```

listening on `http://localhost:8080`, narrator stubbed (no `LOCAL_NARRATOR_URL` needed — see README).

Every `curl` example below is one line so it pastes cleanly into PowerShell or bash. Replace
`RUN_ID` / `CHAR_ID` / `ITEM_ID` with values you get back from earlier calls.

Note on timing: `WindowDuration` defaults to 5 minutes and `LootWindowDuration` to 30 seconds
(main.go / api/server.go `NewServer`). If you want fast manual runs, lower these before starting
the server, or just be patient / rely on the WS stream to see windows open and close in real time.

The built-in demo gameplay (`gameplayId: "demo"`, api/demo.go) has 2 chapters, 1 clause each:
chapter 0 (`conflict`, "a locked door blocks the way forward") and chapter 1 — the final/climax
chapter (`conflict`, "what lies beyond is revealed"). Both require input and a dice roll.

---

## 1. POST /runs — create a run

### 1.1 Positive: create a run

Request:
```bash
curl -i -X POST http://localhost:8080/runs \
  -H "Content-Type: application/json" \
  -d "{\"mode\":\"multi\",\"gameplayId\":\"demo\"}"
```

Expected: `200 OK`
```json
{"runId":"run-1","wsUrl":"/runs/run-1/ws"}
```
(`runId` increments per server process — `run-2`, `run-3`, ... on subsequent calls.) The run is
created in `lobby` status: no LLM call, no clock, no WS pushes yet (contract.md v2).

### 1.2 Negative: malformed JSON body

Request:
```bash
curl -i -X POST http://localhost:8080/runs \
  -H "Content-Type: application/json" \
  -d "{not valid json"
```

Expected: `400 Bad Request`, plain text body `invalid body` (api/server.go `handleCreateRun`).

### 1.3 Negative: empty body

Request:
```bash
curl -i -X POST http://localhost:8080/runs -H "Content-Type: application/json" -d ""
```

Expected: `400 Bad Request` — `json.Decode` fails on EOF, same `invalid body` response.
(Note: `mode`/`gameplayId` aren't actually validated once JSON parses — an empty `{}` body is
accepted as 1.1's positive case, since the demo gameplay ignores those fields.)

---

## 2. POST /runs/{runId}/join — join a run

### 2.1 Positive: first player joins (becomes host)

Request:
```bash
curl -i -X POST http://localhost:8080/runs/run-1/join \
  -H "Content-Type: application/json" \
  -d "{\"characterClass\":\"warrior\",\"name\":\"Aria\"}"
```

Expected: `200 OK`
```json
{"characterId":"char-2","isHost":true}
```
The first character to successfully join a run becomes its host (contract.md v2) — this does
**not** start the run; it still sits in `lobby` until `POST /start`.

### 2.2 Positive: second player joins the same run (not host)

Request:
```bash
curl -i -X POST http://localhost:8080/runs/run-1/join \
  -H "Content-Type: application/json" \
  -d "{\"characterClass\":\"archer\",\"name\":\"Bram\"}"
```

Expected: `200 OK`, a new `characterId` (e.g. `char-3`), `"isHost":false`.

### 2.3 Negative: run does not exist

Request:
```bash
curl -i -X POST http://localhost:8080/runs/run-999/join \
  -H "Content-Type: application/json" \
  -d "{\"characterClass\":\"warrior\",\"name\":\"Aria\"}"
```

Expected: `404 Not Found`, plain text `run not found`.

### 2.4 Negative: malformed body on an existing run

Request:
```bash
curl -i -X POST http://localhost:8080/runs/run-1/join \
  -H "Content-Type: application/json" \
  -d "not json"
```

Expected: `400 Bad Request`, `invalid body`.

---

## 3. POST /runs/{runId}/start — start the run (host only)

### 3.1 Negative: non-host tries to start

Request:
```bash
curl -i -X POST http://localhost:8080/runs/run-1/start \
  -H "Content-Type: application/json" \
  -d "{\"characterId\":\"char-3\"}"
```

Expected: `409 Conflict`
```json
{"started":false,"reason":"not host"}
```

### 3.2 Positive: host starts the run

Request:
```bash
curl -i -X POST http://localhost:8080/runs/run-1/start \
  -H "Content-Type: application/json" \
  -d "{\"characterId\":\"char-2\"}"
```

Expected: `200 OK`
```json
{"started":true}
```
This transitions `lobby` → `active` and triggers chapter 0, clause 0 (PRESENT) — only now does
the background clause loop start, the narrator get called, and WS events begin flowing.

### 3.3 Negative: starting an already-active run

Request (repeat 3.2 after it already succeeded):
```bash
curl -i -X POST http://localhost:8080/runs/run-1/start \
  -H "Content-Type: application/json" \
  -d "{\"characterId\":\"char-2\"}"
```

Expected: `409 Conflict`, `{"started":false,"reason":"already active"}`.

### 3.4 Negative: starting with no players joined

Request (against a freshly created run with nobody joined yet):
```bash
curl -i -X POST http://localhost:8080/runs/run-2/start \
  -H "Content-Type: application/json" \
  -d "{\"characterId\":\"anyone\"}"
```

Expected: `409 Conflict`, `{"started":false,"reason":"no players"}` — note this also implies
`reason:"not host"` never fires first here, since with zero characters nobody could be host
either; the handler checks player count before host identity (api/server.go `handleStart`).

### 3.5 Negative: run does not exist

Request:
```bash
curl -i -X POST http://localhost:8080/runs/run-999/start \
  -H "Content-Type: application/json" \
  -d "{\"characterId\":\"char-2\"}"
```

Expected: `404 Not Found`, `run not found`.

---

## 4. GET /runs/{runId}/state — poll player view

### 4.1 Positive: state while still in the lobby

Request (before calling `/start`):
```bash
curl -i "http://localhost:8080/runs/run-1/state?characterId=char-2"
```

Expected: `200 OK`, `"phase":"LOBBY"`, empty `storyLog`, no `window`.

### 4.2 Positive: state after start, during a clause window

Request:
```bash
curl -i "http://localhost:8080/runs/run-1/state?characterId=char-2"
```

Expected: `200 OK`, a `PlayerView` JSON body, e.g.:
```json
{
  "runId": "run-1",
  "chapterIndex": 0,
  "clauseOrder": 0,
  "phase": "WINDOW",
  "self": {
    "characterId": "char-2",
    "name": "Aria",
    "class": "warrior",
    "stats": {"strength":10,"dexterity":10,"intelligence":10,"charisma":10,"hp":10,"maxHp":10},
    "personality": {},
    "status": {"alive": true},
    "inventory": []
  },
  "party": [ { "characterId": "char-3", "name": "Bram", "class": "archer", "...": "..." } ],
  "storyLog": [ { "chapterIndex": 0, "clauseOrder": 0, "kind": "scene", "textPlain": "[stub scene] a locked door blocks the way forward" } ],
  "window": { "deadline": "2026-07-10T12:34:56Z" }
}
```
`phase` will be one of `LOBBY`, `WINDOW`, `RESOLVING`, `LOOT`, `ENDED` depending on where the run
is when you poll (see `currentPhase()` in api/server.go).

### 4.3 Positive: unmatched characterId is not an error

Request:
```bash
curl -i "http://localhost:8080/runs/run-1/state?characterId=nobody-such"
```

Expected: `200 OK` still — no 404 for a bad/unknown `characterId`; `self` is simply the
zero-value `CharacterSheet` since no character with that ID matches (`playerView` loop finds
nothing). Same for an entirely missing `characterId` query param.

### 4.4 Negative: run does not exist

Request:
```bash
curl -i "http://localhost:8080/runs/run-999/state?characterId=char-2"
```

Expected: `404 Not Found`, `run not found`.

---

## 5. POST /runs/{runId}/chapters/{chapterIndex}/clauses/{clauseOrder}/action — submit an action

A clause is now addressed by `(chapterIndex, clauseOrder)` rather than a flat index. Do this
while the run is in the `WINDOW` phase for that pair (watch the WS `window_opened` event, or poll
`GET .../state` and check `chapterIndex`/`clauseOrder`/`phase`).

### 5.1 Positive: accepted action during the open window

Request:
```bash
curl -i -X POST http://localhost:8080/runs/run-1/chapters/0/clauses/0/action \
  -H "Content-Type: application/json" \
  -d "{\"characterId\":\"char-2\",\"rawText\":\"pick the lock\"}"
```

Expected: `200 OK`
```json
{"accepted":true,"terminalState":"SUBMITTED"}
```

### 5.2 Negative: wrong chapter/clause (already advanced)

Request (assuming the run is actually on chapter 1 clause 0 now):
```bash
curl -i -X POST http://localhost:8080/runs/run-1/chapters/0/clauses/0/action \
  -H "Content-Type: application/json" \
  -d "{\"characterId\":\"char-2\",\"rawText\":\"pick the lock\"}"
```

Expected: `409 Conflict`
```json
{"accepted":false,"reason":"wrong phase"}
```

### 5.3 Negative: window already closed for the current clause

Request: submit twice in a row for the same character/clause after the window has resolved (or
wait past `WindowDuration`), then:
```bash
curl -i -X POST http://localhost:8080/runs/run-1/chapters/0/clauses/0/action \
  -H "Content-Type: application/json" \
  -d "{\"characterId\":\"char-2\",\"rawText\":\"pick the lock\"}"
```

Expected: `409 Conflict`
```json
{"accepted":false,"reason":"window closed"}
```

### 5.4 Negative: non-numeric chapterIndex/clauseOrder

Request:
```bash
curl -i -X POST http://localhost:8080/runs/run-1/chapters/abc/clauses/0/action \
  -H "Content-Type: application/json" \
  -d "{\"characterId\":\"char-2\",\"rawText\":\"pick the lock\"}"
```

Expected: `400 Bad Request`, `invalid chapterIndex/clauseOrder`.

### 5.5 Negative: run does not exist

Request:
```bash
curl -i -X POST http://localhost:8080/runs/run-999/chapters/0/clauses/0/action \
  -H "Content-Type: application/json" \
  -d "{\"characterId\":\"char-2\",\"rawText\":\"pick the lock\"}"
```

Expected: `404 Not Found`, `run not found`.

---

## 6. POST /runs/{runId}/chapters/{chapterIndex}/clauses/{clauseOrder}/pass — pass on the current clause

### 6.1 Positive: pass during the open window

Request:
```bash
curl -i -X POST http://localhost:8080/runs/run-1/chapters/0/clauses/0/pass \
  -H "Content-Type: application/json" \
  -d "{\"characterId\":\"char-3\"}"
```

Expected: `200 OK`
```json
{"terminalState":"PASSED"}
```

### 6.2 Negative: wrong/stale chapter+clause or closed window

Request (run against an already-resolved chapter 0 clause 0):
```bash
curl -i -X POST http://localhost:8080/runs/run-1/chapters/0/clauses/0/pass \
  -H "Content-Type: application/json" \
  -d "{\"characterId\":\"char-3\"}"
```

Expected: `409 Conflict`, plain text `window closed`. Note this endpoint returns a plain
`http.Error`, not a JSON body, unlike `/action`'s `409` (api/server.go `handlePass`).

### 6.3 Negative: non-numeric chapterIndex/clauseOrder

Request:
```bash
curl -i -X POST http://localhost:8080/runs/run-1/chapters/0/clauses/xyz/pass \
  -H "Content-Type: application/json" \
  -d "{\"characterId\":\"char-3\"}"
```

Expected: `400 Bad Request`, `invalid chapterIndex/clauseOrder`.

### 6.4 Negative: run does not exist

Request:
```bash
curl -i -X POST http://localhost:8080/runs/run-999/chapters/0/clauses/0/pass \
  -H "Content-Type: application/json" \
  -d "{\"characterId\":\"char-3\"}"
```

Expected: `404 Not Found`, `run not found`.

---

## 7. POST /runs/{runId}/loot/{itemId}/claim — claim dropped loot

Loot only exists during a `LOOT` phase, after a clause resolves with a drop (the demo gameplay
drops `trinket-<characterId>` on any non-fail outcome — see api/demo.go `demoEffect`). Watch the
WS `loot_window` event for the real `itemId`, or check `GET .../state`.

### 7.1 Positive: first claim on an open loot window

Request:
```bash
curl -i -X POST http://localhost:8080/runs/run-1/loot/trinket-char-2/claim \
  -H "Content-Type: application/json" \
  -d "{\"characterId\":\"char-2\"}"
```

Expected: `200 OK`
```json
{"claimed":true}
```
(Resolution of *who wins* a contested item happens asynchronously and is announced over WS as
`loot_resolved` — a `200`/`claimed:true` here only means the claim was accepted into the window,
not necessarily that this character won it if multiple people claimed the same item.)

### 7.2 Negative: claiming after the loot window already resolved

Request (claim the same item again after `loot_resolved` has already fired):
```bash
curl -i -X POST http://localhost:8080/runs/run-1/loot/trinket-char-2/claim \
  -H "Content-Type: application/json" \
  -d "{\"characterId\":\"char-3\"}"
```

Expected: `409 Conflict`
```json
{"claimed":false,"reason":"already_resolved"}
```

### 7.3 Negative: unknown itemId

Request:
```bash
curl -i -X POST http://localhost:8080/runs/run-1/loot/does-not-exist/claim \
  -H "Content-Type: application/json" \
  -d "{\"characterId\":\"char-2\"}"
```

Expected: `409 Conflict`, `{"claimed":false,"reason":"already_resolved"}` — the handler doesn't
distinguish "never existed" from "already resolved"; both fail the same `ClaimLoot` check (room
package). Worth knowing since the error message can be misleading for a typo'd item ID.

### 7.4 Negative: run does not exist

Request:
```bash
curl -i -X POST http://localhost:8080/runs/run-999/loot/trinket-char-2/claim \
  -H "Content-Type: application/json" \
  -d "{\"characterId\":\"char-2\"}"
```

Expected: `404 Not Found`, `run not found`.

---

## 8. GET /runs/{runId}/ws — WebSocket event stream

Not a REST call — test with a WS client. Two easy options:

**websocat** (`choco install websocat` or download from GitHub releases):
```bash
websocat "ws://localhost:8080/runs/run-1/ws?characterId=char-2"
```

**Browser devtools console** (paste, adjust IDs):
```js
const ws = new WebSocket("ws://localhost:8080/runs/run-1/ws?characterId=char-2");
ws.onmessage = (e) => console.log(JSON.parse(e.data));
```

### 8.1 Positive: connect and observe the event sequence

Expected event order for a full run (fields per api/types.go):
1. `chapter_started` — `{chapterIndex, title}` — fires once per chapter, including chapter 0,
   right before that chapter's first clause is presented.
2. `clause_presented` — `{chapterIndex, clauseOrder, scenePlain, requiresInput}` (`"[stub scene]
   ..."` since narrator is stubbed). The demo gameplay's clauses are both `requiresInput:true`.
3. `window_opened` — `{chapterIndex, clauseOrder, deadline}` — only fires if `requiresInput` was
   true; a `requires_input=false` clause (a `setup`-type clause with `scriptedDeltas`, not used
   by the demo gameplay) skips straight past this.
4. `input_status` — `{submitted, total, per:[{characterId, state}]}` (fires as players act/pass)
5. `resolving` — `{chapterIndex, clauseOrder}`
6. `loot_window` — `{items:[{itemId,name}], deadline}` (only if something dropped)
7. `loot_resolved` — `{itemId, winnerCharacterId}` (only if a loot window opened)
8. `clause_narrated` — `{chapterIndex, clauseOrder, narrationPlain, stateSummary}` (`"[stub
   narration] ..."`; `stateSummary` is the most recent completed chapter's engine-written summary,
   empty until the first chapter finishes)
9. `state_updated` — `{view: <PlayerView>}`
10. steps 1-9 repeat per clause/chapter; the final (climax) chapter's last clause ends with
    `run_ended` — `{outcome, summaryPlain}`, then the server closes the socket.

### 8.2 Positive: send discussion (OOC) chat over the socket

Request (send this as a WS text frame once connected):
```json
{"event":"discussion_send","text":"good luck team"}
```

Expected: every connected client (including the sender) receives:
```json
{"event":"discussion_message","data":{"fromCharacterId":"char-2","name":"Aria","text":"good luck team","at":"2026-07-10T12:34:56Z"}}
```
This message is never persisted to `storyLog` and never reaches the narrator (contract.md).

### 8.3 Negative: connect to a run that doesn't exist

Request:
```bash
websocat "ws://localhost:8080/runs/run-999/ws?characterId=char-2"
```

Expected: HTTP `404 Not Found` at the upgrade attempt (handshake fails before it becomes a WS
connection) — `run not found` body, connection never upgrades.

### 8.4 Negative: send a non-discussion event

Request (WS text frame):
```json
{"event":"something_else","text":"ignored"}
```

Expected: no response, no broadcast — `handleWS` silently ignores any `Event` other than
`"discussion_send"` (api/server.go, the `if msg.Event != "discussion_send" { continue }` guard).

### 8.5 Negative: send invalid JSON over the socket

Request (WS text frame): send the literal string `not json` instead of a JSON object.

Expected: the server's `conn.ReadJSON` fails, the read loop returns, and the server closes the
connection for that client (no error frame is sent back — the disconnect *is* the signal).

---

## Suggested manual walkthrough (happy path end-to-end)

1. §1.1 create run → note `runId`.
2. §8 open a WS connection for `char-2` (join first, see next step, then connect — or connect
   before joining, since `run not found` only triggers on a missing run, not a missing
   character).
3. §2.1 join as Aria → note `char-2`, confirm `isHost:true`.
4. §2.2 join as Bram → note `char-3`, confirm `isHost:false`.
5. §4.1 confirm `GET .../state` shows `"phase":"LOBBY"` — the run has not started yet.
6. §3.1 confirm a non-host `/start` 409s.
7. §3.2 host (`char-2`) calls `/start` → run transitions to `active`; the background clause loop
   begins and the WS stream starts firing.
8. Watch WS for `chapter_started` (chapterIndex 0) then `clause_presented`/`window_opened`; note
   `chapterIndex`/`clauseOrder`.
9. §5.1 submit an action for `char-2` on that chapter/clause.
10. §6.1 pass for `char-3` on the same chapter/clause.
11. Watch WS: `resolving` → maybe `loot_window` → `loot_resolved` → `clause_narrated` →
    `state_updated`. If a `loot_window` fires, race §7.1 in before its `LootWindowDuration`
    expires.
12. Watch for `chapter_started` (chapterIndex 1, the final/climax chapter) and repeat steps 8-11.
13. Watch for `run_ended`; confirm via §4.2-style poll that `phase` is now `ENDED` and any
    claimed loot appears in `self.inventory`.
