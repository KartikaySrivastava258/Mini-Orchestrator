package agent

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"mini-orchestrator/internal/domain"
	"mini-orchestrator/internal/lifecycle"
	"mini-orchestrator/internal/runtime"
	"mini-orchestrator/internal/store"
)

const node = "worker-1"

var t0 = time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

// fakeCP is an in-memory control plane that applies the same transition
// rules as the real API server (lifecycle.ApplyAgentStatus).
type fakeCP struct {
	mu         sync.Mutex
	pods       map[string]domain.Pod
	registered int
	heartbeats int
	nodeKnown  bool
	listErr    error
}

func newFakeCP() *fakeCP { return &fakeCP{pods: map[string]domain.Pod{}, nodeKnown: true} }

func (f *fakeCP) Register(context.Context, domain.RegisterNodeRequest) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.registered++
	f.nodeKnown = true
	return nil
}

func (f *fakeCP) Heartbeat(context.Context, string, domain.HeartbeatRequest) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.nodeKnown {
		return &APIError{Status: http.StatusNotFound}
	}
	f.heartbeats++
	return nil
}

func (f *fakeCP) ListPods(_ context.Context, n string) ([]domain.Pod, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.listErr != nil {
		return nil, f.listErr
	}
	var out []domain.Pod
	for _, p := range f.pods {
		if p.Status.NodeName == n {
			out = append(out, p)
		}
	}
	return out, nil
}

func (f *fakeCP) UpdatePodStatus(_ context.Context, name string, u domain.PodStatusUpdate) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	p, ok := f.pods[name]
	if !ok {
		return &APIError{Status: http.StatusNotFound}
	}
	if err := lifecycle.ApplyAgentStatus(&p, u, t0); err != nil {
		if errors.Is(err, store.ErrNoChange) {
			return nil
		}
		return &APIError{Status: http.StatusConflict, Message: err.Error()}
	}
	f.pods[name] = p
	return nil
}

func (f *fakeCP) FinalizePod(_ context.Context, name string, req domain.FinalizePodRequest) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	p, ok := f.pods[name]
	if !ok {
		return &APIError{Status: http.StatusNotFound}
	}
	if p.UID != req.UID || !p.IsTerminating() {
		return &APIError{Status: http.StatusConflict}
	}
	delete(f.pods, name)
	return nil
}

func (f *fakeCP) put(p domain.Pod) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pods[p.Name] = p
}

func (f *fakeCP) get(name string) (domain.Pod, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	p, ok := f.pods[name]
	return p, ok
}

func scheduledPod(name string, policy domain.RestartPolicy) domain.Pod {
	return domain.Pod{Name: name, UID: "uid-" + name + "-0123456789",
		Spec:   domain.PodSpec{Image: "nginx:stable", RestartPolicy: policy},
		Status: domain.PodStatus{Phase: domain.PodScheduled, NodeName: node}}
}

type harness struct {
	cp  *fakeCP
	rt  *runtime.Fake
	a   *Agent
	now time.Time
	ctx context.Context
}

func newHarness() *harness {
	h := &harness{cp: newFakeCP(), rt: runtime.NewFake(), now: t0, ctx: context.Background()}
	cfg := Config{
		NodeName:           node,
		AdvertiseURL:       "http://127.0.0.1:9101",
		MaxPods:            10,
		HeartbeatInterval:  time.Second,
		SyncInterval:       time.Second,
		MaxRestarts:        2,
		RestartBackoffBase: 10 * time.Second,
		RestartBackoffMax:  time.Minute,
	}
	h.a = New(cfg, h.cp, h.rt, slog.New(slog.DiscardHandler))
	h.a.SetClock(func() time.Time { return h.now })
	return h
}

func (h *harness) sync(t *testing.T) {
	t.Helper()
	if err := h.a.SyncOnce(h.ctx); err != nil {
		t.Fatalf("sync: %v", err)
	}
}

func (h *harness) pod(t *testing.T, name string) domain.Pod {
	t.Helper()
	p, ok := h.cp.get(name)
	if !ok {
		t.Fatalf("pod %s not found", name)
	}
	return p
}

func TestScheduledPodIsStartedAndReportedRunning(t *testing.T) {
	h := newHarness()
	h.cp.put(scheduledPod("web", domain.RestartAlways))
	h.sync(t)
	p := h.pod(t, "web")
	if p.Status.Phase != domain.PodRunning || p.Status.ContainerID == "" || !h.rt.Running(p.Status.ContainerID) {
		t.Fatalf("unexpected status %+v", p.Status)
	}
	// Idempotent: further passes neither start another container nor change status.
	h.sync(t)
	h.sync(t)
	if h.rt.Runs != 1 {
		t.Fatalf("container started %d times", h.rt.Runs)
	}
}

func TestStartFailuresAreClassified(t *testing.T) {
	h := newHarness()
	h.rt.RunErr = errors.Join(runtime.ErrImagePull, errors.New("pull access denied"))
	h.cp.put(scheduledPod("web", domain.RestartAlways))
	h.sync(t)
	if p := h.pod(t, "web"); p.Status.Phase != domain.PodFailed || p.Status.Reason != domain.ReasonImagePullFailed {
		t.Fatalf("status %+v", p.Status)
	}
}

func TestRuntimeOutageDoesNotFailPods(t *testing.T) {
	h := newHarness()
	h.rt.RunErr = errors.Join(runtime.ErrUnavailable, errors.New("daemon down"))
	h.cp.put(scheduledPod("web", domain.RestartAlways))
	if err := h.a.SyncOnce(h.ctx); err == nil {
		t.Fatal("expected an error so the loop backs off")
	}
	if p := h.pod(t, "web"); p.Status.Phase != domain.PodScheduled {
		t.Fatalf("pod should stay Scheduled during a runtime outage, got %s", p.Status.Phase)
	}
}

func TestRestartPolicyNever(t *testing.T) {
	for _, tc := range []struct {
		exit   int
		phase  domain.PodPhase
		reason string
	}{{0, domain.PodSucceeded, domain.ReasonCompleted}, {3, domain.PodFailed, domain.ReasonContainerExited}} {
		h := newHarness()
		h.cp.put(scheduledPod("job", domain.RestartNever))
		h.sync(t)
		h.rt.Exit(h.pod(t, "job").Status.ContainerID, tc.exit)
		h.sync(t)
		p := h.pod(t, "job")
		if p.Status.Phase != tc.phase || p.Status.Reason != tc.reason || p.Status.ExitCode == nil || *p.Status.ExitCode != tc.exit {
			t.Fatalf("exit %d: status %+v", tc.exit, p.Status)
		}
	}
}

func TestRestartWithBackoffThenLimit(t *testing.T) {
	h := newHarness()
	h.cp.put(scheduledPod("web", domain.RestartAlways))
	h.sync(t)
	id := h.pod(t, "web").Status.ContainerID

	h.rt.Exit(id, 1)
	h.sync(t) // first restart is immediate
	p := h.pod(t, "web")
	if p.Status.RestartCount != 1 || p.Status.Reason != domain.ReasonContainerRestarted || !h.rt.Running(id) {
		t.Fatalf("after first crash: %+v", p.Status)
	}

	h.rt.Exit(id, 1)
	h.now = t0.Add(5 * time.Second) // inside the 10s backoff window
	h.sync(t)
	p = h.pod(t, "web")
	if p.Status.Reason != domain.ReasonRestartBackOff || h.rt.Running(id) {
		t.Fatalf("expected back-off: %+v", p.Status)
	}

	h.now = t0.Add(11 * time.Second)
	h.sync(t)
	if p = h.pod(t, "web"); p.Status.RestartCount != 2 {
		t.Fatalf("expected second restart: %+v", p.Status)
	}

	h.rt.Exit(id, 1)
	h.now = t0.Add(time.Hour)
	h.sync(t)
	p = h.pod(t, "web")
	if p.Status.Phase != domain.PodFailed || p.Status.Reason != domain.ReasonRestartLimitExceeded {
		t.Fatalf("expected restart limit: %+v", p.Status)
	}
}

func TestTerminatingPodIsRemovedAndFinalized(t *testing.T) {
	h := newHarness()
	h.cp.put(scheduledPod("web", domain.RestartAlways))
	h.sync(t)
	p := h.pod(t, "web")
	ts := t0
	p.DeletionRequestedAt = &ts
	p.Status.Phase = domain.PodTerminating
	h.cp.put(p)
	h.sync(t)
	if _, ok := h.cp.get("web"); ok {
		t.Fatal("pod should be finalized")
	}
	if len(h.rt.IDs()) != 0 {
		t.Fatal("container should be removed")
	}
}

func TestOrphanCleanupAndListFailureSafety(t *testing.T) {
	h := newHarness()
	orphan := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	h.rt.AddContainer(orphan, runtime.ContainerSpec{Name: "mo-old-x", Labels: runtime.PodLabels(node, "old", "gone-uid")}, true)
	other := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	h.rt.AddContainer(other, runtime.ContainerSpec{Name: "mo-x", Labels: runtime.PodLabels("another-node", "x", "uid-x")}, true)

	h.cp.listErr = errors.New("api down")
	if err := h.a.SyncOnce(h.ctx); err == nil {
		t.Fatal("expected error")
	}
	if !h.rt.Running(orphan) {
		t.Fatal("a failed pod list must never trigger orphan cleanup")
	}

	h.cp.listErr = nil
	h.sync(t)
	if h.rt.Running(orphan) {
		t.Fatal("orphaned container should be removed")
	}
	if !h.rt.Running(other) {
		t.Fatal("containers of other nodes must not be touched")
	}
}

func TestStaleAgentReportIsIgnoredGracefully(t *testing.T) {
	h := newHarness()
	p := scheduledPod("web", domain.RestartAlways)
	p.Status.NodeName = node
	h.cp.put(p)
	// The server-side pod is replaced (new UID) after the agent listed it.
	if err := h.a.report(h.ctx, p, domain.PodRunning, domain.ReasonContainerStarted, "x", "", 0, nil); err != nil {
		t.Fatal(err)
	}
	q := h.pod(t, "web")
	q.UID = "different"
	h.cp.put(q)
	if err := h.a.report(h.ctx, p, domain.PodFailed, domain.ReasonContainerExited, "y", "", 0, nil); err != nil {
		t.Fatalf("409 must be treated as 'pod changed', got %v", err)
	}
}

func TestHeartbeatReRegistersUnknownNode(t *testing.T) {
	h := newHarness()
	h.cp.nodeKnown = false
	if err := h.a.HeartbeatOnce(h.ctx); err != nil {
		t.Fatal(err)
	}
	if h.cp.registered != 1 {
		t.Fatalf("registered=%d", h.cp.registered)
	}
}

func TestRunStopsOnCancel(t *testing.T) {
	h := newHarness()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- h.a.Run(ctx) }()
	time.Sleep(50 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("agent did not stop")
	}
}

func TestLogHandler(t *testing.T) {
	h := newHarness()
	h.cp.put(scheduledPod("web", domain.RestartAlways))
	h.sync(t)
	id := h.pod(t, "web").Status.ContainerID
	h.rt.SetLogs(id, "hello\nworld\n")
	srv := httptest.NewServer(h.a.LogHandler(100, 6))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/v1/containers/" + id + "/logs?tailLines=10")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var lr domain.LogsResponse
	if err := json.NewDecoder(resp.Body).Decode(&lr); err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 || lr.Logs != "world\n" || !lr.Truncated || lr.Mode != "snapshot" {
		t.Fatalf("status=%d body=%+v", resp.StatusCode, lr)
	}

	unknown := "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	cases := []struct {
		path string
		want int
	}{
		{"/v1/containers/not-an-id/logs", http.StatusBadRequest},
		{"/v1/containers/" + id + "/logs?tailLines=1000", http.StatusBadRequest},
		{"/v1/containers/" + unknown + "/logs", http.StatusNotFound},
	}
	for _, c := range cases {
		path, want := c.path, c.want
		r, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		r.Body.Close()
		if r.StatusCode != want {
			t.Errorf("%s: status %d, want %d", path, r.StatusCode, want)
		}
	}
}
