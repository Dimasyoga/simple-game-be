package main

import (
	"log"
	"net/http"
	"os"
	"strconv"

	"simple-game-be/api"
	"simple-game-be/narrator"
	"simple-game-be/scenario"
)

func main() {
	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = ":8080"
	}

	var n narrator.Narrator = narrator.NewStub()
	narratorKind := "stubbed"
	if baseURL := os.Getenv("LOCAL_NARRATOR_URL"); baseURL != "" {
		model := os.Getenv("LOCAL_NARRATOR_MODEL")
		systemPrompt := os.Getenv("LOCAL_NARRATOR_SYSTEM_PROMPT")
		if systemPrompt == "" {
			systemPrompt = "You are a vivid fantasy narrator. Each message starts with a \"Mode:\" line telling you what to do this turn: \"Mode: describe\" means describe the upcoming scene based on the beat premise and known facts; \"Mode: narrate\" means narrate the resolved actions faithfully in the exact order given. Write in plain prose; never decide outcomes or contradict provided data."
		}
		apiKey := os.Getenv("LOCAL_NARRATOR_API_KEY")
		l := narrator.NewLocal(baseURL, model, systemPrompt, apiKey)
		if v := os.Getenv("LOCAL_NARRATOR_MAX_TOKENS"); v != "" {
			if maxTokens, err := strconv.Atoi(v); err == nil {
				l.MaxTokens = maxTokens
			}
		}
		// Optional OpenRouter attribution headers (harmless for OpenAI/local).
		if referer := os.Getenv("LOCAL_NARRATOR_HTTP_REFERER"); referer != "" {
			l.ExtraHeaders = map[string]string{"HTTP-Referer": referer}
		}
		if title := os.Getenv("LOCAL_NARRATOR_TITLE"); title != "" {
			if l.ExtraHeaders == nil {
				l.ExtraHeaders = map[string]string{}
			}
			l.ExtraHeaders["X-Title"] = title
		}
		n = l
		narratorKind = "local (" + baseURL + ", model=" + model + ")"
	}

	server := api.NewServer(n)
	server.RegisterScenario("demo15", &api.ScenarioConfig{
		Gameplay: scenario.Demo15Gameplay(),
		CheckFn:  scenario.Demo15Check,
		EffectFn: scenario.Demo15Effect,
		Catalog:  scenario.Demo15Catalog(),
	})
	log.Printf("simple-game-be listening on %s (narrator: %s)", addr, narratorKind)
	log.Fatal(http.ListenAndServe(addr, server.Handler()))
}
