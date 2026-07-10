package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
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
	_ = httpGet(fmt.Sprintf("http://localhost:18080/runs/%s/state", runID), &view)
	return view.StoryLog
}

func postAction(runID string, chapterIdx, clauseOrder int, charID, rawText string) {
	url := fmt.Sprintf("http://localhost:18080/runs/%s/chapters/%d/clauses/%d/action", runID, chapterIdx, clauseOrder)
	var resp actionResponse
	_ = httpPost(url, map[string]string{"characterId": charID, "rawText": rawText}, &resp)
}

func claimLoot(runID, itemID, charID string) {
	url := fmt.Sprintf("http://localhost:18080/runs/%s/loot/%s/claim", runID, itemID)
	var resp lootClaimResponse
	_ = httpPost(url, map[string]string{"characterId": charID}, &resp)
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
	n := narrator.NewLocal("http://192.168.0.197:8090", "local", "You are a vivid fantasy narrator. In PRESENT mode, describe the upcoming scene based on the beat premise and known facts. In NARRATE mode, narrate resolved actions faithfully in the exact order given. Write in plain prose; never decide outcomes or contradict provided data. Maximum response length is 50 words.", "PCC9zXtmeqYaGJacEOvihiMNugyc1EuAaN7c7G43HW6rjKXP0bF4cMM66qOITQQ9")
	if logFile := os.Getenv("LLM_LOG_FILE"); logFile != "" {
		n.LogFile = logFile
	}
	srv := api.NewServer(n)
	srv.WindowDuration = 5 * time.Second
	srv.LootWindowDuration = 3 * time.Second
	srv.RegisterScenario("demo15", &api.ScenarioConfig{
		Gameplay: demoGameplay(),
		CheckFn:  demoCheck,
		EffectFn: demoEffect,
	})

	httpSrv := &http.Server{Addr: ":18080", Handler: srv.Handler()}
	go httpSrv.ListenAndServe()
	time.Sleep(200 * time.Millisecond)

	var createResp createRunResponse
	if err := httpPost("http://localhost:18080/runs",
		map[string]string{"mode": "single", "scenario": "demo15"}, &createResp); err != nil {
		log.Fatalf("create run: %v", err)
	}
	runID := createResp.RunID

	var joinResp joinResponse
	if err := httpPost(fmt.Sprintf("http://localhost:18080/runs/%s/join", runID),
		map[string]string{"characterClass": "warrior", "name": "Aria"}, &joinResp); err != nil {
		log.Fatalf("join: %v", err)
	}
	charID := joinResp.CharacterID

	wsURL := fmt.Sprintf("ws://localhost:18080/runs/%s/ws?characterId=%s", runID, charID)
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		log.Fatalf("ws dial: %v", err)
	}
	defer conn.Close()

	lw := newLogWriter()
	var results []turnResult
	var mu sync.Mutex
	type clausePos struct {
		ChapterIndex int
		ClauseOrder  int
	}
	clauseCh := make(chan clausePos, 15)

	go func() {
		for {
			var msg wsMessage
			if err := conn.ReadJSON(&msg); err != nil {
				return
			}

			switch msg.Event {
			case "window_opened":
				var data windowOpenedData
				if err := json.Unmarshal(msg.Data, &data); err == nil {
					key := fmt.Sprintf("%d:%d", data.ChapterIndex, data.ClauseOrder)
					postAction(runID, data.ChapterIndex, data.ClauseOrder, charID, demoActionTexts[key])
				}

			case "loot_window":
				var data lootWindowData
				if err := json.Unmarshal(msg.Data, &data); err == nil {
					mu.Lock()
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
				if err := json.Unmarshal(msg.Data, &data); err == nil {
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
		fmt.Println("scene---\n", scene)
		fmt.Println("action---\n", demoActionTexts[key])
		fmt.Println("narration---\n", narration)

		tr := turnResult{
			ChapterIndex: cp.ChapterIndex,
			ClauseOrder:  cp.ClauseOrder,
			Scene:        scene,
			Narration:    narration,
			Action:       demoActionTexts[key],
		}
		mu.Lock()
		results[len(results)-1] = tr
		mu.Unlock()
		lw.append(tr)
	}

	httpSrv.Close()
	lw.close()
}
