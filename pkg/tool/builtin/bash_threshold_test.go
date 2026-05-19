package toolbuiltin

import "testing"

func TestBashToolOutputThresholdSetters(t *testing.T) {
	tool := NewBashTool()
	if got := tool.effectiveOutputThresholdBytes(); got != maxBashOutputLen {
		t.Fatalf("expected default threshold %d got %d", maxBashOutputLen, got)
	}

	tool.SetOutputThresholdBytes(123)
	if got := tool.effectiveOutputThresholdBytes(); got != 123 {
		t.Fatalf("expected threshold 123 got %d", got)
	}

	tool.SetOutputThresholdBytes(BashMaxOutputUpperLimit + 1)
	if got := tool.effectiveOutputThresholdBytes(); got != BashMaxOutputUpperLimit {
		t.Fatalf("expected threshold to cap at %d got %d", BashMaxOutputUpperLimit, got)
	}

	tool.SetOutputThresholdBytes(0)
	if got := tool.effectiveOutputThresholdBytes(); got != maxBashOutputLen {
		t.Fatalf("expected default threshold after reset %d got %d", maxBashOutputLen, got)
	}

	var nilTool *BashTool
	nilTool.SetOutputThresholdBytes(1)
	if got := nilTool.effectiveOutputThresholdBytes(); got != maxBashOutputLen {
		t.Fatalf("expected nil receiver to return default threshold %d got %d", maxBashOutputLen, got)
	}
}
