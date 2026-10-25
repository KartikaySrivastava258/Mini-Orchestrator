package config

import (
	"strings"
	"testing"
	"time"
)

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestControlPlaneDefaults(t *testing.T) {
	c, err := LoadControlPlane(env(nil))
	if err != nil {
		t.Fatal(err)
	}
	if c.ListenAddr != "127.0.0.1:8080" || c.EtcdEndpoints[0] != "127.0.0.1:2379" || c.NodeHeartbeatTimeout != 15*time.Second {
		t.Fatalf("unexpected defaults: %+v", c)
	}
	if len(c.CORSAllowedOrigins) != 2 {
		t.Fatalf("cors defaults: %v", c.CORSAllowedOrigins)
	}
}

func TestControlPlaneOverridesAndLists(t *testing.T) {
	c, err := LoadControlPlane(env(map[string]string{
		"ORCH_ETCD_ENDPOINTS":         "etcd-a:2379, etcd-b:2379 ,",
		"ORCH_NODE_HEARTBEAT_TIMEOUT": "30s",
		"ORCH_POD_EVICTION_TIMEOUT":   "2m",
		"ORCH_LOG_FORMAT":             "JSON",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if len(c.EtcdEndpoints) != 2 || c.EtcdEndpoints[1] != "etcd-b:2379" {
		t.Fatalf("endpoints: %v", c.EtcdEndpoints)
	}
	if c.NodeHeartbeatTimeout != 30*time.Second || c.LogFormat != "json" {
		t.Fatalf("overrides not applied: %+v", c)
	}
}

func TestControlPlaneReportsAllErrors(t *testing.T) {
	_, err := LoadControlPlane(env(map[string]string{
		"ORCH_NODE_HEARTBEAT_TIMEOUT": "soon",
		"ORCH_CORS_ALLOWED_ORIGINS":   "*",
		"ORCH_ETCD_PREFIX":            "no-slash",
		"ORCH_LOG_LEVEL":              "loud",
	}))
	if err == nil {
		t.Fatal("expected errors")
	}
	for _, want := range []string{"ORCH_NODE_HEARTBEAT_TIMEOUT", "ORCH_CORS_ALLOWED_ORIGINS", "ORCH_ETCD_PREFIX", "ORCH_LOG_LEVEL"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error does not mention %s: %v", want, err)
		}
	}
}

func TestEvictionMustNotBeShorterThanHeartbeatTimeout(t *testing.T) {
	_, err := LoadControlPlane(env(map[string]string{"ORCH_NODE_HEARTBEAT_TIMEOUT": "30s", "ORCH_POD_EVICTION_TIMEOUT": "10s"}))
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestAgentNameFromArgsOrEnv(t *testing.T) {
	a, err := LoadAgent(env(nil), []string{"worker-1"})
	if err != nil {
		t.Fatal(err)
	}
	if a.NodeName != "worker-1" || a.AdvertiseURL != "http://127.0.0.1:9101" {
		t.Fatalf("unexpected agent config: %+v", a)
	}
	a, err = LoadAgent(env(map[string]string{"AGENT_NODE_NAME": "from-env"}), []string{"ignored"})
	if err != nil || a.NodeName != "from-env" {
		t.Fatalf("env should win: %v %+v", err, a)
	}
}

func TestAgentValidation(t *testing.T) {
	// Missing name, invalid name, bad max pods, bad API URL, and an advertise
	// URL derived from ":9101" (no host).
	cases := []map[string]string{
		{},
		{"AGENT_NODE_NAME": "Bad_Name"},
		{"AGENT_NODE_NAME": "w1", "AGENT_MAX_PODS": "0"},
		{"AGENT_NODE_NAME": "w1", "AGENT_API_URL": "localhost:8080"},
		{"AGENT_NODE_NAME": "w1", "AGENT_LISTEN_ADDR": ":9101"},
	}
	for i, c := range cases {
		if _, err := LoadAgent(env(c), nil); err == nil {
			t.Errorf("case %d: expected error for %v", i, c)
		}
	}
}
