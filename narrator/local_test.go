package narrator

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLocalPresentSendsPromptAndParsesResponse(t *testing.T) {
	var gotReq chatCompletionsRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("expected /v1/chat/completions, got %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&gotReq); err != nil {
			t.Fatalf("failed to decode request body: %v", err)
		}
		_ = json.NewEncoder(w).Encode(chatCompletionsResponse{
			Choices: []struct {
				Message chatMessage `json:"message"`
			}{{Message: chatMessage{Role: "assistant", Content: "  a goblin blocks the path  "}}},
		})
	}))
	defer server.Close()

	l := NewLocal(server.URL, "mistral-7b", "", "")
	scene, err := l.Present(context.Background(), PresentContext{
		BeatPremise: "a goblin blocks the path",
		BeatType:    "combat",
	})
	if err != nil {
		t.Fatalf("Present failed: %v", err)
	}
	if scene != "a goblin blocks the path" {
		t.Fatalf("expected trimmed response content, got %q", scene)
	}
	if gotReq.Model != "mistral-7b" {
		t.Fatalf("expected model %q sent to server, got %q", "mistral-7b", gotReq.Model)
	}
	if len(gotReq.Messages) == 0 || !strings.Contains(gotReq.Messages[len(gotReq.Messages)-1].Content, "a goblin blocks the path") {
		t.Fatalf("expected beat premise in prompt, got %+v", gotReq.Messages)
	}
}

func TestLocalNarrateIncludesResolvedActionsInPrompt(t *testing.T) {
	var gotReq chatCompletionsRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotReq)
		_ = json.NewEncoder(w).Encode(chatCompletionsResponse{
			Choices: []struct {
				Message chatMessage `json:"message"`
			}{{Message: chatMessage{Content: "the hero strikes true"}}},
		})
	}))
	defer server.Close()

	l := NewLocal(server.URL, "mistral-7b", "", "")
	narration, err := l.Narrate(context.Background(), NarrateContext{
		ResolvedActions: []ResolvedActionSummary{
			{CharacterID: "hero", CharacterName: "Aria", Intent: "attack goblin", Outcome: "SUCCESS", Summary: "kill:goblin1=<nil>"},
		},
	})
	if err != nil {
		t.Fatalf("Narrate failed: %v", err)
	}
	if narration != "the hero strikes true" {
		t.Fatalf("unexpected narration: %q", narration)
	}
	prompt := gotReq.Messages[len(gotReq.Messages)-1].Content
	if !strings.Contains(prompt, "Aria attempted \"attack goblin\" -> SUCCESS") {
		t.Fatalf("expected resolved action in prompt, got: %s", prompt)
	}
}

func TestLocalSystemPromptIsSentWhenSet(t *testing.T) {
	var gotReq chatCompletionsRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotReq)
		_ = json.NewEncoder(w).Encode(chatCompletionsResponse{
			Choices: []struct {
				Message chatMessage `json:"message"`
			}{{Message: chatMessage{Content: "ok"}}},
		})
	}))
	defer server.Close()

	l := NewLocal(server.URL, "mistral-7b", "You are a fantasy narrator.", "")
	if _, err := l.Present(context.Background(), PresentContext{BeatPremise: "test"}); err != nil {
		t.Fatalf("Present failed: %v", err)
	}
	if len(gotReq.Messages) != 2 || gotReq.Messages[0].Role != "system" {
		t.Fatalf("expected a leading system message, got %+v", gotReq.Messages)
	}
}

func TestLocalReturnsErrorOnNonOKStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	l := NewLocal(server.URL, "mistral-7b", "", "")
	if _, err := l.Present(context.Background(), PresentContext{}); err == nil {
		t.Fatal("expected an error on non-200 status")
	}
}

func TestLocalReturnsErrorOnEmptyChoices(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(chatCompletionsResponse{})
	}))
	defer server.Close()

	l := NewLocal(server.URL, "mistral-7b", "", "")
	if _, err := l.Narrate(context.Background(), NarrateContext{}); err == nil {
		t.Fatal("expected an error when the response has no choices")
	}
}

func TestLocalSendsAuthorizationHeaderWhenAPIKeySet(t *testing.T) {
	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_ = json.NewEncoder(w).Encode(chatCompletionsResponse{
			Choices: []struct {
				Message chatMessage `json:"message"`
			}{{Message: chatMessage{Content: "ok"}}},
		})
	}))
	defer server.Close()

	l := NewLocal(server.URL, "mistral-7b", "", "test-secret-key")
	if _, err := l.Present(context.Background(), PresentContext{BeatPremise: "test"}); err != nil {
		t.Fatalf("Present failed: %v", err)
	}
	if gotAuth != "Bearer test-secret-key" {
		t.Fatalf("expected Authorization header %q, got %q", "Bearer test-secret-key", gotAuth)
	}
}

func TestLocalMultiTurnHistory(t *testing.T) {
	var callCount int
	var requests []chatCompletionsRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		var req chatCompletionsRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		requests = append(requests, req)
		_ = json.NewEncoder(w).Encode(chatCompletionsResponse{
			Choices: []struct {
				Message chatMessage `json:"message"`
			}{{Message: chatMessage{Content: "scene one"}}},
		})
	}))
	defer server.Close()

	l := NewLocal(server.URL, "test", "test system", "")
	_, _ = l.Present(context.Background(), PresentContext{
		BeatPremise: "a goblin blocks the path",
		BeatType:    "combat",
	})
	_, _ = l.Narrate(context.Background(), NarrateContext{
		ResolvedActions: []ResolvedActionSummary{
			{CharacterID: "hero", CharacterName: "Aria", Intent: "attack", Outcome: "SUCCESS"},
		},
	})

	if callCount != 2 {
		t.Fatalf("expected 2 calls, got %d", callCount)
	}

	secondReq := requests[1]
	userMessages := 0
	for _, m := range secondReq.Messages {
		if m.Role == "user" {
			userMessages++
		}
	}
	if userMessages != 2 {
		t.Fatalf("expected 2 user messages in second call (history + current), got %d", userMessages)
	}

	historyUserContent := secondReq.Messages[1].Content
	if !strings.Contains(historyUserContent, "a goblin blocks the path") {
		t.Fatalf("expected first turn data in history, got: %s", historyUserContent)
	}
}

func TestLocalHistoryDistinguishesModes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(chatCompletionsResponse{
			Choices: []struct {
				Message chatMessage `json:"message"`
			}{{Message: chatMessage{Content: "ok"}}},
		})
	}))
	defer server.Close()

	l := NewLocal(server.URL, "test", "", "")
	_, _ = l.Present(context.Background(), PresentContext{BeatPremise: "a goblin blocks the path"})
	_, _ = l.Narrate(context.Background(), NarrateContext{
		ResolvedActions: []ResolvedActionSummary{
			{CharacterID: "hero", CharacterName: "Aria", Intent: "attack", Outcome: "SUCCESS"},
		},
	})

	if len(l.messages) != 4 {
		t.Fatalf("expected 4 stored messages (2 turns x user+assistant), got %d", len(l.messages))
	}
	if !strings.Contains(l.messages[0].Content, "Mode: describe") {
		t.Fatalf("expected first turn history to be tagged Mode: describe, got: %s", l.messages[0].Content)
	}
	if !strings.Contains(l.messages[2].Content, "Mode: narrate") {
		t.Fatalf("expected second turn history to be tagged Mode: narrate, got: %s", l.messages[2].Content)
	}
}

func TestLocalEvictsOldMessages(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(chatCompletionsResponse{
			Choices: []struct {
				Message chatMessage `json:"message"`
			}{{Message: chatMessage{Content: "ok"}}},
		})
	}))
	defer server.Close()

	l := NewLocal(server.URL, "test", "sys", "")
	for i := 0; i < 12; i++ {
		_, _ = l.Present(context.Background(), PresentContext{
			BeatPremise: fmt.Sprintf("beat-%d", i),
		})
	}

	if len(l.messages) > 2*maxTurns {
		t.Fatalf("expected at most %d history messages, got %d", 2*maxTurns, len(l.messages))
	}

	firstUser := l.messages[0]
	if strings.Contains(firstUser.Content, "beat-0") {
		t.Fatal("expected beat-0 to be evicted")
	}
	if !strings.Contains(firstUser.Content, "beat-2") {
		t.Fatalf("expected beat-2 to be first retained, got: %s", firstUser.Content)
	}
}

func TestLocalResetClearsHistory(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(chatCompletionsResponse{
			Choices: []struct {
				Message chatMessage `json:"message"`
			}{{Message: chatMessage{Content: "ok"}}},
		})
	}))
	defer server.Close()

	l := NewLocal(server.URL, "test", "", "")
	_, _ = l.Present(context.Background(), PresentContext{BeatPremise: "test"})
	if len(l.messages) == 0 {
		t.Fatal("expected messages after a call")
	}

	l.Reset()
	if len(l.messages) != 0 {
		t.Fatalf("expected empty history after Reset, got %d messages", len(l.messages))
	}
}

func TestLocalNoAuthorizationHeaderWhenAPIKeyEmpty(t *testing.T) {
	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_ = json.NewEncoder(w).Encode(chatCompletionsResponse{
			Choices: []struct {
				Message chatMessage `json:"message"`
			}{{Message: chatMessage{Content: "ok"}}},
		})
	}))
	defer server.Close()

	l := NewLocal(server.URL, "mistral-7b", "", "")
	if _, err := l.Present(context.Background(), PresentContext{BeatPremise: "test"}); err != nil {
		t.Fatalf("expected no Authorization header, got %q", gotAuth)
	}
}
