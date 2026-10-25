// Command agent runs a node agent: it registers a node, heartbeats, runs the
// pods assigned to it as Docker containers and serves their logs.
//
// Usage: AGENT_NODE_NAME=worker-1 agent (or just: agent worker-1)
package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"mini-orchestrator/internal/agent"
	"mini-orchestrator/internal/config"
	"mini-orchestrator/internal/logging"
	"mini-orchestrator/internal/runtime"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "agent:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.LoadAgent(os.Getenv, os.Args[1:])
	if err != nil {
		return fmt.Errorf("invalid configuration:\n%w", err)
	}
	log := logging.New(os.Stderr, cfg.LogLevel, cfg.LogFormat).With("component", "node-agent", "node", cfg.NodeName)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	rt := runtime.NewDocker(cfg.DockerBinary, cfg.RuntimeTimeout, cfg.RuntimeRunTimeout)
	if err := rt.Ping(ctx); err != nil {
		log.Warn("container runtime is not reachable; the node will report RuntimeUnavailable until it is", "error", err)
	}
	cp := agent.NewClient(cfg.APIURL, cfg.APITimeout)
	a := agent.New(agent.Config{
		NodeName:           cfg.NodeName,
		AdvertiseURL:       cfg.AdvertiseURL,
		MaxPods:            cfg.MaxPods,
		HeartbeatInterval:  cfg.HeartbeatInterval,
		SyncInterval:       cfg.SyncInterval,
		MaxRestarts:        cfg.MaxRestarts,
		RestartBackoffBase: cfg.RestartBackoffBase,
		RestartBackoffMax:  cfg.RestartBackoffMax,
	}, cp, rt, log)

	logServer := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           a.LogHandler(cfg.LogMaxTailLines, cfg.LogMaxBytes),
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      cfg.RuntimeTimeout + 5*time.Second,
		IdleTimeout:       60 * time.Second,
	}
	serveErr := make(chan error, 1)
	go func() {
		log.Info("log endpoint listening", "addr", cfg.ListenAddr, "advertise_url", cfg.AdvertiseURL)
		if err := logServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
			stop()
		}
	}()

	log.Info("agent starting", "api_url", cfg.APIURL, "max_pods", cfg.MaxPods)
	runErr := a.Run(ctx)
	if errors.Is(runErr, context.Canceled) {
		runErr = nil
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := logServer.Shutdown(shutdownCtx); err != nil {
		log.Warn("log endpoint shutdown did not complete cleanly", "error", err)
	}
	select {
	case err := <-serveErr:
		return fmt.Errorf("log endpoint: %w", err)
	default:
	}
	log.Info("agent stopped; containers keep running and are re-adopted on next start")
	return runErr
}
