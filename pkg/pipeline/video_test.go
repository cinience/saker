package pipeline

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestFmtStreamDuration(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{0, "00:00"},
		{30 * time.Second, "00:30"},
		{90 * time.Second, "01:30"},
		{5 * time.Minute, "05:00"},
		{61*time.Minute + 30*time.Second, "61:30"},
	}
	for _, tt := range tests {
		got := fmtStreamDuration(tt.d)
		if got != tt.want {
			t.Errorf("fmtStreamDuration(%v) = %q, want %q", tt.d, got, tt.want)
		}
	}
}

func TestVideoController_Stats_InitialZero(t *testing.T) {
	vc := &VideoController{Done: make(chan struct{})}
	p, s, e := vc.Stats()
	if p != 0 || s != 0 || e != 0 {
		t.Fatalf("expected (0,0,0), got (%d,%d,%d)", p, s, e)
	}
}

func TestVideoController_IncProcessed(t *testing.T) {
	vc := &VideoController{Done: make(chan struct{})}
	vc.IncProcessed()
	vc.IncProcessed()
	vc.IncProcessed()
	p, _, _ := vc.Stats()
	if p != 3 {
		t.Fatalf("expected processed=3, got %d", p)
	}
}

func TestVideoController_IncSkipped(t *testing.T) {
	vc := &VideoController{Done: make(chan struct{})}
	vc.IncSkipped()
	vc.IncSkipped()
	_, s, _ := vc.Stats()
	if s != 2 {
		t.Fatalf("expected skipped=2, got %d", s)
	}
}

func TestVideoController_AddEvents(t *testing.T) {
	vc := &VideoController{Done: make(chan struct{})}
	vc.AddEvents(5)
	vc.AddEvents(3)
	_, _, e := vc.Stats()
	if e != 8 {
		t.Fatalf("expected events=8, got %d", e)
	}
}

func TestVideoController_Stop(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		<-ctx.Done()
		close(done)
	}()
	vc := &VideoController{Cancel: cancel, Done: done}

	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	stopped := make(chan struct{})
	go func() {
		vc.Stop()
		close(stopped)
	}()
	select {
	case <-stopped:
		// ok
	case <-timer.C:
		t.Fatal("Stop() did not return within 2 seconds")
	}
}

func TestVideoController_ConcurrentAccess(t *testing.T) {
	vc := &VideoController{Done: make(chan struct{})}
	var wg sync.WaitGroup
	const goroutines = 10
	const iterations = 100

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				vc.IncProcessed()
				vc.IncSkipped()
				vc.AddEvents(1)
			}
		}()
	}
	wg.Wait()

	p, s, e := vc.Stats()
	want := goroutines * iterations
	if p != want || s != want || e != want {
		t.Fatalf("expected (%d,%d,%d), got (%d,%d,%d)", want, want, want, p, s, e)
	}
}
