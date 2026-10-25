// Package lifecycle is the rulebook for pod and node state changes. The API
// and the controllers all go through it, so nobody gets to invent their own
// rules. The full transition table is in docs/ARCHITECTURE.md.
package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"time"

	"mini-orchestrator/internal/domain"
	"mini-orchestrator/internal/store"
)

// ErrRejected means "that move isn't allowed from where the pod is right now",
// like an agent reporting on a pod that's already being deleted. The API
// answers with a 409.
var ErrRejected = errors.New("transition rejected")

// DeletionResult says what RequestPodDeletion actually did.
type DeletionResult string

const (
	// Deleted: the record was removed immediately (no node had a container for it).
	Deleted DeletionResult = "Deleted"
	// Terminating: the assigned node agent must remove the container and then finalize.
	Terminating DeletionResult = "Terminating"
)

// RequestPodDeletion deletes a pod safely, and it's fine to call it twice.
//
// If no node owns the pod (or its node is gone) the record is deleted right
// away. Otherwise the pod goes Terminating, the node agent removes the
// container and then calls finalize to delete the record. Races with the
// scheduler are settled with compare-and-swap plus a few retries.
func RequestPodDeletion(ctx context.Context, st *store.Store, name string, now time.Time) (DeletionResult, error) {
	for attempt := 0; attempt < 3; attempt++ {
		pod, err := st.GetPod(ctx, name)
		if err != nil {
			return "", err
		}
		if pod.IsTerminating() {
			return Terminating, nil
		}
		deleteNow := pod.Status.NodeName == ""
		if !deleteNow {
			if _, err := st.GetNode(ctx, pod.Status.NodeName); errors.Is(err, store.ErrNotFound) {
				deleteNow = true
			} else if err != nil {
				return "", err
			}
		}

		if deleteNow {
			err := st.DeletePod(ctx, name, func(cur domain.Pod) error {
				if cur.UID != pod.UID || cur.Status.NodeName != pod.Status.NodeName {
					return store.ErrConflict // changed underneath us; re-evaluate
				}
				return nil
			})
			if errors.Is(err, store.ErrConflict) {
				continue
			}
			if err != nil {
				return "", err
			}
			return Deleted, nil
		}

		_, err = st.UpdatePod(ctx, name, func(cur *domain.Pod) error {
			if cur.UID != pod.UID {
				return store.ErrConflict
			}
			if cur.IsTerminating() {
				return store.ErrNoChange
			}
			t := now
			cur.DeletionRequestedAt = &t
			cur.Status.Phase = domain.PodTerminating
			cur.Status.Reason = domain.ReasonDeletionRequested
			cur.Status.Message = "waiting for node " + cur.Status.NodeName + " to remove the container"
			cur.Status.LastTransitionTime = now
			cur.Status.UpdatedAt = now
			return nil
		})
		if errors.Is(err, store.ErrConflict) {
			continue
		}
		if err != nil {
			return "", err
		}
		return Terminating, nil
	}
	return "", store.ErrConflict
}

// ApplyAgentStatus applies a node agent's report to pod. It returns
// store.ErrNoChange when nothing would change, and ErrRejected when the report
// is stale or the move isn't allowed.
func ApplyAgentStatus(pod *domain.Pod, u domain.PodStatusUpdate, now time.Time) error {
	if pod.UID != u.UID {
		return fmt.Errorf("%w: pod UID does not match (the pod was re-created)", ErrRejected)
	}
	if pod.Status.NodeName != u.NodeName {
		return fmt.Errorf("%w: pod is not assigned to node %q", ErrRejected, u.NodeName)
	}
	if pod.IsTerminating() {
		return fmt.Errorf("%w: pod is terminating", ErrRejected)
	}
	switch pod.Status.Phase {
	case domain.PodScheduled, domain.PodRunning, domain.PodUnknown:
	default:
		return fmt.Errorf("%w: cannot change phase from %s", ErrRejected, pod.Status.Phase)
	}
	switch u.Phase {
	case domain.PodRunning, domain.PodSucceeded, domain.PodFailed:
	default:
		return fmt.Errorf("%w: agents cannot set phase %s", ErrRejected, u.Phase)
	}

	s := &pod.Status
	unchanged := s.Phase == u.Phase && s.Reason == u.Reason && s.Message == u.Message &&
		s.ContainerID == u.ContainerID && s.RestartCount == u.RestartCount && equalIntPtr(s.ExitCode, u.ExitCode)
	if unchanged {
		return store.ErrNoChange
	}
	if s.Phase != u.Phase {
		s.LastTransitionTime = now
	}
	if u.Phase == domain.PodRunning && (s.StartedAt == nil || s.ContainerID != u.ContainerID || s.RestartCount != u.RestartCount) {
		t := now
		s.StartedAt = &t
	}
	s.Phase = u.Phase
	s.Reason = u.Reason
	s.Message = u.Message
	s.ContainerID = u.ContainerID
	s.RestartCount = u.RestartCount
	s.ExitCode = copyIntPtr(u.ExitCode)
	s.UpdatedAt = now
	return nil
}

// ApplyHeartbeat records a heartbeat (or a registration). A node is Ready only
// while it keeps heartbeating AND can reach Docker. Returns true if Ready
// flipped.
func ApplyHeartbeat(n *domain.Node, runtimeReady bool, runtimeMessage string, now time.Time) bool {
	n.Status.LastHeartbeatTime = now
	n.Status.RuntimeReady = runtimeReady
	reason, message := domain.ReasonNodeReady, "node agent is heartbeating and the container runtime is reachable"
	if !runtimeReady {
		reason = domain.ReasonRuntimeUnavailable
		message = domain.Truncate(runtimeMessage, domain.MaxMessageLength)
		if message == "" {
			message = "node agent reports that the container runtime is unreachable"
		}
	}
	changed := n.Status.Ready != runtimeReady
	if changed {
		n.Status.LastTransitionTime = now
	}
	n.Status.Ready = runtimeReady
	n.Status.Reason = reason
	n.Status.Message = message
	return changed
}

func equalIntPtr(a, b *int) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func copyIntPtr(p *int) *int {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}
