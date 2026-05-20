package api

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/saker-ai/saker/pkg/config"
	"github.com/saker-ai/saker/pkg/model"
	"github.com/saker-ai/saker/pkg/sandbox"
	"github.com/saker-ai/saker/pkg/security"
	"github.com/saker-ai/saker/pkg/tool"
)

type agentHelperStubTool struct {
	name string
}

func (s agentHelperStubTool) Name() string        { return s.name }
func (s agentHelperStubTool) Description() string { return "stub" }
func (s agentHelperStubTool) Schema() *tool.JSONSchema {
	return &tool.JSONSchema{Type: "object"}
}
func (s agentHelperStubTool) Execute(context.Context, map[string]interface{}) (*tool.ToolResult, error) {
	return &tool.ToolResult{Success: true}, nil
}

func TestStreamEmitContextHelpers(t *testing.T) {
	if streamEmitFromContext(context.TODO()) != nil {
		t.Fatalf("expected nil emit from empty context")
	}
	ctx := withStreamEmit(context.Background(), func(context.Context, StreamEvent) {})
	if streamEmitFromContext(ctx) == nil {
		t.Fatalf("expected emit func from context")
	}
	if got := withStreamEmit(context.TODO(), nil); got == nil {
		t.Fatalf("expected non-nil context")
	}
}

func TestPermissionReasonHelpers(t *testing.T) {
	if got := buildPermissionReason(security.PermissionDecision{}); got != "" {
		t.Fatalf("expected empty reason, got %q", got)
	}
	if got := buildPermissionReason(security.PermissionDecision{Target: "x"}); !strings.Contains(got, "target") {
		t.Fatalf("unexpected reason %q", got)
	}
	if got := buildPermissionReason(security.PermissionDecision{Rule: "r"}); !strings.Contains(got, "rule") {
		t.Fatalf("unexpected reason %q", got)
	}
	if got := buildPermissionReason(security.PermissionDecision{Rule: "r", Target: "t"}); !strings.Contains(got, "for") {
		t.Fatalf("unexpected reason %q", got)
	}
	if cmd := formatApprovalCommand("", ""); cmd != "tool" {
		t.Fatalf("expected default tool name, got %q", cmd)
	}
	if cmd := formatApprovalCommand("bash", "ls"); cmd != "bash(ls)" {
		t.Fatalf("unexpected approval command %q", cmd)
	}
	if actor := approvalActor(" "); actor != "host" {
		t.Fatalf("expected host fallback, got %q", actor)
	}
	if actor := approvalActor("alice"); actor != "alice" {
		t.Fatalf("unexpected actor %q", actor)
	}
}

func TestRegisterToolsDisallowedAndDuplicates(t *testing.T) {
	reg := tool.NewRegistry()
	opts := Options{
		Tools: []tool.Tool{
			agentHelperStubTool{name: "bash"},
			agentHelperStubTool{name: "bash"},
			agentHelperStubTool{name: "read"},
		},
	}
	settings := &config.Settings{DisallowedTools: []string{"bash"}}
	_, err := registerTools(reg, opts, settings, nil, nil)
	if err != nil {
		t.Fatalf("register tools failed: %v", err)
	}
	if _, err := reg.Get("read"); err != nil {
		t.Fatalf("expected Read tool registered: %v", err)
	}
	if _, err := reg.Get("bash"); err == nil {
		t.Fatalf("expected Bash to be disallowed")
	}
}


func TestResolveModelErrors(t *testing.T) {
	if _, err := resolveModel(context.Background(), Options{}); !errors.Is(err, ErrMissingModel) {
		t.Fatalf("expected missing model error, got %v", err)
	}
	factoryErr := errors.New("boom")
	_, err := resolveModel(context.Background(), Options{ModelFactory: modelProviderFunc{err: factoryErr}})
	if err == nil || !strings.Contains(err.Error(), "model factory") {
		t.Fatalf("expected model factory error, got %v", err)
	}
}

type modelProviderFunc struct {
	err error
}

func (m modelProviderFunc) Model(context.Context) (model.Model, error) {
	return nil, m.err
}


func TestFilterBuiltinNamesAndAgentRegistration(t *testing.T) {
	order := []string{"file_read", "bash", "spawn_agent"}
	if got := filterBuiltinNames([]string{"FILE-READ", "bash"}, order); len(got) != 2 {
		t.Fatalf("unexpected filtered names %v", got)
	}
	if got := filterBuiltinNames([]string{}, order); got != nil {
		t.Fatalf("expected nil filtered names, got %v", got)
	}
	if !shouldRegisterAgentTools(EntryPointCLI) {
		t.Fatalf("expected agent tools for CLI")
	}
	if shouldRegisterAgentTools(EntryPointCI) {
		t.Fatalf("expected agent tools disabled for CI")
	}
}

func TestEnforceSandboxHost(t *testing.T) {
	mgr := sandbox.NewManager(nil, sandbox.NewDomainAllowList("allowed.com"), nil)
	if err := enforceSandboxHost(mgr, "https://allowed.com"); err != nil {
		t.Fatalf("expected allowed host, got %v", err)
	}
	if err := enforceSandboxHost(mgr, "https://blocked.com"); err == nil {
		t.Fatalf("expected blocked host error")
	}
}
