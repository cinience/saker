package client

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	acpproto "github.com/coder/acp-go-sdk"
	"github.com/saker-ai/saker/pkg/runtime/subagents"
)

func TestEnsureACPFlag(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{"nil args", nil, []string{"--acp=true"}},
		{"empty args", []string{}, []string{"--acp=true"}},
		{"no acp flag", []string{"--mode=code"}, []string{"--mode=code", "--acp=true"}},
		{"has acp flag", []string{"--acp"}, []string{"--acp"}},
		{"has acp=true", []string{"--acp=true"}, []string{"--acp=true"}},
		{"has acp=false", []string{"--acp=false"}, []string{"--acp=false"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ensureACPFlag(tt.args)
			if len(got) != len(tt.want) {
				t.Fatalf("ensureACPFlag(%v) = %v, want %v", tt.args, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("ensureACPFlag(%v)[%d] = %q, want %q", tt.args, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestEnsureACPFlag_DoesNotMutateInput(t *testing.T) {
	original := []string{"--verbose"}
	originalCopy := make([]string, len(original))
	copy(originalCopy, original)

	_ = ensureACPFlag(original)

	for i, v := range original {
		if v != originalCopy[i] {
			t.Fatalf("ensureACPFlag mutated input: original[%d] = %q, want %q", i, v, originalCopy[i])
		}
	}
}

func TestClientHandler_SessionUpdate(t *testing.T) {
	h := newClientHandler()

	if len(h.updatesSnapshot()) != 0 {
		t.Fatal("expected empty updates initially")
	}

	// The handler should collect updates without error.
	// We can't easily construct full SessionNotification without the SDK's
	// internal types, but we can test the basic flow.
	updates := h.updatesSnapshot()
	if len(updates) != 0 {
		t.Fatalf("expected 0 updates, got %d", len(updates))
	}

	h.clearUpdates()
	if len(h.updatesSnapshot()) != 0 {
		t.Fatal("expected empty updates after clear")
	}
}

func TestDialFailsWithEmptyCommand(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	_, err := Dial(ctx, DialOptions{})
	if err == nil {
		t.Fatal("expected error for empty command")
	}
}

func TestDialFailsWithBadCommand(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	_, err := Dial(ctx, DialOptions{Command: "/nonexistent/binary/path"})
	if err == nil {
		t.Fatal("expected error for nonexistent binary")
	}
}

// --- ACPRunner tests ---

type mockRunner struct {
	called bool
	result subagents.Result
	err    error
}

func (m *mockRunner) RunSubagent(_ context.Context, req subagents.RunRequest) (subagents.Result, error) {
	m.called = true
	m.result.Subagent = req.Target
	return m.result, m.err
}

func TestACPRunner_FallbackForUnknownTarget(t *testing.T) {
	fallback := &mockRunner{result: subagents.Result{Output: "fallback-output"}}
	runner := NewACPRunner(ACPRunnerConfig{
		Agents: map[string]ACPAgentConfig{
			"claude": {Command: "claude"},
		},
	}, fallback)

	ctx := context.Background()
	result, err := runner.RunSubagent(ctx, subagents.RunRequest{
		Target:      "some-internal-agent",
		Instruction: "do something",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !fallback.called {
		t.Fatal("expected fallback runner to be called")
	}
	if result.Output != "fallback-output" {
		t.Fatalf("expected fallback output, got %q", result.Output)
	}
}

func TestACPRunner_NoFallbackReturnsError(t *testing.T) {
	runner := NewACPRunner(ACPRunnerConfig{
		Agents: map[string]ACPAgentConfig{
			"claude": {Command: "claude"},
		},
	}, nil)

	ctx := context.Background()
	_, err := runner.RunSubagent(ctx, subagents.RunRequest{
		Target:      "unknown-target",
		Instruction: "do something",
	})
	if err == nil {
		t.Fatal("expected error for unknown target with nil fallback")
	}
}

func TestACPRunner_EmptyConfigReturnsFallback(t *testing.T) {
	fallback := &mockRunner{result: subagents.Result{Output: "direct"}}
	runner := NewACPRunner(ACPRunnerConfig{}, fallback)

	// When config is empty, NewACPRunner returns the fallback directly.
	ctx := context.Background()
	result, err := runner.RunSubagent(ctx, subagents.RunRequest{
		Target:      "anything",
		Instruction: "test",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Output != "direct" {
		t.Fatalf("expected direct fallback output, got %q", result.Output)
	}
}

func TestACPRunner_ACPTargetFailsWithBadCommand(t *testing.T) {
	runner := NewACPRunner(ACPRunnerConfig{
		Agents: map[string]ACPAgentConfig{
			"bad-agent": {Command: "/nonexistent/agent/binary"},
		},
	}, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	result, err := runner.RunSubagent(ctx, subagents.RunRequest{
		Target:      "bad-agent",
		Instruction: "hello",
	})
	if err == nil {
		t.Fatal("expected error for bad command")
	}
	if result.Subagent != "bad-agent" {
		t.Fatalf("expected subagent=%q, got %q", "bad-agent", result.Subagent)
	}
	if result.Metadata["runtime"] != "acp" {
		t.Fatal("expected runtime=acp in metadata")
	}
}

// ---------- extractTextFromUpdates ----------

func TestExtractTextFromUpdates(t *testing.T) {
	sid := acpproto.SessionId("sess-1")

	tests := []struct {
		name    string
		updates []acpproto.SessionNotification
		sid     acpproto.SessionId
		want    string
	}{
		{
			name:    "empty slice",
			updates: nil,
			sid:     sid,
			want:    "",
		},
		{
			name: "single matching notification with text",
			updates: []acpproto.SessionNotification{
				{
					SessionId: sid,
					Update: acpproto.SessionUpdate{
						AgentMessageChunk: &acpproto.SessionUpdateAgentMessageChunk{
							Content: acpproto.ContentBlock{
								Text: &acpproto.ContentBlockText{Text: "hello"},
							},
						},
					},
				},
			},
			sid:  sid,
			want: "hello",
		},
		{
			name: "multiple matching notifications concatenated",
			updates: []acpproto.SessionNotification{
				{
					SessionId: sid,
					Update: acpproto.SessionUpdate{
						AgentMessageChunk: &acpproto.SessionUpdateAgentMessageChunk{
							Content: acpproto.ContentBlock{
								Text: &acpproto.ContentBlockText{Text: "foo"},
							},
						},
					},
				},
				{
					SessionId: sid,
					Update: acpproto.SessionUpdate{
						AgentMessageChunk: &acpproto.SessionUpdateAgentMessageChunk{
							Content: acpproto.ContentBlock{
								Text: &acpproto.ContentBlockText{Text: "bar"},
							},
						},
					},
				},
			},
			sid:  sid,
			want: "foobar",
		},
		{
			name: "non-matching sessionID",
			updates: []acpproto.SessionNotification{
				{
					SessionId: acpproto.SessionId("other"),
					Update: acpproto.SessionUpdate{
						AgentMessageChunk: &acpproto.SessionUpdateAgentMessageChunk{
							Content: acpproto.ContentBlock{
								Text: &acpproto.ContentBlockText{Text: "skip"},
							},
						},
					},
				},
			},
			sid:  sid,
			want: "",
		},
		{
			name: "nil AgentMessageChunk field",
			updates: []acpproto.SessionNotification{
				{
					SessionId: sid,
					Update:    acpproto.SessionUpdate{},
				},
			},
			sid:  sid,
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractTextFromUpdates(tt.updates, tt.sid)
			if got != tt.want {
				t.Fatalf("extractTextFromUpdates() = %q, want %q", got, tt.want)
			}
		})
	}
}

// ---------- clientHandler callback methods ----------

func TestClientHandler_SessionUpdateCollectsAndClears(t *testing.T) {
	h := newClientHandler()

	n1 := acpproto.SessionNotification{SessionId: "s1"}
	n2 := acpproto.SessionNotification{SessionId: "s2"}

	if err := h.SessionUpdate(context.Background(), n1); err != nil {
		t.Fatalf("SessionUpdate(n1): %v", err)
	}
	if err := h.SessionUpdate(context.Background(), n2); err != nil {
		t.Fatalf("SessionUpdate(n2): %v", err)
	}

	snap := h.updatesSnapshot()
	if len(snap) != 2 {
		t.Fatalf("expected 2 updates, got %d", len(snap))
	}
	if snap[0].SessionId != "s1" || snap[1].SessionId != "s2" {
		t.Fatalf("unexpected order: %v", snap)
	}

	h.clearUpdates()
	if len(h.updatesSnapshot()) != 0 {
		t.Fatal("expected empty updates after clear")
	}
}

func TestClientHandler_RequestPermission_WithOptions(t *testing.T) {
	h := newClientHandler()
	resp, err := h.RequestPermission(context.Background(), acpproto.RequestPermissionRequest{
		Options: []acpproto.PermissionOption{
			{OptionId: "opt1"},
			{OptionId: "opt2"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Outcome.Selected == nil {
		t.Fatal("expected Selected outcome")
	}
	if resp.Outcome.Selected.OptionId != "opt1" {
		t.Fatalf("expected first option opt1, got %q", resp.Outcome.Selected.OptionId)
	}
}

func TestClientHandler_RequestPermission_WithoutOptions(t *testing.T) {
	h := newClientHandler()
	resp, err := h.RequestPermission(context.Background(), acpproto.RequestPermissionRequest{
		Options: []acpproto.PermissionOption{},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Outcome.Cancelled == nil {
		t.Fatal("expected Cancelled outcome when no options")
	}
}

func TestClientHandler_UnsupportedMethods(t *testing.T) {
	h := newClientHandler()
	ctx := context.Background()

	tests := []struct {
		name string
		call func() error
	}{
		{"ReadTextFile", func() error {
			_, err := h.ReadTextFile(ctx, acpproto.ReadTextFileRequest{})
			return err
		}},
		{"WriteTextFile", func() error {
			_, err := h.WriteTextFile(ctx, acpproto.WriteTextFileRequest{})
			return err
		}},
		{"CreateTerminal", func() error {
			_, err := h.CreateTerminal(ctx, acpproto.CreateTerminalRequest{})
			return err
		}},
		{"KillTerminalCommand", func() error {
			_, err := h.KillTerminalCommand(ctx, acpproto.KillTerminalCommandRequest{})
			return err
		}},
		{"TerminalOutput", func() error {
			_, err := h.TerminalOutput(ctx, acpproto.TerminalOutputRequest{})
			return err
		}},
		{"ReleaseTerminal", func() error {
			_, err := h.ReleaseTerminal(ctx, acpproto.ReleaseTerminalRequest{})
			return err
		}},
		{"WaitForTerminalExit", func() error {
			_, err := h.WaitForTerminalExit(ctx, acpproto.WaitForTerminalExitRequest{})
			return err
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.call()
			if err == nil {
				t.Fatal("expected error for unsupported method")
			}
			if !strings.Contains(err.Error(), "not supported") {
				t.Fatalf("error should contain 'not supported', got: %v", err)
			}
		})
	}
}

// ---------- Run guard clauses ----------

func TestRun_ClosedClient(t *testing.T) {
	c := &ACPClient{
		closed: true,
		done:   make(chan struct{}),
	}
	_, err := c.Run(context.Background(), RunOptions{Task: "test"})
	if !errors.Is(err, ErrClientClosed) {
		t.Fatalf("expected ErrClientClosed, got %v", err)
	}
}

func TestRun_ProcessExited(t *testing.T) {
	done := make(chan struct{})
	close(done)
	c := &ACPClient{
		done: done,
	}
	_, err := c.Run(context.Background(), RunOptions{Task: "test"})
	if !errors.Is(err, ErrProcessExited) {
		t.Fatalf("expected ErrProcessExited, got %v", err)
	}
}

// ---------- Done / Close ----------

func TestDone_ReturnsChannel(t *testing.T) {
	done := make(chan struct{})
	c := &ACPClient{done: done}
	ch := c.Done()
	if ch == nil {
		t.Fatal("Done() returned nil channel")
	}
	// Channel should not be closed yet.
	select {
	case <-ch:
		t.Fatal("channel should not be closed")
	default:
	}
}

func TestClose_Idempotent(t *testing.T) {
	// Already-closed client: Close returns nil and does not panic.
	done := make(chan struct{})
	close(done)
	c := &ACPClient{
		closed: false,
		done:   done,
		stdin:  nopWriteCloser{},
	}

	if err := c.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("second Close should return nil, got: %v", err)
	}
}

// nopWriteCloser satisfies io.WriteCloser for Close tests without a real process.
type nopWriteCloser struct{}

func (nopWriteCloser) Write(p []byte) (int, error) { return len(p), nil }
func (nopWriteCloser) Close() error                 { return nil }
