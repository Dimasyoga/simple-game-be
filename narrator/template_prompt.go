package narrator

import (
	"fmt"
	"sort"
	"strings"
)

func formatState(state map[string]any) string {
	keys := make([]string, 0, len(state))
	for k := range state {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	for _, k := range keys {
		b.WriteString(fmt.Sprintf("- %s: %v\n", k, state[k]))
	}
	return b.String()
}

func buildPresentPrompt(pc PresentContext) (fullPrompt string, historyData string) {
	var fb, hb strings.Builder

	fb.WriteString("Describe the upcoming scene in plain prose. Do not decide outcomes; only set the scene.\n\n")
	fb.WriteString("Sections:\n")
	fb.WriteString("  State — current world facts to keep consistent.\n")
	fb.WriteString("  Beat premise — the narrative hook for this turn.\n")
	fb.WriteString("  Beat type — the category of this beat (e.g., combat, exploration).\n\n")

	if len(pc.RelevantState) > 0 {
		stateStr := formatState(pc.RelevantState)
		fb.WriteString("State:\n" + stateStr + "\n")
		hb.WriteString("State:\n" + stateStr + "\n")
	}

	fb.WriteString("Beat premise: " + pc.BeatPremise + "\n")
	hb.WriteString("Beat premise: " + pc.BeatPremise + "\n")
	if pc.BeatType != "" {
		fb.WriteString("Beat type: " + pc.BeatType + "\n")
		hb.WriteString("Beat type: " + pc.BeatType + "\n")
	}

	return fb.String(), hb.String()
}

func buildNarratePrompt(nc NarrateContext) (fullPrompt string, historyData string) {
	var fb, hb strings.Builder

	fb.WriteString("Narrate what happened this turn, in the exact order given. Do not change or contradict any outcome.\n\n")
	fb.WriteString("Sections:\n")
	fb.WriteString("  State — current world facts to keep consistent.\n")
	fb.WriteString("  Resolved actions — each line shows character, intent, outcome, and optional summary. Narrate faithfully in order.\n\n")

	if len(nc.RelevantState) > 0 {
		stateStr := formatState(nc.RelevantState)
		fb.WriteString("State:\n" + stateStr + "\n")
		hb.WriteString("State:\n" + stateStr + "\n")
	}

	fb.WriteString("Resolved actions:\n")
	hb.WriteString("Resolved actions:\n")
	for _, a := range nc.ResolvedActions {
		line := fmt.Sprintf("- %s attempted %q -> %s", a.CharacterName, a.Intent, a.Outcome)
		if a.Summary != "" {
			line += " (" + a.Summary + ")"
		}
		line += "\n"
		fb.WriteString(line)
		hb.WriteString(line)
	}

	return fb.String(), hb.String()
}
