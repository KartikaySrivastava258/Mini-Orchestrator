package runtime

import (
	"errors"
	"os/exec"
	"strings"
	"testing"
)

func TestParsePS(t *testing.T) {
	out := "abc123\tmo-web-1234\tuid-1\trunning\n\n" +
		"def456\tmo-api-5678\tuid-2\texited\r\n" +
		"malformed line\n"
	got := parsePS(out)
	if len(got) != 2 {
		t.Fatalf("got %d containers: %+v", len(got), got)
	}
	if got[0].ID != "abc123" || got[0].PodUID != "uid-1" || got[0].State != "running" {
		t.Fatalf("bad first entry: %+v", got[0])
	}
	if got[1].State != "exited" {
		t.Fatalf("CR not trimmed: %+v", got[1])
	}
}

func TestParseInspect(t *testing.T) {
	out := []byte(`[{"Id":"abc","State":{"Status":"exited","Running":false,"ExitCode":3,"Error":"",
	"StartedAt":"2026-01-02T03:04:05.123456789Z","FinishedAt":"0001-01-01T00:00:00Z"},
	"Config":{"Labels":{"io.mini-orchestrator.node":"worker-1"}}}]`)
	st, err := parseInspect(out)
	if err != nil {
		t.Fatal(err)
	}
	if st.ID != "abc" || st.Running || st.ExitCode != 3 || st.Labels[LabelNode] != "worker-1" {
		t.Fatalf("unexpected state: %+v", st)
	}
	if st.StartedAt.Year() != 2026 {
		t.Fatalf("StartedAt not parsed: %v", st.StartedAt)
	}
	if _, err := parseInspect([]byte(`[]`)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("empty inspect should be ErrNotFound, got %v", err)
	}
	if _, err := parseInspect([]byte(`not json`)); err == nil {
		t.Fatal("expected decode error")
	}
}

func TestClassify(t *testing.T) {
	runErr := errors.New("exit status 1")
	cases := []struct {
		stderr string
		want   error
	}{
		{"Cannot connect to the Docker daemon at unix:///var/run/docker.sock. Is the docker daemon running?", ErrUnavailable},
		{"Error response from daemon: No such container: abc", ErrNotFound},
		{"Error: No such object: abc", ErrNotFound},
		{"Unable to find image 'nope:1' locally\ndocker: Error response from daemon: pull access denied for nope", ErrImagePull},
		{"docker: Error response from daemon: manifest unknown", ErrImagePull},
	}
	for _, c := range cases {
		if got := classify(runErr, c.stderr, nil); !errors.Is(got, c.want) {
			t.Errorf("classify(%q) = %v, want %v", c.stderr, got, c.want)
		}
	}
	// A start failure after a successful pull must not be reported as a pull failure.
	other := classify(runErr, "Unable to find image 'x' locally\ndocker: Error response from daemon: failed to create task: exec format error", nil)
	if errors.Is(other, ErrImagePull) || errors.Is(other, ErrUnavailable) {
		t.Fatalf("misclassified start failure: %v", other)
	}
	if got := classify(&exec.Error{Name: "docker", Err: exec.ErrNotFound}, "", nil); !errors.Is(got, ErrUnavailable) {
		t.Fatalf("missing binary should be ErrUnavailable: %v", got)
	}
}

func TestLimitedAndTailWriters(t *testing.T) {
	lw := &limitedWriter{max: 5}
	_, _ = lw.Write([]byte("abc"))
	_, _ = lw.Write([]byte("defg"))
	if lw.String() != "abcde" || !lw.truncated {
		t.Fatalf("limitedWriter: %q truncated=%v", lw.String(), lw.truncated)
	}
	tw := &tailWriter{max: 5}
	_, _ = tw.Write([]byte("line1\n"))
	_, _ = tw.Write([]byte("line2\n"))
	if !strings.HasSuffix(tw.String(), "2\n") || len(tw.String()) != 5 || !tw.truncated {
		t.Fatalf("tailWriter kept %q", tw.String())
	}
}

func TestContainerNameAndLabels(t *testing.T) {
	if got := ContainerName("web", "0123456789abcdef"); got != "mo-web-01234567" {
		t.Fatalf("ContainerName = %q", got)
	}
	l := PodLabels("worker-1", "web", "uid")
	if l[LabelNode] != "worker-1" || l[LabelPodUID] != "uid" || l[LabelManaged] != "true" {
		t.Fatalf("labels: %+v", l)
	}
}
