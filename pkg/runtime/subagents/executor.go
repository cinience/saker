package subagents

import (
	"context"
	"fmt"
	"maps"
	"sync/atomic"
	"time"
)

// OnCompleteFunc is called when a subagent finishes execution.
type OnCompleteFunc func(Instance)

type Executor struct {
	profiles       *Manager
	store          Store
	runner         Runner
	now            func() time.Time
	seq            atomic.Uint64
	onComplete     OnCompleteFunc
	sem            chan struct{} // concurrency semaphore; nil means unlimited
	maxDepth       int          // max nesting depth; <=0 means unlimited
}

// NewExecutor creates a subagent executor. Use options to configure limits.
func NewExecutor(profiles *Manager, store Store, runner Runner, opts ...ExecutorOption) *Executor {
	if store == nil {
		store = NewMemoryStore()
	}
	e := &Executor{
		profiles: profiles,
		store:    store,
		runner:   runner,
		now:      time.Now,
	}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// ExecutorOption configures an Executor.
type ExecutorOption func(*Executor)

// WithMaxConcurrency sets the maximum number of concurrent subagent goroutines.
// Values <= 0 disable the limit.
func WithMaxConcurrency(n int) ExecutorOption {
	return func(e *Executor) {
		if n > 0 {
			e.sem = make(chan struct{}, n)
		}
	}
}

// WithMaxDepth sets the maximum nesting depth for subagent spawns.
func WithMaxDepth(n int) ExecutorOption {
	return func(e *Executor) {
		e.maxDepth = n
	}
}

// SetOnComplete registers a callback that fires when any subagent finishes.
func (e *Executor) SetOnComplete(fn OnCompleteFunc) {
	e.onComplete = fn
}

func (e *Executor) Store() Store {
	return e.store
}

// ErrDepthLimitReached is returned when a subagent spawn exceeds the configured max depth.
var ErrDepthLimitReached = fmt.Errorf("subagents: agent depth limit reached")

// ErrConcurrencyLimitReached is returned when all concurrency slots are occupied.
var ErrConcurrencyLimitReached = fmt.Errorf("subagents: concurrency limit reached")

func (e *Executor) Spawn(ctx context.Context, req SpawnRequest) (SpawnHandle, error) {
	if e == nil || e.profiles == nil {
		return SpawnHandle{}, ErrUnknownSubagent
	}
	if e.runner == nil {
		return SpawnHandle{}, ErrExecutorClosed
	}
	if dispatchSource(ctx) != DispatchSourceTaskTool {
		return SpawnHandle{}, ErrDispatchUnauthorized
	}

	// Depth limit enforcement.
	if e.maxDepth > 0 && req.ParentContext.Depth >= e.maxDepth {
		return SpawnHandle{}, ErrDepthLimitReached
	}

	instruction := req.Instruction

	// Fork path: skip profile lookup since fork agents are synthetic
	// and not registered in the Manager's profile list.
	profileName := ForkSubagentType
	if !IsForkTarget(req.Target) {
		target, err := e.profiles.selectTarget(Request{
			Target:      req.Target,
			Instruction: req.Instruction,
			Activation:  req.Activation,
		})
		if err != nil {
			return SpawnHandle{}, err
		}
		profileName = target.definition.Name
	}
	now := e.now()
	id := fmt.Sprintf("subagent-%d", e.seq.Add(1))
	inst := Instance{
		ID:              id,
		Profile:         profileName,
		ParentSessionID: req.ParentContext.SessionID,
		SessionID:       childSessionID(req.ParentContext, id),
		Status:          StatusQueued,
		CreatedAt:       now,
		Metadata:        cloneMetadata(req.Metadata),
		Background:      req.Background,
		mailbox:         make(chan string, 10),
	}
	if err := e.store.Create(inst); err != nil {
		return SpawnHandle{}, err
	}

	// Acquire concurrency semaphore (non-blocking: fail fast if full).
	if e.sem != nil {
		select {
		case e.sem <- struct{}{}:
		default:
			// Mark instance as failed since we cannot run it.
			_ = e.store.Update(id, func(inst *Instance) error {
				inst.Status = StatusFailed
				inst.Error = ErrConcurrencyLimitReached.Error()
				n := e.now()
				inst.FinishedAt = &n
				return nil
			})
			return SpawnHandle{}, ErrConcurrencyLimitReached
		}
	}

	go e.run(ctx, id, RunRequest{
		InstanceID:    id,
		Target:        req.Target,
		Instruction:   instruction,
		Activation:    req.Activation,
		ToolWhitelist: append([]string(nil), req.ToolWhitelist...),
		Metadata:      cloneMetadata(req.Metadata),
		ParentContext: req.ParentContext.Clone(),
	})

	return SpawnHandle{ID: id}, nil
}

func cloneMetadata(meta map[string]any) map[string]any {
	if len(meta) == 0 {
		return nil
	}
	return maps.Clone(meta)
}

func (e *Executor) run(parentCtx context.Context, id string, req RunRequest) {
	// Release concurrency semaphore on exit.
	if e.sem != nil {
		defer func() { <-e.sem }()
	}

	ctx, cancel := context.WithCancel(parentCtx)
	defer cancel()

	now := e.now()
	_ = e.store.Update(id, func(inst *Instance) error {
		inst.Status = StatusRunning
		inst.StartedAt = &now
		inst.cancelFunc = cancel
		return nil
	})

	res, err := e.runner.RunSubagent(WithTaskDispatch(ctx), req)
	finished := e.now()
	_ = e.store.Update(id, func(inst *Instance) error {
		inst.FinishedAt = &finished
		inst.cancelFunc = nil // clear after completion
		if err != nil {
			inst.Status = StatusFailed
			inst.Error = err.Error()
			if len(res.Metadata) == 0 {
				res.Metadata = map[string]any{}
			}
			res.Metadata["subagent_id"] = id
			res.Error = err.Error()
			inst.Result = &res
			return nil
		}
		inst.Status = StatusCompleted
		if len(res.Metadata) == 0 {
			res.Metadata = map[string]any{}
		}
		res.Metadata["subagent_id"] = id
		inst.Result = &res
		return nil
	})

	// Fire completion notification if registered.
	if e.onComplete != nil {
		if inst, ok := e.store.Get(id); ok {
			e.onComplete(inst)
		}
	}
}

// Cancel aborts a running subagent by canceling its context.
func (e *Executor) Cancel(id string) error {
	if e == nil || e.store == nil {
		return ErrUnknownInstance
	}
	return e.store.Update(id, func(inst *Instance) error {
		if inst.cancelFunc != nil {
			inst.cancelFunc()
			inst.cancelFunc = nil
		}
		if inst.Status == StatusRunning || inst.Status == StatusQueued {
			inst.Status = StatusCancelled
			now := e.now()
			inst.FinishedAt = &now
		}
		return nil
	})
}

// SendInput delivers a message to a running subagent's mailbox.
// The message can be consumed by the subagent's agent loop.
func (e *Executor) SendInput(id string, msg string) error {
	if e == nil || e.store == nil {
		return ErrUnknownInstance
	}
	return e.store.Update(id, func(inst *Instance) error {
		if inst.Status != StatusRunning && inst.Status != StatusQueued {
			return fmt.Errorf("subagents: cannot send to %s instance", inst.Status)
		}
		return inst.SendMessage(msg)
	})
}

func (e *Executor) Wait(ctx context.Context, req WaitRequest) (WaitResult, error) {
	if e == nil || e.store == nil {
		return WaitResult{}, ErrUnknownInstance
	}
	timeout := req.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()

	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()

	for {
		inst, ok := e.store.Get(req.ID)
		if !ok {
			return WaitResult{}, ErrUnknownInstance
		}
		switch inst.Status {
		case StatusCompleted, StatusFailed, StatusCancelled:
			return WaitResult{Instance: inst}, nil
		}
		select {
		case <-ctx.Done():
			return WaitResult{}, ctx.Err()
		case <-deadline.C:
			inst, _ := e.store.Get(req.ID)
			return WaitResult{Instance: inst, TimedOut: true}, nil
		case <-ticker.C:
		}
	}
}

func (e *Executor) Get(ctx context.Context, id string) (Instance, error) {
	if err := ctx.Err(); err != nil {
		return Instance{}, err
	}
	inst, ok := e.store.Get(id)
	if !ok {
		return Instance{}, ErrUnknownInstance
	}
	return inst, nil
}
