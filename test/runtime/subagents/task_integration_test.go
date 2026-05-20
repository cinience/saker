package subagents_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/saker-ai/saker/pkg/api"
	"github.com/saker-ai/saker/pkg/model"
	"github.com/saker-ai/saker/pkg/runtime/subagents"
)

func TestTaskIntegration_NoTaskTool_NoDispatch(t *testing.T) {
	var calls atomic.Int32
	handler := subagents.HandlerFunc(func(context.Context, subagents.Context, subagents.Request) (subagents.Result, error) {
		calls.Add(1)
		return subagents.Result{Output: "should-not-run"}, nil
	})

	rt := newRuntimeWithSubagent(t, handler, subagents.TypeGeneralPurpose, &scriptedModel{
		responses: []*model.Response{{Message: model.Message{Role: "assistant", Content: "ok"}}},
	})

	_, err := rt.Run(context.Background(), api.Request{
		Prompt:         "regular prompt",
		TargetSubagent: subagents.TypeGeneralPurpose,
	})
	if err != nil {
		t.Fatalf("runtime run failed: %v", err)
	}
	if calls.Load() != 0 {
		t.Fatalf("expected subagent to stay idle without Task tool, got %d calls", calls.Load())
	}
}

func newRuntimeWithSubagent(t *testing.T, handler subagents.Handler, target string, mdl model.Model) *api.Runtime {
	t.Helper()
	def, ok := subagents.BuiltinDefinition(target)
	if !ok {
		t.Fatalf("missing builtin definition for %s", target)
	}
	rt, err := api.New(context.Background(), api.Options{
		ProjectRoot: t.TempDir(),
		EntryPoint:  api.EntryPointCLI,
		Model:       mdl,
		Subagents: []api.SubagentRegistration{{
			Definition: def,
			Handler:    handler,
		}},
	})
	if err != nil {
		t.Fatalf("runtime init: %v", err)
	}
	t.Cleanup(func() { _ = rt.Close() })
	return rt
}

type scriptedModel struct {
	mu        sync.Mutex
	responses []*model.Response
	err       error
}

func (s *scriptedModel) Complete(context.Context, model.Request) (*model.Response, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return nil, s.err
	}
	if len(s.responses) == 0 {
		return nil, errors.New("no scripted responses")
	}
	resp := s.responses[0]
	s.responses = s.responses[1:]
	return resp, nil
}

func (s *scriptedModel) CompleteStream(ctx context.Context, req model.Request, cb model.StreamHandler) error {
	resp, err := s.Complete(ctx, req)
	if err != nil {
		return err
	}
	if cb != nil {
		return cb(model.StreamResult{Final: true, Response: resp})
	}
	return nil
}
