package domain

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateName(t *testing.T) {
	tests := []struct {
		name  string
		value string
		ok    bool
	}{
		{"simple", "web-1", true},
		{"digits only", "123", true},
		{"empty", "", false},
		{"uppercase", "Web", false},
		{"leading dash", "-web", false},
		{"trailing dash", "web-", false},
		{"underscore", "web_1", false},
		{"slash (path traversal)", "a/b", false},
		{"too long", strings.Repeat("a", 64), false},
		{"max length", strings.Repeat("a", 63), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateName("name", tt.value, MaxNameLength)
			if (err == nil) != tt.ok {
				t.Fatalf("ValidateName(%q) error = %v, want ok=%v", tt.value, err, tt.ok)
			}
			var ve *ValidationError
			if err != nil && !errors.As(err, &ve) {
				t.Fatalf("expected *ValidationError, got %T", err)
			}
		})
	}
}

func TestValidateImage(t *testing.T) {
	valid := []string{"nginx", "nginx:stable", "library/nginx:1.27", "ghcr.io/org/app@sha256:abc123", "localhost:5000/app"}
	invalidImages := []string{"", "-v", "--privileged", "nginx latest", "nginx;rm", strings.Repeat("a", 256)}
	for _, v := range valid {
		if err := ValidateImage(v); err != nil {
			t.Errorf("ValidateImage(%q) unexpected error: %v", v, err)
		}
	}
	for _, v := range invalidImages {
		if err := ValidateImage(v); err == nil {
			t.Errorf("ValidateImage(%q) expected error", v)
		}
	}
}

func TestCreatePodRequestDefaultsAndValidation(t *testing.T) {
	r := CreatePodRequest{Name: "  web ", Image: " nginx:stable "}
	r.Normalize()
	if err := r.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Name != "web" || r.Image != "nginx:stable" || r.RestartPolicy != RestartAlways {
		t.Fatalf("normalisation failed: %+v", r)
	}
	r.RestartPolicy = "Sometimes"
	if err := r.Validate(); err == nil {
		t.Fatal("expected restartPolicy validation error")
	}
}

func TestReplicaSetReplicasValidation(t *testing.T) {
	neg, ok, tooMany := -1, 3, MaxReplicas+1
	cases := []struct {
		replicas *int
		wantErr  bool
	}{{nil, true}, {&neg, true}, {&ok, false}, {&tooMany, true}}
	for _, c := range cases {
		r := CreateReplicaSetRequest{Name: "web", Image: "nginx", Replicas: c.replicas}
		if err := r.Validate(); (err != nil) != c.wantErr {
			t.Errorf("replicas=%v: err=%v wantErr=%v", c.replicas, err, c.wantErr)
		}
	}
}

func TestValidateAgentAddress(t *testing.T) {
	good := []string{"http://127.0.0.1:9101", "https://node-1.internal:9101", "http://localhost:9101/"}
	bad := []string{"", "http://:9101", "127.0.0.1:9101", "ftp://x", "http://", "http://u:p@host:1", "http://host:1/path", "http://host:1?q=1"}
	for _, a := range good {
		if err := ValidateAgentAddress(a); err != nil {
			t.Errorf("%q: unexpected error %v", a, err)
		}
	}
	for _, a := range bad {
		if err := ValidateAgentAddress(a); err == nil {
			t.Errorf("%q: expected error", a)
		}
	}
}

func TestPodStatusUpdateValidation(t *testing.T) {
	u := PodStatusUpdate{UID: "u1", NodeName: "worker-1", Phase: PodRunning, Message: strings.Repeat("x", MaxMessageLength+10)}
	if err := u.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(u.Message) != MaxMessageLength {
		t.Fatalf("message not truncated: %d", len(u.Message))
	}
	u.Phase = PodScheduled
	if err := u.Validate(); err == nil {
		t.Fatal("agents must not be allowed to report Scheduled")
	}
	u.Phase = PodRunning
	u.ContainerID = "not-hex"
	if err := u.Validate(); err == nil {
		t.Fatal("expected container ID validation error")
	}
}

func TestPhaseHelpers(t *testing.T) {
	if !PodUnknown.IsActive() || PodFailed.IsActive() || PodTerminating.IsActive() {
		t.Fatal("IsActive mismatch")
	}
	if !PodSucceeded.IsTerminal() || PodRunning.IsTerminal() {
		t.Fatal("IsTerminal mismatch")
	}
	if PodPhase("Bogus").Valid() {
		t.Fatal("unknown phase reported valid")
	}
	p := Pod{Status: PodStatus{Phase: PodTerminating, NodeName: "n1"}}
	if !p.OccupiesNode() {
		t.Fatal("terminating pod with node should occupy a slot")
	}
	p.Status.NodeName = ""
	if p.OccupiesNode() {
		t.Fatal("unassigned pod must not occupy a slot")
	}
}

func TestTruncateKeepsUTF8Valid(t *testing.T) {
	s := "héllo" // 'é' is two bytes
	got := Truncate(s, 2)
	if got != "h" {
		t.Fatalf("Truncate split a rune: %q", got)
	}
}

func TestGeneratedIdentifiers(t *testing.T) {
	if a, b := NewUID(), NewUID(); a == b || len(a) != 32 {
		t.Fatalf("bad UIDs %q %q", a, b)
	}
	s := RandomSuffix(5)
	if len(s) != 5 || ValidateName("n", "web-"+s, MaxNameLength) != nil {
		t.Fatalf("bad suffix %q", s)
	}
}
