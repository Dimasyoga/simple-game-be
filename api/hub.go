package api

import (
	"sync"

	"github.com/gorilla/websocket"
)

// hub fans out server->client WS events (contract.md) to every connection
// currently on a run. Writes are serialized with a single mutex — gorilla's
// websocket.Conn forbids concurrent writes from multiple goroutines, and
// traffic volume here (a handful of clients per room) doesn't warrant
// anything finer-grained.
type hub struct {
	mu    sync.Mutex // guards conns
	wmu   sync.Mutex // serializes writes across all connections
	conns map[*websocket.Conn]bool
}

func newHub() *hub {
	return &hub{conns: make(map[*websocket.Conn]bool)}
}

func (h *hub) add(c *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.conns[c] = true
}

func (h *hub) remove(c *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.conns, c)
}

// broadcast pushes a {event, data} envelope to every connected client.
// Best-effort: a write error just drops that one client (its read loop will
// notice the closed connection and clean up via remove).
func (h *hub) broadcast(event string, data any) {
	msg := wsMessage{Event: event, Data: data}

	h.mu.Lock()
	conns := make([]*websocket.Conn, 0, len(h.conns))
	for c := range h.conns {
		conns = append(conns, c)
	}
	h.mu.Unlock()

	h.wmu.Lock()
	defer h.wmu.Unlock()
	for _, c := range conns {
		_ = c.WriteJSON(msg)
	}
}
