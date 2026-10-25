package events

import "testing"

func TestRecorderIsBoundedAndNewestFirst(t *testing.T) {
	r := NewRecorder(3, nil)
	for _, name := range []string{"a", "b", "c", "d"} {
		r.Record("test", TypeNormal, "Pod", name, "Created", "created "+name)
	}
	got := r.List(Filter{})
	if len(got) != 3 {
		t.Fatalf("len=%d, want 3 (capacity)", len(got))
	}
	if got[0].Object.Name != "d" || got[2].Object.Name != "b" {
		t.Fatalf("unexpected order: %+v", got)
	}
	if got[0].ID <= got[1].ID {
		t.Fatal("IDs must increase monotonically")
	}
}

func TestRecorderFilters(t *testing.T) {
	r := NewRecorder(10, nil)
	r.Record("s", TypeNormal, "Pod", "web", "Scheduled", "")
	r.Record("s", TypeWarning, "Node", "w1", "NodeNotReady", "")
	r.Record("s", TypeWarning, "Pod", "web", "ContainerStartFailed", "")
	if n := len(r.List(Filter{Kind: "Pod", Name: "web"})); n != 2 {
		t.Fatalf("kind/name filter: %d", n)
	}
	if n := len(r.List(Filter{Type: TypeWarning})); n != 2 {
		t.Fatalf("type filter: %d", n)
	}
	if n := len(r.List(Filter{Limit: 1})); n != 1 {
		t.Fatalf("limit: %d", n)
	}
}
