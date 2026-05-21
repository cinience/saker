package agui

import (
	"bufio"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/saker-ai/saker/pkg/api"
	"github.com/saker-ai/saker/pkg/config"
)

// recordingRunner captures the api.Request passed to RunStream and immediately
// closes the event channel so the session pump finishes with RUN_STARTED + RUN_FINISHED.
type recordingRunner struct {
	mu       sync.Mutex
	captured api.Request
}

func (r *recordingRunner) RunStream(_ context.Context, req api.Request) (<-chan api.StreamEvent, error) {
	r.mu.Lock()
	r.captured = req
	r.mu.Unlock()
	ch := make(chan api.StreamEvent)
	close(ch)
	return ch, nil
}

func (r *recordingRunner) request() api.Request {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.captured
}

// --- JSON test case schema ---

type testCaseExpectRequest struct {
	ModelEndpoint       []config.FailoverModelEntry `json:"model_endpoint"`
	ModelOverrides      *testModelOverrides         `json:"model_overrides"`
	SystemPromptOvr     *testSystemPromptOverride   `json:"system_prompt_override"`
	ToolWhitelist       []string                    `json:"tool_whitelist"`
	PassthroughTools    []string                    `json:"passthrough_tools"`
}

type testModelOverrides struct {
	Temperature *float64 `json:"temperature,omitempty"`
	TopP        *float64 `json:"top_p,omitempty"`
	MaxTokens   *int     `json:"max_tokens,omitempty"`
}

type testSystemPromptOverride struct {
	Text string `json:"text"`
	Mode string `json:"mode"`
}

type testCaseExpect struct {
	HTTPStatus      int                    `json:"http_status"`
	Request         *testCaseExpectRequest `json:"request,omitempty"`
	ErrorType       string                 `json:"error_type,omitempty"`
	ErrorContains   string                 `json:"error_contains,omitempty"`
	EventsContain   []string               `json:"events_contain,omitempty"`
	EventsNotContain []string              `json:"events_not_contain,omitempty"`
}

type testCaseMCPServer struct {
	Name    string   `json:"name"`
	Type    string   `json:"type"`
	URL     string   `json:"url,omitempty"`
	Command string   `json:"command,omitempty"`
	Args    []string `json:"args,omitempty"`
	Timeout float64  `json:"timeout,omitempty"`
}

type testCaseMCPExpect struct {
	Servers []testCaseMCPServer `json:"servers"`
}

type testCaseOptions struct {
	DenyModelEndpoint       bool `json:"deny_model_endpoint,omitempty"`
	DenySystemPromptReplace bool `json:"deny_system_prompt_replace,omitempty"`
	DenyToolOverride        bool `json:"deny_tool_override,omitempty"`
	MaxMCPServersPerSession int  `json:"max_mcp_servers_per_session,omitempty"`
	AllowMCPStdio           bool `json:"allow_mcp_stdio,omitempty"`
}

func (o testCaseOptions) toOptions() Options {
	return Options{
		DenyModelEndpoint:       o.DenyModelEndpoint,
		DenySystemPromptReplace: o.DenySystemPromptReplace,
		DenyToolOverride:        o.DenyToolOverride,
		MaxMCPServersPerSession: o.MaxMCPServersPerSession,
		AllowMCPStdio:           o.AllowMCPStdio,
	}
}

type forwardedPropsTestCase struct {
	Name           string         `json:"name"`
	Description    string         `json:"description"`
	Input          json.RawMessage `json:"input"`
	Options        testCaseOptions `json:"options"`
	Expect         *testCaseExpect `json:"expect,omitempty"`
	ExpectMCPParse *testCaseMCPExpect `json:"expect_mcp_parse,omitempty"`
}

func loadForwardedPropsTestCases(t *testing.T) []forwardedPropsTestCase {
	t.Helper()
	dir := filepath.Join("testdata", "forwarded_props")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read testdata dir: %v", err)
	}

	var cases []forwardedPropsTestCase
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", entry.Name(), err)
		}
		var tc forwardedPropsTestCase
		if err := json.Unmarshal(data, &tc); err != nil {
			t.Fatalf("unmarshal %s: %v", entry.Name(), err)
		}
		cases = append(cases, tc)
	}
	return cases
}

// setupGatewayWithRunner creates a test gateway with a custom Runner.
func setupGatewayWithRunner(t *testing.T, runner Runner, opts Options) (*Gateway, *gin.Engine) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	opts.Enabled = true
	opts.DevBypassAuth = true
	deps := Deps{
		Runtime: runner,
		Logger:  slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
		Options: opts,
	}
	gw, err := RegisterAGUIGateway(engine, deps)
	if err != nil {
		t.Fatalf("RegisterAGUIGateway: %v", err)
	}
	return gw, engine
}

func parseSSEEvents(body string) []string {
	var events []string
	scanner := bufio.NewScanner(strings.NewReader(body))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "event:") {
			eventType := strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			events = append(events, eventType)
		}
	}
	return events
}

// --- HTTP-level integration tests ---

func TestForwardedProps_Integration(t *testing.T) {
	t.Parallel()
	cases := loadForwardedPropsTestCases(t)

	for _, tc := range cases {
		if tc.Expect == nil {
			continue // MCP-only cases handled by TestForwardedProps_MCPParsing
		}
		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()
			runner := &recordingRunner{}
			opts := tc.Options.toOptions()
			_, engine := setupGatewayWithRunner(t, runner, opts)

			req := httptest.NewRequest(http.MethodPost, "/v1/agents/run", strings.NewReader(string(tc.Input)))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Accept", "text/event-stream")
			w := httptest.NewRecorder()
			engine.ServeHTTP(w, req)

			// Verify HTTP status.
			if w.Code != tc.Expect.HTTPStatus {
				t.Fatalf("HTTP status = %d, want %d; body: %s", w.Code, tc.Expect.HTTPStatus, w.Body.String())
			}

			if tc.Expect.HTTPStatus != http.StatusOK {
				// Error response — verify error fields.
				var errResp struct {
					Error struct {
						Message string `json:"message"`
						Type    string `json:"type"`
					} `json:"error"`
				}
				if err := json.Unmarshal(w.Body.Bytes(), &errResp); err != nil {
					t.Fatalf("unmarshal error response: %v", err)
				}
				if tc.Expect.ErrorType != "" && errResp.Error.Type != tc.Expect.ErrorType {
					t.Errorf("error.type = %q, want %q", errResp.Error.Type, tc.Expect.ErrorType)
				}
				if tc.Expect.ErrorContains != "" && !strings.Contains(errResp.Error.Message, tc.Expect.ErrorContains) {
					t.Errorf("error.message = %q, want contains %q", errResp.Error.Message, tc.Expect.ErrorContains)
				}
				return
			}

			// Success response — verify captured request fields.
			captured := runner.request()
			assertModelEndpoint(t, captured.ModelEndpoint, tc.Expect.Request.ModelEndpoint)
			assertModelOverrides(t, captured.ModelOverrides, tc.Expect.Request.ModelOverrides)
			assertSystemPromptOverride(t, captured.SystemPromptOverride, tc.Expect.Request.SystemPromptOvr)
			assertStringSlice(t, "ToolWhitelist", captured.ToolWhitelist, tc.Expect.Request.ToolWhitelist)
			assertStringSlice(t, "PassthroughTools", captured.PassthroughTools, tc.Expect.Request.PassthroughTools)

			// Verify SSE event stream.
			events := parseSSEEvents(w.Body.String())
			for _, want := range tc.Expect.EventsContain {
				if !containsEvent(events, want) {
					t.Errorf("SSE events missing %q; got: %v", want, events)
				}
			}
			for _, unwanted := range tc.Expect.EventsNotContain {
				if containsEvent(events, unwanted) {
					t.Errorf("SSE events unexpectedly contain %q; got: %v", unwanted, events)
				}
			}
		})
	}
}

// --- MCP parsing tests (no HTTP, direct function call) ---

func TestForwardedProps_MCPParsing(t *testing.T) {
	t.Parallel()
	cases := loadForwardedPropsTestCases(t)

	for _, tc := range cases {
		if tc.ExpectMCPParse == nil {
			continue
		}
		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()

			var input struct {
				ForwardedProps map[string]any `json:"forwardedProps"`
			}
			if err := json.Unmarshal(tc.Input, &input); err != nil {
				t.Fatalf("unmarshal input: %v", err)
			}

			servers, err := extractMCPServers(input.ForwardedProps)
			if err != nil {
				t.Fatalf("extractMCPServers: %v", err)
			}

			if len(servers) != len(tc.ExpectMCPParse.Servers) {
				t.Fatalf("got %d servers, want %d", len(servers), len(tc.ExpectMCPParse.Servers))
			}

			for i, want := range tc.ExpectMCPParse.Servers {
				got := servers[i]
				if got.Name != want.Name {
					t.Errorf("server[%d].Name = %q, want %q", i, got.Name, want.Name)
				}
				if got.Type != want.Type {
					t.Errorf("server[%d].Type = %q, want %q", i, got.Type, want.Type)
				}
				if want.URL != "" && got.URL != want.URL {
					t.Errorf("server[%d].URL = %q, want %q", i, got.URL, want.URL)
				}
				if want.Command != "" && got.Command != want.Command {
					t.Errorf("server[%d].Command = %q, want %q", i, got.Command, want.Command)
				}
				if want.Args != nil {
					if len(got.Args) != len(want.Args) {
						t.Errorf("server[%d].Args = %v, want %v", i, got.Args, want.Args)
					} else {
						for j := range want.Args {
							if got.Args[j] != want.Args[j] {
								t.Errorf("server[%d].Args[%d] = %q, want %q", i, j, got.Args[j], want.Args[j])
							}
						}
					}
				}
				if want.Timeout != 0 && got.Timeout != want.Timeout {
					t.Errorf("server[%d].Timeout = %v, want %v", i, got.Timeout, want.Timeout)
				}
			}
		})
	}
}

// --- MCP security tests (HTTP-level, expects error responses) ---

func TestForwardedProps_MCPSecurity(t *testing.T) {
	t.Parallel()
	cases := loadForwardedPropsTestCases(t)

	for _, tc := range cases {
		if tc.Expect == nil || tc.Expect.HTTPStatus == http.StatusOK {
			continue
		}
		// Only run MCP security cases (those with mcp_servers in forwardedProps)
		var input struct {
			ForwardedProps map[string]any `json:"forwardedProps"`
		}
		if err := json.Unmarshal(tc.Input, &input); err != nil {
			continue
		}
		if _, hasMCP := input.ForwardedProps["mcp_servers"]; !hasMCP {
			continue
		}

		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()
			runner := &recordingRunner{}
			opts := tc.Options.toOptions()
			_, engine := setupGatewayWithRunner(t, runner, opts)

			req := httptest.NewRequest(http.MethodPost, "/v1/agents/run", strings.NewReader(string(tc.Input)))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			engine.ServeHTTP(w, req)

			if w.Code != tc.Expect.HTTPStatus {
				t.Fatalf("HTTP status = %d, want %d; body: %s", w.Code, tc.Expect.HTTPStatus, w.Body.String())
			}

			var errResp struct {
				Error struct {
					Message string `json:"message"`
					Type    string `json:"type"`
				} `json:"error"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &errResp); err != nil {
				t.Fatalf("unmarshal error response: %v", err)
			}
			if tc.Expect.ErrorType != "" && errResp.Error.Type != tc.Expect.ErrorType {
				t.Errorf("error.type = %q, want %q", errResp.Error.Type, tc.Expect.ErrorType)
			}
			if tc.Expect.ErrorContains != "" && !strings.Contains(errResp.Error.Message, tc.Expect.ErrorContains) {
				t.Errorf("error.message = %q, want contains %q", errResp.Error.Message, tc.Expect.ErrorContains)
			}
		})
	}
}

// --- Assertion helpers ---

func assertModelEndpoint(t *testing.T, got []config.FailoverModelEntry, want []config.FailoverModelEntry) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("ModelEndpoint len = %d, want %d; got: %+v", len(got), len(want), got)
		return
	}
	for i := range want {
		if got[i].Provider != want[i].Provider {
			t.Errorf("ModelEndpoint[%d].Provider = %q, want %q", i, got[i].Provider, want[i].Provider)
		}
		if got[i].Model != want[i].Model {
			t.Errorf("ModelEndpoint[%d].Model = %q, want %q", i, got[i].Model, want[i].Model)
		}
		if got[i].APIKey != want[i].APIKey {
			t.Errorf("ModelEndpoint[%d].APIKey = %q, want %q", i, got[i].APIKey, want[i].APIKey)
		}
		if got[i].BaseURL != want[i].BaseURL {
			t.Errorf("ModelEndpoint[%d].BaseURL = %q, want %q", i, got[i].BaseURL, want[i].BaseURL)
		}
	}
}

func assertModelOverrides(t *testing.T, got *api.ModelOverrides, want *testModelOverrides) {
	t.Helper()
	if want == nil {
		if got != nil {
			t.Errorf("ModelOverrides = %+v, want nil", got)
		}
		return
	}
	if got == nil {
		t.Error("ModelOverrides = nil, want non-nil")
		return
	}
	if want.Temperature != nil {
		if got.Temperature == nil || *got.Temperature != *want.Temperature {
			t.Errorf("ModelOverrides.Temperature = %v, want %v", got.Temperature, *want.Temperature)
		}
	}
	if want.TopP != nil {
		if got.TopP == nil || *got.TopP != *want.TopP {
			t.Errorf("ModelOverrides.TopP = %v, want %v", got.TopP, *want.TopP)
		}
	}
	if want.MaxTokens != nil {
		if got.MaxTokens == nil || *got.MaxTokens != *want.MaxTokens {
			t.Errorf("ModelOverrides.MaxTokens = %v, want %v", got.MaxTokens, *want.MaxTokens)
		}
	}
}

func assertSystemPromptOverride(t *testing.T, got *api.SystemPromptOverride, want *testSystemPromptOverride) {
	t.Helper()
	if want == nil {
		if got != nil {
			t.Errorf("SystemPromptOverride = %+v, want nil", got)
		}
		return
	}
	if got == nil {
		t.Error("SystemPromptOverride = nil, want non-nil")
		return
	}
	if got.Text != want.Text {
		t.Errorf("SystemPromptOverride.Text = %q, want %q", got.Text, want.Text)
	}
	if string(got.Mode) != want.Mode {
		t.Errorf("SystemPromptOverride.Mode = %q, want %q", got.Mode, want.Mode)
	}
}

func assertStringSlice(t *testing.T, field string, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("%s len = %d, want %d; got: %v", field, len(got), len(want), got)
		return
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("%s[%d] = %q, want %q", field, i, got[i], want[i])
		}
	}
}

func containsEvent(events []string, target string) bool {
	for _, e := range events {
		if e == target {
			return true
		}
	}
	return false
}
