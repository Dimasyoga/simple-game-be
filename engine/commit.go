package engine

import (
	"fmt"
	"strings"
)

// Commit implements rules.md R9/R9b: fold every StateDelta from an already-
// resolved clause into canonical, cross-clause state (tier-1, lossless),
// record the clause's resolved actions into the chapter's running log, then
// advance the clause order. Flag promotion (R9.2) is not a separate step
// here — it's just an "add_flag" delta among the others; whoever builds the
// ResolvedAction (the EffectResolver) decides which nuance gets promoted.
//
// When this was the chapter's last clause, the engine writes a deterministic
// chapter summary (R9b, no LLM call), appends it to ChapterSummaries, resets
// ChapterLog, and advances to the next chapter — or, if this was the final
// chapter, ends the run (R10).
func Commit(run *Run, resolved []ResolvedAction) {
	for _, ra := range resolved {
		for _, d := range ra.Deltas {
			ApplyDelta(run, d)
		}
	}
	run.ChapterLog = append(run.ChapterLog, resolved...)
	run.ClauseOrder++

	chapter := run.Gameplay.Chapters[run.ChapterIndex]
	if run.ClauseOrder < len(chapter.Clauses) {
		return
	}

	run.ChapterSummaries = append(run.ChapterSummaries, buildChapterSummary(run.ChapterIndex, chapter, run.ChapterLog))
	run.ChapterLog = nil

	if chapter.IsFinal {
		run.Status = "ended"
		return
	}
	run.ChapterIndex++
	run.ClauseOrder = 0
}

// buildChapterSummary implements R9b: a deterministic, engine-written
// one-line-per-action summary of everything that happened in a completed
// chapter. Never LLM-written — this feeds every later chapter's narrator
// context, so a hallucination here would propagate downstream.
func buildChapterSummary(chapterIndex int, chapter ChapterTemplate, log []ResolvedAction) string {
	label := fmt.Sprintf("Ch.%d", chapterIndex)
	if chapter.Title != "" {
		label += " — " + chapter.Title
	}
	if len(log) == 0 {
		return label + ": nothing of note happened."
	}
	lines := make([]string, 0, len(log))
	for _, ra := range log {
		lines = append(lines, chapterLogLine(ra))
	}
	return label + ": " + strings.Join(lines, "; ")
}

// ApplyDelta is the ONLY function that mutates canonical WorldState or
// Character fields (schema.md invariant 2). Every StateDelta.Target that
// names a character must be a characterId; deltas for unknown Ops or
// characterIds are dropped rather than panicking, since malformed deltas are
// a producer bug to catch in tests, not a reason to crash the run.
func ApplyDelta(run *Run, d StateDelta) {
	switch d.Op {
	case "add_flag":
		if run.WorldState.Flags == nil {
			run.WorldState.Flags = map[string]any{}
		}
		run.WorldState.Flags[d.Target] = d.Value

	case "kill":
		run.WorldState.KillList = append(run.WorldState.KillList, d.Target)
		if idx := findCharacter(run.Characters, d.Target); idx != -1 {
			run.Characters[idx].Status.Alive = false
		}

	case "hp":
		if idx := findCharacter(run.Characters, d.Target); idx != -1 {
			if delta, ok := d.Value.(int); ok {
				run.Characters[idx].Stats.HP += delta
			}
		}

	case "add_item":
		if idx := findCharacter(run.Characters, d.Target); idx != -1 {
			if item, ok := d.Value.(Item); ok {
				run.Characters[idx].Inventory = append(run.Characters[idx].Inventory, item)
			}
		}

	case "morality":
		if idx := findCharacter(run.Characters, d.Target); idx != -1 {
			if delta, ok := d.Value.(int); ok {
				run.Characters[idx].Personality.Morality += delta
			}
		}

	case "relationship":
		if run.WorldState.Relationships == nil {
			run.WorldState.Relationships = map[string]int{}
		}
		if delta, ok := d.Value.(int); ok {
			run.WorldState.Relationships[d.Target] += delta
		}
	}
}

func findCharacter(characters []Character, id string) int {
	for i, c := range characters {
		if c.ID == id {
			return i
		}
	}
	return -1
}
