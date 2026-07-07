package narrator

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"
)

// Local is a Narrator backed by any server exposing the OpenAI-compatible
// chat completions API (POST {BaseURL}/v1/chat/completions) — the common
// surface across Ollama, llama.cpp's llama-server, vLLM, LM Studio, and
// text-generation-webui. Which local model actually answers is a BaseURL +
// Model config change, not a code change, as long as the server speaks this
// shape (tasks.md Phase 5).
type Local struct {
	BaseURL      string // e.g. "http://localhost:11434" or "http://localhost:8080"
	Model        string // model name exactly as the server expects it
	SystemPrompt string // optional; sent as the system message on every call
	HTTPClient   *http.Client
}

// NewLocal returns a Local narrator with a sane request timeout. Local
// inference is slower than a hosted API, so the default is generous.
func NewLocal(baseURL, model string) *Local {
	return &Local{
		BaseURL:    strings.TrimRight(baseURL, "/"),
		Model:      model,
		HTTPClient: &http.Client{Timeout: 120 * time.Second},
	}
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatCompletionsRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
	Stream   bool          `json:"stream"`
}

type chatCompletionsResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
}

func (l *Local) Present(ctx context.Context, pc PresentContext) (string, error) {
	return l.complete(ctx, buildPresentPrompt(pc))
}

func (l *Local) Narrate(ctx context.Context, nc NarrateContext) (string, error) {
	return l.complete(ctx, buildNarratePrompt(nc))
}

func (l *Local) complete(ctx context.Context, userPrompt string) (string, error) {
	messages := make([]chatMessage, 0, 2)
	if l.SystemPrompt != "" {
		messages = append(messages, chatMessage{Role: "system", Content: l.SystemPrompt})
	}
	messages = append(messages, chatMessage{Role: "user", Content: userPrompt})

	body, err := json.Marshal(chatCompletionsRequest{Model: l.Model, Messages: messages})
	if err != nil {
		return "", fmt.Errorf("narrator: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, l.BaseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("narrator: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	client := l.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 120 * time.Second}
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("narrator: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("narrator: unexpected status %d from %s", resp.StatusCode, l.BaseURL)
	}

	var parsed chatCompletionsResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return "", fmt.Errorf("narrator: decode response: %w", err)
	}
	if len(parsed.Choices) == 0 {
		return "", fmt.Errorf("narrator: response had no choices")
	}
	return strings.TrimSpace(parsed.Choices[0].Message.Content), nil
}

func buildPresentPrompt(pc PresentContext) string {
	var b strings.Builder
	b.WriteString("Narrate the upcoming scene in plain prose. Do not decide outcomes; only set the scene.\n\n")
	if pc.ProseSummary != "" {
		b.WriteString("Story so far:\n" + pc.ProseSummary + "\n\n")
	}
	if len(pc.RelevantState) > 0 {
		b.WriteString("Relevant known facts:\n" + formatState(pc.RelevantState) + "\n")
	}
	b.WriteString("Beat premise: " + pc.BeatPremise + "\n")
	if pc.BeatType != "" {
		b.WriteString("Beat type: " + pc.BeatType + "\n")
	}
	b.WriteString("\nWrite the scene now.")
	return b.String()
}

func buildNarratePrompt(nc NarrateContext) string {
	var b strings.Builder
	b.WriteString("Narrate what happened this turn, in the exact order given. Do not change or contradict any outcome.\n\n")
	if nc.ProseSummary != "" {
		b.WriteString("Story so far:\n" + nc.ProseSummary + "\n\n")
	}
	if len(nc.RelevantState) > 0 {
		b.WriteString("Relevant known facts:\n" + formatState(nc.RelevantState) + "\n")
	}
	b.WriteString("Resolved actions (already decided; narrate faithfully):\n")
	for _, a := range nc.ResolvedActions {
		b.WriteString(fmt.Sprintf("- %s attempted %q -> %s", a.CharacterID, a.Intent, a.Outcome))
		if a.Summary != "" {
			b.WriteString(" (" + a.Summary + ")")
		}
		b.WriteString("\n")
	}
	b.WriteString("\nWrite the narration now.")
	return b.String()
}

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
