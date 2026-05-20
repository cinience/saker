package subagents

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type stubRunner struct {
	result Result
	err    error
}

func (s stubRunner) RunSubagent(context.Context, RunRequest) (Result, error) {
	return s.result, s.err
}

func TestExecutorSpawnAndWaitCompletesInstance(t *testing.T) {
	profiles := NewManager()
	if err := profiles.Register(Definition{Name: "plan"}, HandlerFunc(func(context.Context, Context, Request) (Result, error) {
		return Result{}, nil
	})); err != nil {
		t.Fatalf("register: %v", err)
	}
	exec := NewExecutor(profiles, NewMemoryStore(), stubRunner{
		result: Result{Output: "done"},
	})

	handle, err := exec.Spawn(WithTaskDispatch(context.Background()), SpawnRequest{
		Target:        "plan",
		Instruction:   "outline this",
		ParentContext: Context{SessionID: "parent"},
	})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	if handle.ID == "" {
		t.Fatal("expected instance id")
	}

	waited, err := exec.Wait(context.Background(), WaitRequest{ID: handle.ID, Timeout: time.Second})
	if err != nil {
		t.Fatalf("wait: %v", err)
	}
	if waited.TimedOut {
		t.Fatal("expected completed wait")
	}
	if waited.Instance.Status != StatusCompleted {
		t.Fatalf("expected completed instance, got %+v", waited.Instance)
	}
	if waited.Instance.Result == nil || waited.Instance.Result.Output != "done" {
		t.Fatalf("expected result output, got %+v", waited.Instance.Result)
	}
}

func TestExecutorDepthLimitEnforced(t *testing.T) {
	profiles := NewManager()
	_ = profiles.Register(Definition{Name: "worker"}, HandlerFunc(func(context.Context, Context, Request) (Result, error) {
		return Result{}, nil
	}))
	exec := NewExecutor(profiles, NewMemoryStore(), stubRunner{result: Result{Output: "ok"}}, WithMaxDepth(2))

	// Depth 1 should succeed (limit is 2, so depth < maxDepth).
	_, err := exec.Spawn(WithTaskDispatch(context.Background()), SpawnRequest{
		Target:        "worker",
		Instruction:   "task",
		ParentContext: Context{SessionID: "s1", Depth: 1},
	})
	if err != nil {
		t.Fatalf("depth=1 should succeed, got: %v", err)
	}

	// Depth 2 should be rejected (depth >= maxDepth).
	_, err = exec.Spawn(WithTaskDispatch(context.Background()), SpawnRequest{
		Target:        "worker",
		Instruction:   "task",
		ParentContext: Context{SessionID: "s2", Depth: 2},
	})
	if err != ErrDepthLimitReached {
		t.Fatalf("depth=2 should hit limit, got: %v", err)
	}
}

func TestExecutorSemaphoreLimits(t *testing.T) {
	profiles := NewManager()
	_ = profiles.Register(Definition{Name: "slow"}, HandlerFunc(func(context.Context, Context, Request) (Result, error) {
		return Result{}, nil
	}))

	// A runner that blocks until released.
	var running atomic.Int32
	var wg sync.WaitGroup
	release := make(chan struct{})
	blockingRunner := blockingStubRunner{
		release: release,
		running: &running,
	}

	exec := NewExecutor(profiles, NewMemoryStore(), blockingRunner, WithMaxConcurrency(2))

	// Spawn 2 — both should succeed and start running.
	wg.Add(2)
	for i := 0; i < 2; i++ {
		go func() {
			defer wg.Done()
			_, _ = exec.Spawn(WithTaskDispatch(context.Background()), SpawnRequest{
				Target:        "slow",
				Instruction:   "work",
				ParentContext: Context{SessionID: "p"},
			})
		}()
	}
	wg.Wait()

	// Wait for both to start running.
	deadline := time.After(2 * time.Second)
	for running.Load() < 2 {
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for 2 runners, got %d", running.Load())
		default:
			time.Sleep(time.Millisecond)
		}
	}

	// Third spawn should fail with concurrency limit.
	_, err := exec.Spawn(WithTaskDispatch(context.Background()), SpawnRequest{
		Target:        "slow",
		Instruction:   "work",
		ParentContext: Context{SessionID: "p"},
	})
	if err != ErrConcurrencyLimitReached {
		t.Fatalf("expected concurrency limit error, got: %v", err)
	}

	// Release and verify cleanup.
	close(release)
}

func TestExecutorOnCompleteCallback(t *testing.T) {
	profiles := NewManager()
	_ = profiles.Register(Definition{Name: "worker"}, HandlerFunc(func(context.Context, Context, Request) (Result, error) {
		return Result{}, nil
	}))

	var completed atomic.Int32
	exec := NewExecutor(profiles, NewMemoryStore(), stubRunner{result: Result{Output: "done"}})
	exec.SetOnComplete(func(inst Instance) {
		completed.Add(1)
	})

	handle, err := exec.Spawn(WithTaskDispatch(context.Background()), SpawnRequest{
		Target:        "worker",
		Instruction:   "do it",
		ParentContext: Context{SessionID: "s"},
	})
	if err != nil {
		t.Fatal(err)
	}

	_, _ = exec.Wait(context.Background(), WaitRequest{ID: handle.ID, Timeout: time.Second})
	if completed.Load() != 1 {
		t.Fatalf("expected onComplete to fire once, got %d", completed.Load())
	}
}

type blockingStubRunner struct {
	release <-chan struct{}
	running *atomic.Int32
}

func (b blockingStubRunner) RunSubagent(ctx context.Context, _ RunRequest) (Result, error) {
	b.running.Add(1)
	select {
	case <-b.release:
	case <-ctx.Done():
	}
	b.running.Add(-1)
	return Result{Output: "done"}, nil
}

func TestExecutorSendInput(t *testing.T) {
	profiles := NewManager()
	_ = profiles.Register(Definition{Name: "worker"}, HandlerFunc(func(context.Context, Context, Request) (Result, error) {
		return Result{}, nil
	}))

	var running atomic.Int32
	release := make(chan struct{})
	exec := NewExecutor(profiles, NewMemoryStore(), blockingStubRunner{release: release, running: &running})

	handle, err := exec.Spawn(WithTaskDispatch(context.Background()), SpawnRequest{
		Target:        "worker",
		Instruction:   "work",
		ParentContext: Context{SessionID: "s"},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Wait for the runner to start.
	deadline := time.After(2 * time.Second)
	for running.Load() < 1 {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for runner to start")
		default:
			time.Sleep(time.Millisecond)
		}
	}

	// SendInput should succeed while running.
	if err := exec.SendInput(handle.ID, "hello"); err != nil {
		t.Fatalf("SendInput error: %v", err)
	}

	// Verify message is in the mailbox by reading from the instance.
	inst, _ := exec.Get(context.Background(), handle.ID)
	mbox := inst.Mailbox()
	if mbox == nil {
		t.Fatal("expected mailbox channel")
	}
	select {
	case msg := <-mbox:
		if msg != "hello" {
			t.Errorf("got %q, want %q", msg, "hello")
		}
	default:
		t.Error("expected message in mailbox")
	}

	close(release)
}

func TestExecutorSendInputMailboxOverflow(t *testing.T) {
	profiles := NewManager()
	_ = profiles.Register(Definition{Name: "worker"}, HandlerFunc(func(context.Context, Context, Request) (Result, error) {
		return Result{}, nil
	}))

	var running atomic.Int32
	release := make(chan struct{})
	exec := NewExecutor(profiles, NewMemoryStore(), blockingStubRunner{release: release, running: &running})

	handle, err := exec.Spawn(WithTaskDispatch(context.Background()), SpawnRequest{
		Target:        "worker",
		Instruction:   "work",
		ParentContext: Context{SessionID: "s"},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Wait for start.
	deadline := time.After(2 * time.Second)
	for running.Load() < 1 {
		select {
		case <-deadline:
			t.Fatal("timed out")
		default:
			time.Sleep(time.Millisecond)
		}
	}

	// Fill the mailbox (capacity 10).
	for i := 0; i < 10; i++ {
		if err := exec.SendInput(handle.ID, "msg"); err != nil {
			t.Fatalf("SendInput %d: %v", i, err)
		}
	}

	// 11th should fail with mailbox full.
	err = exec.SendInput(handle.ID, "overflow")
	if err == nil {
		t.Fatal("expected mailbox full error")
	}
	if err != ErrMailboxFull {
		t.Errorf("got %v, want ErrMailboxFull", err)
	}

	close(release)
}
