//go:build agui_e2e

package agui_e2e

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

// ParsedEvent represents a single AG-UI SSE event.
type ParsedEvent struct {
	ID   string
	Type string
	Raw  json.RawMessage
}

// ToolCallInfo holds parsed TOOL_CALL_START data.
type ToolCallInfo struct {
	Name   string `json:"toolCallName"`
	CallID string `json:"toolCallId"`
}

// RunAgentInput is the request payload for POST /v1/agents/run.
type RunAgentInput struct {
	ThreadID       string        `json:"threadId"`
	RunID          string        `json:"runId"`
	Messages       []Message     `json:"messages"`
	Tools          []interface{} `json:"tools,omitempty"`
	Context        []interface{} `json:"context,omitempty"`
	ForwardedProps interface{}   `json:"forwardedProps,omitempty"`
}

// Message represents a chat message.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// newRunInput creates a RunAgentInput with fresh IDs.
func newRunInput(messages ...Message) RunAgentInput {
	return RunAgentInput{
		ThreadID: uuid.New().String(),
		RunID:    uuid.New().String(),
		Messages: messages,
	}
}

// doRun sends a run request and collects all SSE events until the stream closes.
func doRun(t *testing.T, ctx context.Context, input RunAgentInput) []ParsedEvent {
	t.Helper()
	body, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("marshal input: %v", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, serverURL+"/v1/agents/run", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("execute request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("unexpected status %d: %s", resp.StatusCode, string(respBody))
	}

	return parseSSEStream(t, resp.Body)
}

// doRunPartial sends a run request, collects events until count reached or predicate met, then cancels.
// Returns collected events and the last event ID.
func doRunPartial(t *testing.T, parentCtx context.Context, input RunAgentInput, stopAfter int) ([]ParsedEvent, string) {
	t.Helper()
	ctx, cancel := context.WithCancel(parentCtx)
	defer cancel()

	body, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("marshal input: %v", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, serverURL+"/v1/agents/run", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("execute request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("unexpected status %d: %s", resp.StatusCode, string(respBody))
	}

	var events []ParsedEvent
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	var currentID, currentEvent string
	var dataLines []string

	for scanner.Scan() {
		line := scanner.Text()

		if line == "" {
			if len(dataLines) > 0 && currentEvent != "" {
				data := strings.Join(dataLines, "\n")
				events = append(events, ParsedEvent{
					ID:   currentID,
					Type: currentEvent,
					Raw:  json.RawMessage(data),
				})
				if len(events) >= stopAfter {
					cancel()
					return events, currentID
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

	return events, currentID
}

// doReconnect reconnects to an existing run with a last event ID.
func doReconnect(t *testing.T, ctx context.Context, input RunAgentInput, lastEventID string) []ParsedEvent {
	t.Helper()
	body, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("marshal input: %v", err)
	}

	url := fmt.Sprintf("%s/v1/agents/run?last_event_id=%s", serverURL, lastEventID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Last-Event-ID", lastEventID)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("reconnect request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("reconnect: unexpected status %d: %s", resp.StatusCode, string(respBody))
	}

	return parseSSEStream(t, resp.Body)
}

// parseSSEStream parses an SSE stream into ParsedEvents.
func parseSSEStream(t *testing.T, r io.Reader) []ParsedEvent {
	t.Helper()
	var events []ParsedEvent
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	var currentID, currentEvent string
	var dataLines []string

	for scanner.Scan() {
		line := scanner.Text()

		if line == "" {
			if len(dataLines) > 0 && currentEvent != "" {
				data := strings.Join(dataLines, "\n")
				events = append(events, ParsedEvent{
					ID:   currentID,
					Type: currentEvent,
					Raw:  json.RawMessage(data),
				})
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

	// Flush last event if stream ended without trailing blank line.
	if len(dataLines) > 0 && currentEvent != "" {
		data := strings.Join(dataLines, "\n")
		events = append(events, ParsedEvent{
			ID:   currentID,
			Type: currentEvent,
			Raw:  json.RawMessage(data),
		})
	}

	return events
}

// assertRunLifecycle checks that events contain RUN_STARTED and end with RUN_FINISHED or RUN_ERROR.
func assertRunLifecycle(t *testing.T, events []ParsedEvent) {
	t.Helper()
	if len(events) == 0 {
		t.Fatal("no events received")
	}
	foundStart := false
	for _, e := range events {
		if e.Type == "RUN_STARTED" {
			foundStart = true
			break
		}
	}
	if !foundStart {
		t.Errorf("RUN_STARTED not found in events, got types: %v", eventTypes(events[:min(10, len(events))]))
	}
	last := events[len(events)-1]
	if last.Type != "RUN_FINISHED" && last.Type != "RUN_ERROR" {
		t.Errorf("last event should be RUN_FINISHED or RUN_ERROR, got %s", last.Type)
	}
}

// assertNoRunError checks that no RUN_ERROR event is present.
func assertNoRunError(t *testing.T, events []ParsedEvent) {
	t.Helper()
	for _, e := range events {
		if e.Type == "RUN_ERROR" {
			t.Fatalf("unexpected RUN_ERROR: %s", string(e.Raw))
		}
	}
}

// assertHasEventType checks at least one event of the given type exists.
func assertHasEventType(t *testing.T, events []ParsedEvent, eventType string) ParsedEvent {
	t.Helper()
	for _, e := range events {
		if e.Type == eventType {
			return e
		}
	}
	types := eventTypes(events)
	t.Fatalf("expected event type %q not found in: %v", eventType, types)
	return ParsedEvent{}
}

// assertEventSequenceContains checks that expectedTypes appear in order (not necessarily consecutively).
func assertEventSequenceContains(t *testing.T, events []ParsedEvent, expectedTypes ...string) {
	t.Helper()
	idx := 0
	for _, e := range events {
		if idx < len(expectedTypes) && e.Type == expectedTypes[idx] {
			idx++
		}
	}
	if idx < len(expectedTypes) {
		types := eventTypes(events)
		t.Fatalf("event sequence missing %q (found %d/%d), full types: %v",
			expectedTypes[idx], idx, len(expectedTypes), types)
	}
}

// extractText concatenates all TEXT_MESSAGE_CONTENT delta texts.
func extractText(events []ParsedEvent) string {
	var sb strings.Builder
	for _, e := range events {
		if e.Type == "TEXT_MESSAGE_CONTENT" {
			var content struct {
				Delta string `json:"delta"`
			}
			if json.Unmarshal(e.Raw, &content) == nil {
				sb.WriteString(content.Delta)
			}
		}
	}
	return sb.String()
}

// extractToolCalls returns all TOOL_CALL_START events parsed as ToolCallInfo.
func extractToolCalls(events []ParsedEvent) []ToolCallInfo {
	var calls []ToolCallInfo
	for _, e := range events {
		if e.Type == "TOOL_CALL_START" {
			var info ToolCallInfo
			if json.Unmarshal(e.Raw, &info) == nil {
				calls = append(calls, info)
			}
		}
	}
	return calls
}

// eventTypes returns just the type strings from a slice of events.
func eventTypes(events []ParsedEvent) []string {
	types := make([]string, len(events))
	for i, e := range events {
		types[i] = e.Type
	}
	return types
}

// hasEventType checks if any event has the given type.
func hasEventType(events []ParsedEvent, t string) bool {
	for _, e := range events {
		if e.Type == t {
			return true
		}
	}
	return false
}

// doRunWithAutoAnswer is like doRun but auto-answers any HITL questions.
func doRunWithAutoAnswer(t *testing.T, ctx context.Context, input RunAgentInput) []ParsedEvent {
	t.Helper()
	body, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("marshal input: %v", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, serverURL+"/v1/agents/run", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("execute request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("unexpected status %d: %s", resp.StatusCode, string(respBody))
	}

	var events []ParsedEvent
	var runID string
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	var currentID, currentEvent string
	var dataLines []string
	pendingQuestion := false

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if len(dataLines) > 0 && currentEvent != "" {
				data := strings.Join(dataLines, "\n")
				evt := ParsedEvent{ID: currentID, Type: currentEvent, Raw: json.RawMessage(data)}
				events = append(events, evt)

				if currentEvent == "RUN_STARTED" && runID == "" {
					var rs struct {
						RunID string `json:"runId"`
					}
					json.Unmarshal(evt.Raw, &rs)
					runID = rs.RunID
				}

				if currentEvent == "TOOL_CALL_START" {
					var tc struct {
						ToolCallName string `json:"toolCallName"`
					}
					if json.Unmarshal(evt.Raw, &tc) == nil && tc.ToolCallName == "question_request" {
						pendingQuestion = true
					}
				}

				if pendingQuestion && currentEvent == "TOOL_CALL_ARGS" {
					pendingQuestion = false
					var args struct {
						Delta string `json:"delta"`
					}
					if json.Unmarshal(evt.Raw, &args) == nil {
						var payload struct {
							QuestionID string `json:"question_id"`
						}
						if json.Unmarshal([]byte(args.Delta), &payload) == nil && payload.QuestionID != "" && runID != "" {
							qID := payload.QuestionID
							rID := runID
							go func() {
								time.Sleep(200 * time.Millisecond)
								answerURL := fmt.Sprintf("%s/v1/agents/run/%s/answer", serverURL, rID)
								answerBody := fmt.Sprintf(`{"question_id":"%s","answers":{"0":"确认"}}`, qID)
								http.Post(answerURL, "application/json", strings.NewReader(answerBody))
							}()
						}
					}
				}

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
	return events
}

// defaultTimeout returns a context with a 2-minute timeout for LLM scenarios.
func defaultTimeout(t *testing.T) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 2*time.Minute)
}

// mediaTimeout returns a context with a 3-minute timeout for media generation scenarios.
func mediaTimeout(t *testing.T) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 3*time.Minute)
}

// postJSON sends a JSON POST request and returns the response.
func postJSON(t *testing.T, url string, body interface{}) *http.Response {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	resp, err := http.Post(url, "application/json", bytes.NewReader(data))
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	return resp
}

// getJSON sends a GET request and returns decoded JSON.
func getJSON(t *testing.T, url string, out interface{}) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("GET %s: status %d: %s", url, resp.StatusCode, string(body))
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		t.Fatalf("decode response from %s: %v", url, err)
	}
}

// mediaLongTimeout returns a context with a 5-minute timeout for slow media tasks.
func mediaLongTimeout(t *testing.T) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 5*time.Minute)
}

// ArtifactInfo holds parsed artifact data from STATE_DELTA events.
type ArtifactInfo struct {
	Type string
	URL  string
	Name string
}

// extractArtifacts parses STATE_DELTA events for artifact information.
// STATE_DELTA raw format: {"type":"STATE_DELTA","timestamp":...,"delta":[{json patch ops}]}
func extractArtifacts(events []ParsedEvent) []ArtifactInfo {
	var artifacts []ArtifactInfo
	for _, e := range events {
		if e.Type != "STATE_DELTA" {
			continue
		}
		// Try wrapped format first: {"type":"STATE_DELTA","delta":[...]}
		var wrapper struct {
			Delta json.RawMessage `json:"delta"`
		}
		if json.Unmarshal(e.Raw, &wrapper) == nil && len(wrapper.Delta) > 0 {
			var patches []struct {
				Op    string          `json:"op"`
				Path  string          `json:"path"`
				Value json.RawMessage `json:"value"`
			}
			if json.Unmarshal(wrapper.Delta, &patches) == nil {
				for _, p := range patches {
					if strings.Contains(p.Path, "artifact") {
						var art struct {
							Type string `json:"type"`
							URL  string `json:"url"`
							Name string `json:"name"`
						}
						if json.Unmarshal(p.Value, &art) == nil && art.Type != "" {
							artifacts = append(artifacts, ArtifactInfo{Type: art.Type, URL: art.URL, Name: art.Name})
						}
					}
				}
				continue
			}
		}
		// Fallback: try raw as direct JSON Patch array.
		var patches []struct {
			Op    string          `json:"op"`
			Path  string          `json:"path"`
			Value json.RawMessage `json:"value"`
		}
		if json.Unmarshal(e.Raw, &patches) == nil {
			for _, p := range patches {
				if strings.Contains(p.Path, "artifact") {
					var art struct {
						Type string `json:"type"`
						URL  string `json:"url"`
						Name string `json:"name"`
					}
					if json.Unmarshal(p.Value, &art) == nil && art.Type != "" {
						artifacts = append(artifacts, ArtifactInfo{Type: art.Type, URL: art.URL, Name: art.Name})
					}
				}
			}
		}
	}
	return artifacts
}

// hasToolCall checks if any TOOL_CALL_START event has the given tool name.
func hasToolCall(events []ParsedEvent, toolName string) bool {
	for _, c := range extractToolCalls(events) {
		if c.Name == toolName {
			return true
		}
	}
	return false
}

// doHTTP sends an HTTP request and returns status code + response body.
func doHTTP(t *testing.T, ctx context.Context, method, url string, body interface{}) (int, []byte) {
	t.Helper()
	var reqBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		reqBody = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, respBody
}

// doRunSSE sends a raw body to the run endpoint and returns status + body (for non-SSE error responses).
func doRunSSE(ctx context.Context, url string, body []byte) (int, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, respBody, nil
}
