package main

import (
	"log"
	"net/http"
	"os"

	"simple-game-be/api"
	"simple-game-be/narrator"
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
		n = narrator.NewLocal(baseURL, model, systemPrompt, apiKey)
		narratorKind = "local (" + baseURL + ", model=" + model + ")"
	}

	server := api.NewServer(n)
	log.Printf("simple-game-be listening on %s (narrator: %s)", addr, narratorKind)
	log.Fatal(http.ListenAndServe(addr, server.Handler()))
}
