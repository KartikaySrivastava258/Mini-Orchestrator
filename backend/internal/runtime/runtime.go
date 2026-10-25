// Package runtime is the border between the node agent and the container
// engine. The agent only knows the Runtime interface: Docker implements it
// with the Docker CLI, and Fake implements it in memory for tests.
//
// This is not Kubernetes' CRI. It's much smaller and way less ambitious.
package runtime

import (
	"context"
	"errors"
	"time"
)

// Labels we stamp on every container we start. They let the agent find its
// own containers after a restart and spot orphans.
const (
	LabelManaged = "io.mini-orchestrator.managed"
	LabelNode    = "io.mini-orchestrator.node"
	LabelPodName = "io.mini-orchestrator.pod-name"
	LabelPodUID  = "io.mini-orchestrator.pod-uid"
)

// Classified runtime errors.
var (
	ErrNotFound    = errors.New("container not found")
	ErrUnavailable = errors.New("container runtime unavailable")
	ErrImagePull   = errors.New("image could not be pulled")
)

// ContainerSpec says what to start.
type ContainerSpec struct {
	Name   string
	Image  string
	Labels map[string]string
}

// Container is the short version, from List.
type Container struct {
	ID     string
	Name   string
	PodUID string
	State  string // e.g. "running", "exited", "created"
}

// ContainerState is the detailed version, from Inspect.
type ContainerState struct {
	ID         string
	Status     string
	Running    bool
	ExitCode   int
	Error      string
	StartedAt  time.Time
	FinishedAt time.Time
	Labels     map[string]string
}

// LogResult is a size-capped log snapshot.
type LogResult struct {
	Text      string
	Truncated bool
}

// Runtime manages containers on one machine. Implementations must respect ctx
// deadlines, and Remove must be idempotent: removing something that's already
// gone counts as a win.
type Runtime interface {
	Ping(ctx context.Context) error
	Run(ctx context.Context, spec ContainerSpec) (id string, err error)
	Start(ctx context.Context, id string) error
	Inspect(ctx context.Context, id string) (ContainerState, error)
	// List returns all containers (running or not) labelled for nodeName.
	List(ctx context.Context, nodeName string) ([]Container, error)
	Remove(ctx context.Context, id string) error
	Logs(ctx context.Context, id string, tailLines, maxBytes int) (LogResult, error)
}

// ContainerName builds mo-{pod}-{first 8 chars of the UID}. Re-create a pod
// with the same name and its new container still won't trip over the old one.
func ContainerName(podName, podUID string) string {
	short := podUID
	if len(short) > 8 {
		short = short[:8]
	}
	return "mo-" + podName + "-" + short
}

// PodLabels returns the labels for a pod's container.
func PodLabels(nodeName, podName, podUID string) map[string]string {
	return map[string]string{
		LabelManaged: "true",
		LabelNode:    nodeName,
		LabelPodName: podName,
		LabelPodUID:  podUID,
	}
}
