package textutil

import "testing"

func TestTruncateRunes(t *testing.T) {
	if got := TruncateRunes("这是一个很长的文本", 5); got != "这是一个很" {
		t.Fatalf("TruncateRunes() = %q", got)
	}
}

func TestTruncateRunesAfter(t *testing.T) {
	if got := TruncateRunesAfter("这是一个很长的文本", 5, "..."); got != "这是一个很..." {
		t.Fatalf("TruncateRunesAfter() = %q", got)
	}
}

func TestTruncateBytesDoesNotSplitUTF8(t *testing.T) {
	if got := TruncateBytes("这是一个很长的文本", 10); got != "这是一" {
		t.Fatalf("TruncateBytes() = %q", got)
	}
}

func TestTailRunes(t *testing.T) {
	if got := TailRunes("这是一个很长的文本", 4); got != "长的文本" {
		t.Fatalf("TailRunes() = %q", got)
	}
}

func TestTruncateRunesWithin(t *testing.T) {
	if got := TruncateRunesWithin("这是一个很长的文本", 8, "..."); got != "这是一个很..." {
		t.Fatalf("TruncateRunesWithin() = %q", got)
	}
	if got := TruncateRunesWithin("abcdef", 3, "..."); got != "..." {
		t.Fatalf("TruncateRunesWithin() with suffix-only budget = %q", got)
	}
}
