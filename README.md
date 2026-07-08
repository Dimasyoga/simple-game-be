# simple-game-be

Authoritative game server backend. See `spec.md`, `schema.md`, `rules.md`,
`contract.md`, and `tasks.md` for the full design and build order — this file
just covers running it and hitting the API.

## Requirements

- Go 1.22+

## Running the server

```sh
go run .
```

By default it listens on `:8080` and uses the stubbed narrator (canned prose,
no LLM calls — see `narrator/narrator.go`).

### Environment variables

| Variable | Default | Purpose |
| --- | --- | --- |
| `ADDR` | `:8080` | Address the HTTP server listens on |
| `LOCAL_NARRATOR_URL` | unset (uses the stub) | Base URL of a local model server exposing an OpenAI-compatible `/v1/chat/completions` endpoint (Ollama, llama.cpp's `llama-server`, vLLM, LM Studio, text-generation-webui, ...) |
| `LOCAL_NARRATOR_MODEL` | `""` | Model name to send in each request, as the local server expects it |
| `LOCAL_NARRATOR_SYSTEM_PROMPT` | `""` | Optional system prompt prepended to every PRESENT/NARRATE call |
| `LOCAL_NARRATOR_API_KEY` | unset | Bearer token for the local model server's API |

Example running against a local model server on `localhost:11434`:

```sh
LOCAL_NARRATOR_URL=http://localhost:11434 \
LOCAL_NARRATOR_MODEL=mistral \
go run .
```

The startup log line confirms which narrator is active:

```
simple-game-be listening on :8080 (narrator: stubbed)
simple-game-be listening on :8080 (narrator: local (http://localhost:11434, model=mistral))
```

## Running the tests

```sh
go test ./...
```

Everything is seeded-RNG deterministic and runs with the stubbed narrator —
no network calls, no local model required. Package breakdown:

| Package | Covers |
| --- | --- |
| `engine` | GATE/INITIATIVE/RESOLVE/LOOT/COMMIT mechanics, the clause phase machine, multi-clause runs |
| `room` | The timed input barrier and loot claim window — deadlock-freedom tests included |
| `api` | The REST + WebSocket contract surface, end-to-end against a real HTTP server |
| `narrator` | The seeded RNG helper and the local-model HTTP client (against a fake server) |

Run a single package with `-v` for verbose output, e.g. `go test ./room/... -v`.

## Testing the API manually

Start the server (`go run .`), then drive a run over the real contract from
another terminal.

### 1. Create a run

```sh
curl -s -X POST localhost:8080/runs \
  -H "Content-Type: application/json" \
  -d '{"mode": "multi", "scenario": "demo"}'
# => {"runId":"run-1","wsUrl":"/runs/run-1/ws"}
```

### 2. Join with one or more characters

```sh
curl -s -X POST localhost:8080/runs/run-1/join \
  -H "Content-Type: application/json" \
  -d '{"characterClass": "warrior", "name": "Aria"}'
# => {"characterId":"char-2"}
```

The run starts driving itself (PRESENT → WINDOW → ...) as soon as the first
character joins — watch the WebSocket (step 4) to see it progress.

### 3. Fetch state

```sh
curl -s "localhost:8080/runs/run-1/state?characterId=char-2" | jq
```

`phase` tells you what's happening (`WINDOW`, `RESOLVING`, `LOOT`, `ENDED`),
and `window.deadline` is the server-issued countdown target while a window is
open.

### 4. Watch events over WebSocket

Use any WS client, e.g. [`websocat`](https://github.com/vi/websocat):

```sh
websocat "ws://localhost:8080/runs/run-1/ws?characterId=char-2"
```

You'll see a stream of `{"event": "...", "data": {...}}` messages:
`clause_presented`, `window_opened`, `input_status`, `resolving`,
`loot_window`, `loot_resolved`, `clause_narrated`, `state_updated`, and
eventually `run_ended`.

### 5. Submit an action during the WINDOW phase

Use the `clauseIndex` from the latest `window_opened` event (or `GET state`):

```sh
curl -s -X POST localhost:8080/runs/run-1/clauses/0/action \
  -H "Content-Type: application/json" \
  -d '{"characterId": "char-2", "rawText": "open the door"}'
# => {"accepted":true,"terminalState":"SUBMITTED"}
```

Or pass instead of acting:

```sh
curl -s -X POST localhost:8080/runs/run-1/clauses/0/pass \
  -H "Content-Type: application/json" \
  -d '{"characterId": "char-2"}'
# => {"terminalState":"PASSED"}
```

A 409 response means the window already closed or the clause index is stale
— re-fetch state to see the current phase/index.

### 6. Claim a dropped item during LOOT

Watch for a `loot_window` event, then:

```sh
curl -s -X POST localhost:8080/runs/run-1/loot/trinket-char-2/claim \
  -H "Content-Type: application/json" \
  -d '{"characterId": "char-2"}'
# => {"claimed":true}
```

The backend resolves ownership (roll/initiative) and pushes `loot_resolved`;
the client never assigns the item itself.

### 7. Send OOC discussion over the WebSocket

Discussion is out-of-character table-talk: relayed to everyone connected,
never persisted to canonical state, never sent to the narrator.

```json
{"event": "discussion_send", "text": "should we open it?"}
```

sent as a WS text frame produces a broadcast `discussion_message` event to
every connected client.

## Project layout

- `engine/` — pure, deterministic game rules (no network, no LLM calls)
- `room/` — per-run concurrency: the timed input barrier, loot claim windows
- `api/` — the public REST + WebSocket surface (`contract.md`)
- `narrator/` — the LLM seam: `Stub` (canned prose) and `Local` (any
  OpenAI-compatible local model server)
