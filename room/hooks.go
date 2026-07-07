package room

import (
	"time"

	"simple-game-be/engine"
)

// Hooks lets a transport layer (api/) observe clause phase transitions to
// push contract.md's WebSocket events, without Room depending on any
// transport type. Every field is optional; a nil Hooks or nil field is a
// silent no-op.
type Hooks struct {
	OnScenePresented func(scenePlain string)
	OnWindowOpened   func(deadline time.Time)
	OnInputStatus    func(actingIDs []string, status map[string]engine.InputTerminal)
	OnResolving      func()
	OnLootWindow     func(items []engine.Item, deadline time.Time)
	OnLootResolved   func(claim engine.LootClaim)
}

func (h *Hooks) scenePresented(s string) {
	if h != nil && h.OnScenePresented != nil {
		h.OnScenePresented(s)
	}
}

func (h *Hooks) windowOpened(d time.Time) {
	if h != nil && h.OnWindowOpened != nil {
		h.OnWindowOpened(d)
	}
}

func (h *Hooks) inputStatus(actingIDs []string, status map[string]engine.InputTerminal) {
	if h != nil && h.OnInputStatus != nil {
		h.OnInputStatus(actingIDs, status)
	}
}

func (h *Hooks) resolving() {
	if h != nil && h.OnResolving != nil {
		h.OnResolving()
	}
}

func (h *Hooks) lootWindow(items []engine.Item, d time.Time) {
	if h != nil && h.OnLootWindow != nil {
		h.OnLootWindow(items, d)
	}
}

func (h *Hooks) lootResolved(c engine.LootClaim) {
	if h != nil && h.OnLootResolved != nil {
		h.OnLootResolved(c)
	}
}
