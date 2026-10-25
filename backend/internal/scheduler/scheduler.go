// Package scheduler finds a home for Pending pods.
//
// Policy LeastPods: out of all eligible nodes, pick the one with the fewest
// pods; ties go to the alphabetically first name, because randomness is not a
// scheduling strategy. Eligible means Ready, not cordoned, a fresh heartbeat,
// and below its declared maxPods.
//
// There's no CPU or memory awareness here. maxPods is a slot count the agent
// declares, nothing fancier.
package scheduler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"mini-orchestrator/internal/domain"
	"mini-orchestrator/internal/events"
	"mini-orchestrator/internal/store"
)

// PolicyName shows up in the API and the console.
const PolicyName = "LeastPods"

// Decision is what Place came up with for one pod.
type Decision struct {
	NodeName string // empty when the pod cannot be placed
	Reason   string
	Message  string
}

// Place is the pure, zero-I/O placement function. load maps a node name to
// how many pods are already sitting on it.
func Place(nodes []domain.Node, load map[string]int, now time.Time, heartbeatTimeout time.Duration) Decision {
	sorted := make([]domain.Node, len(nodes))
	copy(sorted, nodes)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })

	var notReady, stale, cordoned, full int
	best := -1
	bestLoad := 0
	for i, n := range sorted {
		switch {
		case n.Spec.Unschedulable:
			cordoned++
			continue
		case !n.Status.Ready:
			notReady++
			continue
		case now.Sub(n.Status.LastHeartbeatTime) > heartbeatTimeout:
			stale++
			continue
		}
		l := load[n.Name]
		if n.Status.MaxPods > 0 && l >= n.Status.MaxPods {
			full++
			continue
		}
		if best == -1 || l < bestLoad {
			best, bestLoad = i, l
		}
	}

	if best >= 0 {
		n := sorted[best]
		return Decision{
			NodeName: n.Name,
			Reason:   domain.ReasonScheduled,
			Message:  fmt.Sprintf("assigned to node %s by policy %s (%d/%d pod slots in use before placement)", n.Name, PolicyName, bestLoad, n.Status.MaxPods),
		}
	}
	summary := fmt.Sprintf("0/%d nodes available: %d not ready, %d heartbeat stale, %d cordoned, %d at pod capacity",
		len(nodes), notReady, stale, cordoned, full)
	if full > 0 {
		return Decision{Reason: domain.ReasonInsufficientCapacity, Message: summary}
	}
	return Decision{Reason: domain.ReasonNoEligibleNodes, Message: summary}
}

// Scheduler binds Pending pods to nodes.
type Scheduler struct {
	store            *store.Store
	events           *events.Recorder
	log              *slog.Logger
	heartbeatTimeout time.Duration
	now              func() time.Time
}

// New returns a Scheduler.
func New(st *store.Store, rec *events.Recorder, log *slog.Logger, heartbeatTimeout time.Duration) *Scheduler {
	return &Scheduler{store: st, events: rec, log: log, heartbeatTimeout: heartbeatTimeout, now: func() time.Time { return time.Now().UTC() }}
}

// SetClock swaps the time source, so tests never have to sleep.
func (s *Scheduler) SetClock(now func() time.Time) { s.now = now }

// RunOnce does one pass over every Pending pod, oldest first. Binding is
// compare-and-swap, so if a pod changed while we were thinking (deleted, or
// already bound) it just gets skipped. Double-booking a pod is not on the menu.
func (s *Scheduler) RunOnce(ctx context.Context) error {
	pods, err := s.store.ListPods(ctx)
	if err != nil {
		return fmt.Errorf("list pods: %w", err)
	}
	var pending []domain.Pod
	load := map[string]int{}
	for _, p := range pods {
		if p.OccupiesNode() {
			load[p.Status.NodeName]++
		}
		if p.Status.Phase == domain.PodPending && p.DeletionRequestedAt == nil {
			pending = append(pending, p)
		}
	}
	if len(pending) == 0 {
		return nil
	}
	nodes, err := s.store.ListNodes(ctx)
	if err != nil {
		return fmt.Errorf("list nodes: %w", err)
	}
	sort.Slice(pending, func(i, j int) bool {
		if !pending[i].CreatedAt.Equal(pending[j].CreatedAt) {
			return pending[i].CreatedAt.Before(pending[j].CreatedAt)
		}
		return pending[i].Name < pending[j].Name
	})

	var errs []error
	for _, p := range pending {
		if err := ctx.Err(); err != nil {
			return err
		}
		now := s.now()
		d := Place(nodes, load, now, s.heartbeatTimeout)
		if d.NodeName == "" {
			if err := s.recordUnschedulable(ctx, p, d, now); err != nil {
				errs = append(errs, err)
			}
			continue
		}
		bound, err := s.bind(ctx, p, d, now)
		switch {
		case errors.Is(err, store.ErrConflict):
			s.events.Record("scheduler", events.TypeWarning, "Pod", p.Name, "SchedulingConflict",
				"pod changed concurrently while binding; it will be retried on the next pass")
		case err != nil && !errors.Is(err, store.ErrNotFound):
			errs = append(errs, fmt.Errorf("bind pod %s: %w", p.Name, err))
		case bound:
			load[d.NodeName]++
			s.events.Record("scheduler", events.TypeNormal, "Pod", p.Name, domain.ReasonScheduled, d.Message)
		}
	}
	return errors.Join(errs...)
}

func (s *Scheduler) bind(ctx context.Context, p domain.Pod, d Decision, now time.Time) (bool, error) {
	bound := false
	_, err := s.store.UpdatePod(ctx, p.Name, func(cur *domain.Pod) error {
		bound = false
		if cur.UID != p.UID || cur.Status.Phase != domain.PodPending || cur.DeletionRequestedAt != nil {
			return store.ErrNoChange
		}
		t := now
		cur.Status.Phase = domain.PodScheduled
		cur.Status.NodeName = d.NodeName
		cur.Status.Reason = d.Reason
		cur.Status.Message = d.Message
		cur.Status.ScheduledAt = &t
		cur.Status.LastTransitionTime = now
		cur.Status.UpdatedAt = now
		bound = true
		return nil
	})
	if err != nil {
		return false, err
	}
	return bound, nil
}

// recordUnschedulable writes down why a pod is still Pending, but only when
// the reason changes. Otherwise a stuck pod would hit etcd every two seconds
// for nothing.
func (s *Scheduler) recordUnschedulable(ctx context.Context, p domain.Pod, d Decision, now time.Time) error {
	changed := false
	_, err := s.store.UpdatePod(ctx, p.Name, func(cur *domain.Pod) error {
		changed = false
		if cur.UID != p.UID || cur.Status.Phase != domain.PodPending {
			return store.ErrNoChange
		}
		if cur.Status.Reason == d.Reason && cur.Status.Message == d.Message {
			return store.ErrNoChange
		}
		cur.Status.Reason = d.Reason
		cur.Status.Message = d.Message
		cur.Status.UpdatedAt = now
		changed = true
		return nil
	})
	if errors.Is(err, store.ErrNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("record pending reason for %s: %w", p.Name, err)
	}
	if changed {
		s.events.Record("scheduler", events.TypeWarning, "Pod", p.Name, "FailedScheduling", d.Message)
	}
	return nil
}
