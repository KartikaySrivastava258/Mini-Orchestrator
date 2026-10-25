package runtime

import (
	"context"
	"fmt"
	"sync"
)

// Fake is a pretend Docker that lives in a map. Great for tests, terrible for prod.
type Fake struct {
	mu         sync.Mutex
	seq        int
	containers map[string]*fakeContainer

	// Injectable failures.
	PingErr  error
	RunErr   error
	StartErr error
	ListErr  error

	// Call counters.
	Runs, Starts, Removes int
}

type fakeContainer struct {
	spec  ContainerSpec
	state ContainerState
	logs  string
}

// NewFake returns an empty Fake.
func NewFake() *Fake { return &Fake{containers: map[string]*fakeContainer{}} }

func (f *Fake) Ping(context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.PingErr
}

func (f *Fake) Run(_ context.Context, spec ContainerSpec) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Runs++
	if f.RunErr != nil {
		return "", f.RunErr
	}
	f.seq++
	id := fmt.Sprintf("%064x", f.seq)
	f.containers[id] = &fakeContainer{
		spec:  spec,
		state: ContainerState{ID: id, Status: "running", Running: true, Labels: spec.Labels},
	}
	return id, nil
}

func (f *Fake) Start(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Starts++
	if f.StartErr != nil {
		return f.StartErr
	}
	c, ok := f.containers[id]
	if !ok {
		return ErrNotFound
	}
	c.state.Running = true
	c.state.Status = "running"
	c.state.ExitCode = 0
	return nil
}

func (f *Fake) Inspect(_ context.Context, id string) (ContainerState, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	c, ok := f.containers[id]
	if !ok {
		return ContainerState{}, ErrNotFound
	}
	return c.state, nil
}

func (f *Fake) List(_ context.Context, nodeName string) ([]Container, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.ListErr != nil {
		return nil, f.ListErr
	}
	var out []Container
	for id, c := range f.containers {
		if c.spec.Labels[LabelNode] != nodeName {
			continue
		}
		out = append(out, Container{ID: id, Name: c.spec.Name, PodUID: c.spec.Labels[LabelPodUID], State: c.state.Status})
	}
	return out, nil
}

func (f *Fake) Remove(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Removes++
	delete(f.containers, id)
	return nil
}

func (f *Fake) Logs(_ context.Context, id string, _, maxBytes int) (LogResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	c, ok := f.containers[id]
	if !ok {
		return LogResult{}, ErrNotFound
	}
	if len(c.logs) > maxBytes {
		return LogResult{Text: c.logs[len(c.logs)-maxBytes:], Truncated: true}, nil
	}
	return LogResult{Text: c.logs}, nil
}

// --- test helpers ----------------------------------------------------------

// Exit marks a container as exited with code. A simulated crash, zero real damage.
func (f *Fake) Exit(id string, code int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if c, ok := f.containers[id]; ok {
		c.state.Running = false
		c.state.Status = "exited"
		c.state.ExitCode = code
	}
}

// SetLogs sets a container's log output.
func (f *Fake) SetLogs(id, logs string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if c, ok := f.containers[id]; ok {
		c.logs = logs
	}
}

// AddContainer inserts a container directly (e.g. an orphan).
func (f *Fake) AddContainer(id string, spec ContainerSpec, running bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	status := "exited"
	if running {
		status = "running"
	}
	f.containers[id] = &fakeContainer{spec: spec, state: ContainerState{ID: id, Status: status, Running: running, Labels: spec.Labels}}
}

// IDs returns the IDs of all containers.
func (f *Fake) IDs() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	ids := make([]string, 0, len(f.containers))
	for id := range f.containers {
		ids = append(ids, id)
	}
	return ids
}

// Running reports whether the container exists and is running.
func (f *Fake) Running(id string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	c, ok := f.containers[id]
	return ok && c.state.Running
}

var _ Runtime = (*Fake)(nil)
var _ Runtime = (*Docker)(nil)
