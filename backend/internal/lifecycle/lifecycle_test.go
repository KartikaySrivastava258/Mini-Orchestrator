package lifecycle

import (
	"context"
	"errors"
	"testing"
	"time"

	"mini-orchestrator/internal/domain"
	"mini-orchestrator/internal/store"
)

var t0 = time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

func newStore() *store.Store { return store.New(store.NewMemoryKV(), store.DefaultPrefix, nil) }

func pod(name, node string, phase domain.PodPhase) domain.Pod {
	return domain.Pod{
		Name:      name,
		UID:       "uid-" + name,
		CreatedAt: t0,
		Spec:      domain.PodSpec{Image: "nginx", RestartPolicy: domain.RestartAlways},
		Status:    domain.PodStatus{Phase: phase, NodeName: node},
	}
}

func TestDeleteUnassignedPodIsImmediate(t *testing.T) {
	ctx := context.Background()
	st := newStore()
	_ = st.CreatePod(ctx, pod("web", "", domain.PodPending))
	res, err := RequestPodDeletion(ctx, st, "web", t0)
	if err != nil || res != Deleted {
		t.Fatalf("res=%v err=%v", res, err)
	}
	if _, err := st.GetPod(ctx, "web"); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("pod should be gone")
	}
}

func TestDeleteAssignedPodBecomesTerminatingAndIsIdempotent(t *testing.T) {
	ctx := context.Background()
	st := newStore()
	_ = st.CreateNode(ctx, domain.Node{Name: "w1"})
	_ = st.CreatePod(ctx, pod("web", "w1", domain.PodRunning))
	for i := 0; i < 2; i++ {
		res, err := RequestPodDeletion(ctx, st, "web", t0)
		if err != nil || res != Terminating {
			t.Fatalf("attempt %d: res=%v err=%v", i, res, err)
		}
	}
	got, _ := st.GetPod(ctx, "web")
	if got.Status.Phase != domain.PodTerminating || got.DeletionRequestedAt == nil {
		t.Fatalf("unexpected pod: %+v", got.Status)
	}
}

func TestDeletePodOnMissingNodeIsImmediate(t *testing.T) {
	ctx := context.Background()
	st := newStore()
	_ = st.CreatePod(ctx, pod("web", "gone", domain.PodRunning))
	if res, err := RequestPodDeletion(ctx, st, "web", t0); err != nil || res != Deleted {
		t.Fatalf("res=%v err=%v", res, err)
	}
}

func TestDeleteMissingPod(t *testing.T) {
	if _, err := RequestPodDeletion(context.Background(), newStore(), "nope", t0); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("got %v", err)
	}
}

func TestApplyAgentStatusTransitions(t *testing.T) {
	exit1 := 1
	tests := []struct {
		name    string
		pod     domain.Pod
		upd     domain.PodStatusUpdate
		wantErr error
	}{
		{"scheduled to running", pod("a", "w1", domain.PodScheduled),
			domain.PodStatusUpdate{UID: "uid-a", NodeName: "w1", Phase: domain.PodRunning, ContainerID: "c1"}, nil},
		{"unknown back to running", pod("a", "w1", domain.PodUnknown),
			domain.PodStatusUpdate{UID: "uid-a", NodeName: "w1", Phase: domain.PodRunning}, nil},
		{"running to failed", pod("a", "w1", domain.PodRunning),
			domain.PodStatusUpdate{UID: "uid-a", NodeName: "w1", Phase: domain.PodFailed, ExitCode: &exit1}, nil},
		{"wrong uid", pod("a", "w1", domain.PodRunning),
			domain.PodStatusUpdate{UID: "other", NodeName: "w1", Phase: domain.PodRunning}, ErrRejected},
		{"wrong node", pod("a", "w1", domain.PodRunning),
			domain.PodStatusUpdate{UID: "uid-a", NodeName: "w2", Phase: domain.PodRunning}, ErrRejected},
		{"terminal pods are final", pod("a", "w1", domain.PodFailed),
			domain.PodStatusUpdate{UID: "uid-a", NodeName: "w1", Phase: domain.PodRunning}, ErrRejected},
		{"pending pods are not agent-owned", pod("a", "w1", domain.PodPending),
			domain.PodStatusUpdate{UID: "uid-a", NodeName: "w1", Phase: domain.PodRunning}, ErrRejected},
		{"terminating pods reject updates", pod("a", "w1", domain.PodTerminating),
			domain.PodStatusUpdate{UID: "uid-a", NodeName: "w1", Phase: domain.PodRunning}, ErrRejected},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := tt.pod
			err := ApplyAgentStatus(&p, tt.upd, t0)
			if !errors.Is(err, tt.wantErr) && !(tt.wantErr == nil && err == nil) {
				t.Fatalf("err=%v want %v", err, tt.wantErr)
			}
			if tt.wantErr == nil && p.Status.Phase != tt.upd.Phase {
				t.Fatalf("phase=%s want %s", p.Status.Phase, tt.upd.Phase)
			}
		})
	}
}

func TestApplyAgentStatusDetectsNoChange(t *testing.T) {
	p := pod("a", "w1", domain.PodScheduled)
	u := domain.PodStatusUpdate{UID: "uid-a", NodeName: "w1", Phase: domain.PodRunning, ContainerID: "c1", Reason: domain.ReasonContainerStarted}
	if err := ApplyAgentStatus(&p, u, t0); err != nil {
		t.Fatal(err)
	}
	if p.Status.StartedAt == nil || !p.Status.LastTransitionTime.Equal(t0) {
		t.Fatalf("timestamps not set: %+v", p.Status)
	}
	if err := ApplyAgentStatus(&p, u, t0.Add(time.Minute)); !errors.Is(err, store.ErrNoChange) {
		t.Fatalf("repeat report should be ErrNoChange, got %v", err)
	}
}

func TestApplyHeartbeat(t *testing.T) {
	n := domain.Node{Name: "w1"}
	if !ApplyHeartbeat(&n, true, "", t0) || !n.Status.Ready || n.Status.Reason != domain.ReasonNodeReady {
		t.Fatalf("expected transition to Ready: %+v", n.Status)
	}
	if ApplyHeartbeat(&n, true, "", t0.Add(time.Second)) {
		t.Fatal("second Ready heartbeat must not count as a transition")
	}
	if !ApplyHeartbeat(&n, false, "docker down", t0.Add(2*time.Second)) || n.Status.Ready ||
		n.Status.Reason != domain.ReasonRuntimeUnavailable || n.Status.Message != "docker down" {
		t.Fatalf("expected RuntimeUnavailable: %+v", n.Status)
	}
	if !n.Status.LastHeartbeatTime.Equal(t0.Add(2 * time.Second)) {
		t.Fatal("heartbeat time not recorded")
	}
}
