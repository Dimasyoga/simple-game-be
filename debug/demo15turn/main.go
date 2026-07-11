package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"simple-game-be/api"
	"simple-game-be/narrator"

	"github.com/gorilla/websocket"
)

var demoActionTexts = map[string]string{
	"0:0": "I step into the forest, eyes scanning the tree line",
	"0:1": "I follow the narrow path through the undergrowth",
	"0:2": "I investigate the strange tracks on the ground",
	"0:3": "I examine the ruins of the old shrine",
	"0:4": "I draw my sword and attack the shadow wolves",
	"1:0": "I search the archway for a hidden lever",
	"1:1": "I examine the locked gate for a keyhole",
	"1:2": "I ask the hermit about the forest's curse",
	"1:3": "I climb out of the sinkhole",
	"1:4": "I drink from the hidden spring",
	"2:0": "I cross the bridge of roots",
	"2:1": "I break through the witch's illusions",
	"2:2": "I approach the heart of the forest",
	"2:3": "I speak the words to lift the curse",
	"2:4": "I watch the dawn break over the clearing",
}

func shouldClaimLoot(chapterIdx, clauseOrder int) bool {
	return !(chapterIdx == 2 && clauseOrder == 1)
}

// envOr returns the value of environment variable key, or def when unset/empty.
func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

type turnResult struct {
	ChapterIndex int    `json:"chapterIndex"`
	ClauseOrder  int    `json:"clauseOrder"`
	Scene        string `json:"scene"`
	Narration    string `json:"narration"`
	Action       string `json:"action"`
}

type wsMessage struct {
	Event string          `json:"event"`
	Data  json.RawMessage `json:"data"`
}

type windowOpenedData struct {
	ChapterIndex int `json:"chapterIndex"`
	ClauseOrder  int `json:"clauseOrder"`
}

type lootWindowData struct {
	Items []struct {
		ItemID string `json:"itemId"`
		Name   string `json:"name"`
	} `json:"items"`
}

type clauseNarratedData struct {
	ChapterIndex   int    `json:"chapterIndex"`
	ClauseOrder    int    `json:"clauseOrder"`
	NarrationPlain string `json:"narrationPlain"`
}

type clausePresentedData struct {
	ChapterIndex  int  `json:"chapterIndex"`
	ClauseOrder   int  `json:"clauseOrder"`
	RequiresInput bool `json:"requiresInput"`
}

type createRunResponse struct {
	RunID string `json:"runId"`
	WSURL string `json:"wsUrl"`
}

type joinResponse struct {
	CharacterID string `json:"characterId"`
}

type actionResponse struct {
	Accepted      bool   `json:"accepted"`
	TerminalState string `json:"terminalState,omitempty"`
	Reason        string `json:"reason,omitempty"`
}

type lootClaimResponse struct {
	Claimed bool   `json:"claimed"`
	Reason  string `json:"reason,omitempty"`
}

type storyLogEntry struct {
	ChapterIndex int    `json:"chapterIndex"`
	ClauseOrder  int    `json:"clauseOrder"`
	Kind         string `json:"kind"`
	TextPlain    string `json:"textPlain"`
}

type playerView struct {
	StoryLog []storyLogEntry `json:"storyLog"`
}

func httpGet(url string, out any) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return json.NewDecoder(resp.Body).Decode(out)
}

func httpPost(url string, body any, out any) error {
	b, err := json.Marshal(body)
	if err != nil {
		return err
	}
	resp, err := http.Post(url, "application/json", strings.NewReader(string(b)))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return json.NewDecoder(resp.Body).Decode(out)
}

func getStoryLog(runID string) []storyLogEntry {
	var view playerView
	if err := httpGet(fmt.Sprintf("http://localhost:18080/runs/%s/state", runID), &view); err != nil {
		log.Printf("getStoryLog(%s): %v", runID, err)
		return nil
	}
	return view.StoryLog
}

func postAction(runID string, chapterIdx, clauseOrder int, charID, rawText string) {
	url := fmt.Sprintf("http://localhost:18080/runs/%s/chapters/%d/clauses/%d/action", runID, chapterIdx, clauseOrder)
	var resp actionResponse
	if err := httpPost(url, map[string]string{"characterId": charID, "rawText": rawText}, &resp); err != nil {
		log.Printf("postAction(%s, %d:%d): %v", runID, chapterIdx, clauseOrder, err)
	}
}

func claimLoot(runID, itemID, charID string) {
	url := fmt.Sprintf("http://localhost:18080/runs/%s/loot/%s/claim", runID, itemID)
	var resp lootClaimResponse
	if err := httpPost(url, map[string]string{"characterId": charID}, &resp); err != nil {
		log.Printf("claimLoot(%s, %s): %v", runID, itemID, err)
	}
}

type logWriter struct {
	file      *os.File
	path      string
	headerWrt bool
}

func newLogWriter() logWriter {
	ts := time.Now().UTC().Format("20060102_150405")
	filename := fmt.Sprintf("demo_%s.json", ts)

	logDir := "./logs"
	_ = os.MkdirAll(logDir, 0755)
	path := filepath.Join(logDir, filename)

	f, err := os.Create(path)
	if err != nil {
		log.Fatalf("create log: %v", err)
	}
	return logWriter{file: f, path: path}
}

func (lw *logWriter) append(r turnResult) {
	if !lw.headerWrt {
		_, _ = lw.file.Write([]byte("[\n"))
		lw.headerWrt = true
	} else {
		_, _ = lw.file.Write([]byte(",\n"))
	}
	b, err := json.MarshalIndent(r, "  ", "  ")
	if err != nil {
		log.Printf("marshal log entry: %v", err)
		return
	}
	_, _ = lw.file.Write(b)
}

func (lw *logWriter) close() {
	if lw.headerWrt {
		_, _ = lw.file.Write([]byte("\n]"))
	} else {
		_, _ = lw.file.Write([]byte("[]"))
	}
	_ = lw.file.Close()
	fmt.Printf("Log written to %s\n", lw.path)
}

func main() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)

	log.Println("Creating narrator...")
	// Same LOCAL_NARRATOR_* env vars as the main server (see main.go), so the
	// demo can be pointed at OpenAI/OpenRouter instead of the local box. When
	// unset, the hardcoded defaults keep the demo runnable out of the box.
	baseURL := envOr("LOCAL_NARRATOR_URL", "http://192.168.0.197:8090")
	model := envOr("LOCAL_NARRATOR_MODEL", "local")
	systemPrompt := envOr("LOCAL_NARRATOR_SYSTEM_PROMPT", "You are a vivid fantasy narrator. Each message starts with a \"Mode:\" line telling you what to do this turn: \"Mode: describe\" means describe the upcoming scene based on the beat premise and known facts; \"Mode: narrate\" means narrate the resolved actions faithfully in the exact order given. Write in plain prose; never decide outcomes or contradict provided data. Maximum response length is 50 words.")
	apiKey := os.Getenv("LOCAL_NARRATOR_API_KEY")
	n := narrator.NewLocal(baseURL, model, systemPrompt, apiKey)
	if logFile := os.Getenv("LLM_LOG_FILE"); logFile != "" {
		n.LogFile = logFile
	}
	if v := os.Getenv("LOCAL_NARRATOR_MAX_TOKENS"); v != "" {
		if maxTokens, err := strconv.Atoi(v); err == nil {
			n.MaxTokens = maxTokens
		}
	}
	// Optional OpenRouter attribution headers (harmless for OpenAI/local).
	if referer := os.Getenv("LOCAL_NARRATOR_HTTP_REFERER"); referer != "" {
		n.ExtraHeaders = map[string]string{"HTTP-Referer": referer}
	}
	if title := os.Getenv("LOCAL_NARRATOR_TITLE"); title != "" {
		if n.ExtraHeaders == nil {
			n.ExtraHeaders = map[string]string{}
		}
		n.ExtraHeaders["X-Title"] = title
	}
	log.Printf("Narrator: %s (model=%s)", baseURL, model)

	log.Println("Creating server...")
	srv := api.NewServer(n)
	srv.WindowDuration = 5 * time.Second
	srv.LootWindowDuration = 3 * time.Second

	log.Println("Registering scenario demo15...")
	srv.RegisterScenario("demo15", &api.ScenarioConfig{
		Gameplay: demoGameplay(),
		CheckFn:  demoCheck,
		EffectFn: demoEffect,
	})

	log.Println("Starting HTTP server on :18080...")
	httpSrv := &http.Server{Addr: ":18080", Handler: srv.Handler()}
	go func() {
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server error: %v", err)
		}
	}()
	time.Sleep(200 * time.Millisecond)
	log.Println("Server startup sleep done")

	log.Println("Creating run...")
	var createResp createRunResponse
	if err := httpPost("http://localhost:18080/runs",
		map[string]string{"mode": "single", "scenario": "demo15"}, &createResp); err != nil {
		log.Fatalf("create run: %v", err)
	}
	runID := createResp.RunID
	log.Printf("Run created: %s", runID)

	log.Println("Joining run...")
	var joinResp joinResponse
	if err := httpPost(fmt.Sprintf("http://localhost:18080/runs/%s/join", runID),
		map[string]string{"characterClass": "warrior", "name": "Aria"}, &joinResp); err != nil {
		log.Fatalf("join: %v", err)
	}
	charID := joinResp.CharacterID
	log.Printf("Joined as character: %s", charID)

	log.Println("Connecting WebSocket...")
	wsURL := fmt.Sprintf("ws://localhost:18080/runs/%s/ws?characterId=%s", runID, charID)
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		log.Fatalf("ws dial: %v", err)
	}
	defer conn.Close()
	log.Println("WebSocket connected")

	log.Println("Starting run...")
	var startResp struct {
		Started bool   `json:"started"`
		Reason  string `json:"reason,omitempty"`
	}
	if err := httpPost(fmt.Sprintf("http://localhost:18080/runs/%s/start", runID),
		map[string]string{"characterId": charID}, &startResp); err != nil {
		log.Fatalf("start run: %v", err)
	}
	if !startResp.Started {
		log.Fatalf("start run failed: %s", startResp.Reason)
	}
	log.Println("Run started")

	lw := newLogWriter()
	var results []turnResult
	var mu sync.Mutex
	type clausePos struct {
		ChapterIndex int
		ClauseOrder  int
		DescribeOnly bool // setup clause: scene only, no action, no narration
	}
	clauseCh := make(chan clausePos, 15)

	go func() {
		for {
			var msg wsMessage
			if err := conn.ReadJSON(&msg); err != nil {
				log.Printf("ws read: %v", err)
				return
			}

			switch msg.Event {
			case "clause_presented":
				var data clausePresentedData
				if err := json.Unmarshal(msg.Data, &data); err != nil {
					log.Printf("unmarshal clause_presented: %v", err)
				} else if !data.RequiresInput {
					// Describe-only (setup) clause: no window opens, no action
					// is submitted, and NARRATE is skipped, so no
					// clause_narrated will ever arrive for it. Record it here
					// as a scene-only turn from PRESENT (describe mode).
					clauseCh <- clausePos{
						ChapterIndex: data.ChapterIndex,
						ClauseOrder:  data.ClauseOrder,
						DescribeOnly: true,
					}
				}

			case "window_opened":
				var data windowOpenedData
				if err := json.Unmarshal(msg.Data, &data); err != nil {
					log.Printf("unmarshal window_opened: %v", err)
				} else {
					key := fmt.Sprintf("%d:%d", data.ChapterIndex, data.ClauseOrder)
					postAction(runID, data.ChapterIndex, data.ClauseOrder, charID, demoActionTexts[key])
				}

			case "loot_window":
				var data lootWindowData
				if err := json.Unmarshal(msg.Data, &data); err != nil {
					log.Printf("unmarshal loot_window: %v", err)
				} else {
					mu.Lock()
					if len(results) == 0 {
						log.Printf("loot_window: results is empty, skipping claim")
						mu.Unlock()
						break
					}
					current := results[len(results)-1]
					mu.Unlock()
					if shouldClaimLoot(current.ChapterIndex, current.ClauseOrder) {
						for _, item := range data.Items {
							claimLoot(runID, item.ItemID, charID)
						}
					}
				}

			case "clause_narrated":
				var data clauseNarratedData
				if err := json.Unmarshal(msg.Data, &data); err != nil {
					log.Printf("unmarshal clause_narrated: %v", err)
				} else {
					mu.Lock()
					results = append(results, turnResult{
						ChapterIndex: data.ChapterIndex,
						ClauseOrder:  data.ClauseOrder,
					})
					mu.Unlock()
					clauseCh <- clausePos{ChapterIndex: data.ChapterIndex, ClauseOrder: data.ClauseOrder}
				}

			case "run_ended":
				close(clauseCh)
				return

			default:
				log.Printf("unknown ws event: %s", msg.Event)
			}
		}
	}()

	for cp := range clauseCh {
		var scene, narration string
		for _, e := range getStoryLog(runID) {
			if e.ChapterIndex == cp.ChapterIndex && e.ClauseOrder == cp.ClauseOrder {
				if e.Kind == "scene" {
					scene = e.TextPlain
				} else if e.Kind == "narration" {
					narration = e.TextPlain
				}
			}
		}

		key := fmt.Sprintf("%d:%d", cp.ChapterIndex, cp.ClauseOrder)
		action := demoActionTexts[key]
		if cp.DescribeOnly {
			// Setup clause: PRESENT set the scene and that was the whole beat.
			// No action was submitted and NARRATE was skipped, so both stay empty.
			action = ""
		}
		fmt.Println("scene---\n", scene)
		fmt.Println("action---\n", action)
		fmt.Println("narration---\n", narration)

		tr := turnResult{
			ChapterIndex: cp.ChapterIndex,
			ClauseOrder:  cp.ClauseOrder,
			Scene:        scene,
			Narration:    narration,
			Action:       action,
		}
		if cp.DescribeOnly {
			// Not tracked in results (the loot handler reads results' last
			// entry as the current acting clause); just record it in the log.
			lw.append(tr)
			continue
		}
		mu.Lock()
		if len(results) == 0 {
			log.Printf("clause loop: results is empty, cannot update index")
			mu.Unlock()
		} else {
			results[len(results)-1] = tr
			mu.Unlock()
		}
		lw.append(tr)
	}

	httpSrv.Close()
	lw.close()
}
