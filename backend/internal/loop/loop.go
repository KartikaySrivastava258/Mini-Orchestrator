// Package loop runs the background reconcile loops. One goroutine per loop,
// no overlapping passes, exponential backoff when things break, and a clean
// exit when the context is cancelled. Every loop in the project goes through
// here.
package loop

import (
	"context"
	"log/slog"
	"time"
)

// Trigger is a doorbell for a loop. Ring it fifty times and the loop still
// only gets out of bed once.
type Trigger chan struct{}

// NewTrigger returns a ready-to-ring Trigger.
func NewTrigger() Trigger { return make(Trigger, 1) }

// Fire asks for an early pass. Never blocks.
func (t Trigger) Fire() {
	select {
	case t <- struct{}{}:
	default:
	}
}

// Backoff returns base * 2^(failures-1), capped at max. Zero failures just gives you base.
func Backoff(base, max time.Duration, failures int) time.Duration {
	d := base
	for i := 1; i < failures && d < max; i++ {
		d *= 2
	}
	if d > max {
		d = max
	}
	return d
}

// Run calls pass right away, then every interval, and early whenever trigger
// fires (a nil trigger is fine). Keep failing and the wait doubles up to
// maxBackoff. Returns when ctx is cancelled, so pass should respect ctx too.
func Run(ctx context.Context, name string, interval, maxBackoff time.Duration, trigger <-chan struct{}, log *slog.Logger, pass func(context.Context) error) {
	if maxBackoff < interval {
		maxBackoff = interval
	}
	log = log.With("loop", name)
	log.Info("loop started", "interval", interval.String())
	defer log.Info("loop stopped")

	timer := time.NewTimer(0)
	defer timer.Stop()
	failures := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		case <-trigger:
			timer.Stop()
		}

		start := time.Now()
		err := pass(ctx)
		if ctx.Err() != nil {
			return
		}
		wait := interval
		if err != nil {
			failures++
			wait = Backoff(interval, maxBackoff, failures)
			log.Warn("pass failed", "error", err, "consecutive_failures", failures, "retry_in", wait.String())
		} else {
			if failures > 0 {
				log.Info("pass recovered", "after_failures", failures)
			}
			failures = 0
			log.Debug("pass completed", "duration", time.Since(start).String())
		}
		timer.Reset(wait)
	}
}
