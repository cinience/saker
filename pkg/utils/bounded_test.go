package utils

import "testing"

func TestBoundedLoopStopsAtLimit(t *testing.T) {
	calls := 0
	BoundedLoop(3, func() bool {
		calls++
		return true
	})
	if calls != 3 {
		t.Fatalf("calls = %d, want 3", calls)
	}
}

func TestBoundedLoopStopsWhenFunctionReturnsFalse(t *testing.T) {
	calls := 0
	BoundedLoop(5, func() bool {
		calls++
		return calls < 2
	})
	if calls != 2 {
		t.Fatalf("calls = %d, want 2", calls)
	}
}

func TestBoundedLoopIgnoresInvalidInputs(t *testing.T) {
	BoundedLoop(1, nil)
	calls := 0
	BoundedLoop(0, func() bool {
		calls++
		return true
	})
	if calls != 0 {
		t.Fatalf("calls = %d, want 0", calls)
	}
}
