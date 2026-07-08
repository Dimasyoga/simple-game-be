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
		local := narrator.NewLocal(baseURL, model)
		local.SystemPrompt = os.Getenv("LOCAL_NARRATOR_SYSTEM_PROMPT")
		local.APIKey = os.Getenv("LOCAL_NARRATOR_API_KEY")
		n = local
		narratorKind = "local (" + baseURL + ", model=" + model + ")"
	}

	server := api.NewServer(n)
	log.Printf("simple-game-be listening on %s (narrator: %s)", addr, narratorKind)
	log.Fatal(http.ListenAndServe(addr, server.Handler()))
}
