package loop

import (
	"context"
	"errors"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"
)

func TestBackoff(t *testing.T) {
	base, max := time.Second, 10*time.Second
	cases := map[int]time.Duration{0: time.Second, 1: time.Second, 2: 2 * time.Second, 3: 4 * time.Second, 4: 8 * time.Second, 5: 10 * time.Second, 50: 10 * time.Second}
	for failures, want := range cases {
		if got := Backoff(base, max, failures); got != want {
			t.Errorf("Backoff(%d) = %v, want %v", failures, got, want)
		}
	}
}

func TestRunStopsOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var passes atomic.Int32
	done := make(chan struct{})
	go func() {
		Run(ctx, "test", 5*time.Millisecond, 5*time.Millisecond, nil, slog.New(slog.DiscardHandler), func(context.Context) error {
			passes.Add(1)
			return nil
		})
		close(done)
	}()
	time.Sleep(30 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Run did not return after cancellation")
	}
	if passes.Load() < 2 {
		t.Fatalf("expected repeated passes, got %d", passes.Load())
	}
}

func TestTriggerCausesEarlyPassAndCoalesces(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	trig := NewTrigger()
	var passes atomic.Int32
	go Run(ctx, "test", time.Hour, time.Hour, trig, slog.New(slog.DiscardHandler), func(context.Context) error {
		passes.Add(1)
		return nil
	})
	waitFor(t, func() bool { return passes.Load() == 1 }) // initial pass
	trig.Fire()
	trig.Fire()
	trig.Fire() // coalesced: buffer of one
	waitFor(t, func() bool { return passes.Load() >= 2 })
	time.Sleep(20 * time.Millisecond)
	if n := passes.Load(); n > 3 {
		t.Fatalf("triggers were not coalesced: %d passes", n)
	}
}

func TestRunKeepsGoingAfterErrors(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var passes atomic.Int32
	go Run(ctx, "test", time.Millisecond, 4*time.Millisecond, nil, slog.New(slog.DiscardHandler), func(context.Context) error {
		passes.Add(1)
		return errors.New("transient")
	})
	waitFor(t, func() bool { return passes.Load() >= 3 })
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatal("condition not met before deadline")
}
