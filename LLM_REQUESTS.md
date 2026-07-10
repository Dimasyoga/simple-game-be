# LLM_REQUESTS.md — example requests to the narrator

The only place this backend talks to an LLM is `narrator.Local` (`narrator/local.go`),
invoked from the engine only in PRESENT and NARRATE (spec.md §9, narrator.md's `Narrator`
interface). It speaks the OpenAI-compatible chat completions shape — the common surface
across Ollama, llama.cpp `llama-server`, vLLM, LM Studio, text-generation-webui — so which
model actually answers is a `BaseURL`/`Model` config change, not a code change.

```
POST {BaseURL}/v1/chat/completions
Content-Type: application/json
```

Request body shape (`chatCompletionsRequest` in `narrator/local.go`):
```json
{
  "model": "mistral-7b",
  "messages": [
    { "role": "system", "content": "..." },   // only present if Local.SystemPrompt is set
    { "role": "user", "content": "..." }
  ],
  "stream": false
}
```

Expected response shape (`chatCompletionsResponse`):
```json
{
  "choices": [
    { "message": { "role": "assistant", "content": "..." } }
  ]
}
```
The engine trims whitespace off `choices[0].message.content` and uses it verbatim as
`scenePlain` (PRESENT) or `narrationPlain` (NARRATE). A non-200 status or an empty `choices`
array is treated as an error and propagated up (`narrator: unexpected status %d ...` /
`narrator: response had no choices`).

---

## PRESENT — scene prose before a clause's window opens

Built by `buildPresentPrompt` from an `engine.PresentContext`, which the engine assembles in
`engine.PresentClause` from `run.ChapterSummaries` + `run.ChapterLog` (the bounded memory
context, spec.md §5) plus the current clause template's `Description`/`Type`.

### Example request

Mid-run, chapter 1 (the climax) about to present, after chapter 0 completed:

```json
{
  "model": "mistral-7b",
  "messages": [
    {
      "role": "system",
      "content": "You are the narrator for a fantasy adventure game. Write tight, vivid prose in a few sentences. Never decide outcomes — only set scenes and describe what already happened."
    },
    {
      "role": "user",
      "content": "Narrate the upcoming scene in plain prose. Do not decide outcomes; only set the scene.\n\nStory so far:\nCh.0 — The Door: hero: pick the lock -> SUCCESS\n\nRelevant known facts:\n- cleared:hero: true\n- killList: []\n- relationships: {}\n\nBeat premise: what lies beyond is revealed\nBeat type: conflict\n\nWrite the scene now."
    }
  ],
  "stream": false
}
```

### Example response

```json
{
  "choices": [
    {
      "message": {
        "role": "assistant",
        "content": "Beyond the door, torchlight flickers across a vaulted chamber. At its center, coiled atop a hoard of gold, a dragon lifts its head."
      }
    }
  ]
}
```

The engine takes `content` as `scenePlain` and pushes it to clients via the `clause_presented`
WS event (contract.md) — it introduces no canonical state itself; if the scene implies new
facts, the engine (not the model) is responsible for recording them as structured `SceneState`.

---

## NARRATE — outcome prose after a clause resolves

Built by `buildNarratePrompt` from an `engine.NarrateContext`, called in `engine.FinishClause`
after RESOLVE/LOOT have already decided everything — the model only renders already-resolved
facts, in order, and must not contradict them.

### Example request

```json
{
  "model": "mistral-7b",
  "messages": [
    {
      "role": "system",
      "content": "You are the narrator for a fantasy adventure game. Write tight, vivid prose in a few sentences. Never decide outcomes — only set scenes and describe what already happened."
    },
    {
      "role": "user",
      "content": "Narrate what happened this turn, in the exact order given. Do not change or contradict any outcome.\n\nStory so far:\nCh.0 — The Door: hero: pick the lock -> SUCCESS\n\nRelevant known facts:\n- cleared:hero: true\n- killList: []\n- relationships: {}\n\nResolved actions (already decided; narrate faithfully):\n- hero attempted \"fight dragon\" -> SUCCESS (add_flag:dragon_slain=true)\n\nWrite the narration now."
    }
  ],
  "stream": false
}
```

### Example response

```json
{
  "choices": [
    {
      "message": {
        "role": "assistant",
        "content": "Hero's blade finds its mark between the dragon's scales. With a final roar, the beast collapses — the hoard, and the path forward, are yours."
      }
    }
  ]
}
```

The engine takes `content` as `narrationPlain`, pushes it via the `clause_narrated` WS event,
then COMMIT folds the already-decided `StateDelta`s into canonical state — the model's prose
never changes what happened, only how it reads (rules.md R0, "the golden rule").

---

## Deterministic (`PROCEEDS`) actions still get narrated

A `requires_roll=false` action has `roll: null` and `outcome: "PROCEEDS"` instead of
`SUCCESS`/`PARTIAL`/`FAIL`. The NARRATE prompt renders it the same way — no roll is implied in
the text the model is given:

```
Resolved actions (already decided; narrate faithfully):
- hero attempted "scripted" -> PROCEEDS (drop_item:satchel1=...)
```

The narrator must not invent a success/failure where none was rolled (schema.md invariant 8).

---

## No system prompt configured

`Local.SystemPrompt` is optional (`LOCAL_NARRATOR_SYSTEM_PROMPT` env var in `main.go`). If unset,
the request omits the system message entirely — `messages` starts directly with the `user` role:

```json
{
  "model": "mistral-7b",
  "messages": [
    { "role": "user", "content": "Narrate the upcoming scene in plain prose. ..." }
  ],
  "stream": false
}
```

## Error responses the engine treats as failures

- Non-2xx HTTP status → `narrator: unexpected status 500 from http://localhost:11434`
- `200 OK` with an empty `choices` array → `narrator: response had no choices`
- Either error aborts the clause; `room.RunNextClause` surfaces it as a WS `error` event
  (`{code: "clause_failed", message: "..."}`) rather than silently continuing the story.
