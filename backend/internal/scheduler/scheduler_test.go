package scheduler

import (
	"context"
	"log/slog"
	"sync"
	"testing"
	"time"

	"mini-orchestrator/internal/domain"
	"mini-orchestrator/internal/events"
	"mini-orchestrator/internal/store"
)

var now = time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

const timeout = 15 * time.Second

func node(name string, ready bool, maxPods int) domain.Node {
	n := domain.Node{Name: name}
	n.Status.Ready = ready
	n.Status.RuntimeReady = ready
	n.Status.MaxPods = maxPods
	n.Status.LastHeartbeatTime = now.Add(-time.Second)
	return n
}

func TestPlace(t *testing.T) {
	cordoned := node("c", true, 10)
	cordoned.Spec.Unschedulable = true
	stale := node("s", true, 10)
	stale.Status.LastHeartbeatTime = now.Add(-time.Minute)

	tests := []struct {
		name       string
		nodes      []domain.Node
		load       map[string]int
		wantNode   string
		wantReason string
	}{
		{"no nodes", nil, nil, "", domain.ReasonNoEligibleNodes},
		{"only not-ready", []domain.Node{node("a", false, 10)}, nil, "", domain.ReasonNoEligibleNodes},
		{"cordoned excluded", []domain.Node{cordoned, node("b", true, 10)}, nil, "b", domain.ReasonScheduled},
		{"stale heartbeat excluded even if marked ready", []domain.Node{stale}, nil, "", domain.ReasonNoEligibleNodes},
		{"least pods wins", []domain.Node{node("a", true, 10), node("b", true, 10)}, map[string]int{"a": 3, "b": 1}, "b", domain.ReasonScheduled},
		{"tie broken by name", []domain.Node{node("b", true, 10), node("a", true, 10)}, map[string]int{"a": 2, "b": 2}, "a", domain.ReasonScheduled},
		{"full nodes skipped", []domain.Node{node("a", true, 2), node("b", true, 5)}, map[string]int{"a": 2, "b": 4}, "b", domain.ReasonScheduled},
		{"all full", []domain.Node{node("a", true, 1)}, map[string]int{"a": 1}, "", domain.ReasonInsufficientCapacity},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := Place(tt.nodes, tt.load, now, timeout)
			if d.NodeName != tt.wantNode || d.Reason != tt.wantReason {
				t.Fatalf("got %+v, want node=%q reason=%q", d, tt.wantNode, tt.wantReason)
			}
			if d.Message == "" {
				t.Fatal("decisions must explain themselves")
			}
		})
	}
}

func TestPlaceIsDeterministicRegardlessOfInputOrder(t *testing.T) {
	a := []domain.Node{node("x", true, 5), node("y", true, 5), node("z", true, 5)}
	b := []domain.Node{a[2], a[0], a[1]}
	for i := 0; i < 20; i++ {
		if Place(a, nil, now, timeout).NodeName != Place(b, nil, now, timeout).NodeName {
			t.Fatal("placement depends on input order")
		}
	}
}

type fixture struct {
	st    *store.Store
	sched *Scheduler
	rec   *events.Recorder
}

func newFixture() fixture {
	st := store.New(store.NewMemoryKV(), store.DefaultPrefix, nil)
	rec := events.NewRecorder(100, nil)
	s := New(st, rec, discard(), timeout)
	s.SetClock(func() time.Time { return now })
	return fixture{st: st, sched: s, rec: rec}
}

func pendingPod(name string, created time.Time) domain.Pod {
	return domain.Pod{Name: name, UID: "uid-" + name, CreatedAt: created,
		Spec: domain.PodSpec{Image: "nginx", RestartPolicy: domain.RestartAlways}, Status: domain.PodStatus{Phase: domain.PodPending}}
}

func TestRunOnceSpreadsPodsAndRespectsCapacity(t *testing.T) {
	ctx := context.Background()
	f := newFixture()
	_ = f.st.CreateNode(ctx, node("a", true, 2))
	_ = f.st.CreateNode(ctx, node("b", true, 2))
	for i, n := range []string{"p1", "p2", "p3", "p4", "p5"} {
		_ = f.st.CreatePod(ctx, pendingPod(n, now.Add(time.Duration(i)*time.Second)))
	}
	if err := f.sched.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	pods, _ := f.st.ListPods(ctx)
	perNode := map[string]int{}
	var stillPending []domain.Pod
	for _, p := range pods {
		if p.Status.Phase == domain.PodScheduled {
			perNode[p.Status.NodeName]++
			if p.Status.ScheduledAt == nil {
				t.Fatalf("scheduledAt missing on %s", p.Name)
			}
		} else {
			stillPending = append(stillPending, p)
		}
	}
	if perNode["a"] != 2 || perNode["b"] != 2 {
		t.Fatalf("expected 2 pods per node, got %v", perNode)
	}
	if len(stillPending) != 1 || stillPending[0].Name != "p5" {
		t.Fatalf("oldest-first ordering violated; pending=%+v", stillPending)
	}
	if stillPending[0].Status.Reason != domain.ReasonInsufficientCapacity {
		t.Fatalf("pending reason = %q", stillPending[0].Status.Reason)
	}
}

func TestRunOnceRecordsNoEligibleNodesOnce(t *testing.T) {
	ctx := context.Background()
	f := newFixture()
	_ = f.st.CreatePod(ctx, pendingPod("web", now))
	for i := 0; i < 3; i++ {
		if err := f.sched.RunOnce(ctx); err != nil {
			t.Fatal(err)
		}
	}
	p, _ := f.st.GetPod(ctx, "web")
	if p.Status.Phase != domain.PodPending || p.Status.Reason != domain.ReasonNoEligibleNodes {
		t.Fatalf("unexpected status %+v", p.Status)
	}
	if n := len(f.rec.List(events.Filter{Name: "web"})); n != 1 {
		t.Fatalf("expected exactly one FailedScheduling event, got %d", n)
	}
}

func TestRunOnceIsIdempotent(t *testing.T) {
	ctx := context.Background()
	f := newFixture()
	_ = f.st.CreateNode(ctx, node("a", true, 10))
	_ = f.st.CreatePod(ctx, pendingPod("web", now))
	_ = f.sched.RunOnce(ctx)
	first, _ := f.st.GetPod(ctx, "web")
	_ = f.sched.RunOnce(ctx)
	second, _ := f.st.GetPod(ctx, "web")
	if first.Status.NodeName != second.Status.NodeName || !first.Status.UpdatedAt.Equal(second.Status.UpdatedAt) {
		t.Fatal("second pass must not modify an already-scheduled pod")
	}
}

func TestConcurrentSchedulersNeverDoubleBind(t *testing.T) {
	ctx := context.Background()
	f := newFixture()
	_ = f.st.CreateNode(ctx, node("a", true, 100))
	_ = f.st.CreateNode(ctx, node("b", true, 100))
	_ = f.st.CreatePod(ctx, pendingPod("web", now))
	other := New(f.st, f.rec, discard(), timeout)
	other.SetClock(func() time.Time { return now })

	var wg sync.WaitGroup
	for _, s := range []*Scheduler{f.sched, other} {
		wg.Add(1)
		go func(s *Scheduler) {
			defer wg.Done()
			_ = s.RunOnce(ctx)
		}(s)
	}
	wg.Wait()
	scheduled := 0
	for _, e := range f.rec.List(events.Filter{Name: "web"}) {
		if e.Reason == domain.ReasonScheduled {
			scheduled++
		}
	}
	if scheduled != 1 {
		t.Fatalf("pod bound %d times", scheduled)
	}
}

func TestDeletedPodsAreNotScheduled(t *testing.T) {
	ctx := context.Background()
	f := newFixture()
	_ = f.st.CreateNode(ctx, node("a", true, 10))
	p := pendingPod("web", now)
	ts := now
	p.DeletionRequestedAt = &ts
	_ = f.st.CreatePod(ctx, p)
	_ = f.sched.RunOnce(ctx)
	got, _ := f.st.GetPod(ctx, "web")
	if got.Status.NodeName != "" {
		t.Fatal("a pod marked for deletion must not be scheduled")
	}
}

func discard() *slog.Logger { return slog.New(slog.DiscardHandler) }
