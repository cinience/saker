package agui

import "testing"

func TestEventRing_PushAndLen(t *testing.T) {
	t.Parallel()
	r := newEventRing(4)
	if r.Len() != 0 {
		t.Fatalf("empty ring len = %d, want 0", r.Len())
	}
	r.Push(1, []byte("a"))
	r.Push(2, []byte("b"))
	if r.Len() != 2 {
		t.Fatalf("len = %d, want 2", r.Len())
	}
}

func TestEventRing_SinceBeforeWrap(t *testing.T) {
	t.Parallel()
	r := newEventRing(4)
	r.Push(1, []byte("a"))
	r.Push(2, []byte("b"))
	r.Push(3, []byte("c"))

	entries := r.Since(1)
	if len(entries) != 2 {
		t.Fatalf("since(1) got %d entries, want 2", len(entries))
	}
	if entries[0].seq != 2 {
		t.Errorf("entries[0].seq = %d, want 2", entries[0].seq)
	}
	if entries[1].seq != 3 {
		t.Errorf("entries[1].seq = %d, want 3", entries[1].seq)
	}
}

func TestEventRing_SinceAfterWrap(t *testing.T) {
	t.Parallel()
	r := newEventRing(3)
	r.Push(1, []byte("a"))
	r.Push(2, []byte("b"))
	r.Push(3, []byte("c"))
	r.Push(4, []byte("d")) // wraps, overwrites seq=1

	if r.Len() != 3 {
		t.Fatalf("len = %d, want 3 (full)", r.Len())
	}

	entries := r.Since(2)
	if len(entries) != 2 {
		t.Fatalf("since(2) got %d entries, want 2", len(entries))
	}
	if entries[0].seq != 3 {
		t.Errorf("entries[0].seq = %d, want 3", entries[0].seq)
	}
	if entries[1].seq != 4 {
		t.Errorf("entries[1].seq = %d, want 4", entries[1].seq)
	}
}

func TestEventRing_SinceZeroReturnsAll(t *testing.T) {
	t.Parallel()
	r := newEventRing(4)
	r.Push(1, []byte("x"))
	r.Push(2, []byte("y"))

	entries := r.Since(0)
	if len(entries) != 2 {
		t.Fatalf("since(0) got %d entries, want 2", len(entries))
	}
}

func TestEventRing_EmptyRing(t *testing.T) {
	t.Parallel()
	r := newEventRing(4)
	entries := r.Since(0)
	if entries != nil {
		t.Fatalf("since on empty ring should return nil, got %v", entries)
	}
}

func TestEventRing_DefaultSize(t *testing.T) {
	t.Parallel()
	r := newEventRing(0)
	if r.size != defaultEventRingSize {
		t.Fatalf("size = %d, want default %d", r.size, defaultEventRingSize)
	}
}
