package engine

import (
	"sort"
	"strings"
)

// ItemCatalog is the set of known item names GATE can recognize in free
// text, keyed by lowercase name. It is deliberately separate from any one
// character's inventory: a player can reference an item that exists in the
// fiction ("shoot with gun") without owning it, and GATE must catch that.
type ItemCatalog map[string]Item

func NewItemCatalog(items ...Item) ItemCatalog {
	c := make(ItemCatalog, len(items))
	for _, it := range items {
		c[strings.ToLower(it.Name)] = it
	}
	return c
}

// Gate implements rules.md R4: parse each SUBMITTED input and hard-check
// item references against the acting character's actual inventory, before
// any generation happens. PASSED/TIMED_OUT inputs are no-ops and are not
// gated. Illegal actions are marked, never silently dropped — RESOLVE
// decides the no-op/improvised-fallback policy (R4.3).
func Gate(inputs map[string]ActionInput, characters map[string]Character, catalog ItemCatalog) map[string]ParsedAction {
	names := catalogNamesLongestFirst(catalog)

	out := make(map[string]ParsedAction, len(inputs))
	for charID, input := range inputs {
		if input.TerminalState != InputSubmitted {
			continue
		}

		text := strings.ToLower(strings.TrimSpace(input.RawText))
		pa := ParsedAction{CharacterID: charID, Intent: text, Legal: true}

		for _, name := range names {
			if !strings.Contains(text, name) {
				continue
			}
			item := catalog[name]
			pa.ReferencedItemID = item.ID
			if !characterHasItem(characters[charID], item.ID) {
				pa.Legal = false
				pa.RejectionReason = "no " + item.Name
			}
			break
		}

		out[charID] = pa
	}
	return out
}

// catalogNamesLongestFirst orders names so multi-word item names are matched
// before shorter substrings of them, and ties break alphabetically — GATE
// must be deterministic given identical input.
func catalogNamesLongestFirst(catalog ItemCatalog) []string {
	names := make([]string, 0, len(catalog))
	for name := range catalog {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool {
		if len(names[i]) != len(names[j]) {
			return len(names[i]) > len(names[j])
		}
		return names[i] < names[j]
	})
	return names
}

func characterHasItem(c Character, itemID string) bool {
	for _, it := range c.Inventory {
		if it.ID == itemID {
			return true
		}
	}
	return false
}
