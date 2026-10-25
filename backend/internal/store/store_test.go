package store

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"mini-orchestrator/internal/domain"
)

func newTestStore() (*Store, *MemoryKV) {
	kv := NewMemoryKV()
	return New(kv, DefaultPrefix, nil), kv
}

func testPod(name string) domain.Pod {
	return domain.Pod{
		Name:      name,
		UID:       "uid-" + name,
		CreatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		Spec:      domain.PodSpec{Image: "nginx", RestartPolicy: domain.RestartAlways},
		Status:    domain.PodStatus{Phase: domain.PodPending},
	}
}

func TestPodRoundTripAndKeyLayout(t *testing.T) {
	ctx := context.Background()
	s, kv := newTestStore()
	if err := s.CreatePod(ctx, testPod("web")); err != nil {
		t.Fatal(err)
	}
	if _, err := kv.Get(ctx, "/mini-orchestrator/v1/pods/web"); err != nil {
		t.Fatalf("pod not stored under the documented key: %v", err)
	}
	got, err := s.GetPod(ctx, "web")
	if err != nil {
		t.Fatal(err)
	}
	if got.UID != "uid-web" || got.Spec.Image != "nginx" || !got.CreatedAt.Equal(testPod("web").CreatedAt) {
		t.Fatalf("round trip mismatch: %+v", got)
	}
}

func TestCreateDuplicateReturnsAlreadyExists(t *testing.T) {
	ctx := context.Background()
	s, _ := newTestStore()
	if err := s.CreatePod(ctx, testPod("web")); err != nil {
		t.Fatal(err)
	}
	if err := s.CreatePod(ctx, testPod("web")); !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("got %v, want ErrAlreadyExists", err)
	}
}

func TestMissingKeysReturnNotFound(t *testing.T) {
	ctx := context.Background()
	s, _ := newTestStore()
	if _, err := s.GetPod(ctx, "nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("get: %v", err)
	}
	if _, err := s.UpdatePod(ctx, "nope", func(*domain.Pod) error { return nil }); !errors.Is(err, ErrNotFound) {
		t.Fatalf("update: %v", err)
	}
	if err := s.DeletePod(ctx, "nope", nil); !errors.Is(err, ErrNotFound) {
		t.Fatalf("delete: %v", err)
	}
}

func TestMalformedDataIsDistinguishedAndSkippedInLists(t *testing.T) {
	ctx := context.Background()
	s, kv := newTestStore()
	kv.PutRaw("/mini-orchestrator/v1/pods/bad", []byte("{not json"))
	if err := s.CreatePod(ctx, testPod("good")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetPod(ctx, "bad"); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("got %v, want ErrCorrupt", err)
	}
	pods, err := s.ListPods(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(pods) != 1 || pods[0].Name != "good" {
		t.Fatalf("expected only the good pod, got %+v", pods)
	}
}

func TestListIsSortedAndScopedToKind(t *testing.T) {
	ctx := context.Background()
	s, _ := newTestStore()
	for _, n := range []string{"c", "a", "b"} {
		if err := s.CreatePod(ctx, testPod(n)); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.CreateNode(ctx, domain.Node{Name: "a"}); err != nil {
		t.Fatal(err)
	}
	pods, err := s.ListPods(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(pods) != 3 || pods[0].Name != "a" || pods[2].Name != "c" {
		t.Fatalf("unexpected order/contents: %+v", pods)
	}
}

func TestUpdateNoChangeSkipsWrite(t *testing.T) {
	ctx := context.Background()
	s, kv := newTestStore()
	if err := s.CreatePod(ctx, testPod("web")); err != nil {
		t.Fatal(err)
	}
	before, _ := kv.Get(ctx, "/mini-orchestrator/v1/pods/web")
	if _, err := s.UpdatePod(ctx, "web", func(*domain.Pod) error { return ErrNoChange }); err != nil {
		t.Fatal(err)
	}
	after, _ := kv.Get(ctx, "/mini-orchestrator/v1/pods/web")
	if before.Revision != after.Revision {
		t.Fatal("ErrNoChange must not write")
	}
}

func TestUpdateCallbackErrorIsReturned(t *testing.T) {
	ctx := context.Background()
	s, _ := newTestStore()
	_ = s.CreatePod(ctx, testPod("web"))
	boom := errors.New("boom")
	if _, err := s.UpdatePod(ctx, "web", func(*domain.Pod) error { return boom }); !errors.Is(err, boom) {
		t.Fatalf("got %v", err)
	}
}

// Concurrent read-modify-write must not lose increments: CAS + retry.
func TestConcurrentUpdatesDoNotLoseWrites(t *testing.T) {
	ctx := context.Background()
	s, _ := newTestStore()
	_ = s.CreatePod(ctx, testPod("web"))
	const workers = 4
	var wg sync.WaitGroup
	var mu sync.Mutex
	succeeded := 0
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := s.UpdatePod(ctx, "web", func(p *domain.Pod) error {
				p.Status.RestartCount++
				return nil
			})
			if err == nil {
				mu.Lock()
				succeeded++
				mu.Unlock()
			} else if !errors.Is(err, ErrConflict) {
				t.Errorf("unexpected error: %v", err)
			}
		}()
	}
	wg.Wait()
	got, _ := s.GetPod(ctx, "web")
	if got.Status.RestartCount != succeeded {
		t.Fatalf("restartCount=%d but %d updates reported success (lost update)", got.Status.RestartCount, succeeded)
	}
}

func TestMemoryKVRevisionConflict(t *testing.T) {
	ctx := context.Background()
	kv := NewMemoryKV()
	rev, err := kv.Create(ctx, "k", []byte("1"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := kv.Update(ctx, "k", []byte("2"), rev); err != nil {
		t.Fatal(err)
	}
	if _, err := kv.Update(ctx, "k", []byte("3"), rev); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale revision accepted: %v", err)
	}
	if err := kv.Delete(ctx, "k", rev); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale delete accepted: %v", err)
	}
}

func TestDeleteCheckCanVeto(t *testing.T) {
	ctx := context.Background()
	s, _ := newTestStore()
	_ = s.CreatePod(ctx, testPod("web"))
	veto := errors.New("uid mismatch")
	if err := s.DeletePod(ctx, "web", func(p domain.Pod) error { return veto }); !errors.Is(err, veto) {
		t.Fatalf("got %v", err)
	}
	if _, err := s.GetPod(ctx, "web"); err != nil {
		t.Fatal("pod should still exist")
	}
}

func TestDependencyErrorsPropagate(t *testing.T) {
	ctx := context.Background()
	s, kv := newTestStore()
	kv.SetFailure(fmt.Errorf("%w: simulated outage", ErrUnavailable))
	if _, err := s.ListPods(ctx); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("list: %v", err)
	}
	if err := s.Ping(ctx); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("ping: %v", err)
	}
	if err := s.CreatePod(ctx, testPod("x")); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("create: %v", err)
	}
}
