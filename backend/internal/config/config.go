// Package config reads settings from environment variables and reports every
// bad one at once, instead of one per restart. The full list is in the README
// under Configuration.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strconv"
	"strings"
	"time"

	"mini-orchestrator/internal/domain"
	"mini-orchestrator/internal/logging"
)

// ControlPlane is everything the control-plane process needs to know.
type ControlPlane struct {
	ListenAddr           string
	EtcdEndpoints        []string
	EtcdDialTimeout      time.Duration
	EtcdOpTimeout        time.Duration
	EtcdPrefix           string
	CORSAllowedOrigins   []string
	NodeHeartbeatTimeout time.Duration
	PodEvictionTimeout   time.Duration
	SchedulerInterval    time.Duration
	ControllerInterval   time.Duration
	NodeMonitorInterval  time.Duration
	LoopMaxBackoff       time.Duration
	AgentRequestTimeout  time.Duration
	MaxBodyBytes         int64
	EventsCapacity       int
	LogLevel             slog.Level
	LogFormat            string
}

// Agent is everything a node agent process needs to know.
type Agent struct {
	NodeName           string
	APIURL             string
	ListenAddr         string
	AdvertiseURL       string
	MaxPods            int
	HeartbeatInterval  time.Duration
	SyncInterval       time.Duration
	MaxRestarts        int
	RestartBackoffBase time.Duration
	RestartBackoffMax  time.Duration
	DockerBinary       string
	RuntimeTimeout     time.Duration
	RuntimeRunTimeout  time.Duration
	APITimeout         time.Duration
	LogMaxTailLines    int
	LogMaxBytes        int
	LogLevel           slog.Level
	LogFormat          string
}

// parser reads env vars and collects every problem it finds, so you fix your
// config in one go instead of playing whack-a-mole.
type parser struct {
	getenv func(string) string
	errs   []error
}

func (p *parser) str(key, def string) string {
	if v := strings.TrimSpace(p.getenv(key)); v != "" {
		return v
	}
	return def
}

func (p *parser) list(key, def string) []string {
	var out []string
	for _, part := range strings.Split(p.str(key, def), ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

func (p *parser) duration(key string, def, min time.Duration) time.Duration {
	raw := p.str(key, "")
	if raw == "" {
		return def
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		p.errs = append(p.errs, fmt.Errorf("%s: %q is not a duration (examples: 500ms, 5s, 1m)", key, raw))
		return def
	}
	if d < min {
		p.errs = append(p.errs, fmt.Errorf("%s: must be at least %s, got %s", key, min, d))
		return def
	}
	return d
}

func (p *parser) integer(key string, def, min, max int) int {
	raw := p.str(key, "")
	if raw == "" {
		return def
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < min || n > max {
		p.errs = append(p.errs, fmt.Errorf("%s: must be an integer between %d and %d, got %q", key, min, max, raw))
		return def
	}
	return n
}

func (p *parser) level(key string) slog.Level {
	lvl, err := logging.ParseLevel(p.getenv(key))
	if err != nil {
		p.errs = append(p.errs, fmt.Errorf("%s: %v", key, err))
	}
	return lvl
}

func (p *parser) format(key string) string {
	f := strings.ToLower(p.str(key, "text"))
	if f != "text" && f != "json" {
		p.errs = append(p.errs, fmt.Errorf("%s: must be text or json, got %q", key, f))
		return "text"
	}
	return f
}

func (p *parser) fail(format string, args ...any) {
	p.errs = append(p.errs, fmt.Errorf(format, args...))
}

// LoadControlPlane reads the ORCH_* variables.
func LoadControlPlane(getenv func(string) string) (ControlPlane, error) {
	p := &parser{getenv: getenv}
	c := ControlPlane{
		ListenAddr:           p.str("ORCH_LISTEN_ADDR", "127.0.0.1:8080"),
		EtcdEndpoints:        p.list("ORCH_ETCD_ENDPOINTS", "127.0.0.1:2379"),
		EtcdDialTimeout:      p.duration("ORCH_ETCD_DIAL_TIMEOUT", 5*time.Second, 100*time.Millisecond),
		EtcdOpTimeout:        p.duration("ORCH_ETCD_OP_TIMEOUT", 3*time.Second, 100*time.Millisecond),
		EtcdPrefix:           p.str("ORCH_ETCD_PREFIX", "/mini-orchestrator/v1"),
		CORSAllowedOrigins:   p.list("ORCH_CORS_ALLOWED_ORIGINS", "http://localhost:5173,http://127.0.0.1:5173"),
		NodeHeartbeatTimeout: p.duration("ORCH_NODE_HEARTBEAT_TIMEOUT", 15*time.Second, time.Second),
		PodEvictionTimeout:   p.duration("ORCH_POD_EVICTION_TIMEOUT", 60*time.Second, time.Second),
		SchedulerInterval:    p.duration("ORCH_SCHEDULER_INTERVAL", 2*time.Second, 50*time.Millisecond),
		ControllerInterval:   p.duration("ORCH_CONTROLLER_INTERVAL", 5*time.Second, 50*time.Millisecond),
		NodeMonitorInterval:  p.duration("ORCH_NODE_MONITOR_INTERVAL", 5*time.Second, 50*time.Millisecond),
		LoopMaxBackoff:       p.duration("ORCH_LOOP_MAX_BACKOFF", 30*time.Second, 100*time.Millisecond),
		AgentRequestTimeout:  p.duration("ORCH_AGENT_REQUEST_TIMEOUT", 5*time.Second, 100*time.Millisecond),
		MaxBodyBytes:         int64(p.integer("ORCH_MAX_BODY_BYTES", 1<<20, 1024, 16<<20)),
		EventsCapacity:       p.integer("ORCH_EVENTS_CAPACITY", 500, 10, 100000),
		LogLevel:             p.level("ORCH_LOG_LEVEL"),
		LogFormat:            p.format("ORCH_LOG_FORMAT"),
	}
	if len(c.EtcdEndpoints) == 0 {
		p.fail("ORCH_ETCD_ENDPOINTS: at least one endpoint is required")
	}
	if !strings.HasPrefix(c.EtcdPrefix, "/") || strings.HasSuffix(c.EtcdPrefix, "/") {
		p.fail("ORCH_ETCD_PREFIX: must start with '/' and must not end with '/', got %q", c.EtcdPrefix)
	}
	for _, o := range c.CORSAllowedOrigins {
		if o == "*" {
			p.fail("ORCH_CORS_ALLOWED_ORIGINS: wildcard '*' is not allowed; list explicit origins")
			continue
		}
		if u, err := url.Parse(o); err != nil || u.Scheme == "" || u.Host == "" || (u.Path != "" && u.Path != "/") {
			p.fail("ORCH_CORS_ALLOWED_ORIGINS: %q is not an origin like http://localhost:5173", o)
		}
	}
	if c.PodEvictionTimeout < c.NodeHeartbeatTimeout {
		p.fail("ORCH_POD_EVICTION_TIMEOUT (%s) must be >= ORCH_NODE_HEARTBEAT_TIMEOUT (%s)", c.PodEvictionTimeout, c.NodeHeartbeatTimeout)
	}
	return c, errors.Join(p.errs...)
}

// LoadAgent reads the AGENT_* variables. The node name can also come in as
// the first CLI argument, because typing env vars gets old fast.
func LoadAgent(getenv func(string) string, args []string) (Agent, error) {
	p := &parser{getenv: getenv}
	name := p.str("AGENT_NODE_NAME", "")
	if name == "" && len(args) > 0 {
		name = strings.TrimSpace(args[0])
	}
	listen := p.str("AGENT_LISTEN_ADDR", "127.0.0.1:9101")
	a := Agent{
		NodeName:           name,
		APIURL:             strings.TrimRight(p.str("AGENT_API_URL", "http://127.0.0.1:8080"), "/"),
		ListenAddr:         listen,
		AdvertiseURL:       strings.TrimRight(p.str("AGENT_ADVERTISE_URL", "http://"+listen), "/"),
		MaxPods:            p.integer("AGENT_MAX_PODS", 10, 1, domain.MaxPodsPerNodeLimit),
		HeartbeatInterval:  p.duration("AGENT_HEARTBEAT_INTERVAL", 5*time.Second, 100*time.Millisecond),
		SyncInterval:       p.duration("AGENT_SYNC_INTERVAL", 3*time.Second, 100*time.Millisecond),
		MaxRestarts:        p.integer("AGENT_MAX_RESTARTS", 5, 0, 1000),
		RestartBackoffBase: p.duration("AGENT_RESTART_BACKOFF_BASE", 5*time.Second, 10*time.Millisecond),
		RestartBackoffMax:  p.duration("AGENT_RESTART_BACKOFF_MAX", 2*time.Minute, 10*time.Millisecond),
		DockerBinary:       p.str("AGENT_DOCKER_BINARY", "docker"),
		RuntimeTimeout:     p.duration("AGENT_RUNTIME_TIMEOUT", 15*time.Second, time.Second),
		RuntimeRunTimeout:  p.duration("AGENT_RUNTIME_RUN_TIMEOUT", 3*time.Minute, time.Second),
		APITimeout:         p.duration("AGENT_API_TIMEOUT", 5*time.Second, 100*time.Millisecond),
		LogMaxTailLines:    p.integer("AGENT_LOG_MAX_TAIL_LINES", 2000, 1, 100000),
		LogMaxBytes:        p.integer("AGENT_LOG_MAX_BYTES", 256<<10, 1024, 16<<20),
		LogLevel:           p.level("AGENT_LOG_LEVEL"),
		LogFormat:          p.format("AGENT_LOG_FORMAT"),
	}
	if a.NodeName == "" {
		p.fail("AGENT_NODE_NAME: required (or pass the node name as the first argument)")
	} else if err := domain.ValidateName("AGENT_NODE_NAME", a.NodeName, domain.MaxNameLength); err != nil {
		p.errs = append(p.errs, err)
	}
	if u, err := url.Parse(a.APIURL); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		p.fail("AGENT_API_URL: must be an http(s) URL, got %q", a.APIURL)
	}
	if err := domain.ValidateAgentAddress(a.AdvertiseURL); err != nil {
		p.fail("AGENT_ADVERTISE_URL: %v", err)
	}
	if a.RestartBackoffMax < a.RestartBackoffBase {
		p.fail("AGENT_RESTART_BACKOFF_MAX must be >= AGENT_RESTART_BACKOFF_BASE")
	}
	return a, errors.Join(p.errs...)
}
