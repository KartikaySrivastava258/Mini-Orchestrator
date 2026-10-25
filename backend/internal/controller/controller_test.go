package controller

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"mini-orchestrator/internal/domain"
	"mini-orchestrator/internal/events"
	"mini-orchestrator/internal/store"
)

var t0 = time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

func discard() *slog.Logger { return slog.New(slog.DiscardHandler) }

type env struct {
	ctx context.Context
	kv  *store.MemoryKV
	st  *store.Store
	rec *events.Recorder
	rsc *ReplicaSetController
	mon *NodeMonitor
	now time.Time
}

func newEnv() *env {
	kv := store.NewMemoryKV()
	e := &env{ctx: context.Background(), kv: kv, st: store.New(kv, store.DefaultPrefix, nil), rec: events.NewRecorder(500, nil), now: t0}
	e.rsc = NewReplicaSetController(e.st, e.rec, discard())
	e.rsc.SetClock(func() time.Time { return e.now })
	e.mon = NewNodeMonitor(e.st, e.rec, discard(), 15*time.Second, 60*time.Second)
	e.mon.SetClock(func() time.Time { return e.now })
	return e
}

func (e *env) createRS(name string, replicas int) domain.ReplicaSet {
	rs := domain.ReplicaSet{Name: name, UID: "rs-uid-" + name, CreatedAt: e.now,
		Spec: domain.ReplicaSetSpec{Replicas: replicas, Template: domain.PodTemplate{Image: "nginx"}}}
	if err := e.st.CreateReplicaSet(e.ctx, rs); err != nil {
		panic(err)
	}
	return rs
}

func (e *env) ownedPods(rsName string) (active []domain.Pod, all []domain.Pod) {
	pods, _ := e.st.ListPods(e.ctx)
	for _, p := range pods {
		if p.Owner != nil && p.Owner.Name == rsName {
			all = append(all, p)
			if !p.IsTerminating() && p.Status.Phase.IsActive() {
				active = append(active, p)
			}
		}
	}
	return active, all
}

func (e *env) reconcile(t *testing.T) {
	t.Helper()
	if err := e.rsc.Reconcile(e.ctx); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
}

func TestReplicaSetConvergesAndIsIdempotent(t *testing.T) {
	e := newEnv()
	e.createRS("web", 3)
	e.reconcile(t)
	active, _ := e.ownedPods("web")
	if len(active) != 3 {
		t.Fatalf("after first pass: %d active pods, want 3", len(active))
	}
	for i := 0; i < 3; i++ {
		e.reconcile(t)
	}
	active, all := e.ownedPods("web")
	if len(active) != 3 || len(all) != 3 {
		t.Fatalf("repeated passes changed pod count: active=%d all=%d", len(active), len(all))
	}
	for _, p := range all {
		if p.Spec.RestartPolicy != domain.RestartAlways || p.Spec.Image != "nginx" || p.Owner.UID != "rs-uid-web" {
			t.Fatalf("pod not built from template: %+v", p)
		}
	}
	rs, _ := e.st.GetReplicaSet(e.ctx, "web")
	if rs.Status.Replicas != 3 {
		t.Fatalf("status.replicas=%d", rs.Status.Replicas)
	}
}

func TestReplicaSetScaleUpIsBoundedPerPass(t *testing.T) {
	e := newEnv()
	e.createRS("big", maxCreatesPerPass+5)
	e.reconcile(t)
	if active, _ := e.ownedPods("big"); len(active) != maxCreatesPerPass {
		t.Fatalf("first pass created %d, want %d", len(active), maxCreatesPerPass)
	}
	e.reconcile(t)
	if active, _ := e.ownedPods("big"); len(active) != maxCreatesPerPass+5 {
		t.Fatalf("second pass: %d active", len(active))
	}
}

func TestReplicaSetScaleDownPrefersUnscheduledThenNewest(t *testing.T) {
	e := newEnv()
	rs := e.createRS("web", 3)
	_ = e.st.CreateNode(e.ctx, domain.Node{Name: "w1"})
	mk := func(name string, phase domain.PodPhase, age time.Duration) {
		node := "w1"
		if phase == domain.PodPending {
			node = ""
		}
		_ = e.st.CreatePod(e.ctx, domain.Pod{
			Name:      name,
			UID:       "u-" + name,
			CreatedAt: t0.Add(-age),
			Owner:     &domain.OwnerReference{Kind: domain.KindReplicaSet, Name: rs.Name, UID: rs.UID},
			Spec:      domain.PodSpec{Image: "nginx", RestartPolicy: domain.RestartAlways},
			Status:    domain.PodStatus{Phase: phase, NodeName: node},
		})
	}
	mk("old-running", domain.PodRunning, time.Hour)
	mk("new-running", domain.PodRunning, time.Minute)
	mk("pending", domain.PodPending, 2*time.Hour)

	_, _ = e.st.UpdateReplicaSet(e.ctx, "web", func(r *domain.ReplicaSet) error { r.Spec.Replicas = 1; return nil })
	e.reconcile(t)

	if _, err := e.st.GetPod(e.ctx, "pending"); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("unscheduled pod should be deleted first (immediately)")
	}
	newer, _ := e.st.GetPod(e.ctx, "new-running")
	if newer.Status.Phase != domain.PodTerminating {
		t.Fatalf("newer running pod should be terminating, got %s", newer.Status.Phase)
	}
	older, _ := e.st.GetPod(e.ctx, "old-running")
	if older.Status.Phase != domain.PodRunning {
		t.Fatal("oldest running pod should be kept")
	}
	e.reconcile(t) // terminating pods are not counted and not re-deleted
	if active, _ := e.ownedPods("web"); len(active) != 1 {
		t.Fatalf("active=%d after scale down", len(active))
	}
}

func TestReplicaSetReplacesFailedPods(t *testing.T) {
	e := newEnv()
	e.createRS("web", 1)
	e.reconcile(t)
	active, _ := e.ownedPods("web")
	failed := active[0].Name
	_, _ = e.st.UpdatePod(e.ctx, failed, func(p *domain.Pod) error {
		p.Status.Phase = domain.PodFailed
		p.Status.Reason = domain.ReasonRestartLimitExceeded
		return nil
	})
	e.reconcile(t)
	if _, err := e.st.GetPod(e.ctx, failed); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("failed unscheduled pod should be deleted, got %v", err)
	}
	if active, _ := e.ownedPods("web"); len(active) != 1 || active[0].Name == failed {
		t.Fatalf("expected one replacement pod, got %+v", active)
	}
}

func TestOrphanedPodsAreDeletedButBarePodsAreNot(t *testing.T) {
	e := newEnv()
	e.createRS("web", 2)
	e.reconcile(t)
	_ = e.st.CreatePod(e.ctx, domain.Pod{Name: "bare", UID: "u-bare", Status: domain.PodStatus{Phase: domain.PodPending}})
	_ = e.st.DeleteReplicaSet(e.ctx, "web", nil)
	e.reconcile(t)
	if _, all := e.ownedPods("web"); len(all) != 0 {
		t.Fatalf("orphans remain: %d", len(all))
	}
	if _, err := e.st.GetPod(e.ctx, "bare"); err != nil {
		t.Fatal("a pod without an owner must never be deleted by the controller")
	}
}

func TestRecreatedReplicaSetDoesNotAdoptOldPods(t *testing.T) {
	e := newEnv()
	e.createRS("web", 1)
	e.reconcile(t)
	_, before := e.ownedPods("web")
	_ = e.st.DeleteReplicaSet(e.ctx, "web", nil)
	rs := domain.ReplicaSet{Name: "web", UID: "new-uid", Spec: domain.ReplicaSetSpec{Replicas: 1, Template: domain.PodTemplate{Image: "nginx"}}}
	_ = e.st.CreateReplicaSet(e.ctx, rs)
	e.reconcile(t)
	if _, err := e.st.GetPod(e.ctx, before[0].Name); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("pod owned by the old UID should have been garbage-collected")
	}
	active, _ := e.ownedPods("web")
	if len(active) != 1 || active[0].Owner.UID != "new-uid" {
		t.Fatalf("expected one pod owned by the new ReplicaSet: %+v", active)
	}
}

func TestReconcileReportsStoreErrors(t *testing.T) {
	e := newEnv()
	e.kv.SetFailure(fmt.Errorf("%w: down", store.ErrUnavailable))
	if err := e.rsc.Reconcile(e.ctx); !errors.Is(err, store.ErrUnavailable) {
		t.Fatalf("got %v", err)
	}
	if err := e.mon.Reconcile(e.ctx); !errors.Is(err, store.ErrUnavailable) {
		t.Fatalf("got %v", err)
	}
}

func TestReconcileHonoursCancellation(t *testing.T) {
	e := newEnv()
	e.createRS("web", 3)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := e.rsc.Reconcile(ctx); err == nil {
		t.Fatal("expected an error from a cancelled context")
	}
}

// --- node monitor ----------------------------------------------------------

func (e *env) readyNode(name string, heartbeat time.Time) {
	n := domain.Node{Name: name}
	n.Status.Ready = true
	n.Status.RuntimeReady = true
	n.Status.MaxPods = 10
	n.Status.LastHeartbeatTime = heartbeat
	n.Status.LastTransitionTime = heartbeat
	_ = e.st.CreateNode(e.ctx, n)
}

func (e *env) podOn(name, node string, phase domain.PodPhase) {
	_ = e.st.CreatePod(e.ctx, domain.Pod{Name: name, UID: "u-" + name,
		Spec: domain.PodSpec{Image: "nginx", RestartPolicy: domain.RestartAlways}, Status: domain.PodStatus{Phase: phase, NodeName: node}})
}

func TestNodeMonitorLifecycle(t *testing.T) {
	e := newEnv()
	e.readyNode("w1", t0)
	e.podOn("web", "w1", domain.PodRunning)

	// Fresh heartbeat: nothing changes.
	e.now = t0.Add(10 * time.Second)
	if err := e.mon.Reconcile(e.ctx); err != nil {
		t.Fatal(err)
	}
	if n, _ := e.st.GetNode(e.ctx, "w1"); !n.Status.Ready {
		t.Fatal("node marked not ready too early")
	}

	// Heartbeat expired: node NotReady, pod Unknown.
	e.now = t0.Add(20 * time.Second)
	if err := e.mon.Reconcile(e.ctx); err != nil {
		t.Fatal(err)
	}
	n, _ := e.st.GetNode(e.ctx, "w1")
	if n.Status.Ready || n.Status.Reason != domain.ReasonHeartbeatTimeout {
		t.Fatalf("node status %+v", n.Status)
	}
	p, _ := e.st.GetPod(e.ctx, "web")
	if p.Status.Phase != domain.PodUnknown || p.Status.Reason != domain.ReasonNodeNotReady {
		t.Fatalf("pod status %+v", p.Status)
	}

	// Past the eviction timeout: pod Failed/NodeLost.
	e.now = t0.Add(20*time.Second + 61*time.Second)
	if err := e.mon.Reconcile(e.ctx); err != nil {
		t.Fatal(err)
	}
	p, _ = e.st.GetPod(e.ctx, "web")
	if p.Status.Phase != domain.PodFailed || p.Status.Reason != domain.ReasonNodeLost {
		t.Fatalf("pod status %+v", p.Status)
	}
}

func TestNodeMonitorForceDeletesTerminatingPodsOnLostNodes(t *testing.T) {
	e := newEnv()
	e.podOn("orphan", "ghost-node", domain.PodTerminating)
	if err := e.mon.Reconcile(e.ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := e.st.GetPod(e.ctx, "orphan"); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("terminating pod on a missing node should be force-deleted")
	}
}

func TestNodeMonitorIgnoresHealthyNodes(t *testing.T) {
	e := newEnv()
	e.readyNode("w1", t0)
	e.podOn("web", "w1", domain.PodRunning)
	e.now = t0.Add(time.Second)
	for i := 0; i < 3; i++ {
		_ = e.mon.Reconcile(e.ctx)
	}
	if p, _ := e.st.GetPod(e.ctx, "web"); p.Status.Phase != domain.PodRunning {
		t.Fatal("pod on a healthy node must not change")
	}
	if len(e.rec.List(events.Filter{})) != 0 {
		t.Fatal("no events expected for a healthy cluster")
	}
}
