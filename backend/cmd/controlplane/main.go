// Command controlplane runs the brains of the operation: the REST API, the
// scheduler, the ReplicaSet controller and the node monitor, all in one
// process backed by etcd.
package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"mini-orchestrator/internal/api"
	"mini-orchestrator/internal/config"
	"mini-orchestrator/internal/controller"
	"mini-orchestrator/internal/events"
	"mini-orchestrator/internal/logging"
	"mini-orchestrator/internal/loop"
	"mini-orchestrator/internal/scheduler"
	"mini-orchestrator/internal/store"
)

// version gets set at build time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "controlplane:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.LoadControlPlane(os.Getenv)
	if err != nil {
		return fmt.Errorf("invalid configuration:\n%w", err)
	}
	log := logging.New(os.Stderr, cfg.LogLevel, cfg.LogFormat).With("component", "control-plane")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	kv, err := store.NewEtcdKV(cfg.EtcdEndpoints, cfg.EtcdDialTimeout, cfg.EtcdOpTimeout)
	if err != nil {
		return err
	}
	st := store.New(kv, cfg.EtcdPrefix, log.With("component", "store"))
	defer func() {
		if err := st.Close(); err != nil {
			log.Warn("closing etcd client failed", "error", err)
		}
	}()

	pingCtx, cancelPing := context.WithTimeout(ctx, cfg.EtcdDialTimeout)
	if err := st.Ping(pingCtx); err != nil {
		log.Warn("etcd is not reachable yet; /readyz reports not ready until it is", "endpoints", cfg.EtcdEndpoints, "error", err)
	} else {
		log.Info("connected to etcd", "endpoints", cfg.EtcdEndpoints, "prefix", cfg.EtcdPrefix)
	}
	cancelPing()

	rec := events.NewRecorder(cfg.EventsCapacity, log.With("component", "events"))
	schedTrigger, rsTrigger := loop.NewTrigger(), loop.NewTrigger()

	sched := scheduler.New(st, rec, log.With("component", "scheduler"), cfg.NodeHeartbeatTimeout)
	rsc := controller.NewReplicaSetController(st, rec, log.With("component", "replicaset-controller"))
	mon := controller.NewNodeMonitor(st, rec, log.With("component", "node-monitor"), cfg.NodeHeartbeatTimeout, cfg.PodEvictionTimeout)

	apiServer := api.New(st, rec, log.With("component", "api"), api.Options{
		Version:              version,
		AllowedOrigins:       cfg.CORSAllowedOrigins,
		MaxBodyBytes:         cfg.MaxBodyBytes,
		AgentRequestTimeout:  cfg.AgentRequestTimeout,
		HeartbeatTimeout:     cfg.NodeHeartbeatTimeout,
		EvictionTimeout:      cfg.PodEvictionTimeout,
		SchedulerPolicy:      scheduler.PolicyName,
		OnPodsChanged:        schedTrigger.Fire,
		OnReplicaSetsChanged: rsTrigger.Fire,
	})
	httpServer := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           apiServer.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	var wg sync.WaitGroup
	start := func(name string, interval time.Duration, trigger loop.Trigger, pass func(context.Context) error) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			loop.Run(ctx, name, interval, cfg.LoopMaxBackoff, trigger, log, pass)
		}()
	}
	start("scheduler", cfg.SchedulerInterval, schedTrigger, sched.RunOnce)
	start("replicaset-controller", cfg.ControllerInterval, rsTrigger, func(ctx context.Context) error {
		err := rsc.Reconcile(ctx)
		schedTrigger.Fire() // fresh pods probably want a node, so poke the scheduler
		return err
	})
	start("node-monitor", cfg.NodeMonitorInterval, nil, mon.Reconcile)

	serveErr := make(chan error, 1)
	go func() {
		log.Info("API server listening", "addr", cfg.ListenAddr, "version", version)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
		}
	}()

	var runErr error
	select {
	case <-ctx.Done():
		log.Info("shutdown signal received")
	case err := <-serveErr:
		runErr = fmt.Errorf("http server: %w", err)
		stop()
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Warn("HTTP shutdown did not complete cleanly", "error", err)
	}
	wg.Wait()
	log.Info("shutdown complete")
	return runErr
}
