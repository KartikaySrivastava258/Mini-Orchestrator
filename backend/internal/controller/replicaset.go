// Package controller holds the control-plane loops: the ReplicaSet controller
// and the node monitor. Each one has a Reconcile method that does a single
// idempotent pass; package loop takes care of running it.
package controller

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"mini-orchestrator/internal/domain"
	"mini-orchestrator/internal/events"
	"mini-orchestrator/internal/lifecycle"
	"mini-orchestrator/internal/store"
)

// maxCreatesPerPass stops a huge scale-up from dumping everything on etcd in
// one burst. Patience is a virtue.
const maxCreatesPerPass = 10

// ReplicaSetController keeps every ReplicaSet at its desired pod count and
// cleans up pods whose owner has left the chat.
type ReplicaSetController struct {
	store  *store.Store
	events *events.Recorder
	log    *slog.Logger
	now    func() time.Time
}

// NewReplicaSetController returns a controller.
func NewReplicaSetController(st *store.Store, rec *events.Recorder, log *slog.Logger) *ReplicaSetController {
	return &ReplicaSetController{store: st, events: rec, log: log, now: func() time.Time { return time.Now().UTC() }}
}

// SetClock swaps the time source for tests.
func (c *ReplicaSetController) SetClock(now func() time.Time) { c.now = now }

// Reconcile does one pass. One broken ReplicaSet doesn't block the others;
// errors get collected and returned so the loop backs off.
func (c *ReplicaSetController) Reconcile(ctx context.Context) error {
	sets, err := c.store.ListReplicaSets(ctx)
	if err != nil {
		return fmt.Errorf("list replicasets: %w", err)
	}
	pods, err := c.store.ListPods(ctx)
	if err != nil {
		return fmt.Errorf("list pods: %w", err)
	}

	byName := make(map[string]domain.ReplicaSet, len(sets))
	for _, rs := range sets {
		byName[rs.Name] = rs
	}
	owned := map[string][]domain.Pod{}
	var errs []error

	for _, p := range pods {
		if p.Owner == nil || p.Owner.Kind != domain.KindReplicaSet {
			continue // pods you made by hand are none of our business
		}
		if rs, ok := byName[p.Owner.Name]; ok && rs.UID == p.Owner.UID {
			owned[rs.Name] = append(owned[rs.Name], p)
			continue
		}
		if p.IsTerminating() {
			continue
		}
		// the owner is gone (or got re-created with a new UID), so this pod goes too
		if err := c.deletePod(ctx, p, "OrphanDeleted", "owner ReplicaSet "+p.Owner.Name+" no longer exists"); err != nil {
			errs = append(errs, err)
		}
	}

	for _, rs := range sets {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := c.reconcileOne(ctx, rs, owned[rs.Name]); err != nil {
			errs = append(errs, fmt.Errorf("replicaset %s: %w", rs.Name, err))
		}
	}
	return errors.Join(errs...)
}

func (c *ReplicaSetController) reconcileOne(ctx context.Context, rs domain.ReplicaSet, pods []domain.Pod) error {
	var active []domain.Pod
	running := 0
	var errs []error
	for _, p := range pods {
		if p.IsTerminating() {
			continue
		}
		if p.Status.Phase.IsTerminal() {
			// terminal pods get swapped out: delete this one, a fresh one is created below
			if err := c.deletePod(ctx, p, "ReplacingTerminatedPod",
				fmt.Sprintf("pod %s ended in phase %s (%s); it will be replaced", p.Name, p.Status.Phase, p.Status.Reason)); err != nil {
				errs = append(errs, err)
			}
			continue
		}
		if p.Status.Phase.IsActive() {
			active = append(active, p)
			if p.Status.Phase == domain.PodRunning {
				running++
			}
		}
	}

	diff := rs.Spec.Replicas - len(active)
	switch {
	case diff > 0:
		n := min(diff, maxCreatesPerPass)
		for i := 0; i < n; i++ {
			name, err := c.createPod(ctx, rs)
			if err != nil {
				errs = append(errs, err)
				break
			}
			c.events.Record("replicaset-controller", events.TypeNormal, domain.KindReplicaSet, rs.Name, "SuccessfulCreate",
				fmt.Sprintf("created pod %s (%d/%d active before this pass)", name, len(active), rs.Spec.Replicas))
		}
	case diff < 0:
		sortForScaleDown(active)
		for _, p := range active[:-diff] {
			if err := c.deletePod(ctx, p, "ScaledDown",
				fmt.Sprintf("deleting pod %s to scale %s down to %d replicas", p.Name, rs.Name, rs.Spec.Replicas)); err != nil {
				errs = append(errs, err)
			}
		}
	}

	if err := c.updateStatus(ctx, rs, len(active), running); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

// sortForScaleDown puts the least valuable pods first: unscheduled, then
// unknown, then scheduled, then running. Newer before older, then by name so
// the result is always the same.
func sortForScaleDown(pods []domain.Pod) {
	rank := func(p domain.PodPhase) int {
		switch p {
		case domain.PodPending:
			return 0
		case domain.PodUnknown:
			return 1
		case domain.PodScheduled:
			return 2
		default:
			return 3
		}
	}
	sort.Slice(pods, func(i, j int) bool {
		ri, rj := rank(pods[i].Status.Phase), rank(pods[j].Status.Phase)
		if ri != rj {
			return ri < rj
		}
		if !pods[i].CreatedAt.Equal(pods[j].CreatedAt) {
			return pods[i].CreatedAt.After(pods[j].CreatedAt)
		}
		return pods[i].Name > pods[j].Name
	})
}

func (c *ReplicaSetController) createPod(ctx context.Context, rs domain.ReplicaSet) (string, error) {
	now := c.now()
	for attempt := 0; attempt < 3; attempt++ {
		pod := domain.Pod{
			Name:      rs.Name + "-" + domain.RandomSuffix(5),
			UID:       domain.NewUID(),
			CreatedAt: now,
			Owner:     &domain.OwnerReference{Kind: domain.KindReplicaSet, Name: rs.Name, UID: rs.UID},
			Spec:      domain.PodSpec{Image: rs.Spec.Template.Image, RestartPolicy: domain.RestartAlways},
			Status: domain.PodStatus{
				Phase:              domain.PodPending,
				Reason:             domain.ReasonAwaitingScheduling,
				Message:            "created by ReplicaSet " + rs.Name,
				LastTransitionTime: now,
				UpdatedAt:          now,
			},
		}
		err := c.store.CreatePod(ctx, pod)
		if errors.Is(err, store.ErrAlreadyExists) {
			continue // name collision, roll the dice again
		}
		if err != nil {
			return "", fmt.Errorf("create pod: %w", err)
		}
		return pod.Name, nil
	}
	return "", errors.New("could not generate a unique pod name")
}

func (c *ReplicaSetController) deletePod(ctx context.Context, p domain.Pod, reason, message string) error {
	if _, err := lifecycle.RequestPodDeletion(ctx, c.store, p.Name, c.now()); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil
		}
		return fmt.Errorf("delete pod %s: %w", p.Name, err)
	}
	eventType := events.TypeNormal
	if reason == "ReplacingTerminatedPod" {
		eventType = events.TypeWarning
	}
	c.events.Record("replicaset-controller", eventType, "Pod", p.Name, reason, message)
	return nil
}

func (c *ReplicaSetController) updateStatus(ctx context.Context, rs domain.ReplicaSet, active, running int) error {
	now := c.now()
	_, err := c.store.UpdateReplicaSet(ctx, rs.Name, func(cur *domain.ReplicaSet) error {
		if cur.UID != rs.UID {
			return store.ErrNoChange
		}
		if cur.Status.Replicas == active && cur.Status.RunningReplicas == running && cur.Status.ObservedAt != nil {
			return store.ErrNoChange
		}
		t := now
		cur.Status.Replicas = active
		cur.Status.RunningReplicas = running
		cur.Status.ObservedAt = &t
		return nil
	})
	if errors.Is(err, store.ErrNotFound) {
		return nil
	}
	return err
}
