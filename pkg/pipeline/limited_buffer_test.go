package pipeline

import (
	"testing"
)

func TestLimitedBuffer(t *testing.T) {
	t.Run("UnderLimit", func(t *testing.T) {
		b := &limitedBuffer{max: 100}
		data := make([]byte, 50)
		for i := range data {
			data[i] = 'a'
		}
		n, err := b.Write(data)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if n != 50 {
			t.Fatalf("expected n=50, got %d", n)
		}
		if len(b.String()) != 50 {
			t.Fatalf("expected 50 bytes captured, got %d", len(b.String()))
		}
	})

	t.Run("ExactLimit", func(t *testing.T) {
		b := &limitedBuffer{max: 100}
		data := make([]byte, 100)
		for i := range data {
			data[i] = 'b'
		}
		n, err := b.Write(data)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if n != 100 {
			t.Fatalf("expected n=100, got %d", n)
		}
		if len(b.String()) != 100 {
			t.Fatalf("expected 100 bytes captured, got %d", len(b.String()))
		}
	})

	t.Run("OverLimit", func(t *testing.T) {
		b := &limitedBuffer{max: 100}
		data := make([]byte, 150)
		for i := range data {
			data[i] = 'c'
		}
		n, err := b.Write(data)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Write truncates p locally, so n reflects the truncated length
		if n != 100 {
			t.Fatalf("expected n=100 (truncated), got %d", n)
		}
		if len(b.String()) != 100 {
			t.Fatalf("expected 100 bytes captured, got %d", len(b.String()))
		}
	})

	t.Run("MultipleWrites", func(t *testing.T) {
		b := &limitedBuffer{max: 100}
		first := make([]byte, 60)
		for i := range first {
			first[i] = 'd'
		}
		second := make([]byte, 60)
		for i := range second {
			second[i] = 'e'
		}
		b.Write(first)
		b.Write(second)

		got := b.String()
		if len(got) != 100 {
			t.Fatalf("expected 100 bytes total, got %d", len(got))
		}
		// First 60 should be 'd', next 40 should be 'e'
		for i := 0; i < 60; i++ {
			if got[i] != 'd' {
				t.Fatalf("byte %d: expected 'd', got %q", i, got[i])
			}
		}
		for i := 60; i < 100; i++ {
			if got[i] != 'e' {
				t.Fatalf("byte %d: expected 'e', got %q", i, got[i])
			}
		}
	})

	t.Run("ZeroMax", func(t *testing.T) {
		b := &limitedBuffer{max: 0}
		data := []byte("hello")
		n, err := b.Write(data)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// remaining is 0, so the truncation branch is skipped entirely;
		// len(p) is still the original 5 since p is never re-sliced.
		if n != 5 {
			t.Fatalf("expected n=5, got %d", n)
		}
		if b.String() != "" {
			t.Fatalf("expected empty string, got %q", b.String())
		}
	})
}
