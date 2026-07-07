package narrator

import (
	"context"
	"encoding/json"
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

	l := NewLocal(server.URL, "mistral-7b")
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

	l := NewLocal(server.URL, "mistral-7b")
	narration, err := l.Narrate(context.Background(), NarrateContext{
		ResolvedActions: []ResolvedActionSummary{
			{CharacterID: "hero", Intent: "attack goblin", Outcome: "SUCCESS", Summary: "kill:goblin1=<nil>"},
		},
	})
	if err != nil {
		t.Fatalf("Narrate failed: %v", err)
	}
	if narration != "the hero strikes true" {
		t.Fatalf("unexpected narration: %q", narration)
	}
	prompt := gotReq.Messages[len(gotReq.Messages)-1].Content
	if !strings.Contains(prompt, "hero attempted \"attack goblin\" -> SUCCESS") {
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

	l := NewLocal(server.URL, "mistral-7b")
	l.SystemPrompt = "You are a fantasy narrator."
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

	l := NewLocal(server.URL, "mistral-7b")
	if _, err := l.Present(context.Background(), PresentContext{}); err == nil {
		t.Fatal("expected an error on non-200 status")
	}
}

func TestLocalReturnsErrorOnEmptyChoices(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(chatCompletionsResponse{})
	}))
	defer server.Close()

	l := NewLocal(server.URL, "mistral-7b")
	if _, err := l.Narrate(context.Background(), NarrateContext{}); err == nil {
		t.Fatal("expected an error when the response has no choices")
	}
}
