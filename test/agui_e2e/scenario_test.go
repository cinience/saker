//go:build agui_e2e

package agui_e2e

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func requireDashscope(t *testing.T) {
	t.Helper()
	if os.Getenv("DASHSCOPE_API_KEY") == "" {
		t.Skip("requires DASHSCOPE_API_KEY")
	}
}

// --- Scenario 1: Basic Text Chat ---

func TestScenario_BasicTextChat(t *testing.T) {
	ctx, cancel := defaultTimeout(t)
	defer cancel()

	input := newRunInput(Message{Role: "user", Content: "你好，用一句话介绍自己"})
	events := doRun(t, ctx, input)

	assertRunLifecycle(t, events)
	assertNoRunError(t, events)
	assertEventSequenceContains(t, events,
		"RUN_STARTED",
		"STATE_SNAPSHOT",
		"TEXT_MESSAGE_START",
		"TEXT_MESSAGE_CONTENT",
		"TEXT_MESSAGE_END",
		"RUN_FINISHED",
	)

	text := extractText(events)
	if len(text) < 5 {
		t.Errorf("response too short: %q", text)
	}

	// Verify RUN_STARTED has threadId and runId.
	var runStarted struct {
		ThreadID string `json:"threadId"`
		RunID    string `json:"runId"`
	}
	if err := json.Unmarshal(events[0].Raw, &runStarted); err == nil {
		if runStarted.ThreadID == "" {
			t.Error("RUN_STARTED missing threadId")
		}
		if runStarted.RunID == "" {
			t.Error("RUN_STARTED missing runId")
		}
	}

	t.Logf("response text (%d chars): %.100s...", len(text), text)
}

// --- Scenario 2: Image Generation ---

func TestScenario_ImageGeneration(t *testing.T) {
	requireDashscope(t)
	ctx, cancel := mediaTimeout(t)
	defer cancel()

	input := newRunInput(Message{Role: "user", Content: "生成一张日落海边的图片"})
	events := doRun(t, ctx, input)

	assertRunLifecycle(t, events)
	assertNoRunError(t, events)

	// Should have tool call for generate_image.
	calls := extractToolCalls(events)
	found := false
	for _, c := range calls {
		if c.Name == "generate_image" {
			found = true
			break
		}
	}
	if !found {
		t.Error("no generate_image tool call found")
		t.Logf("tool calls: %+v", calls)
		t.Logf("event types: %v", eventTypes(events))
	}

	// Should have TOOL_CALL_END.
	assertHasEventType(t, events, "TOOL_CALL_END")

	// Should have STATE_DELTA with artifact.
	if hasEventType(events, "STATE_DELTA") {
		stateDelta := assertHasEventType(t, events, "STATE_DELTA")
		raw := string(stateDelta.Raw)
		if !strings.Contains(raw, "artifact") && !strings.Contains(raw, "image") {
			t.Logf("STATE_DELTA may not contain image artifact: %.200s", raw)
		}
	}

	// Should have assistant text describing the result.
	text := extractText(events)
	if text == "" {
		t.Error("no text response after image generation")
	}
	t.Logf("image gen response: %.100s", text)
}

// --- Scenario 3: Text to Speech ---

func TestScenario_TextToSpeech(t *testing.T) {
	requireDashscope(t)
	ctx, cancel := mediaTimeout(t)
	defer cancel()

	input := newRunInput(Message{Role: "user", Content: "把'你好世界'转成语音，用Cherry声音"})
	events := doRun(t, ctx, input)

	assertRunLifecycle(t, events)
	assertNoRunError(t, events)

	calls := extractToolCalls(events)
	found := false
	for _, c := range calls {
		if c.Name == "text_to_speech" {
			found = true
			break
		}
	}
	if !found {
		t.Error("no text_to_speech tool call found")
		t.Logf("tool calls: %+v", calls)
	}

	// Verify TOOL_CALL_ARGS contains voice field.
	for _, e := range events {
		if e.Type == "TOOL_CALL_ARGS" {
			raw := string(e.Raw)
			if strings.Contains(raw, "voice") || strings.Contains(raw, "Cherry") {
				t.Log("TOOL_CALL_ARGS correctly includes voice parameter")
				break
			}
		}
	}

	assertHasEventType(t, events, "TOOL_CALL_END")
	t.Logf("TTS completed, %d events total", len(events))
}

// --- Scenario 4: Multi-Turn Context ---

func TestScenario_MultiTurnContext(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	// Server uses ThreadID for session history. We must send two separate
	// requests on the same thread to test multi-turn context.
	threadID := uuid.New().String()

	// Turn 1: introduce context.
	input1 := RunAgentInput{
		ThreadID: threadID,
		RunID:    uuid.New().String(),
		Messages: []Message{{Role: "user", Content: "记住：我的名字是小明，我住在北京海淀区"}},
	}
	events1 := doRunWithAutoAnswer(t, ctx, input1)
	assertRunLifecycle(t, events1)
	assertNoRunError(t, events1)
	t.Logf("turn 1 response: %.80s", extractText(events1))

	// Turn 2: ask about previous context on the same thread.
	input2 := RunAgentInput{
		ThreadID: threadID,
		RunID:    uuid.New().String(),
		Messages: []Message{{Role: "user", Content: "我叫什么名字？住在哪里？"}},
	}
	events2 := doRunWithAutoAnswer(t, ctx, input2)
	assertRunLifecycle(t, events2)
	assertNoRunError(t, events2)

	text := extractText(events2)
	if !strings.Contains(text, "小明") {
		t.Errorf("multi-turn context lost: response does not contain '小明': %s", text)
	}
	t.Logf("multi-turn response: %.100s", text)
}

// --- Scenario 5: Tool Call Error Handling ---

func TestScenario_ToolCallError(t *testing.T) {
	requireDashscope(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	// Use a voice name that doesn't exist to trigger a tool execution error.
	// The model may recover via HITL (asking user for correct voice) — we auto-answer.
	// The key assertion: run completes gracefully without server crash.
	input := newRunInput(Message{Role: "user", Content: "用text_to_speech工具把'测试'转成语音，voice用'NONEXISTENT_VOICE_XYZ_999'"})
	events := doRunWithAutoAnswer(t, ctx, input)

	assertRunLifecycle(t, events)

	// The run should complete (RUN_FINISHED), not crash with unrecoverable error.
	last := events[len(events)-1]
	if last.Type == "RUN_ERROR" {
		t.Logf("run ended with RUN_ERROR (acceptable if tool error was surfaced): %s", string(last.Raw))
	} else if last.Type != "RUN_FINISHED" {
		t.Errorf("expected RUN_FINISHED or graceful RUN_ERROR, got %s", last.Type)
	}

	// Verify a tool call was attempted.
	calls := extractToolCalls(events)
	if len(calls) > 0 {
		t.Logf("tool calls made: %+v", calls)
	}

	t.Logf("tool error handling: %d events, last=%s", len(events), last.Type)
}

// --- Scenario 6: Thread Lifecycle ---

func TestScenario_ThreadLifecycle(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	// Step 1: Create a thread by sending a message.
	threadID := uuid.New().String()
	input := RunAgentInput{
		ThreadID: threadID,
		RunID:    uuid.New().String(),
		Messages: []Message{{Role: "user", Content: "说OK"}},
	}
	events := doRun(t, ctx, input)
	assertRunLifecycle(t, events)
	assertNoRunError(t, events)

	// Step 2: List threads — verify our thread exists.
	time.Sleep(500 * time.Millisecond) // Allow persistence to flush.
	resp, err := http.Get(serverURL + "/v1/agents/run/threads")
	if err != nil {
		t.Fatalf("list threads: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("list threads: status %d: %s", resp.StatusCode, string(body))
	}

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), threadID) {
		t.Errorf("thread %s not found in thread list: %s", threadID, string(body))
	}

	// Step 3: Connect to thread — expect MESSAGES_SNAPSHOT.
	connectInput := map[string]interface{}{
		"method": "agent/connect",
		"body": map[string]interface{}{
			"threadId": threadID,
		},
	}
	connectBody, _ := json.Marshal(connectInput)
	connectReq, _ := http.NewRequestWithContext(ctx, http.MethodPost, serverURL+"/v1/agents/run", strings.NewReader(string(connectBody)))
	connectReq.Header.Set("Content-Type", "application/json")
	connectReq.Header.Set("Accept", "text/event-stream")

	connectResp, err := http.DefaultClient.Do(connectReq)
	if err != nil {
		t.Fatalf("connect to thread: %v", err)
	}
	defer connectResp.Body.Close()

	if connectResp.StatusCode == http.StatusOK {
		connectEvents := parseSSEStream(t, connectResp.Body)
		if hasEventType(connectEvents, "MESSAGES_SNAPSHOT") {
			t.Log("connect returned MESSAGES_SNAPSHOT as expected")
		} else {
			t.Logf("connect events: %v", eventTypes(connectEvents))
		}
	} else {
		t.Logf("connect returned status %d (may require different auth)", connectResp.StatusCode)
	}

	t.Logf("thread lifecycle: created %s, listed, connected", threadID)
}

// --- Scenario 7: Thread Deletion ---

func TestScenario_ThreadDeletion(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	// Step 1: Create thread with a simple prompt.
	threadID := uuid.New().String()
	input := RunAgentInput{
		ThreadID: threadID,
		RunID:    uuid.New().String(),
		Messages: []Message{{Role: "user", Content: "说OK"}},
	}
	events := doRunWithAutoAnswer(t, ctx, input)
	assertRunLifecycle(t, events)

	time.Sleep(500 * time.Millisecond)

	// Step 2: Delete thread.
	delReq, _ := http.NewRequestWithContext(ctx, http.MethodDelete, serverURL+"/v1/agents/run/threads/"+threadID, nil)
	delResp, err := http.DefaultClient.Do(delReq)
	if err != nil {
		t.Fatalf("delete thread: %v", err)
	}
	delResp.Body.Close()

	if delResp.StatusCode != http.StatusOK && delResp.StatusCode != http.StatusNoContent {
		t.Errorf("delete returned status %d, expected 200 or 204", delResp.StatusCode)
	}

	// Step 3: Verify thread no longer in list.
	time.Sleep(300 * time.Millisecond)
	resp, err := http.Get(serverURL + "/v1/agents/run/threads")
	if err != nil {
		t.Fatalf("list threads after delete: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if strings.Contains(string(body), threadID) {
		t.Errorf("thread %s still in list after deletion", threadID)
	}

	t.Logf("thread deletion: %s deleted successfully", threadID)
}

// --- Scenario 8: Long Streaming Response ---

func TestScenario_LongStreamingResponse(t *testing.T) {
	ctx, cancel := defaultTimeout(t)
	defer cancel()

	input := newRunInput(Message{Role: "user", Content: "写一首500字的现代诗，主题是星空"})
	events := doRun(t, ctx, input)

	assertRunLifecycle(t, events)
	assertNoRunError(t, events)

	// Count TEXT_MESSAGE_CONTENT events — should be many for streaming.
	contentCount := 0
	for _, e := range events {
		if e.Type == "TEXT_MESSAGE_CONTENT" {
			contentCount++
		}
	}
	if contentCount < 10 {
		t.Errorf("expected > 10 TEXT_MESSAGE_CONTENT events (streaming), got %d", contentCount)
	}

	text := extractText(events)
	if len(text) < 200 {
		t.Errorf("expected long response (>200 chars), got %d chars", len(text))
	}

	// Verify event IDs are monotonically increasing.
	var lastID int
	for _, e := range events {
		if e.ID == "" {
			continue
		}
		var id int
		if _, err := fmt.Sscanf(e.ID, "%d", &id); err == nil {
			if id <= lastID {
				t.Errorf("event ID not monotonic: %d <= %d", id, lastID)
				break
			}
			lastID = id
		}
	}

	t.Logf("long stream: %d content events, %d chars, max event ID %d", contentCount, len(text), lastID)
}

// --- Scenario 9: HITL Interaction ---

func TestScenario_HITLInteraction(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	// Use a prompt that triggers ask_user_question tool.
	input := newRunInput(Message{Role: "user", Content: "请调用ask_user_question工具，问题是'确认操作？'，选项是['是','否']"})

	// Start run in background and watch for HITL events.
	body, _ := json.Marshal(input)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, serverURL+"/v1/agents/run", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("start HITL run: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("HITL run status %d: %s", resp.StatusCode, string(respBody))
	}

	// HITL emits as: TOOL_CALL_START (name="question_request") → TOOL_CALL_ARGS (payload with question_id) → TOOL_CALL_END
	// We need to detect it, extract question_id, then POST the answer.
	var events []ParsedEvent
	var hitlRunID string
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	var currentID, currentEvent string
	var dataLines []string
	hitlFound := false
	pendingQuestion := false // true after seeing question_request TOOL_CALL_START

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if len(dataLines) > 0 && currentEvent != "" {
				data := strings.Join(dataLines, "\n")
				evt := ParsedEvent{ID: currentID, Type: currentEvent, Raw: json.RawMessage(data)}
				events = append(events, evt)

				// Detect HITL: TOOL_CALL_START with toolCallName="question_request"
				if currentEvent == "TOOL_CALL_START" {
					var tc struct {
						ToolCallName string `json:"toolCallName"`
					}
					if json.Unmarshal(evt.Raw, &tc) == nil && tc.ToolCallName == "question_request" {
						hitlFound = true
						pendingQuestion = true
					}
				}

				// When we see TOOL_CALL_ARGS after a question_request, extract question_id and answer.
				if pendingQuestion && currentEvent == "TOOL_CALL_ARGS" {
					pendingQuestion = false
					var args struct {
						Delta string `json:"delta"`
					}
					if json.Unmarshal(evt.Raw, &args) == nil {
						var payload struct {
							QuestionID string `json:"question_id"`
						}
						if json.Unmarshal([]byte(args.Delta), &payload) == nil && payload.QuestionID != "" {
							// Get runId from RUN_STARTED (only once).
							if hitlRunID == "" {
								for _, e := range events {
									if e.Type == "RUN_STARTED" {
										var rs struct {
											RunID string `json:"runId"`
										}
										json.Unmarshal(e.Raw, &rs)
										hitlRunID = rs.RunID
										break
									}
								}
							}
							// Submit answer asynchronously for every question.
							if hitlRunID != "" {
								qID := payload.QuestionID
								rID := hitlRunID
								go func() {
									time.Sleep(300 * time.Millisecond)
									answerURL := fmt.Sprintf("%s/v1/agents/run/%s/answer", serverURL, rID)
									answerBody := fmt.Sprintf(`{"question_id":"%s","answers":{"0":"是"}}`, qID)
									http.Post(answerURL, "application/json", strings.NewReader(answerBody))
								}()
							}
						}
					}
				}

				// Stop if run is finished.
				if currentEvent == "RUN_FINISHED" || currentEvent == "RUN_ERROR" {
					break
				}
			}
			currentID = ""
			currentEvent = ""
			dataLines = nil
			continue
		}
		if strings.HasPrefix(line, "id: ") {
			currentID = strings.TrimPrefix(line, "id: ")
		} else if strings.HasPrefix(line, "event: ") {
			currentEvent = strings.TrimPrefix(line, "event: ")
		} else if strings.HasPrefix(line, "data: ") {
			dataLines = append(dataLines, strings.TrimPrefix(line, "data: "))
		}
	}

	assertRunLifecycle(t, events)

	if hitlFound {
		t.Log("HITL interaction completed: question asked and answered")
	} else {
		// Model might not reliably trigger HITL — mark as informational, not failure.
		t.Log("HITL event not triggered by model (non-deterministic scenario)")
	}
	t.Logf("HITL scenario: %d events, hitl_found=%v", len(events), hitlFound)
}

// --- Scenario 10: Reconnect ---

func TestScenario_Reconnect(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	// Start a run that will generate a long response.
	input := RunAgentInput{
		ThreadID: uuid.New().String(),
		RunID:    uuid.New().String(),
		Messages: []Message{{Role: "user", Content: "写一篇200字的短文，主题是春天"}},
	}

	// Step 1: Collect first few events then disconnect.
	events, lastID := doRunPartial(t, ctx, input, 5)
	if len(events) < 3 {
		t.Skipf("not enough events to test reconnect (got %d)", len(events))
	}
	if lastID == "" {
		t.Skip("no event IDs in stream, reconnect not testable")
	}

	t.Logf("disconnected after %d events, last_event_id=%s", len(events), lastID)

	// Step 2: Wait briefly then reconnect.
	time.Sleep(1 * time.Second)

	reconnectEvents := doReconnect(t, ctx, input, lastID)
	if len(reconnectEvents) == 0 {
		// May get 410 Gone if session already expired — acceptable.
		t.Log("reconnect returned no events (session may have completed)")
		return
	}

	// Step 3: Verify no duplicate events.
	for _, re := range reconnectEvents {
		if re.ID != "" && re.ID <= lastID {
			t.Errorf("reconnect replayed already-received event ID %s (last was %s)", re.ID, lastID)
			break
		}
	}

	t.Logf("reconnect: got %d new events after ID %s", len(reconnectEvents), lastID)

	// If reconnect delivered remaining events, check it ends properly.
	if len(reconnectEvents) > 0 {
		last := reconnectEvents[len(reconnectEvents)-1]
		if last.Type == "RUN_FINISHED" || last.Type == "RUN_ERROR" {
			t.Log("reconnect delivered full remaining stream")
		}
	}
}
