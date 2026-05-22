package pipeline

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/saker-ai/saker/pkg/artifact"
	"github.com/saker-ai/saker/pkg/runtime/cache"
	"github.com/saker-ai/saker/pkg/tool"
)

func TestExecutorSequentialStepExecution(t *testing.T) {
	var calls []string
	exec := Executor{
		RunTool: func(ctx context.Context, step Step, input []artifact.ArtifactRef) (*tool.ToolResult, error) {
			calls = append(calls, step.Name)
			switch step.Name {
			case "extract":
				return &tool.ToolResult{
					Output: "extracted",
					Artifacts: []artifact.ArtifactRef{
						artifact.NewGeneratedRef("art_extract", artifact.ArtifactKindText),
					},
				}, nil
			case "summarize":
				if len(input) != 1 || input[0].ArtifactID != "art_extract" {
					t.Fatalf("expected summarize step to receive previous artifacts, got %+v", input)
				}
				return &tool.ToolResult{
					Output: "summary complete",
					Artifacts: []artifact.ArtifactRef{
						artifact.NewGeneratedRef("art_summary", artifact.ArtifactKindDocument),
					},
				}, nil
			default:
				return nil, fmt.Errorf("unexpected step %q", step.Name)
			}
		},
	}

	result, err := exec.Execute(context.Background(), Step{
		Batch: &Batch{
			Steps: []Step{
				{Name: "extract", Tool: "extractor"},
				{Name: "summarize", Tool: "summarizer"},
			},
		},
	}, Input{})
	if err != nil {
		t.Fatalf("execute batch: %v", err)
	}

	if fmt.Sprint(calls) != "[extract summarize]" {
		t.Fatalf("expected sequential call order, got %v", calls)
	}
	if result.Output != "summary complete" {
		t.Fatalf("expected final step output, got %+v", result)
	}
	if len(result.Artifacts) != 1 || result.Artifacts[0].ArtifactID != "art_summary" {
		t.Fatalf("expected final artifacts to come from last step, got %+v", result.Artifacts)
	}
}

func TestExecutorFanOutOverArtifactSets(t *testing.T) {
	exec := Executor{
		RunTool: func(ctx context.Context, step Step, input []artifact.ArtifactRef) (*tool.ToolResult, error) {
			if len(input) != 1 {
				t.Fatalf("expected single fan-out artifact, got %+v", input)
			}
			return &tool.ToolResult{
				Output: fmt.Sprintf("caption:%s", input[0].ArtifactID),
				Artifacts: []artifact.ArtifactRef{
					artifact.NewGeneratedRef("caption_"+input[0].ArtifactID, artifact.ArtifactKindText),
				},
			}, nil
		},
	}

	result, err := exec.Execute(context.Background(), Step{
		FanOut: &FanOut{
			Collection: "frames",
			Step:       Step{Name: "caption", Tool: "captioner"},
		},
	}, Input{
		Collections: map[string][]artifact.ArtifactRef{
			"frames": {
				artifact.NewGeneratedRef("f1", artifact.ArtifactKindImage),
				artifact.NewGeneratedRef("f2", artifact.ArtifactKindImage),
			},
		},
	})
	if err != nil {
		t.Fatalf("execute fan-out: %v", err)
	}

	if len(result.Items) != 2 {
		t.Fatalf("expected two fan-out item results, got %+v", result.Items)
	}
	if result.Items[0].Output != "caption:f1" || result.Items[1].Output != "caption:f2" {
		t.Fatalf("expected ordered per-item outputs, got %+v", result.Items)
	}
	if len(result.Artifacts) != 2 || result.Artifacts[0].ArtifactID != "caption_f1" || result.Artifacts[1].ArtifactID != "caption_f2" {
		t.Fatalf("expected fan-out artifacts to remain ordered, got %+v", result.Artifacts)
	}
}

func TestExecutorFanInAggregationOrdering(t *testing.T) {
	exec := Executor{
		RunTool: func(ctx context.Context, step Step, input []artifact.ArtifactRef) (*tool.ToolResult, error) {
			return &tool.ToolResult{
				Output: fmt.Sprintf("caption:%s", input[0].ArtifactID),
			}, nil
		},
	}

	result, err := exec.Execute(context.Background(), Step{
		Batch: &Batch{
			Steps: []Step{
				{
					FanOut: &FanOut{
						Collection: "frames",
						Step:       Step{Name: "caption", Tool: "captioner"},
					},
				},
				{
					FanIn: &FanIn{
						Strategy: "ordered",
						Into:     "joined_captions",
					},
				},
			},
		},
	}, Input{
		Collections: map[string][]artifact.ArtifactRef{
			"frames": {
				artifact.NewGeneratedRef("f1", artifact.ArtifactKindImage),
				artifact.NewGeneratedRef("f2", artifact.ArtifactKindImage),
			},
		},
	})
	if err != nil {
		t.Fatalf("execute fan-in batch: %v", err)
	}

	joined, ok := result.Structured.(map[string]any)
	if !ok {
		t.Fatalf("expected structured fan-in output, got %+v", result.Structured)
	}
	values, ok := joined["joined_captions"].([]string)
	if !ok {
		t.Fatalf("expected ordered caption slice, got %+v", joined)
	}
	if fmt.Sprint(values) != "[caption:f1 caption:f2]" {
		t.Fatalf("expected ordered fan-in aggregation, got %+v", values)
	}
}

func TestExecutorRetryingFailedStepOnly(t *testing.T) {
	attempts := 0
	var calls []string
	exec := Executor{
		RunTool: func(ctx context.Context, step Step, input []artifact.ArtifactRef) (*tool.ToolResult, error) {
			calls = append(calls, step.Name)
			if step.Name == "stable" {
				return &tool.ToolResult{Output: "ok"}, nil
			}
			attempts++
			if attempts < 3 {
				return nil, errors.New("temporary failure")
			}
			return &tool.ToolResult{Output: "recovered"}, nil
		},
	}

	result, err := exec.Execute(context.Background(), Step{
		Batch: &Batch{
			Steps: []Step{
				{Name: "stable", Tool: "noop"},
				{
					Retry: &Retry{
						Attempts: 3,
						Step:     Step{Name: "unstable", Tool: "flaky"},
					},
				},
			},
		},
	}, Input{})
	if err != nil {
		t.Fatalf("execute retry batch: %v", err)
	}

	if attempts != 3 {
		t.Fatalf("expected retry wrapper to retry failed step only, got %d attempts", attempts)
	}
	if fmt.Sprint(calls) != "[stable unstable unstable unstable]" {
		t.Fatalf("expected stable step once and flaky step retried, got %v", calls)
	}
	if result.Output != "recovered" {
		t.Fatalf("expected retry result to surface final success, got %+v", result)
	}
}

func TestExecutorCacheHitSkipsExpensiveStep(t *testing.T) {
	calls := 0
	exec := Executor{
		Cache: cache.NewMemoryStore(),
		RunTool: func(ctx context.Context, step Step, input []artifact.ArtifactRef) (*tool.ToolResult, error) {
			calls++
			return &tool.ToolResult{Output: "generated"}, nil
		},
	}
	step := Step{
		Name: "caption",
		Tool: "captioner",
		With: map[string]any{"prompt": "describe"},
		Input: []artifact.ArtifactRef{
			artifact.NewGeneratedRef("art_1", artifact.ArtifactKindImage),
		},
	}

	first, err := exec.Execute(context.Background(), step, Input{})
	if err != nil {
		t.Fatalf("first execution: %v", err)
	}
	second, err := exec.Execute(context.Background(), step, Input{})
	if err != nil {
		t.Fatalf("second execution: %v", err)
	}

	if calls != 1 {
		t.Fatalf("expected cached second execution to skip tool call, got %d calls", calls)
	}
	if first.Output != second.Output {
		t.Fatalf("expected cached result to match original, got %+v and %+v", first, second)
	}
}

func TestExecutorCacheMissWhenInputsChange(t *testing.T) {
	calls := 0
	exec := Executor{
		Cache: cache.NewMemoryStore(),
		RunTool: func(ctx context.Context, step Step, input []artifact.ArtifactRef) (*tool.ToolResult, error) {
			calls++
			return &tool.ToolResult{Output: fmt.Sprintf("call-%d", calls)}, nil
		},
	}

	_, err := exec.Execute(context.Background(), Step{
		Name: "caption",
		Tool: "captioner",
		With: map[string]any{"prompt": "first"},
		Input: []artifact.ArtifactRef{
			artifact.NewGeneratedRef("art_1", artifact.ArtifactKindImage),
		},
	}, Input{})
	if err != nil {
		t.Fatalf("first execution: %v", err)
	}
	_, err = exec.Execute(context.Background(), Step{
		Name: "caption",
		Tool: "captioner",
		With: map[string]any{"prompt": "second"},
		Input: []artifact.ArtifactRef{
			artifact.NewGeneratedRef("art_1", artifact.ArtifactKindImage),
		},
	}, Input{})
	if err != nil {
		t.Fatalf("second execution: %v", err)
	}

	if calls != 2 {
		t.Fatalf("expected changed params to bypass cache, got %d calls", calls)
	}
}

func TestConditionalStepReturnsNotImplementedError(t *testing.T) {
	exec := Executor{
		RunTool: func(ctx context.Context, step Step, refs []artifact.ArtifactRef) (*tool.ToolResult, error) {
			return &tool.ToolResult{Output: "ok"}, nil
		},
	}

	_, err := exec.Execute(context.Background(), Step{
		Name: "branch",
		Conditional: &Conditional{
			Condition: "has_audio",
			Then:      Step{Name: "process-audio", Tool: "processor"},
		},
	}, Input{})

	if err == nil {
		t.Fatal("expected error for unimplemented conditional, got nil")
	}
	if !strings.Contains(err.Error(), "conditional steps are not yet supported") {
		t.Fatalf("expected 'not yet supported' error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "branch") {
		t.Fatalf("error should include step name 'branch', got: %v", err)
	}
}

func TestFanOutConcurrentExecution(t *testing.T) {
	var mu sync.Mutex
	var concurrent int
	var maxConcurrent int

	exec := Executor{
		RunTool: func(ctx context.Context, step Step, refs []artifact.ArtifactRef) (*tool.ToolResult, error) {
			mu.Lock()
			concurrent++
			if concurrent > maxConcurrent {
				maxConcurrent = concurrent
			}
			mu.Unlock()

			time.Sleep(20 * time.Millisecond)

			mu.Lock()
			concurrent--
			mu.Unlock()

			return &tool.ToolResult{Output: refs[0].ArtifactID}, nil
		},
	}

	const numArtifacts = 12
	const maxConc = 4
	refs := make([]artifact.ArtifactRef, numArtifacts)
	for i := range refs {
		refs[i] = artifact.NewGeneratedRef(fmt.Sprintf("a%d", i), artifact.ArtifactKindImage)
	}

	result, err := exec.Execute(context.Background(), Step{
		FanOut: &FanOut{
			Collection:  "items",
			Concurrency: maxConc,
			Step:        Step{Name: "process", Tool: "worker"},
		},
	}, Input{
		Collections: map[string][]artifact.ArtifactRef{"items": refs},
	})
	if err != nil {
		t.Fatalf("fan-out concurrent: %v", err)
	}

	// Verify result count
	if len(result.Items) != numArtifacts {
		t.Fatalf("expected %d items, got %d", numArtifacts, len(result.Items))
	}

	// Verify result ORDER is preserved (critical for correctness)
	for i, item := range result.Items {
		expected := fmt.Sprintf("a%d", i)
		if item.Output != expected {
			t.Fatalf("item[%d]: expected output %q, got %q — ordering broken", i, expected, item.Output)
		}
	}

	// Verify concurrency was bounded
	if maxConcurrent > maxConc {
		t.Fatalf("max concurrent %d exceeded limit %d", maxConcurrent, maxConc)
	}
	if maxConcurrent < 2 {
		t.Fatalf("max concurrent was %d — concurrency not effective (expected >= 2)", maxConcurrent)
	}
}

func TestFanOutConcurrencyDefaultUnbounded(t *testing.T) {
	var mu sync.Mutex
	var maxConcurrent int
	var concurrent int

	exec := Executor{
		RunTool: func(ctx context.Context, step Step, refs []artifact.ArtifactRef) (*tool.ToolResult, error) {
			mu.Lock()
			concurrent++
			if concurrent > maxConcurrent {
				maxConcurrent = concurrent
			}
			mu.Unlock()

			time.Sleep(20 * time.Millisecond)

			mu.Lock()
			concurrent--
			mu.Unlock()

			return &tool.ToolResult{Output: "ok"}, nil
		},
	}

	refs := make([]artifact.ArtifactRef, 8)
	for i := range refs {
		refs[i] = artifact.NewGeneratedRef(fmt.Sprintf("a%d", i), artifact.ArtifactKindImage)
	}

	_, err := exec.Execute(context.Background(), Step{
		FanOut: &FanOut{
			Collection: "items",
			// Concurrency: 0 → default = len(refs)
			Step: Step{Name: "work", Tool: "worker"},
		},
	}, Input{
		Collections: map[string][]artifact.ArtifactRef{"items": refs},
	})
	if err != nil {
		t.Fatalf("unbounded fan-out: %v", err)
	}

	// With 8 items and 20ms sleep, all should run concurrently
	if maxConcurrent < 4 {
		t.Fatalf("expected high concurrency with default (unbounded), got max %d", maxConcurrent)
	}
}

func TestFanOutEmptyCollection(t *testing.T) {
	exec := Executor{
		RunTool: func(ctx context.Context, step Step, refs []artifact.ArtifactRef) (*tool.ToolResult, error) {
			t.Fatal("should not be called for empty collection")
			return nil, nil
		},
	}

	result, err := exec.Execute(context.Background(), Step{
		FanOut: &FanOut{
			Collection: "empty",
			Step:       Step{Name: "noop", Tool: "worker"},
		},
	}, Input{
		Collections: map[string][]artifact.ArtifactRef{"empty": {}},
	})
	if err != nil {
		t.Fatalf("empty fan-out: %v", err)
	}
	if len(result.Items) != 0 {
		t.Fatalf("expected zero items for empty collection, got %d", len(result.Items))
	}
}

func TestFanOutConcurrentErrorPropagation(t *testing.T) {
	exec := Executor{
		RunTool: func(ctx context.Context, step Step, refs []artifact.ArtifactRef) (*tool.ToolResult, error) {
			if refs[0].ArtifactID == "a2" {
				return nil, fmt.Errorf("step failed on a2")
			}
			time.Sleep(10 * time.Millisecond)
			return &tool.ToolResult{Output: "ok"}, nil
		},
	}

	refs := make([]artifact.ArtifactRef, 5)
	for i := range refs {
		refs[i] = artifact.NewGeneratedRef(fmt.Sprintf("a%d", i), artifact.ArtifactKindImage)
	}

	_, err := exec.Execute(context.Background(), Step{
		FanOut: &FanOut{
			Collection:  "items",
			Concurrency: 3,
			Step:        Step{Name: "work", Tool: "worker"},
		},
	}, Input{
		Collections: map[string][]artifact.ArtifactRef{"items": refs},
	})
	if err == nil {
		t.Fatal("expected error to propagate from concurrent fan-out")
	}
	if !strings.Contains(err.Error(), "a2") {
		t.Fatalf("expected error about a2, got: %v", err)
	}
}

func TestRetryBackoffTiming(t *testing.T) {
	attempts := 0
	exec := Executor{
		RunTool: func(ctx context.Context, step Step, refs []artifact.ArtifactRef) (*tool.ToolResult, error) {
			attempts++
			return nil, fmt.Errorf("fail #%d", attempts)
		},
	}

	start := time.Now()
	_, err := exec.Execute(context.Background(), Step{
		Retry: &Retry{
			Attempts:  4,
			BackoffMs: 50,
			Step:      Step{Name: "flaky", Tool: "unreliable"},
		},
	}, Input{})
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected error after all retries exhausted")
	}
	if attempts != 4 {
		t.Fatalf("expected 4 attempts, got %d", attempts)
	}

	// Exponential backoff: 50ms*1 + 50ms*2 + 50ms*4 = 350ms
	// Allow generous tolerance for CI environments
	if elapsed < 300*time.Millisecond {
		t.Fatalf("backoff too fast: elapsed %v, expected >= 300ms", elapsed)
	}
	if elapsed > 700*time.Millisecond {
		t.Fatalf("backoff too slow: elapsed %v, expected <= 700ms", elapsed)
	}
}

func TestRetryBackoffRespectsContextCancellation(t *testing.T) {
	attempts := 0
	exec := Executor{
		RunTool: func(ctx context.Context, step Step, refs []artifact.ArtifactRef) (*tool.ToolResult, error) {
			attempts++
			return nil, fmt.Errorf("fail")
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()

	_, err := exec.Execute(ctx, Step{
		Retry: &Retry{
			Attempts:  10,
			BackoffMs: 100, // 100ms*1 for first wait — should exceed 80ms timeout
			Step:      Step{Name: "slow", Tool: "worker"},
		},
	}, Input{})

	if err == nil {
		t.Fatal("expected error from context cancellation")
	}
	// Should have done at most 2 attempts (first immediate, second after ~100ms wait cancelled)
	if attempts > 3 {
		t.Fatalf("expected context to cancel retries early, got %d attempts", attempts)
	}
}

func TestRetryZeroBackoffBehavesAsImmediate(t *testing.T) {
	attempts := 0
	exec := Executor{
		RunTool: func(ctx context.Context, step Step, refs []artifact.ArtifactRef) (*tool.ToolResult, error) {
			attempts++
			if attempts < 3 {
				return nil, fmt.Errorf("fail")
			}
			return &tool.ToolResult{Output: "recovered"}, nil
		},
	}

	start := time.Now()
	result, err := exec.Execute(context.Background(), Step{
		Retry: &Retry{
			Attempts:  5,
			BackoffMs: 0, // zero = no backoff, same as before
			Step:      Step{Name: "fast-retry", Tool: "worker"},
		},
	}, Input{})
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("expected recovery, got: %v", err)
	}
	if result.Output != "recovered" {
		t.Fatalf("expected recovered output, got: %v", result.Output)
	}
	if elapsed > 100*time.Millisecond {
		t.Fatalf("zero backoff should be nearly instant, took %v", elapsed)
	}
}

func TestFanOutStressHighConcurrency(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	exec := Executor{
		RunTool: func(ctx context.Context, step Step, refs []artifact.ArtifactRef) (*tool.ToolResult, error) {
			// Simulate variable latency
			time.Sleep(time.Duration(len(refs[0].ArtifactID)%5) * time.Millisecond)
			return &tool.ToolResult{
				Output: refs[0].ArtifactID,
				Artifacts: []artifact.ArtifactRef{
					artifact.NewGeneratedRef("out_"+refs[0].ArtifactID, artifact.ArtifactKindText),
				},
			}, nil
		},
	}

	const numArtifacts = 500
	const concurrency = 32
	refs := make([]artifact.ArtifactRef, numArtifacts)
	for i := range refs {
		refs[i] = artifact.NewGeneratedRef(fmt.Sprintf("stress_%04d", i), artifact.ArtifactKindImage)
	}

	result, err := exec.Execute(context.Background(), Step{
		FanOut: &FanOut{
			Collection:  "bulk",
			Concurrency: concurrency,
			Step:        Step{Name: "mass-process", Tool: "worker"},
		},
	}, Input{
		Collections: map[string][]artifact.ArtifactRef{"bulk": refs},
	})
	if err != nil {
		t.Fatalf("stress fan-out: %v", err)
	}
	if len(result.Items) != numArtifacts {
		t.Fatalf("expected %d items, got %d", numArtifacts, len(result.Items))
	}
	// Verify ordering
	for i, item := range result.Items {
		expected := fmt.Sprintf("stress_%04d", i)
		if item.Output != expected {
			t.Fatalf("item[%d] ordering broken: expected %q, got %q", i, expected, item.Output)
		}
	}
	// Verify lineage edges collected
	if len(result.Artifacts) != numArtifacts {
		t.Fatalf("expected %d output artifacts, got %d", numArtifacts, len(result.Artifacts))
	}
}

func TestMergeResults(t *testing.T) {
	t.Run("BothEmpty", func(t *testing.T) {
		got := MergeResults(Result{}, Result{})
		if got.Output != "" || got.Summary != "" || len(got.Artifacts) != 0 || len(got.Items) != 0 || len(got.Lineage.Edges) != 0 {
			t.Fatalf("expected zero-value result, got %+v", got)
		}
	})

	t.Run("NextOutputOverridesBase", func(t *testing.T) {
		got := MergeResults(Result{Output: "base"}, Result{Output: "next"})
		if got.Output != "next" {
			t.Fatalf("expected next output to override, got %q", got.Output)
		}
	})

	t.Run("EmptyNextPreservesBaseOutput", func(t *testing.T) {
		got := MergeResults(Result{Output: "base"}, Result{})
		if got.Output != "base" {
			t.Fatalf("expected base output preserved, got %q", got.Output)
		}
	})

	t.Run("NextArtifactsReplaceBase", func(t *testing.T) {
		base := Result{Artifacts: []artifact.ArtifactRef{
			artifact.NewGeneratedRef("a1", artifact.ArtifactKindText),
		}}
		next := Result{Artifacts: []artifact.ArtifactRef{
			artifact.NewGeneratedRef("a2", artifact.ArtifactKindImage),
		}}
		got := MergeResults(base, next)
		if len(got.Artifacts) != 1 || got.Artifacts[0].ArtifactID != "a2" {
			t.Fatalf("expected next artifacts to replace base, got %+v", got.Artifacts)
		}
	})

	t.Run("ArtifactsAreIndependentCopies", func(t *testing.T) {
		next := Result{Artifacts: []artifact.ArtifactRef{
			artifact.NewGeneratedRef("a1", artifact.ArtifactKindText),
		}}
		got := MergeResults(Result{}, next)
		// Mutate next after merge
		next.Artifacts[0] = artifact.NewGeneratedRef("mutated", artifact.ArtifactKindText)
		if got.Artifacts[0].ArtifactID != "a1" {
			t.Fatalf("merge result should be independent of next, got %+v", got.Artifacts)
		}
	})

	t.Run("LineageEdgesAccumulated", func(t *testing.T) {
		base := Result{}
		base.Lineage.AddEdge(
			artifact.NewGeneratedRef("p1", artifact.ArtifactKindText),
			artifact.NewGeneratedRef("c1", artifact.ArtifactKindText),
			"step1",
		)
		next := Result{}
		next.Lineage.AddEdge(
			artifact.NewGeneratedRef("p2", artifact.ArtifactKindText),
			artifact.NewGeneratedRef("c2", artifact.ArtifactKindText),
			"step2",
		)
		got := MergeResults(base, next)
		if len(got.Lineage.Edges) != 2 {
			t.Fatalf("expected 2 lineage edges, got %d", len(got.Lineage.Edges))
		}
		if got.Lineage.Edges[0].Operation != "step1" || got.Lineage.Edges[1].Operation != "step2" {
			t.Fatalf("lineage edges not accumulated correctly: %+v", got.Lineage.Edges)
		}
	})

	t.Run("ItemsReplacedWhenNextHasItems", func(t *testing.T) {
		base := Result{Items: []Result{{Output: "old"}}}
		next := Result{Items: []Result{{Output: "new1"}, {Output: "new2"}}}
		got := MergeResults(base, next)
		if len(got.Items) != 2 || got.Items[0].Output != "new1" || got.Items[1].Output != "new2" {
			t.Fatalf("expected next items to replace base, got %+v", got.Items)
		}
	})
}

func TestCloneInput(t *testing.T) {
	t.Run("NilInput", func(t *testing.T) {
		got := CloneInput(Input{})
		if len(got.Artifacts) != 0 || got.Collections != nil || len(got.Items) != 0 {
			t.Fatalf("expected zero-value clone, got %+v", got)
		}
	})

	t.Run("PopulatedInputDeepCopy", func(t *testing.T) {
		orig := Input{
			Artifacts: []artifact.ArtifactRef{
				artifact.NewGeneratedRef("a1", artifact.ArtifactKindText),
			},
			Collections: map[string][]artifact.ArtifactRef{
				"frames": {artifact.NewGeneratedRef("f1", artifact.ArtifactKindImage)},
			},
			Items: []Result{{Output: "item1"}},
		}
		got := CloneInput(orig)

		// Mutate original
		orig.Artifacts[0] = artifact.NewGeneratedRef("mutated", artifact.ArtifactKindText)
		orig.Collections["frames"][0] = artifact.NewGeneratedRef("mutated", artifact.ArtifactKindImage)
		orig.Items[0].Output = "mutated"

		if got.Artifacts[0].ArtifactID != "a1" {
			t.Fatalf("clone artifacts should be independent, got %+v", got.Artifacts)
		}
		if got.Collections["frames"][0].ArtifactID != "f1" {
			t.Fatalf("clone collections should be independent, got %+v", got.Collections)
		}
		if got.Items[0].Output != "item1" {
			t.Fatalf("clone items should be independent, got %q", got.Items[0].Output)
		}
	})
}

func TestCloneCollections(t *testing.T) {
	t.Run("Nil", func(t *testing.T) {
		if got := CloneCollections(nil); got != nil {
			t.Fatalf("expected nil, got %+v", got)
		}
	})

	t.Run("EmptyMap", func(t *testing.T) {
		if got := CloneCollections(map[string][]artifact.ArtifactRef{}); got != nil {
			t.Fatalf("expected nil for empty map, got %+v", got)
		}
	})

	t.Run("PopulatedMap", func(t *testing.T) {
		orig := map[string][]artifact.ArtifactRef{
			"frames": {
				artifact.NewGeneratedRef("f1", artifact.ArtifactKindImage),
				artifact.NewGeneratedRef("f2", artifact.ArtifactKindImage),
			},
		}
		got := CloneCollections(orig)

		// Mutate original
		orig["frames"][0] = artifact.NewGeneratedRef("mutated", artifact.ArtifactKindImage)

		if got["frames"][0].ArtifactID != "f1" {
			t.Fatalf("clone should be independent, got %+v", got["frames"])
		}
		if len(got["frames"]) != 2 {
			t.Fatalf("expected 2 refs, got %d", len(got["frames"]))
		}
	})
}

func TestInputFromResult(t *testing.T) {
	prev := Input{
		Artifacts: []artifact.ArtifactRef{
			artifact.NewGeneratedRef("old_art", artifact.ArtifactKindText),
		},
		Collections: map[string][]artifact.ArtifactRef{
			"frames": {artifact.NewGeneratedRef("f1", artifact.ArtifactKindImage)},
		},
		Items: []Result{{Output: "old_item"}},
	}
	res := Result{
		Artifacts: []artifact.ArtifactRef{
			artifact.NewGeneratedRef("new_art", artifact.ArtifactKindDocument),
		},
		Items: []Result{{Output: "new_item1"}, {Output: "new_item2"}},
	}

	got := InputFromResult(prev, res)

	// Artifacts from result replace previous
	if len(got.Artifacts) != 1 || got.Artifacts[0].ArtifactID != "new_art" {
		t.Fatalf("expected result artifacts to replace previous, got %+v", got.Artifacts)
	}
	// Items from result replace previous
	if len(got.Items) != 2 || got.Items[0].Output != "new_item1" {
		t.Fatalf("expected result items to replace previous, got %+v", got.Items)
	}
	// Collections from previous preserved
	if got.Collections["frames"][0].ArtifactID != "f1" {
		t.Fatalf("expected previous collections preserved, got %+v", got.Collections)
	}
}

func TestCloneResult(t *testing.T) {
	t.Run("Empty", func(t *testing.T) {
		got := CloneResult(Result{})
		if got.Output != "" || len(got.Artifacts) != 0 || len(got.Items) != 0 || len(got.Lineage.Edges) != 0 {
			t.Fatalf("expected zero-value result, got %+v", got)
		}
	})

	t.Run("PopulatedIndependentCopy", func(t *testing.T) {
		orig := Result{
			Output: "hello",
			Artifacts: []artifact.ArtifactRef{
				artifact.NewGeneratedRef("a1", artifact.ArtifactKindText),
			},
			Items: []Result{{Output: "item1"}},
		}
		orig.Lineage.AddEdge(
			artifact.NewGeneratedRef("p1", artifact.ArtifactKindText),
			artifact.NewGeneratedRef("c1", artifact.ArtifactKindText),
			"op",
		)

		got := CloneResult(orig)

		// Mutate original after clone
		orig.Artifacts[0] = artifact.NewGeneratedRef("mutated", artifact.ArtifactKindText)
		orig.Items[0].Output = "mutated"
		orig.Lineage.Edges[0].Operation = "mutated"

		if got.Artifacts[0].ArtifactID != "a1" {
			t.Fatalf("clone artifacts should be independent, got %+v", got.Artifacts)
		}
		if got.Items[0].Output != "item1" {
			t.Fatalf("clone items should be independent, got %q", got.Items[0].Output)
		}
		if got.Lineage.Edges[0].Operation != "op" {
			t.Fatalf("clone lineage should be independent, got %q", got.Lineage.Edges[0].Operation)
		}
	})
}
