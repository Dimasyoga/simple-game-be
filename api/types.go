package api

import "simple-game-be/engine"

// Request/response payloads exactly as shaped in contract.md.

type CreateRunRequest struct {
	Mode       string `json:"mode"`
	GameplayID string `json:"gameplayId"`
}

type CreateRunResponse struct {
	RunID string `json:"runId"`
	WSURL string `json:"wsUrl"`
}

type JoinRequest struct {
	CharacterClass string `json:"characterClass"`
	Name           string `json:"name"`
}

type JoinResponse struct {
	CharacterID string `json:"characterId"`
	IsHost      bool   `json:"isHost"`
}

type StartRequest struct {
	CharacterID string `json:"characterId"`
}

type StartResponse struct {
	Started bool   `json:"started"`
	Reason  string `json:"reason,omitempty"`
}

type ActionRequest struct {
	CharacterID string `json:"characterId"`
	RawText     string `json:"rawText"`
}

type ActionResponse struct {
	Accepted      bool   `json:"accepted"`
	TerminalState string `json:"terminalState,omitempty"`
	Reason        string `json:"reason,omitempty"`
}

type PassRequest struct {
	CharacterID string `json:"characterId"`
}

type PassResponse struct {
	TerminalState string `json:"terminalState"`
}

type LootClaimRequest struct {
	CharacterID string `json:"characterId"`
}

type LootClaimResponse struct {
	Claimed bool   `json:"claimed"`
	Reason  string `json:"reason,omitempty"`
}

// PlayerView and its nested shapes, per contract.md's "Payloads" section.

type ItemView struct {
	ItemID string `json:"itemId"`
	Name   string `json:"name"`
}

type CharacterSheetItem struct {
	ItemID string `json:"itemId"`
	Name   string `json:"name"`
	Type   string `json:"type"`
}

type CharacterSheet struct {
	CharacterID string                 `json:"characterId"`
	Name        string                 `json:"name"`
	Class       string                 `json:"class"`
	Stats       engine.Stats           `json:"stats"`
	Personality engine.Personality     `json:"personality"`
	Status      engine.CharacterStatus `json:"status"`
	Inventory   []CharacterSheetItem   `json:"inventory"`
}

type CharacterSheetPublic struct {
	CharacterID string                 `json:"characterId"`
	Name        string                 `json:"name"`
	Class       string                 `json:"class"`
	Stats       engine.Stats           `json:"stats"`
	Status      engine.CharacterStatus `json:"status"`
}

type StoryLogEntry struct {
	ChapterIndex int    `json:"chapterIndex"`
	ClauseOrder  int    `json:"clauseOrder"`
	Kind         string `json:"kind"` // "scene" | "narration"
	TextPlain    string `json:"textPlain"`
}

type WindowView struct {
	Deadline string `json:"deadline"`
}

type PlayerView struct {
	RunID        string                 `json:"runId"`
	ChapterIndex int                    `json:"chapterIndex"`
	ClauseOrder  int                    `json:"clauseOrder"`
	Phase        string                 `json:"phase"`
	Self         CharacterSheet         `json:"self"`
	Party        []CharacterSheetPublic `json:"party"`
	StoryLog     []StoryLogEntry        `json:"storyLog"`
	Window       *WindowView            `json:"window,omitempty"`
	DroppedItems []ItemView             `json:"droppedItems,omitempty"`
}

// WebSocket event envelope and payloads, per contract.md's "WebSocket
// events" section. The envelope itself ({event, data}) isn't literally
// specified by contract.md beyond the event names/fields, so this is the
// one wire-format decision this package makes.

type wsMessage struct {
	Event string `json:"event"`
	Data  any    `json:"data"`
}

// discussionSendMessage is the only Client -> Server WS message contract.md
// defines: OOC table-talk, relayed to the room but never persisted or sent
// to the narrator.
type discussionSendMessage struct {
	Event string `json:"event"`
	Text  string `json:"text"`
}

type ChapterStartedEvent struct {
	ChapterIndex int    `json:"chapterIndex"`
	Title        string `json:"title"`
}

type ClausePresentedEvent struct {
	ChapterIndex  int    `json:"chapterIndex"`
	ClauseOrder   int    `json:"clauseOrder"`
	ScenePlain    string `json:"scenePlain"`
	RequiresInput bool   `json:"requiresInput"`
}

type WindowOpenedEvent struct {
	ChapterIndex int    `json:"chapterIndex"`
	ClauseOrder  int    `json:"clauseOrder"`
	Deadline     string `json:"deadline"`
}

type InputStatusPer struct {
	CharacterID string `json:"characterId"`
	State       string `json:"state"`
}

type InputStatusEvent struct {
	Submitted int              `json:"submitted"`
	Total     int              `json:"total"`
	Per       []InputStatusPer `json:"per"`
}

type ResolvingEvent struct {
	ChapterIndex int `json:"chapterIndex"`
	ClauseOrder  int `json:"clauseOrder"`
}

type ClauseNarratedEvent struct {
	ChapterIndex   int    `json:"chapterIndex"`
	ClauseOrder    int    `json:"clauseOrder"`
	NarrationPlain string `json:"narrationPlain"`
	StateSummary   string `json:"stateSummary"`
}

type StateUpdatedEvent struct {
	View PlayerView `json:"view"`
}

type LootWindowEvent struct {
	Items    []ItemView `json:"items"`
	Deadline string     `json:"deadline"`
}

type LootResolvedEvent struct {
	ItemID            string `json:"itemId"`
	WinnerCharacterID string `json:"winnerCharacterId"`
}

type DiscussionMessageEvent struct {
	FromCharacterID string `json:"fromCharacterId"`
	Name            string `json:"name"`
	Text            string `json:"text"`
	At              string `json:"at"`
}

type RunEndedEvent struct {
	Outcome      string `json:"outcome"`
	SummaryPlain string `json:"summaryPlain"`
}

type ErrorEvent struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
