// Package scenario holds registerable game content (Gameplay templates plus
// their check/effect resolvers and item catalogs) that the API server can
// expose as selectable scenarios via Server.RegisterScenario.
package scenario

import (
	"fmt"
	"strings"

	"simple-game-be/engine"
)

// Droppable loot for the demo15 scenario, defined once so Demo15Effect (which
// places the drop) and Demo15Catalog (which lets GATE recognize the item in
// free-text actions) share the exact same ID and Name. IDs are stable — no
// per-character suffix — because the demo is single-player and GATE checks
// inventory by the catalog item's ID, which must match the ID that landed in
// the inventory.
var (
	itemTanto          = engine.Item{ID: "tanto", Name: "tanto", Type: "weapon"}
	itemWarFan         = engine.Item{ID: "war-fan", Name: "war-fan", Type: "weapon"}
	itemSalve          = engine.Item{ID: "healing salve", Name: "healing salve", Type: "consumable"}
	itemSeal           = engine.Item{ID: "iron seal", Name: "iron seal", Type: "key"}
	itemAncestralBlade = engine.Item{ID: "ancestral blade", Name: "ancestral blade", Type: "weapon"}
)

// Demo15Catalog is the run's GATE catalog: reference one of these by name in an
// action ("hurl the tanto") and GATE hard-checks it against the actor's
// inventory. Kaede's katana is deliberately NOT here — "my katana" stays free
// text so her inherent weapon is never gated.
func Demo15Catalog() engine.ItemCatalog {
	return engine.NewItemCatalog(itemTanto, itemWarFan, itemSalve, itemSeal, itemAncestralBlade)
}

func Demo15Gameplay() engine.Gameplay {
	return engine.Gameplay{
		ID:    "demo15",
		Title: "The Ronin of Tsukikage",
		Chapters: []engine.ChapterTemplate{
			{
				Index: 0,
				Title: "The Broken Bridge",
				Clauses: []engine.ClauseTemplate{
					{ChapterIndex: 0, Order: 0, Type: engine.ClauseSetup, Description: "dusk over the burning village of Tsukikage; a shattered torii gate marks the road, and crows wheel above the smoke"},
					{ChapterIndex: 0, Order: 1, Type: engine.ClauseSetup, Description: "Old Genta, a dying scout slumped against a cedar, gasps that the bandit lord Ryuzo and his men hold the village bridge"},
					{ChapterIndex: 0, Order: 2, Type: engine.ClauseConflict, Description: "a bandit sentry bars the narrow mountain path"},
					{ChapterIndex: 0, Order: 3, Type: engine.ClauseConflict, Description: "two bandits ambush at the bridge, their captain leading the charge"},
					{ChapterIndex: 0, Order: 4, Type: engine.ClauseConflict, Description: "the last archer looses a volley from the bridgehead"},
				},
			},
			{
				Index: 1,
				Title: "The Village of Ash",
				Clauses: []engine.ClauseTemplate{
					{ChapterIndex: 1, Order: 0, Type: engine.ClauseSetup, Description: "inside the ash-choked village, a frightened girl named Hana hides in the ruins of a well-house"},
					{ChapterIndex: 1, Order: 1, Type: engine.ClauseConflict, Description: "a looter ransacks the village apothecary"},
					{ChapterIndex: 1, Order: 2, Type: engine.ClauseConflict, Description: "wounded and bleeding, Kaede must force the barred storehouse door"},
					{ChapterIndex: 1, Order: 3, Type: engine.ClauseSetup, Description: "Lady Shizu, the village elder, reveals that Ryuzo hunts the clan's ancestral blade sealed in the mountain keep"},
					{ChapterIndex: 1, Order: 4, Type: engine.ClauseSetup, Description: "night falls at a roadside shrine; the ronin sharpens her katana and steels herself for the keep"},
				},
			},
			{
				Index:   2,
				Title:   "The Oni's Keep",
				IsFinal: true,
				Clauses: []engine.ClauseTemplate{
					{ChapterIndex: 2, Order: 0, Type: engine.ClauseSetup, Description: "the mountain keep looms under a breaking storm, oni-masked guards posted at its gate"},
					{ChapterIndex: 2, Order: 1, Type: engine.ClauseConflict, Description: "a sealed gate mechanism bars the way into the keep"},
					{ChapterIndex: 2, Order: 2, Type: engine.ClauseConflict, Description: "Ryuzo, the bandit lord, meets Kaede's blade in the great hall"},
					{ChapterIndex: 2, Order: 3, Type: engine.ClauseResolution, Description: "the keep falls; Hana and Lady Shizu are freed from the inner cells"},
					{ChapterIndex: 2, Order: 4, Type: engine.ClauseResolution, Description: "dawn breaks; the ronin departs, the village of Tsukikage saved"},
				},
			},
		},
	}
}

func Demo15Check(_ engine.ParsedAction, c engine.Character, _ engine.ClauseType, _ engine.SceneState) engine.CheckSpec {
	// Low DC so legal conflict actions reliably succeed and the intended
	// drop->use item chain never breaks on an unlucky roll. The one intended
	// failure (using the skipped war-fan) is forced by GATE, not the dice.
	return engine.CheckSpec{DC: 6, StatModifier: c.Stats.Dexterity}
}

// demoDrop maps a clause position to the item that drops there (if any), so the
// hero can pick it up and use it on the following clause.
func demoDrop(chapterIdx, clauseOrder int) (engine.Item, bool) {
	switch {
	case chapterIdx == 0 && clauseOrder == 2:
		return itemTanto, true
	case chapterIdx == 0 && clauseOrder == 3:
		return itemWarFan, true
	case chapterIdx == 1 && clauseOrder == 1:
		return itemSalve, true
	case chapterIdx == 1 && clauseOrder == 2:
		return itemSeal, true
	case chapterIdx == 2 && clauseOrder == 2:
		return itemAncestralBlade, true
	default:
		return engine.Item{}, false
	}
}

// demoFlags maps a clause position to the thematic world flag and optional kill
// it records at COMMIT, so the narrator's State/KillList carries real
// continuity from turn to turn (fallen foes stay fallen, taken ground stays
// taken).
func demoFlags(chapterIdx, clauseOrder int) (flag, kill string) {
	switch {
	case chapterIdx == 0 && clauseOrder == 2:
		return "sentry_down", "bandit-sentry"
	case chapterIdx == 0 && clauseOrder == 3:
		return "bridge_captain_down", "bandit-captain"
	case chapterIdx == 1 && clauseOrder == 1:
		return "looter_driven_off", ""
	case chapterIdx == 1 && clauseOrder == 2:
		return "storehouse_forced", ""
	case chapterIdx == 2 && clauseOrder == 1:
		return "keep_gate_open", ""
	case chapterIdx == 2 && clauseOrder == 2:
		return "ryuzo_defeated", "ryuzo"
	default:
		return "", ""
	}
}

func Demo15Effect(_ engine.ParsedAction, c engine.Character, _ engine.ClauseType, outcome engine.Outcome) []engine.StateDelta {
	if outcome == engine.OutcomeFail {
		return nil
	}

	var chapterIdx, clauseOrder int
	for _, cond := range c.Status.Conditions {
		if strings.HasPrefix(cond, "clause_idx:") {
			parts := strings.Split(cond[len("clause_idx:"):], ":")
			if len(parts) == 2 {
				fmt.Sscanf(parts[0], "%d", &chapterIdx)
				fmt.Sscanf(parts[1], "%d", &clauseOrder)
				break
			}
		}
	}

	var deltas []engine.StateDelta
	if flag, kill := demoFlags(chapterIdx, clauseOrder); flag != "" {
		deltas = append(deltas, engine.StateDelta{Op: "add_flag", Target: flag, Value: true})
		if kill != "" {
			deltas = append(deltas, engine.StateDelta{Op: "kill", Target: kill})
		}
	}
	if item, ok := demoDrop(chapterIdx, clauseOrder); ok {
		deltas = append(deltas, engine.DropItemDelta(item))
	}
	return deltas
}
