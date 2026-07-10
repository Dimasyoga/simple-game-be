package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"simple-game-be/narrator"
)

// TestFullStoryCompletesOverRealContractDeadlockFree is the Phase 3 gate
// from tasks.md: a full story runs to completion over the real REST+WS
// contract with the narrator stubbed, deadlock-free. It exercises join,
// action, pass, the WS event stream, and a real loot claim end-to-end.
func TestFullStoryCompletesOverRealContractDeadlockFree(t *testing.T) {
	s := NewServer(narrator.NewStub())
	s.WindowDuration = 300 * time.Millisecond
	s.LootWindowDuration = 300 * time.Millisecond

	httpServer := httptest.NewServer(s.Handler())
	defer httpServer.Close()

	var created CreateRunResponse
	decodeJSON(t, postJSON(t, httpServer.URL+"/runs", CreateRunRequest{Mode: "multi", GameplayID: "demo"}), &created)

	var char1, char2 JoinResponse
	decodeJSON(t, postJSON(t, httpServer.URL+"/runs/"+created.RunID+"/join", JoinRequest{CharacterClass: "warrior", Name: "Aria"}), &char1)
	decodeJSON(t, postJSON(t, httpServer.URL+"/runs/"+created.RunID+"/join", JoinRequest{CharacterClass: "archer", Name: "Bram"}), &char2)

	if !char1.IsHost {
		t.Fatal("expected the first character to join to be host")
	}
	if char2.IsHost {
		t.Fatal("expected the second character to join not to be host")
	}

	var started StartResponse
	decodeJSON(t, postJSON(t, httpServer.URL+"/runs/"+created.RunID+"/start", StartRequest{CharacterID: char1.CharacterID}), &started)
	if !started.Started {
		t.Fatalf("expected the host's start to succeed, got %+v", started)
	}

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http") + "/runs/" + created.RunID + "/ws?characterId=" + char1.CharacterID
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("failed to dial WS: %v", err)
	}
	defer conn.Close()

	events := make(chan wsMessage, 64)
	go func() {
		for {
			var msg wsMessage
			if err := conn.ReadJSON(&msg); err != nil {
				close(events)
				return
			}
			events <- msg
		}
	}()

	lootItemID := ""
	actedOnFirstWindow := false
	runEnded := false
	deadline := time.After(5 * time.Second)

drain:
	for {
		select {
		case msg, ok := <-events:
			if !ok {
				break drain
			}
			switch msg.Event {
			case "window_opened":
				if actedOnFirstWindow {
					continue
				}
				actedOnFirstWindow = true
				var data WindowOpenedEvent
				remarshal(t, msg.Data, &data)

				var ar ActionResponse
				decodeJSON(t, postJSON(t, fmt.Sprintf("%s/runs/%s/chapters/%d/clauses/%d/action", httpServer.URL, created.RunID, data.ChapterIndex, data.ClauseOrder),
					ActionRequest{CharacterID: char1.CharacterID, RawText: "pick the lock"}), &ar)
				if !ar.Accepted {
					t.Fatalf("expected action to be accepted, got %+v", ar)
				}

				passResp := postJSON(t, fmt.Sprintf("%s/runs/%s/chapters/%d/clauses/%d/pass", httpServer.URL, created.RunID, data.ChapterIndex, data.ClauseOrder),
					PassRequest{CharacterID: char2.CharacterID})
				if passResp.StatusCode != http.StatusOK {
					t.Fatalf("expected pass to be accepted, got %d", passResp.StatusCode)
				}

			case "loot_window":
				var data LootWindowEvent
				remarshal(t, msg.Data, &data)
				if len(data.Items) == 0 {
					continue
				}
				lootItemID = data.Items[0].ItemID
				var cr LootClaimResponse
				decodeJSON(t, postJSON(t, httpServer.URL+"/runs/"+created.RunID+"/loot/"+lootItemID+"/claim",
					LootClaimRequest{CharacterID: char1.CharacterID}), &cr)
				if !cr.Claimed {
					t.Fatalf("expected loot claim to be accepted, got %+v", cr)
				}

			case "loot_resolved":
				var data LootResolvedEvent
				remarshal(t, msg.Data, &data)
				if data.ItemID == lootItemID && data.WinnerCharacterID != char1.CharacterID {
					t.Fatalf("expected char1 to win the uncontested loot claim, got %s", data.WinnerCharacterID)
				}

			case "error":
				var data ErrorEvent
				remarshal(t, msg.Data, &data)
				t.Fatalf("server reported an error: %+v", data)

			case "run_ended":
				runEnded = true
				break drain
			}
		case <-deadline:
			t.Fatal("story did not complete within timeout: not deadlock-free over the real contract")
		}
	}

	if !runEnded {
		t.Fatal("expected a run_ended event before the WS stream closed")
	}
	if lootItemID == "" {
		t.Fatal("expected at least one item to drop and be claimed during the run")
	}

	stateResp, err := http.Get(httpServer.URL + "/runs/" + created.RunID + "/state?characterId=" + char1.CharacterID)
	if err != nil {
		t.Fatalf("GET state failed: %v", err)
	}
	var view PlayerView
	decodeJSON(t, stateResp, &view)
	if view.Phase != "ENDED" {
		t.Fatalf("expected final phase ENDED, got %s", view.Phase)
	}
	foundItem := false
	for _, it := range view.Self.Inventory {
		if it.ItemID == lootItemID {
			foundItem = true
		}
	}
	if !foundItem {
		t.Fatal("expected the claimed item to persist in char1's inventory in the final state")
	}
}

func postJSON(t *testing.T, url string, body any) *http.Response {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	resp, err := http.Post(url, "application/json", bytes.NewReader(b))
	if err != nil {
		t.Fatalf("POST %s failed: %v", url, err)
	}
	return resp
}

func decodeJSON(t *testing.T, resp *http.Response, v any) {
	t.Helper()
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
}

func remarshal(t *testing.T, from any, to any) {
	t.Helper()
	b, err := json.Marshal(from)
	if err != nil {
		t.Fatalf("remarshal failed: %v", err)
	}
	if err := json.Unmarshal(b, to); err != nil {
		t.Fatalf("remarshal unmarshal failed: %v", err)
	}
}
