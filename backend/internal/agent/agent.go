// Package agent is the node agent. It registers a node, keeps heartbeating,
// runs whatever the scheduler assigned here, reports how it's going and serves
// logs.
//
// Think kubelet, but pocket-sized: it polls the API instead of watching,
// drives the Docker CLI through package runtime, and does not speak CRI.
package agent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"mini-orchestrator/internal/domain"
	"mini-orchestrator/internal/loop"
	"mini-orchestrator/internal/runtime"
)

// Config holds the agent's behaviour settings.
type Config struct {
	NodeName           string
	AdvertiseURL       string
	MaxPods            int
	HeartbeatInterval  time.Duration
	SyncInterval       time.Duration
	MaxRestarts        int
	RestartBackoffBase time.Duration
	RestartBackoffMax  time.Duration
}

// Agent keeps this node's containers in sync with the pods assigned to it.
type Agent struct {
	cfg Config
	cp  ControlPlane
	rt  runtime.Runtime
	log *slog.Logger
	now func() time.Time

	// nextRestart is only touched by the sync loop (a single goroutine);
	// the mutex makes that guarantee explicit for future callers.
	mu          sync.Mutex
	nextRestart map[string]time.Time // pod UID -> earliest next restart
}

// New returns an Agent.
func New(cfg Config, cp ControlPlane, rt runtime.Runtime, log *slog.Logger) *Agent {
	return &Agent{cfg: cfg, cp: cp, rt: rt, log: log, now: func() time.Time { return time.Now().UTC() }, nextRestart: map[string]time.Time{}}
}

// SetClock swaps the time source for tests.
func (a *Agent) SetClock(now func() time.Time) { a.now = now }

// Run registers the node, then runs the heartbeat and sync loops until ctx is
// cancelled. It only returns once both loops have stopped.
func (a *Agent) Run(ctx context.Context) error {
	if err := a.registerWithRetry(ctx); err != nil {
		return err
	}
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		// heartbeats never back off: show up late and the node gets marked stale
		loop.Run(ctx, "heartbeat", a.cfg.HeartbeatInterval, a.cfg.HeartbeatInterval, nil, a.log, a.HeartbeatOnce)
	}()
	go func() {
		defer wg.Done()
		loop.Run(ctx, "sync", a.cfg.SyncInterval, 30*time.Second, nil, a.log, a.SyncOnce)
	}()
	wg.Wait()
	return nil
}

func (a *Agent) registrationRequest(ctx context.Context) domain.RegisterNodeRequest {
	ready, msg := a.runtimeHealth(ctx)
	return domain.RegisterNodeRequest{
		Name:           a.cfg.NodeName,
		Address:        a.cfg.AdvertiseURL,
		MaxPods:        a.cfg.MaxPods,
		RuntimeReady:   ready,
		RuntimeMessage: msg,
	}
}

func (a *Agent) runtimeHealth(ctx context.Context) (bool, string) {
	if err := a.rt.Ping(ctx); err != nil {
		return false, domain.Truncate(err.Error(), domain.MaxMessageLength)
	}
	return true, ""
}

func (a *Agent) registerWithRetry(ctx context.Context) error {
	for attempt := 1; ; attempt++ {
		err := a.cp.Register(ctx, a.registrationRequest(ctx))
		if err == nil {
			a.log.Info("node registered", "node", a.cfg.NodeName, "advertise_url", a.cfg.AdvertiseURL, "max_pods", a.cfg.MaxPods)
			return nil
		}
		wait := loop.Backoff(time.Second, 30*time.Second, attempt)
		a.log.Warn("registration failed; retrying", "error", err, "attempt", attempt, "retry_in", wait.String())
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(wait):
		}
	}
}

// HeartbeatOnce says "still here" and reports whether Docker answers. If the
// control plane forgot about us (404), we just register again. No hard
// feelings.
func (a *Agent) HeartbeatOnce(ctx context.Context) error {
	ready, msg := a.runtimeHealth(ctx)
	err := a.cp.Heartbeat(ctx, a.cfg.NodeName, domain.HeartbeatRequest{RuntimeReady: ready, RuntimeMessage: msg})
	if errors.Is(err, ErrNotFound) {
		a.log.Warn("control plane does not know this node; registering again")
		return a.cp.Register(ctx, a.registrationRequest(ctx))
	}
	if err != nil {
		return fmt.Errorf("heartbeat: %w", err)
	}
	if !ready {
		a.log.Warn("container runtime unavailable", "detail", msg)
	}
	return nil
}

// SyncOnce does one reconcile pass for this node. Orphan cleanup only runs when
// both the pod list and the container list loaded fine, so a flaky API can
// never trick us into deleting running work.
func (a *Agent) SyncOnce(ctx context.Context) error {
	pods, err := a.cp.ListPods(ctx, a.cfg.NodeName)
	if err != nil {
		return fmt.Errorf("list assigned pods: %w", err)
	}
	containers, err := a.rt.List(ctx, a.cfg.NodeName)
	if err != nil {
		return fmt.Errorf("list containers: %w", err)
	}

	var errs []error
	byUID := map[string]runtime.Container{}
	for _, c := range containers {
		if _, dup := byUID[c.PodUID]; dup || c.PodUID == "" {
			a.log.Warn("removing duplicate or unlabelled container", "container", c.ID, "pod_uid", c.PodUID)
			if err := a.rt.Remove(ctx, c.ID); err != nil {
				errs = append(errs, err)
			}
			continue
		}
		byUID[c.PodUID] = c
	}

	known := map[string]bool{}
	for _, p := range pods {
		if p.Status.NodeName != a.cfg.NodeName {
			continue
		}
		known[p.UID] = true
		c, has := byUID[p.UID]
		if err := a.syncPod(ctx, p, c, has); err != nil {
			errs = append(errs, fmt.Errorf("pod %s: %w", p.Name, err))
		}
	}

	for uid, c := range byUID {
		if known[uid] {
			continue
		}
		a.log.Info("removing orphaned container", "container", c.ID, "name", c.Name, "pod_uid", uid)
		if err := a.rt.Remove(ctx, c.ID); err != nil {
			errs = append(errs, fmt.Errorf("remove orphan %s: %w", c.Name, err))
		}
	}

	a.mu.Lock()
	for uid := range a.nextRestart {
		if !known[uid] {
			delete(a.nextRestart, uid)
		}
	}
	a.mu.Unlock()
	return errors.Join(errs...)
}

func (a *Agent) syncPod(ctx context.Context, p domain.Pod, c runtime.Container, has bool) error {
	switch {
	case p.IsTerminating():
		if has {
			if err := a.rt.Remove(ctx, c.ID); err != nil {
				return fmt.Errorf("remove container: %w", err)
			}
		}
		err := a.cp.FinalizePod(ctx, p.Name, domain.FinalizePodRequest{UID: p.UID, NodeName: a.cfg.NodeName})
		if err != nil && !errors.Is(err, ErrNotFound) && !errors.Is(err, ErrConflict) {
			return fmt.Errorf("finalize: %w", err)
		}
		return nil

	case p.Status.Phase.IsTerminal():
		// Keep exited containers for their logs; stop anything still running
		// (e.g. after the control plane declared this node lost).
		if has && c.State == "running" {
			a.log.Info("stopping container of a terminal pod", "pod", p.Name, "phase", p.Status.Phase)
			return a.rt.Remove(ctx, c.ID)
		}
		return nil

	case p.Status.Phase == domain.PodScheduled, p.Status.Phase == domain.PodRunning, p.Status.Phase == domain.PodUnknown:
		if !has {
			return a.handleMissing(ctx, p)
		}
		st, err := a.rt.Inspect(ctx, c.ID)
		if errors.Is(err, runtime.ErrNotFound) {
			return a.handleMissing(ctx, p)
		}
		if err != nil {
			return fmt.Errorf("inspect container: %w", err)
		}
		if st.Running {
			reason, msg := domain.ReasonContainerStarted, "container is running"
			if p.Status.ContainerID == c.ID && p.Status.Phase == domain.PodRunning && p.Status.Reason != domain.ReasonRestartBackOff && p.Status.Reason != domain.ReasonRestartFailed {
				reason, msg = p.Status.Reason, p.Status.Message // keep e.g. ContainerRestarted
			}
			return a.report(ctx, p, domain.PodRunning, reason, msg, c.ID, p.Status.RestartCount, nil)
		}
		return a.handleExited(ctx, p, c, st)
	}
	return nil
}

// handleMissing starts a container for a pod that doesn't have one.
func (a *Agent) handleMissing(ctx context.Context, p domain.Pod) error {
	if p.Status.ContainerID == "" {
		return a.start(ctx, p, p.Status.RestartCount, "container started")
	}
	// it existed before and now it's gone; someone probably docker rm'd it
	if p.Spec.RestartPolicy == domain.RestartNever {
		return a.report(ctx, p, domain.PodFailed, domain.ReasonContainerMissing, "the container was removed outside the orchestrator", "", p.Status.RestartCount, nil)
	}
	if p.Status.RestartCount >= a.cfg.MaxRestarts {
		return a.report(ctx, p, domain.PodFailed, domain.ReasonRestartLimitExceeded,
			fmt.Sprintf("the container disappeared and the restart limit (%d) was reached", a.cfg.MaxRestarts), "", p.Status.RestartCount, nil)
	}
	return a.start(ctx, p, p.Status.RestartCount+1, "the container was missing and has been recreated")
}

func (a *Agent) start(ctx context.Context, p domain.Pod, restarts int, msg string) error {
	id, err := a.rt.Run(ctx, runtime.ContainerSpec{
		Name:   runtime.ContainerName(p.Name, p.UID),
		Image:  p.Spec.Image,
		Labels: runtime.PodLabels(a.cfg.NodeName, p.Name, p.UID),
	})
	if err != nil {
		if errors.Is(err, runtime.ErrUnavailable) {
			// Do not fail the pod because Docker is down; heartbeats report the
			// node as RuntimeUnavailable and the node monitor handles it.
			return fmt.Errorf("start container: %w", err)
		}
		reason := domain.ReasonContainerStartFailed
		if errors.Is(err, runtime.ErrImagePull) {
			reason = domain.ReasonImagePullFailed
		}
		return a.report(ctx, p, domain.PodFailed, reason, err.Error(), "", restarts, nil)
	}
	a.log.Info("container started", "pod", p.Name, "container", id, "image", p.Spec.Image)
	return a.report(ctx, p, domain.PodRunning, domain.ReasonContainerStarted, msg, id, restarts, nil)
}

func (a *Agent) handleExited(ctx context.Context, p domain.Pod, c runtime.Container, st runtime.ContainerState) error {
	exit := st.ExitCode
	if p.Spec.RestartPolicy == domain.RestartNever {
		if exit == 0 {
			return a.report(ctx, p, domain.PodSucceeded, domain.ReasonCompleted, "container exited with code 0", c.ID, p.Status.RestartCount, &exit)
		}
		return a.report(ctx, p, domain.PodFailed, domain.ReasonContainerExited, fmt.Sprintf("container exited with code %d", exit), c.ID, p.Status.RestartCount, &exit)
	}
	if p.Status.RestartCount >= a.cfg.MaxRestarts {
		return a.report(ctx, p, domain.PodFailed, domain.ReasonRestartLimitExceeded,
			fmt.Sprintf("container exited with code %d; restart limit (%d) reached", exit, a.cfg.MaxRestarts), c.ID, p.Status.RestartCount, &exit)
	}

	now := a.now()
	a.mu.Lock()
	next, waiting := a.nextRestart[p.UID]
	a.mu.Unlock()
	if waiting && now.Before(next) {
		// phase stays Running, like Kubernetes during CrashLoopBackOff; the reason tells the real story
		return a.report(ctx, p, domain.PodRunning, domain.ReasonRestartBackOff,
			fmt.Sprintf("container exited with code %d; next restart attempt after %s", exit, next.Format(time.RFC3339)),
			c.ID, p.Status.RestartCount, &exit)
	}

	restarts := p.Status.RestartCount + 1
	a.mu.Lock()
	a.nextRestart[p.UID] = now.Add(loop.Backoff(a.cfg.RestartBackoffBase, a.cfg.RestartBackoffMax, restarts))
	a.mu.Unlock()
	if err := a.rt.Start(ctx, c.ID); err != nil {
		if errors.Is(err, runtime.ErrUnavailable) {
			return fmt.Errorf("restart container: %w", err)
		}
		return a.report(ctx, p, domain.PodRunning, domain.ReasonRestartFailed, err.Error(), c.ID, restarts, &exit)
	}
	a.log.Info("container restarted", "pod", p.Name, "exit_code", exit, "restart", restarts)
	return a.report(ctx, p, domain.PodRunning, domain.ReasonContainerRestarted,
		fmt.Sprintf("container restarted after exiting with code %d (restart %d of at most %d)", exit, restarts, a.cfg.MaxRestarts),
		c.ID, restarts, nil)
}

// report sends a status update unless nothing changed. A 409 means the pod
// changed on the server (say, it's being deleted); the next sync pass deals
// with it.
func (a *Agent) report(ctx context.Context, p domain.Pod, phase domain.PodPhase, reason, msg, containerID string, restarts int, exit *int) error {
	msg = domain.Truncate(msg, domain.MaxMessageLength)
	s := p.Status
	if s.Phase == phase && s.Reason == reason && s.Message == msg && s.ContainerID == containerID &&
		s.RestartCount == restarts && sameExit(s.ExitCode, exit) {
		return nil
	}
	err := a.cp.UpdatePodStatus(ctx, p.Name, domain.PodStatusUpdate{
		UID:          p.UID,
		NodeName:     a.cfg.NodeName,
		Phase:        phase,
		Reason:       reason,
		Message:      msg,
		ContainerID:  containerID,
		RestartCount: restarts,
		ExitCode:     exit,
	})
	if errors.Is(err, ErrConflict) || errors.Is(err, ErrNotFound) {
		a.log.Debug("status update rejected; pod changed on the server", "pod", p.Name, "error", err)
		return nil
	}
	if err != nil {
		return fmt.Errorf("report status: %w", err)
	}
	return nil
}

func sameExit(a, b *int) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}
