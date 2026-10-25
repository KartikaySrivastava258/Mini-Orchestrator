package controller

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"mini-orchestrator/internal/domain"
	"mini-orchestrator/internal/events"
	"mini-orchestrator/internal/store"
)

// NodeMonitor is the friend who notices when someone goes quiet in the group
// chat.
//
// When a Ready node misses heartbeats for longer than heartbeatTimeout, it
// becomes NotReady and its Scheduled and Running pods turn Unknown. If it
// stays gone past evictionTimeout (or its record disappears), those pods fail
// with reason NodeLost so their ReplicaSets can replace them, and Terminating
// pods get force-deleted because there's no agent left to finalize them.
//
// If the node comes back, its agent reports the Unknown pods as Running again.
// Pods that already failed stay failed, and the agent stops their containers.
type NodeMonitor struct {
	store            *store.Store
	events           *events.Recorder
	log              *slog.Logger
	heartbeatTimeout time.Duration
	evictionTimeout  time.Duration
	now              func() time.Time
}

// NewNodeMonitor returns a NodeMonitor.
func NewNodeMonitor(st *store.Store, rec *events.Recorder, log *slog.Logger, heartbeatTimeout, evictionTimeout time.Duration) *NodeMonitor {
	return &NodeMonitor{
		store:            st,
		events:           rec,
		log:              log,
		heartbeatTimeout: heartbeatTimeout,
		evictionTimeout:  evictionTimeout,
		now:              func() time.Time { return time.Now().UTC() },
	}
}

// SetClock swaps the time source for tests.
func (m *NodeMonitor) SetClock(now func() time.Time) { m.now = now }

// Reconcile does one pass.
func (m *NodeMonitor) Reconcile(ctx context.Context) error {
	now := m.now()
	nodes, err := m.store.ListNodes(ctx)
	if err != nil {
		return fmt.Errorf("list nodes: %w", err)
	}
	var errs []error
	byName := make(map[string]domain.Node, len(nodes))
	for _, n := range nodes {
		byName[n.Name] = n
		if !n.Status.Ready || now.Sub(n.Status.LastHeartbeatTime) <= m.heartbeatTimeout {
			continue
		}
		updated, err := m.store.UpdateNode(ctx, n.Name, func(cur *domain.Node) error {
			if !cur.Status.Ready || now.Sub(cur.Status.LastHeartbeatTime) <= m.heartbeatTimeout {
				return store.ErrNoChange // a heartbeat snuck in, false alarm
			}
			cur.Status.Ready = false
			cur.Status.Reason = domain.ReasonHeartbeatTimeout
			cur.Status.Message = fmt.Sprintf("no heartbeat since %s (timeout %s)",
				cur.Status.LastHeartbeatTime.UTC().Format(time.RFC3339), m.heartbeatTimeout)
			cur.Status.LastTransitionTime = now
			return nil
		})
		if errors.Is(err, store.ErrNotFound) {
			delete(byName, n.Name)
			continue
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("mark node %s not ready: %w", n.Name, err))
			continue
		}
		byName[n.Name] = updated
		if !updated.Status.Ready {
			m.events.Record("node-monitor", events.TypeWarning, "Node", n.Name, domain.ReasonHeartbeatTimeout, updated.Status.Message)
		}
	}

	pods, err := m.store.ListPods(ctx)
	if err != nil {
		return errors.Join(append(errs, fmt.Errorf("list pods: %w", err))...)
	}
	for _, p := range pods {
		if err := ctx.Err(); err != nil {
			return err
		}
		if p.Status.NodeName == "" {
			continue
		}
		n, exists := byName[p.Status.NodeName]
		if exists && n.Status.Ready {
			continue
		}
		lost := !exists || now.Sub(n.Status.LastTransitionTime) > m.evictionTimeout
		if err := m.handlePod(ctx, p, exists, lost, now); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (m *NodeMonitor) handlePod(ctx context.Context, p domain.Pod, nodeExists, lost bool, now time.Time) error {
	switch {
	case p.IsTerminating():
		if !lost {
			return nil
		}
		err := m.store.DeletePod(ctx, p.Name, func(cur domain.Pod) error {
			if cur.UID != p.UID {
				return store.ErrConflict
			}
			return nil
		})
		if err != nil && !errors.Is(err, store.ErrNotFound) && !errors.Is(err, store.ErrConflict) {
			return fmt.Errorf("force-delete pod %s: %w", p.Name, err)
		}
		if err == nil {
			msg := "node " + p.Status.NodeName + " is unavailable, so no agent can confirm container removal; the record was deleted and the container may remain on that host"
			m.events.Record("node-monitor", events.TypeWarning, "Pod", p.Name, "ForceDeleted", msg)
		}
		return nil

	case p.Status.Phase == domain.PodScheduled || p.Status.Phase == domain.PodRunning || p.Status.Phase == domain.PodUnknown:
		target, reason := domain.PodUnknown, domain.ReasonNodeNotReady
		msg := "node " + p.Status.NodeName + " is not ready; pod state cannot be confirmed"
		if lost {
			target, reason = domain.PodFailed, domain.ReasonNodeLost
			msg = "node " + p.Status.NodeName + " has been unavailable longer than " + m.evictionTimeout.String()
			if !nodeExists {
				msg = "node " + p.Status.NodeName + " no longer exists"
			}
		}
		if p.Status.Phase == target {
			return nil
		}
		changed := false
		_, err := m.store.UpdatePod(ctx, p.Name, func(cur *domain.Pod) error {
			changed = false
			if cur.UID != p.UID || cur.IsTerminating() || cur.Status.Phase != p.Status.Phase {
				return store.ErrNoChange // changed concurrently; next pass re-evaluates
			}
			cur.Status.Phase = target
			cur.Status.Reason = reason
			cur.Status.Message = msg
			cur.Status.LastTransitionTime = now
			cur.Status.UpdatedAt = now
			changed = true
			return nil
		})
		if errors.Is(err, store.ErrNotFound) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("update pod %s: %w", p.Name, err)
		}
		if changed {
			m.events.Record("node-monitor", events.TypeWarning, "Pod", p.Name, reason, msg)
		}
	}
	return nil
}
