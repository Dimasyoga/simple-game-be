package narrator

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const maxTurns = 10

// Local is a Narrator backed by any server exposing the OpenAI-compatible
// chat completions API (POST {BaseURL}/v1/chat/completions) with Bearer auth —
// the common surface across local runtimes (Ollama, llama.cpp's llama-server,
// vLLM, LM Studio, text-generation-webui) AND hosted providers that speak the
// same shape, namely OpenAI (ChatGPT API) and OpenRouter. Which model answers
// is a BaseURL + Model + APIKey config change, not a code change (tasks.md
// Phase 5). Example targets:
//
//	local llama: BaseURL "http://localhost:8080",     Model "local"
//	OpenAI:      BaseURL "https://api.openai.com",    Model "gpt-4o-mini"
//	OpenRouter:  BaseURL "https://openrouter.ai/api", Model "openai/gpt-4o"
//
// BaseURL must be the host WITHOUT the "/v1/chat/completions" suffix — that
// path is appended for you.
type Local struct {
	BaseURL      string // e.g. "http://localhost:11434" or "http://localhost:8080"
	Model        string // model name exactly as the server expects it
	SystemPrompt string // optional; sent as the system message on every call
	APIKey       string // optional; sent as Bearer token in Authorization header
	HTTPClient   *http.Client
	LogFile      string // optional; if set, logs request/response JSON payloads to this file
	MaxTokens    int    // optional; caps completion length when > 0 (omitted otherwise)
	// ExtraHeaders are added to every request, e.g. OpenRouter's optional
	// "HTTP-Referer" / "X-Title" attribution headers. Nil is fine.
	ExtraHeaders map[string]string

	messages []chatMessage
}

// NewLocal returns a Local narrator with a sane request timeout. Local
// inference is slower than a hosted API, so the default is generous.
func NewLocal(baseURL, model, systemPrompt, apiKey string) *Local {
	return &Local{
		BaseURL:      strings.TrimRight(baseURL, "/"),
		Model:        model,
		SystemPrompt: systemPrompt,
		APIKey:       apiKey,
		HTTPClient:   &http.Client{Timeout: 120 * time.Second},
	}
}

// Reset clears the conversation history. Call once at the start of a new run.
func (l *Local) Reset() {
	l.messages = nil
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatCompletionsRequest struct {
	Model     string        `json:"model"`
	Messages  []chatMessage `json:"messages"`
	Stream    bool          `json:"stream"`
	MaxTokens int           `json:"max_tokens,omitempty"`
}

type chatCompletionsResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
}

func (l *Local) Present(ctx context.Context, pc PresentContext) (string, error) {
	fullPrompt, historyData := buildPresentPrompt(pc)
	return l.complete(ctx, fullPrompt, historyData)
}

func (l *Local) Narrate(ctx context.Context, nc NarrateContext) (string, error) {
	fullPrompt, historyData := buildNarratePrompt(nc)
	return l.complete(ctx, fullPrompt, historyData)
}

func (l *Local) complete(ctx context.Context, fullPrompt string, historyData string) (string, error) {
	messages := make([]chatMessage, 0, 1+len(l.messages)+1)
	if l.SystemPrompt != "" {
		messages = append(messages, chatMessage{Role: "system", Content: l.SystemPrompt})
	}
	messages = append(messages, l.messages...)
	messages = append(messages, chatMessage{Role: "user", Content: fullPrompt})

	body, err := json.Marshal(chatCompletionsRequest{Model: l.Model, Messages: messages, MaxTokens: l.MaxTokens})
	if err != nil {
		return "", fmt.Errorf("narrator: marshal request: %w", err)
	}
	if l.LogFile != "" {
		logPayload(l.LogFile, "REQUEST", string(body))
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, l.BaseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("narrator: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if l.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+l.APIKey)
	}
	for k, v := range l.ExtraHeaders {
		httpReq.Header.Set(k, v)
	}

	client := l.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 120 * time.Second}
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("narrator: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("narrator: read response: %w", err)
	}
	if l.LogFile != "" {
		logPayload(l.LogFile, "RESPONSE", string(respBody))
	}

	// Surface the provider's error body (invalid model / auth / rate limit),
	// not just the bare status code — hosted APIs return actionable JSON here.
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("narrator: status %d from %s: %s", resp.StatusCode, l.BaseURL, string(respBody))
	}

	var parsed chatCompletionsResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return "", fmt.Errorf("narrator: decode response: %w", err)
	}
	if len(parsed.Choices) == 0 {
		return "", fmt.Errorf("narrator: response had no choices")
	}
	assistantContent := strings.TrimSpace(parsed.Choices[0].Message.Content)

	l.messages = append(l.messages,
		chatMessage{Role: "user", Content: historyData},
		chatMessage{Role: "assistant", Content: assistantContent},
	)
	l.evict()

	return assistantContent, nil
}

func (l *Local) evict() {
	if len(l.messages) <= 2*maxTurns {
		return
	}
	keep := 2 * maxTurns
	l.messages = l.messages[len(l.messages)-keep:]
}

func logPayload(path, label, payload string) {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "[%s] %s\n%s\n\n", time.Now().Format(time.RFC3339), label, payload)
}


