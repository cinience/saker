//go:build agui_e2e

package agui_e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

// =============================================================================
// A. 多媒体深度测试
// =============================================================================

// --- Scenario 11: Video Generation ---

func TestScenario_VideoGeneration(t *testing.T) {
	requireDashscope(t)
	ctx, cancel := mediaLongTimeout(t)
	defer cancel()

	input := newRunInput(Message{Role: "user", Content: "生成一个5秒的日出延时摄影视频"})
	events := doRunWithAutoAnswer(t, ctx, input)

	assertRunLifecycle(t, events)
	assertNoRunError(t, events)

	if !hasToolCall(events, "generate_video") {
		t.Error("no generate_video tool call found")
		t.Logf("tool calls: %+v", extractToolCalls(events))
	}

	assertHasEventType(t, events, "TOOL_CALL_END")

	artifacts := extractArtifacts(events)
	videoFound := false
	for _, a := range artifacts {
		if a.Type == "video" && a.URL != "" {
			videoFound = true
			break
		}
	}
	if !videoFound {
		t.Logf("no video artifact in STATE_DELTA (may be in text response)")
	}

	t.Logf("video generation: %d events, %d artifacts", len(events), len(artifacts))
}

// --- Scenario 12: Music Generation ---

func TestScenario_MusicGeneration(t *testing.T) {
	requireDashscope(t)
	ctx, cancel := mediaLongTimeout(t)
	defer cancel()

	input := newRunInput(Message{Role: "user", Content: "生成一段30秒的轻松钢琴纯音乐，is_instrumental设为true"})
	events := doRunWithAutoAnswer(t, ctx, input)

	assertRunLifecycle(t, events)
	assertNoRunError(t, events)

	if !hasToolCall(events, "generate_music") {
		t.Error("no generate_music tool call found")
		t.Logf("tool calls: %+v", extractToolCalls(events))
	}

	assertHasEventType(t, events, "TOOL_CALL_END")

	artifacts := extractArtifacts(events)
	audioFound := false
	for _, a := range artifacts {
		if a.Type == "audio" && a.URL != "" {
			audioFound = true
			break
		}
	}
	if !audioFound {
		t.Logf("no audio artifact in STATE_DELTA (may be in text response)")
	}

	t.Logf("music generation: %d events, %d artifacts", len(events), len(artifacts))
}

// --- Scenario 13: Image Edit ---

func TestScenario_ImageEdit(t *testing.T) {
	requireDashscope(t)
	ctx, cancel := mediaLongTimeout(t)
	defer cancel()

	threadID := uuid.New().String()

	// Turn 1: Generate an image.
	input1 := RunAgentInput{
		ThreadID: threadID,
		RunID:    uuid.New().String(),
		Messages: []Message{{Role: "user", Content: "生成一张白色背景上的红色圆形图片"}},
	}
	events1 := doRunWithAutoAnswer(t, ctx, input1)
	assertRunLifecycle(t, events1)
	assertNoRunError(t, events1)

	if !hasToolCall(events1, "generate_image") {
		t.Fatal("turn 1: no generate_image tool call")
	}
	t.Logf("turn 1: image generated, %d events", len(events1))

	// Turn 2: Edit the image.
	input2 := RunAgentInput{
		ThreadID: threadID,
		RunID:    uuid.New().String(),
		Messages: []Message{{Role: "user", Content: "把刚才生成的图片中的红色圆形改成蓝色"}},
	}
	events2 := doRunWithAutoAnswer(t, ctx, input2)
	assertRunLifecycle(t, events2)
	assertNoRunError(t, events2)

	// Should use edit_image or generate_image with reference.
	calls := extractToolCalls(events2)
	editFound := false
	for _, c := range calls {
		if c.Name == "edit_image" || c.Name == "generate_image" {
			editFound = true
			break
		}
	}
	if !editFound {
		t.Error("turn 2: no image editing tool call found")
		t.Logf("tool calls: %+v", calls)
	}
	t.Logf("turn 2: image edit, %d events, calls: %+v", len(events2), calls)
}

// --- Scenario 14: Audio Transcription ---

func TestScenario_AudioTranscription(t *testing.T) {
	requireDashscope(t)
	ctx, cancel := mediaLongTimeout(t)
	defer cancel()

	threadID := uuid.New().String()

	// Turn 1: Generate speech.
	input1 := RunAgentInput{
		ThreadID: threadID,
		RunID:    uuid.New().String(),
		Messages: []Message{{Role: "user", Content: "把'人工智能正在改变世界'转成语音，用Cherry声音"}},
	}
	events1 := doRunWithAutoAnswer(t, ctx, input1)
	assertRunLifecycle(t, events1)
	assertNoRunError(t, events1)

	if !hasToolCall(events1, "text_to_speech") {
		t.Fatal("turn 1: no text_to_speech tool call")
	}

	// Turn 2: Transcribe the audio.
	input2 := RunAgentInput{
		ThreadID: threadID,
		RunID:    uuid.New().String(),
		Messages: []Message{{Role: "user", Content: "请转录刚才生成的音频内容"}},
	}
	events2 := doRunWithAutoAnswer(t, ctx, input2)
	assertRunLifecycle(t, events2)
	assertNoRunError(t, events2)

	if hasToolCall(events2, "transcribe_audio") {
		t.Log("transcribe_audio tool was called correctly")
	} else {
		// Model might just recall the text from context.
		text := extractText(events2)
		if strings.Contains(text, "人工智能") || strings.Contains(text, "改变世界") {
			t.Log("model recalled audio content from context (acceptable)")
		} else {
			t.Log("transcribe_audio not called and content not recalled (non-deterministic)")
		}
	}
	t.Logf("audio transcription: turn2 %d events", len(events2))
}

// --- Scenario 15: Multi-Tool Chain ---

func TestScenario_MultiToolChain(t *testing.T) {
	requireDashscope(t)
	ctx, cancel := mediaLongTimeout(t)
	defer cancel()

	input := newRunInput(Message{Role: "user", Content: "先生成一张夕阳海边的图片，然后把'美丽的夕阳'这句话转成语音，用Cherry声音"})
	events := doRunWithAutoAnswer(t, ctx, input)

	assertRunLifecycle(t, events)
	assertNoRunError(t, events)

	calls := extractToolCalls(events)
	hasImage := false
	hasTTS := false
	for _, c := range calls {
		if c.Name == "generate_image" {
			hasImage = true
		}
		if c.Name == "text_to_speech" {
			hasTTS = true
		}
	}

	if !hasImage {
		t.Error("multi-tool: no generate_image call")
	}
	if !hasTTS {
		t.Error("multi-tool: no text_to_speech call")
	}

	artifacts := extractArtifacts(events)
	t.Logf("multi-tool chain: %d tool calls, %d artifacts", len(calls), len(artifacts))
}

// =============================================================================
// B. 协议合规性边界
// =============================================================================

// --- Scenario 16: Concurrent Thread Runs ---

func TestScenario_ConcurrentThreadRuns(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	threadID := uuid.New().String()

	// Start two runs almost simultaneously on the same thread.
	var wg sync.WaitGroup
	var events1, events2 []ParsedEvent
	var err1 error

	// Run 1: long response.
	wg.Add(1)
	go func() {
		defer wg.Done()
		input := RunAgentInput{
			ThreadID: threadID,
			RunID:    uuid.New().String(),
			Messages: []Message{{Role: "user", Content: "写一篇500字关于宇宙的文章"}},
		}
		body, _ := json.Marshal(input)
		req, _ := http.NewRequestWithContext(ctx, http.MethodPost, serverURL+"/v1/agents/run", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "text/event-stream")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			err1 = err
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			events1 = parseSSEStream(t, resp.Body)
		}
	}()

	// Brief delay, then start Run 2.
	time.Sleep(500 * time.Millisecond)

	input2 := RunAgentInput{
		ThreadID: threadID,
		RunID:    uuid.New().String(),
		Messages: []Message{{Role: "user", Content: "说OK"}},
	}
	events2 = doRunWithAutoAnswer(t, ctx, input2)

	wg.Wait()

	// Run 2 should succeed.
	assertRunLifecycle(t, events2)

	// Run 1 should be cancelled (RUN_ERROR or interrupted).
	if err1 != nil {
		t.Logf("run 1 connection error (acceptable): %v", err1)
	} else if len(events1) > 0 {
		last := events1[len(events1)-1]
		if last.Type == "RUN_ERROR" {
			t.Log("run 1 correctly received RUN_ERROR (cancelled by run 2)")
		} else if last.Type == "RUN_FINISHED" {
			// If run 1 finished very quickly before run 2 started, that's also ok.
			t.Log("run 1 finished before cancellation (race condition, acceptable)")
		} else {
			t.Logf("run 1 last event: %s (may have been interrupted mid-stream)", last.Type)
		}
	}

	t.Logf("concurrent: run1=%d events, run2=%d events", len(events1), len(events2))
}

// --- Scenario 17: Invalid Input ---

func TestScenario_InvalidInput(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cases := []struct {
		name       string
		body       string
		wantStatus int
	}{
		{
			name:       "threadId too long",
			body:       fmt.Sprintf(`{"method":"agent/run","body":{"threadId":"%s","runId":"abc","messages":[{"role":"user","content":"hi"}]}}`, strings.Repeat("a", 200)),
			wantStatus: 400,
		},
		{
			name:       "invalid threadId chars",
			body:       `{"threadId":"bad thread id!!@#","runId":"abc123","messages":[{"role":"user","content":"hi"}]}`,
			wantStatus: 400,
		},
		{
			name:       "invalid role",
			body:       `{"threadId":"abc","runId":"def","messages":[{"role":"hacker","content":"hi"}]}`,
			wantStatus: 400,
		},
		{
			name:       "oversized content",
			body:       fmt.Sprintf(`{"threadId":"abc","runId":"def","messages":[{"role":"user","content":"%s"}]}`, strings.Repeat("x", 130*1024)),
			wantStatus: 400,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			status, respBody, err := doRunSSE(ctx, serverURL+"/v1/agents/run", []byte(tc.body))
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			if status != tc.wantStatus {
				t.Errorf("expected status %d, got %d: %s", tc.wantStatus, status, string(respBody))
			} else {
				t.Logf("%s: correctly returned %d", tc.name, status)
			}
		})
	}
}

// --- Scenario 18: Event ID Monotonic (with tool calls) ---

func TestScenario_EventIDMonotonic(t *testing.T) {
	requireDashscope(t)
	ctx, cancel := mediaTimeout(t)
	defer cancel()

	input := newRunInput(Message{Role: "user", Content: "写100字介绍AI，然后生成一张AI主题的图片"})
	events := doRunWithAutoAnswer(t, ctx, input)

	assertRunLifecycle(t, events)
	assertNoRunError(t, events)

	// Verify all event IDs are strictly monotonically increasing.
	var lastID int
	gaps := 0
	for _, e := range events {
		if e.ID == "" {
			continue
		}
		var id int
		if _, err := fmt.Sscanf(e.ID, "%d", &id); err == nil {
			if id <= lastID {
				t.Errorf("event ID not monotonic: %d <= %d (event type: %s)", id, lastID, e.Type)
				break
			}
			if lastID > 0 && id != lastID+1 {
				gaps++
			}
			lastID = id
		}
	}

	// Should have tool call events mixed with text events.
	calls := extractToolCalls(events)
	if len(calls) == 0 {
		t.Log("no tool calls in this run (model may have skipped image gen)")
	}

	t.Logf("event ID monotonic: max_id=%d, gaps=%d, tool_calls=%d, total_events=%d", lastID, gaps, len(calls), len(events))
}

// --- Scenario 19: State Snapshot and Delta ---

func TestScenario_StateSnapshotAndDelta(t *testing.T) {
	requireDashscope(t)
	ctx, cancel := mediaTimeout(t)
	defer cancel()

	input := newRunInput(Message{Role: "user", Content: "生成一张星空的图片"})
	events := doRunWithAutoAnswer(t, ctx, input)

	assertRunLifecycle(t, events)
	assertNoRunError(t, events)

	// Verify STATE_SNAPSHOT appears after RUN_STARTED.
	snapshotFound := false
	runStartIdx := -1
	snapshotIdx := -1
	for i, e := range events {
		if e.Type == "RUN_STARTED" {
			runStartIdx = i
		}
		if e.Type == "STATE_SNAPSHOT" {
			snapshotIdx = i
			snapshotFound = true
			// Format may be wrapped: {"type":"STATE_SNAPSHOT","snapshot":{"artifacts":[]}}
			var wrapped struct {
				Snapshot struct {
					Artifacts []interface{} `json:"artifacts"`
				} `json:"snapshot"`
			}
			var direct struct {
				Artifacts []interface{} `json:"artifacts"`
			}
			if json.Unmarshal(e.Raw, &wrapped) == nil && wrapped.Snapshot.Artifacts != nil {
				t.Log("STATE_SNAPSHOT has wrapped format with artifacts")
			} else if json.Unmarshal(e.Raw, &direct) == nil && direct.Artifacts != nil {
				t.Log("STATE_SNAPSHOT has direct format with artifacts")
			} else {
				t.Logf("STATE_SNAPSHOT format: %.200s", string(e.Raw))
			}
			break
		}
	}
	if !snapshotFound {
		t.Error("STATE_SNAPSHOT not found in events")
	}
	if runStartIdx >= 0 && snapshotIdx >= 0 && snapshotIdx <= runStartIdx {
		t.Error("STATE_SNAPSHOT should appear after RUN_STARTED")
	}

	// Verify STATE_DELTA contains valid JSON Patch ops.
	deltaCount := 0
	for _, e := range events {
		if e.Type != "STATE_DELTA" {
			continue
		}
		deltaCount++
		// Try wrapped format: {"type":"STATE_DELTA","delta":[...]}
		var wrapper struct {
			Delta json.RawMessage `json:"delta"`
		}
		var patchData json.RawMessage
		if json.Unmarshal(e.Raw, &wrapper) == nil && len(wrapper.Delta) > 0 {
			patchData = wrapper.Delta
		} else {
			patchData = e.Raw
		}

		var patches []struct {
			Op    string      `json:"op"`
			Path  string      `json:"path"`
			Value interface{} `json:"value"`
		}
		if err := json.Unmarshal(patchData, &patches); err != nil {
			t.Logf("STATE_DELTA not a JSON Patch array (may be other format): %.100s", string(e.Raw))
			continue
		}
		for _, p := range patches {
			if p.Op != "add" && p.Op != "replace" && p.Op != "remove" {
				t.Errorf("STATE_DELTA has invalid op: %s", p.Op)
			}
			if p.Path == "" {
				t.Error("STATE_DELTA patch missing path")
			}
		}
	}
	if deltaCount == 0 {
		t.Log("no STATE_DELTA events (image artifact may not have been generated)")
	}

	artifacts := extractArtifacts(events)
	t.Logf("state snapshot/delta: snapshot_at=%d, %d artifacts", snapshotIdx, len(artifacts))
}

// --- Scenario 20: Thread Update (Rename) ---

func TestScenario_ThreadUpdate(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	// Step 1: Create thread.
	threadID := uuid.New().String()
	input := RunAgentInput{
		ThreadID: threadID,
		RunID:    uuid.New().String(),
		Messages: []Message{{Role: "user", Content: "说OK"}},
	}
	events := doRunWithAutoAnswer(t, ctx, input)
	assertRunLifecycle(t, events)

	time.Sleep(500 * time.Millisecond)

	// Step 2: Rename thread.
	newTitle := "测试重命名-" + threadID[:8]
	renameBody := map[string]string{"title": newTitle}
	status, respBody := doHTTP(t, ctx, http.MethodPatch, serverURL+"/v1/agents/run/threads/"+threadID, renameBody)
	if status != http.StatusOK {
		t.Fatalf("PATCH thread: status %d: %s", status, string(respBody))
	}

	// Step 3: Verify in thread list.
	time.Sleep(300 * time.Millisecond)
	status, body := doHTTP(t, ctx, http.MethodGet, serverURL+"/v1/agents/run/threads", nil)
	if status != http.StatusOK {
		t.Fatalf("GET threads: status %d", status)
	}
	if !strings.Contains(string(body), newTitle) {
		t.Errorf("thread title %q not found in list: %s", newTitle, string(body)[:200])
	}

	t.Logf("thread update: renamed to %q", newTitle)
}

// =============================================================================
// C. 复杂对话流程
// =============================================================================

// --- Scenario 21: Long Conversation History (5 turns) ---

func TestScenario_LongConversationHistory(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	threadID := uuid.New().String()

	// Send 5 turns with specific information in each.
	turns := []struct {
		content string
		keyword string
	}{
		{"记住：我最喜欢的水果是芒果", ""},
		{"记住：我的宠物叫小白，是一只猫", ""},
		{"记住：我住在杭州西湖附近", ""},
		{"记住：我喜欢的颜色是紫色", ""},
		{"我最喜欢的水果是什么？", "芒果"},
	}

	for i, turn := range turns {
		input := RunAgentInput{
			ThreadID: threadID,
			RunID:    uuid.New().String(),
			Messages: []Message{{Role: "user", Content: turn.content}},
		}
		events := doRunWithAutoAnswer(t, ctx, input)
		assertRunLifecycle(t, events)
		assertNoRunError(t, events)

		if turn.keyword != "" {
			text := extractText(events)
			if !strings.Contains(text, turn.keyword) {
				t.Errorf("turn %d: expected %q in response, got: %.100s", i+1, turn.keyword, text)
			} else {
				t.Logf("turn %d: correctly recalled %q", i+1, turn.keyword)
			}
		} else {
			t.Logf("turn %d: context stored (%.50s)", i+1, extractText(events))
		}
	}
}

// --- Scenario 22: Step Events ---

func TestScenario_StepEvents(t *testing.T) {
	requireDashscope(t)
	ctx, cancel := mediaLongTimeout(t)
	defer cancel()

	input := newRunInput(Message{Role: "user", Content: "生成一张可爱猫咪的图片，然后写一首关于猫的四行诗"})
	events := doRunWithAutoAnswer(t, ctx, input)

	assertRunLifecycle(t, events)
	assertNoRunError(t, events)

	// Check for STEP_STARTED / STEP_FINISHED events.
	stepStarted := 0
	stepFinished := 0
	for _, e := range events {
		if e.Type == "STEP_STARTED" {
			stepStarted++
		}
		if e.Type == "STEP_FINISHED" {
			stepFinished++
		}
	}

	if stepStarted == 0 {
		t.Log("no STEP_STARTED events (model may have completed in single iteration)")
	} else {
		t.Logf("step events: %d started, %d finished", stepStarted, stepFinished)
	}

	// At least verify tool call + text output (multi-iteration evidence).
	calls := extractToolCalls(events)
	text := extractText(events)
	if len(calls) > 0 && len(text) > 0 {
		t.Log("multi-iteration confirmed: has tool calls + text output")
	}
}

// --- Scenario 23: Activity Events ---

func TestScenario_ActivityEvents(t *testing.T) {
	requireDashscope(t)
	ctx, cancel := mediaTimeout(t)
	defer cancel()

	input := newRunInput(Message{Role: "user", Content: "生成一张山水画图片"})
	events := doRunWithAutoAnswer(t, ctx, input)

	assertRunLifecycle(t, events)
	assertNoRunError(t, events)

	// Check for ACTIVITY_SNAPSHOT and ACTIVITY_DELTA events.
	activitySnapshot := false
	activityDelta := false
	for _, e := range events {
		if e.Type == "ACTIVITY_SNAPSHOT" {
			activitySnapshot = true
		}
		if e.Type == "ACTIVITY_DELTA" {
			activityDelta = true
		}
	}

	if !activitySnapshot {
		t.Error("no ACTIVITY_SNAPSHOT event found")
	}
	if !activityDelta {
		t.Log("no ACTIVITY_DELTA event (may not be emitted for all tool results)")
	}

	t.Logf("activity events: snapshot=%v, delta=%v", activitySnapshot, activityDelta)
}

// --- Scenario 24: Connect Messages Snapshot ---

func TestScenario_ConnectMessagesSnapshot(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	threadID := uuid.New().String()

	// Turn 1: Create conversation.
	input1 := RunAgentInput{
		ThreadID: threadID,
		RunID:    uuid.New().String(),
		Messages: []Message{{Role: "user", Content: "我最喜欢的水果是芒果"}},
	}
	events1 := doRunWithAutoAnswer(t, ctx, input1)
	assertRunLifecycle(t, events1)

	// Turn 2: Add more context.
	input2 := RunAgentInput{
		ThreadID: threadID,
		RunID:    uuid.New().String(),
		Messages: []Message{{Role: "user", Content: "我最喜欢的颜色是绿色"}},
	}
	events2 := doRunWithAutoAnswer(t, ctx, input2)
	assertRunLifecycle(t, events2)

	time.Sleep(500 * time.Millisecond)

	// Connect to thread and expect MESSAGES_SNAPSHOT.
	connectInput := map[string]interface{}{
		"method": "agent/connect",
		"body": map[string]interface{}{
			"threadId": threadID,
		},
	}
	connectBody, _ := json.Marshal(connectInput)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, serverURL+"/v1/agents/run", bytes.NewReader(connectBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("connect status %d: %s", resp.StatusCode, string(body))
	}

	connectEvents := parseSSEStream(t, resp.Body)

	// Verify MESSAGES_SNAPSHOT exists.
	snapshotFound := false
	for _, e := range connectEvents {
		if e.Type == "MESSAGES_SNAPSHOT" {
			snapshotFound = true
			raw := string(e.Raw)
			// Should contain previous conversation content.
			if strings.Contains(raw, "芒果") || strings.Contains(raw, "绿色") {
				t.Log("MESSAGES_SNAPSHOT contains previous conversation content")
			} else {
				t.Logf("MESSAGES_SNAPSHOT may not contain full history: %.200s", raw)
			}
			break
		}
	}

	if !snapshotFound {
		t.Error("MESSAGES_SNAPSHOT not found in connect response")
		t.Logf("connect events: %v", eventTypes(connectEvents))
	}

	t.Logf("connect messages snapshot: %d events, snapshot=%v", len(connectEvents), snapshotFound)
}

// --- Scenario 25: Envelope Format ---

func TestScenario_EnvelopeFormat(t *testing.T) {
	ctx, cancel := defaultTimeout(t)
	defer cancel()

	// Test 1: agent/run via envelope format.
	runInput := newRunInput(Message{Role: "user", Content: "说OK"})
	envelope := map[string]interface{}{
		"method": "agent/run",
		"body":   runInput,
	}
	body, _ := json.Marshal(envelope)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, serverURL+"/v1/agents/run", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("envelope run: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("envelope run: status %d: %s", resp.StatusCode, string(respBody))
	}
	events := parseSSEStream(t, resp.Body)
	assertRunLifecycle(t, events)
	t.Logf("envelope agent/run: %d events, PASS", len(events))

	// Test 2: info via envelope format.
	infoEnvelope := map[string]interface{}{"method": "info"}
	status, infoBody := doHTTP(t, ctx, http.MethodPost, serverURL+"/v1/agents/run", infoEnvelope)
	if status != http.StatusOK {
		t.Errorf("envelope info: status %d: %s", status, string(infoBody))
	} else {
		var info map[string]interface{}
		if json.Unmarshal(infoBody, &info) == nil {
			if _, ok := info["name"]; ok {
				t.Log("envelope info: returned agent name")
			}
		}
		t.Logf("envelope info: %s", string(infoBody)[:min(100, len(infoBody))])
	}

	// Test 3: capabilities via envelope format.
	capEnvelope := map[string]interface{}{"method": "capabilities"}
	status, capBody := doHTTP(t, ctx, http.MethodPost, serverURL+"/v1/agents/run", capEnvelope)
	if status != http.StatusOK {
		t.Errorf("envelope capabilities: status %d: %s", status, string(capBody))
	} else {
		if strings.Contains(string(capBody), "streaming") || strings.Contains(string(capBody), "tools") {
			t.Log("envelope capabilities: contains expected fields")
		}
		t.Logf("envelope capabilities: %s", string(capBody)[:min(150, len(capBody))])
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
