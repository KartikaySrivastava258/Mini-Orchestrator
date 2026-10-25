package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"

	"mini-orchestrator/internal/domain"
)

// Docker implements Runtime with the Docker CLI. No shell involved: args go
// in as an argv slice, so there's nothing to inject into. Images are validated
// before they get here, and containers never run privileged, mount nothing
// from the host and publish no ports.
type Docker struct {
	binary     string
	timeout    time.Duration // default per-command deadline
	runTimeout time.Duration // docker run may pull an image, so it gets extra time
}

// NewDocker returns a Docker runtime that uses the given CLI binary.
func NewDocker(binary string, timeout, runTimeout time.Duration) *Docker {
	return &Docker{binary: binary, timeout: timeout, runTimeout: runTimeout}
}

// maxCLIOutput caps how much CLI output we keep in memory.
const maxCLIOutput = 4 << 20

func (d *Docker) exec(ctx context.Context, timeout time.Duration, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, d.binary, args...)
	stdout := &limitedWriter{max: maxCLIOutput}
	stderr := &limitedWriter{max: 64 << 10}
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		return stdout.Bytes(), classify(err, stderr.String(), ctx.Err())
	}
	return stdout.Bytes(), nil
}

// Ping checks that the CLI can actually reach the daemon.
func (d *Docker) Ping(ctx context.Context) error {
	_, err := d.exec(ctx, d.timeout, "version", "--format", "{{.Server.Version}}")
	return err
}

// Run creates and starts a detached container and returns its ID.
func (d *Docker) Run(ctx context.Context, spec ContainerSpec) (string, error) {
	if err := domain.ValidateImage(spec.Image); err != nil {
		return "", fmt.Errorf("refusing to run invalid image: %w", err)
	}
	args := []string{"run", "--detach", "--name", spec.Name, "--restart", "no"}
	keys := make([]string, 0, len(spec.Labels))
	for k := range spec.Labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		args = append(args, "--label", k+"="+spec.Labels[k])
	}
	args = append(args, spec.Image)

	out, err := d.exec(ctx, d.runTimeout, args...)
	if err != nil {
		return "", err
	}
	id := lastLine(string(out))
	if !domain.ValidContainerID(id) {
		return "", fmt.Errorf("docker run returned an unexpected container ID %q", domain.Truncate(id, 80))
	}
	return id, nil
}

// Start starts an existing, stopped container.
func (d *Docker) Start(ctx context.Context, id string) error {
	_, err := d.exec(ctx, d.timeout, "start", id)
	return err
}

// Inspect returns the container's full state.
func (d *Docker) Inspect(ctx context.Context, id string) (ContainerState, error) {
	out, err := d.exec(ctx, d.timeout, "inspect", "--type", "container", id)
	if err != nil {
		return ContainerState{}, err
	}
	return parseInspect(out)
}

// psFormat makes docker ps print one tab-separated line per container.
// Parsing pretty tables is pain; this is less pain.
const psFormat = "{{.ID}}\t{{.Names}}\t{{.Label \"" + LabelPodUID + "\"}}\t{{.State}}"

// List returns this node's containers, stopped ones included.
func (d *Docker) List(ctx context.Context, nodeName string) ([]Container, error) {
	out, err := d.exec(ctx, d.timeout, "ps", "--all", "--no-trunc",
		"--filter", "label="+LabelNode+"="+nodeName,
		"--format", psFormat)
	if err != nil {
		return nil, err
	}
	return parsePS(string(out)), nil
}

// Remove force-removes a container. Already gone? Cool, that counts as success.
func (d *Docker) Remove(ctx context.Context, id string) error {
	_, err := d.exec(ctx, d.timeout, "rm", "--force", id)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	return err
}

// Logs returns the last tailLines lines of stdout and stderr. If that's more
// than maxBytes, we keep the newest part and set Truncated.
func (d *Docker) Logs(ctx context.Context, id string, tailLines, maxBytes int) (LogResult, error) {
	ctx, cancel := context.WithTimeout(ctx, d.timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, d.binary, "logs", "--tail", strconv.Itoa(tailLines), id)
	// Same writer for both streams: exec guarantees no concurrent Write calls.
	w := &tailWriter{max: maxBytes}
	cmd.Stdout = w
	cmd.Stderr = w
	if err := cmd.Run(); err != nil {
		return LogResult{}, classify(err, w.String(), ctx.Err())
	}
	return LogResult{Text: w.String(), Truncated: w.truncated}, nil
}

// --- parsing and classification (unit tested) ------------------------------

type inspectJSON struct {
	ID    string `json:"Id"`
	State struct {
		Status     string `json:"Status"`
		Running    bool   `json:"Running"`
		ExitCode   int    `json:"ExitCode"`
		Error      string `json:"Error"`
		StartedAt  string `json:"StartedAt"`
		FinishedAt string `json:"FinishedAt"`
	} `json:"State"`
	Config struct {
		Labels map[string]string `json:"Labels"`
	} `json:"Config"`
}

func parseInspect(out []byte) (ContainerState, error) {
	var items []inspectJSON
	if err := json.Unmarshal(out, &items); err != nil {
		return ContainerState{}, fmt.Errorf("decode docker inspect output: %w", err)
	}
	if len(items) == 0 {
		return ContainerState{}, ErrNotFound
	}
	it := items[0]
	st := ContainerState{
		ID:       it.ID,
		Status:   it.State.Status,
		Running:  it.State.Running,
		ExitCode: it.State.ExitCode,
		Error:    it.State.Error,
		Labels:   it.Config.Labels,
	}
	if t, err := time.Parse(time.RFC3339Nano, it.State.StartedAt); err == nil {
		st.StartedAt = t.UTC()
	}
	if t, err := time.Parse(time.RFC3339Nano, it.State.FinishedAt); err == nil {
		st.FinishedAt = t.UTC()
	}
	return st, nil
}

func parsePS(out string) []Container {
	var result []Container
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		f := strings.Split(line, "\t")
		if len(f) != 4 {
			continue
		}
		result = append(result, Container{ID: f[0], Name: f[1], PodUID: f[2], State: f[3]})
	}
	return result
}

// classify turns Docker's error text into something the agent can act on:
// not found, daemon down, or bad image.
func classify(runErr error, stderr string, ctxErr error) error {
	msg := strings.TrimSpace(stderr)
	if msg == "" {
		msg = runErr.Error()
	}
	msg = domain.Truncate(msg, 512)
	lower := strings.ToLower(msg)
	switch {
	case errors.Is(runErr, exec.ErrNotFound):
		return fmt.Errorf("%w: docker CLI not found: %v", ErrUnavailable, runErr)
	case ctxErr != nil:
		return fmt.Errorf("%w: docker command did not finish: %v", ErrUnavailable, ctxErr)
	case containsAny(lower, "cannot connect to the docker daemon", "error during connect", "is the docker daemon running"):
		return fmt.Errorf("%w: %s", ErrUnavailable, msg)
	case containsAny(lower, "no such container", "no such object"):
		return fmt.Errorf("%w: %s", ErrNotFound, msg)
	case containsAny(lower, "pull access denied", "manifest unknown", "repository does not exist",
		"invalid reference format", "failed to resolve reference", "toomanyrequests", "error pulling image"):
		return fmt.Errorf("%w: %s", ErrImagePull, msg)
	default:
		return fmt.Errorf("docker: %s", msg)
	}
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}

// limitedWriter keeps the first max bytes and remembers whether more showed up.
type limitedWriter struct {
	buf       bytes.Buffer
	max       int
	truncated bool
}

func (w *limitedWriter) Write(p []byte) (int, error) {
	remaining := w.max - w.buf.Len()
	if remaining <= 0 {
		if len(p) > 0 {
			w.truncated = true
		}
		return len(p), nil
	}
	if len(p) > remaining {
		w.buf.Write(p[:remaining])
		w.truncated = true
		return len(p), nil
	}
	w.buf.Write(p)
	return len(p), nil
}

func (w *limitedWriter) Bytes() []byte  { return w.buf.Bytes() }
func (w *limitedWriter) String() string { return w.buf.String() }

// tailWriter keeps the last max bytes written to it.
type tailWriter struct {
	buf       bytes.Buffer
	max       int
	truncated bool
}

func (w *tailWriter) Write(p []byte) (int, error) {
	w.buf.Write(p)
	if over := w.buf.Len() - w.max; over > 0 {
		w.buf.Next(over)
		w.truncated = true
	}
	return len(p), nil
}

func (w *tailWriter) String() string { return w.buf.String() }
