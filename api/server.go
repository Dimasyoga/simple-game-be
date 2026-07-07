// Package api implements the REST + WebSocket surface defined in
// contract.md. It is the only public boundary of this server; internals
// (rolls, DCs, beat-deck contents, resolution order) never cross it — this
// package only ever forwards to room.Room, never touches engine dice/DC
// logic directly.
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"simple-game-be/engine"
	"simple-game-be/narrator"
	"simple-game-be/room"
)

// Server holds every in-memory run. There is no persistence in v1
// (spec.md §10/§11).
type Server struct {
	mu     sync.Mutex
	runs   map[string]*managedRun
	nextID int

	Narrator narrator.Narrator
	// NewRNG is overridable so tests can get deterministic dice; defaults
	// to a time-seeded RNG.
	NewRNG func() engine.RNG

	WindowDuration     time.Duration
	LootWindowDuration time.Duration
}

func NewServer(n narrator.Narrator) *Server {
	return &Server{
		runs:               make(map[string]*managedRun),
		Narrator:           n,
		NewRNG:             func() engine.RNG { return engine.NewSeededRNG(time.Now().UnixNano()) },
		WindowDuration:     5 * time.Minute,
		LootWindowDuration: 30 * time.Second,
	}
}

// Handler returns the full contract.md HTTP surface as an http.Handler.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /runs", s.handleCreateRun)
	mux.HandleFunc("POST /runs/{runId}/join", s.handleJoin)
	mux.HandleFunc("GET /runs/{runId}/state", s.handleState)
	mux.HandleFunc("POST /runs/{runId}/clauses/{clauseIndex}/action", s.handleAction)
	mux.HandleFunc("POST /runs/{runId}/clauses/{clauseIndex}/pass", s.handlePass)
	mux.HandleFunc("POST /runs/{runId}/loot/{itemId}/claim", s.handleLootClaim)
	mux.HandleFunc("GET /runs/{runId}/ws", s.handleWS)
	return mux
}

type managedRun struct {
	mu       sync.Mutex
	id       string
	room     *room.Room
	hub      *hub
	catalog  engine.ItemCatalog
	rng      engine.RNG
	started  bool
	storyLog []StoryLogEntry
}

func (mr *managedRun) appendStoryLog(e StoryLogEntry) {
	mr.mu.Lock()
	defer mr.mu.Unlock()
	mr.storyLog = append(mr.storyLog, e)
}

func (mr *managedRun) storyLogSnapshot() []StoryLogEntry {
	mr.mu.Lock()
	defer mr.mu.Unlock()
	return append([]StoryLogEntry(nil), mr.storyLog...)
}

func (mr *managedRun) characterName(id string) string {
	for _, c := range mr.room.Snapshot().Characters {
		if c.ID == id {
			return c.Name
		}
	}
	return ""
}

// hooks wires room.Hooks to push contract.md's WS events over this run's hub.
func (mr *managedRun) hooks() *room.Hooks {
	return &room.Hooks{
		OnScenePresented: func(scenePlain string) {
			idx := mr.room.Snapshot().ClauseIndex
			mr.appendStoryLog(StoryLogEntry{ClauseIndex: idx, Kind: "scene", TextPlain: scenePlain})
			mr.hub.broadcast("clause_presented", ClausePresentedEvent{ClauseIndex: idx, ScenePlain: scenePlain})
		},
		OnWindowOpened: func(deadline time.Time) {
			mr.hub.broadcast("window_opened", WindowOpenedEvent{
				ClauseIndex: mr.room.Snapshot().ClauseIndex,
				Deadline:    formatDeadline(deadline),
			})
		},
		OnInputStatus: func(actingIDs []string, status map[string]engine.InputTerminal) {
			per := make([]InputStatusPer, 0, len(actingIDs))
			submitted := 0
			for _, id := range actingIDs {
				state := "PENDING"
				if st, ok := status[id]; ok {
					state = string(st)
					submitted++
				}
				per = append(per, InputStatusPer{CharacterID: id, State: state})
			}
			mr.hub.broadcast("input_status", InputStatusEvent{Submitted: submitted, Total: len(actingIDs), Per: per})
		},
		OnResolving: func() {
			mr.hub.broadcast("resolving", ResolvingEvent{ClauseIndex: mr.room.Snapshot().ClauseIndex})
		},
		OnLootWindow: func(items []engine.Item, deadline time.Time) {
			views := make([]ItemView, 0, len(items))
			for _, it := range items {
				views = append(views, ItemView{ItemID: it.ID, Name: it.Name})
			}
			mr.hub.broadcast("loot_window", LootWindowEvent{Items: views, Deadline: formatDeadline(deadline)})
		},
		OnLootResolved: func(claim engine.LootClaim) {
			mr.hub.broadcast("loot_resolved", LootResolvedEvent{ItemID: claim.ItemID, WinnerCharacterID: claim.ResolvedTo})
		},
	}
}

func (mr *managedRun) currentPhase() string {
	snap := mr.room.Snapshot()
	switch {
	case snap.Status == "ended":
		return "ENDED"
	case mr.room.Barrier() != nil:
		return "WINDOW"
	case mr.room.HasOpenLootWindow():
		return "LOOT"
	default:
		return "RESOLVING"
	}
}

func (mr *managedRun) playerView(selfID string) PlayerView {
	snap := mr.room.Snapshot()
	view := PlayerView{
		RunID:       mr.id,
		ClauseIndex: snap.ClauseIndex,
		Phase:       mr.currentPhase(),
		StoryLog:    mr.storyLogSnapshot(),
	}
	for _, c := range snap.Characters {
		if c.ID == selfID {
			view.Self = toCharacterSheet(c)
		} else {
			view.Party = append(view.Party, toCharacterSheetPublic(c))
		}
	}
	if b := mr.room.Barrier(); b != nil {
		d := formatDeadline(b.Deadline())
		view.Window = &WindowView{Deadline: d}
	}
	return view
}

func toCharacterSheet(c engine.Character) CharacterSheet {
	items := make([]CharacterSheetItem, 0, len(c.Inventory))
	for _, it := range c.Inventory {
		items = append(items, CharacterSheetItem{ItemID: it.ID, Name: it.Name, Type: it.Type})
	}
	return CharacterSheet{
		CharacterID: c.ID,
		Name:        c.Name,
		Class:       c.Class,
		Stats:       c.Stats,
		Personality: c.Personality,
		Status:      c.Status,
		Inventory:   items,
	}
}

func toCharacterSheetPublic(c engine.Character) CharacterSheetPublic {
	return CharacterSheetPublic{CharacterID: c.ID, Name: c.Name, Class: c.Class, Stats: c.Stats, Status: c.Status}
}

func formatDeadline(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}

func (s *Server) getRun(runID string) *managedRun {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.runs[runID]
}

// driveRun runs the story to completion, one clause at a time, pushing
// clause_narrated/state_updated/run_ended as each clause resolves. Every
// other event (clause_presented, window_opened, input_status, resolving,
// loot_window, loot_resolved) is pushed by the Hooks room.RunNextClause
// invokes mid-flight.
func (s *Server) driveRun(mr *managedRun) {
	ctx := context.Background()
	for {
		if mr.room.Snapshot().Status == "ended" {
			mr.hub.broadcast("run_ended", RunEndedEvent{Outcome: "completed", SummaryPlain: mr.room.Snapshot().ProseSummary})
			return
		}

		result, err := mr.room.RunNextClause(ctx, s.Narrator, mr.rng, mr.catalog, demoCheck, demoEffect, engine.LootByRoll)
		if err != nil {
			mr.hub.broadcast("error", ErrorEvent{Code: "clause_failed", Message: err.Error()})
			return
		}

		mr.appendStoryLog(StoryLogEntry{ClauseIndex: result.Clause.Index, Kind: "narration", TextPlain: result.NarrationPlain})
		mr.hub.broadcast("clause_narrated", ClauseNarratedEvent{
			ClauseIndex:    result.Clause.Index,
			NarrationPlain: result.NarrationPlain,
			StateSummary:   mr.room.Snapshot().ProseSummary,
		})
		mr.hub.broadcast("state_updated", StateUpdatedEvent{View: mr.playerView("")})
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (s *Server) handleCreateRun(w http.ResponseWriter, r *http.Request) {
	var req CreateRunRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	s.nextID++
	runID := fmt.Sprintf("run-%d", s.nextID)
	s.mu.Unlock()

	rng := s.NewRNG()
	run := engine.NewRun(runID, demoSkeleton(), nil, rng, demoBeatPool(), demoBeatOverrides())
	rm := room.NewRoom(run, room.RealClock(), s.WindowDuration, s.LootWindowDuration)

	mr := &managedRun{id: runID, room: rm, hub: newHub(), catalog: engine.NewItemCatalog(), rng: rng}
	rm.Hooks = mr.hooks()

	s.mu.Lock()
	s.runs[runID] = mr
	s.mu.Unlock()

	writeJSON(w, http.StatusOK, CreateRunResponse{RunID: runID, WSURL: "/runs/" + runID + "/ws"})
}

func (s *Server) handleJoin(w http.ResponseWriter, r *http.Request) {
	mr := s.getRun(r.PathValue("runId"))
	if mr == nil {
		http.Error(w, "run not found", http.StatusNotFound)
		return
	}

	var req JoinRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	s.nextID++
	characterID := fmt.Sprintf("char-%d", s.nextID)
	s.mu.Unlock()

	mr.room.AppendCharacter(engine.Character{
		ID:     characterID,
		Name:   req.Name,
		Class:  req.CharacterClass,
		Stats:  engine.Stats{Strength: 10, Dexterity: 10, Intelligence: 10, Charisma: 10, HP: 10, MaxHP: 10},
		Status: engine.CharacterStatus{Alive: true},
	})

	mr.mu.Lock()
	alreadyStarted := mr.started
	mr.started = true
	mr.mu.Unlock()
	if !alreadyStarted {
		go s.driveRun(mr)
	}

	writeJSON(w, http.StatusOK, JoinResponse{CharacterID: characterID})
}

func (s *Server) handleState(w http.ResponseWriter, r *http.Request) {
	mr := s.getRun(r.PathValue("runId"))
	if mr == nil {
		http.Error(w, "run not found", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, mr.playerView(r.URL.Query().Get("characterId")))
}

func (s *Server) handleAction(w http.ResponseWriter, r *http.Request) {
	mr := s.getRun(r.PathValue("runId"))
	if mr == nil {
		http.Error(w, "run not found", http.StatusNotFound)
		return
	}
	clauseIndex, err := strconv.Atoi(r.PathValue("clauseIndex"))
	if err != nil {
		http.Error(w, "invalid clauseIndex", http.StatusBadRequest)
		return
	}
	var req ActionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}

	if clauseIndex != mr.room.Snapshot().ClauseIndex {
		writeJSON(w, http.StatusConflict, ActionResponse{Accepted: false, Reason: "wrong phase"})
		return
	}
	if !mr.room.Submit(req.CharacterID, req.RawText) {
		writeJSON(w, http.StatusConflict, ActionResponse{Accepted: false, Reason: "window closed"})
		return
	}
	writeJSON(w, http.StatusOK, ActionResponse{Accepted: true, TerminalState: "SUBMITTED"})
}

func (s *Server) handlePass(w http.ResponseWriter, r *http.Request) {
	mr := s.getRun(r.PathValue("runId"))
	if mr == nil {
		http.Error(w, "run not found", http.StatusNotFound)
		return
	}
	clauseIndex, err := strconv.Atoi(r.PathValue("clauseIndex"))
	if err != nil {
		http.Error(w, "invalid clauseIndex", http.StatusBadRequest)
		return
	}
	var req PassRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}

	if clauseIndex != mr.room.Snapshot().ClauseIndex || !mr.room.Pass(req.CharacterID) {
		http.Error(w, "window closed", http.StatusConflict)
		return
	}
	writeJSON(w, http.StatusOK, PassResponse{TerminalState: "PASSED"})
}

func (s *Server) handleLootClaim(w http.ResponseWriter, r *http.Request) {
	mr := s.getRun(r.PathValue("runId"))
	if mr == nil {
		http.Error(w, "run not found", http.StatusNotFound)
		return
	}
	itemID := r.PathValue("itemId")
	var req LootClaimRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}

	if !mr.room.ClaimLoot(itemID, req.CharacterID) {
		writeJSON(w, http.StatusConflict, LootClaimResponse{Claimed: false, Reason: "already_resolved"})
		return
	}
	writeJSON(w, http.StatusOK, LootClaimResponse{Claimed: true})
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     func(*http.Request) bool { return true }, // v1: no cross-origin restriction
}

func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	mr := s.getRun(r.PathValue("runId"))
	if mr == nil {
		http.Error(w, "run not found", http.StatusNotFound)
		return
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	mr.hub.add(conn)
	characterID := r.URL.Query().Get("characterId")
	defer func() {
		mr.hub.remove(conn)
		_ = conn.Close()
	}()

	for {
		var msg discussionSendMessage
		if err := conn.ReadJSON(&msg); err != nil {
			return
		}
		if msg.Event != "discussion_send" {
			continue
		}
		// OOC table-talk: relayed to the room, never persisted to canonical
		// state or forwarded to the narrator (contract.md).
		mr.hub.broadcast("discussion_message", DiscussionMessageEvent{
			FromCharacterID: characterID,
			Name:            mr.characterName(characterID),
			Text:            msg.Text,
			At:              time.Now().UTC().Format(time.RFC3339),
		})
	}
}
