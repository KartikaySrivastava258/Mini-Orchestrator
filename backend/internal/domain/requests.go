package domain

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// Hard limits. Validation says no to anything past these.
const (
	MaxNameLength           = 63
	MaxReplicaSetNameLength = 57 // leaves room for the "-xxxxx" pod suffix
	MaxImageLength          = 255
	MaxReplicas             = 50
	MaxPodsPerNodeLimit     = 110
	MaxReasonLength         = 64
	MaxMessageLength        = 1024
)

var (
	nameRE        = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)
	imageRE       = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._/:@-]*$`)
	containerIDRE = regexp.MustCompile(`^[a-f0-9]{12,64}$`)
)

// ValidationError is a polite "nope" about one field. The API turns it into a 400.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string { return e.Field + ": " + e.Message }

func invalid(field, format string, args ...any) error {
	return &ValidationError{Field: field, Message: fmt.Sprintf(format, args...)}
}

// ValidateName enforces DNS-label names: lowercase, digits and dashes, nothing spicy.
func ValidateName(field, v string, maxLen int) error {
	switch {
	case v == "":
		return invalid(field, "is required")
	case len(v) > maxLen:
		return invalid(field, "must be at most %d characters", maxLen)
	case !nameRE.MatchString(v):
		return invalid(field, "must consist of lowercase letters, digits and '-', and start and end with a letter or digit")
	}
	return nil
}

// ValidateImage is a vibe check for image references, not a full parser.
// It blocks whitespace and a leading '-', so nobody can sneak a flag like
// --privileged into docker run through the image field.
func ValidateImage(v string) error {
	switch {
	case v == "":
		return invalid("image", "is required")
	case len(v) > MaxImageLength:
		return invalid("image", "must be at most %d characters", MaxImageLength)
	case !imageRE.MatchString(v):
		return invalid("image", "must be a valid image reference such as nginx:stable or registry.example.com/team/app:1.2")
	}
	return nil
}

// ValidContainerID checks that s looks like a Docker container ID (12 to 64 hex chars).
func ValidContainerID(s string) bool { return containerIDRE.MatchString(s) }

// List wraps every collection response.
type List[T any] struct {
	Items []T `json:"items"`
	Count int `json:"count"`
}

// NewList makes sure an empty list goes out as [] and not null. Frontends hate null.
func NewList[T any](items []T) List[T] {
	if items == nil {
		items = []T{}
	}
	return List[T]{Items: items, Count: len(items)}
}

// CreatePodRequest is the body for POST /api/v1/pods.
type CreatePodRequest struct {
	Name          string        `json:"name"`
	Image         string        `json:"image"`
	RestartPolicy RestartPolicy `json:"restartPolicy,omitempty"`
}

// Normalize trims stray spaces and fills in defaults.
func (r *CreatePodRequest) Normalize() {
	r.Name = strings.TrimSpace(r.Name)
	r.Image = strings.TrimSpace(r.Image)
	if r.RestartPolicy == "" {
		r.RestartPolicy = RestartAlways
	}
}

// Validate runs after Normalize.
func (r *CreatePodRequest) Validate() error {
	if err := ValidateName("name", r.Name, MaxNameLength); err != nil {
		return err
	}
	if err := ValidateImage(r.Image); err != nil {
		return err
	}
	if r.RestartPolicy != RestartAlways && r.RestartPolicy != RestartNever {
		return invalid("restartPolicy", "must be %q or %q", RestartAlways, RestartNever)
	}
	return nil
}

// CreateReplicaSetRequest is the body for POST /api/v1/replicasets.
// Replicas is a pointer so we can tell "0" apart from "you forgot it".
type CreateReplicaSetRequest struct {
	Name     string `json:"name"`
	Image    string `json:"image"`
	Replicas *int   `json:"replicas"`
}

// Normalize trims stray spaces.
func (r *CreateReplicaSetRequest) Normalize() {
	r.Name = strings.TrimSpace(r.Name)
	r.Image = strings.TrimSpace(r.Image)
}

// Validate runs after Normalize.
func (r *CreateReplicaSetRequest) Validate() error {
	if err := ValidateName("name", r.Name, MaxReplicaSetNameLength); err != nil {
		return err
	}
	if err := ValidateImage(r.Image); err != nil {
		return err
	}
	return validateReplicas(r.Replicas)
}

// PatchReplicaSetRequest is the body for PATCH /api/v1/replicasets/{name}, aka scaling.
type PatchReplicaSetRequest struct {
	Replicas *int `json:"replicas"`
}

// Validate checks the replica count.
func (r *PatchReplicaSetRequest) Validate() error { return validateReplicas(r.Replicas) }

func validateReplicas(v *int) error {
	if v == nil {
		return invalid("replicas", "is required")
	}
	if *v < 0 || *v > MaxReplicas {
		return invalid("replicas", "must be between 0 and %d", MaxReplicas)
	}
	return nil
}

// RegisterNodeRequest is what an agent sends to say hi.
type RegisterNodeRequest struct {
	Name           string `json:"name"`
	Address        string `json:"address"`
	MaxPods        int    `json:"maxPods"`
	RuntimeReady   bool   `json:"runtimeReady"`
	RuntimeMessage string `json:"runtimeMessage,omitempty"`
}

// Validate checks the registration.
func (r *RegisterNodeRequest) Validate() error {
	if err := ValidateName("name", r.Name, MaxNameLength); err != nil {
		return err
	}
	if err := ValidateAgentAddress(r.Address); err != nil {
		return err
	}
	if r.MaxPods < 1 || r.MaxPods > MaxPodsPerNodeLimit {
		return invalid("maxPods", "must be between 1 and %d", MaxPodsPerNodeLimit)
	}
	return nil
}

// ValidateAgentAddress wants a plain http(s) URL with a host. No paths, no
// queries, no user:pass@ tricks, because the control plane bolts its own
// paths onto it.
func ValidateAgentAddress(addr string) error {
	if addr == "" {
		return invalid("address", "is required")
	}
	u, err := url.Parse(addr)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return invalid("address", "must be an absolute http or https URL such as http://127.0.0.1:9101")
	}
	if u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return invalid("address", "must not contain credentials, a path, a query or a fragment")
	}
	return nil
}

// HeartbeatRequest is the agent's "still alive" ping, plus whether Docker is answering.
type HeartbeatRequest struct {
	RuntimeReady   bool   `json:"runtimeReady"`
	RuntimeMessage string `json:"runtimeMessage,omitempty"`
}

// PatchNodeRequest is the body for PATCH /api/v1/nodes/{name} (cordon / uncordon).
type PatchNodeRequest struct {
	Unschedulable *bool `json:"unschedulable"`
}

// Validate makes sure unschedulable was actually sent.
func (r *PatchNodeRequest) Validate() error {
	if r.Unschedulable == nil {
		return invalid("unschedulable", "is required")
	}
	return nil
}

// PodStatusUpdate is the agent's status report for one pod.
type PodStatusUpdate struct {
	UID          string   `json:"uid"`
	NodeName     string   `json:"nodeName"`
	Phase        PodPhase `json:"phase"`
	Reason       string   `json:"reason,omitempty"`
	Message      string   `json:"message,omitempty"`
	ContainerID  string   `json:"containerID,omitempty"`
	RestartCount int      `json:"restartCount"`
	ExitCode     *int     `json:"exitCode,omitempty"`
}

// Validate checks the report. Long messages get trimmed instead of rejected,
// because a chatty Docker error still beats no error at all.
func (u *PodStatusUpdate) Validate() error {
	if u.UID == "" {
		return invalid("uid", "is required")
	}
	if err := ValidateName("nodeName", u.NodeName, MaxNameLength); err != nil {
		return err
	}
	switch u.Phase {
	case PodRunning, PodSucceeded, PodFailed:
	default:
		return invalid("phase", "agents may only report Running, Succeeded or Failed")
	}
	if len(u.Reason) > MaxReasonLength {
		return invalid("reason", "must be at most %d characters", MaxReasonLength)
	}
	if u.ContainerID != "" && !ValidContainerID(u.ContainerID) {
		return invalid("containerID", "must be a hexadecimal Docker container ID")
	}
	if u.RestartCount < 0 {
		return invalid("restartCount", "must not be negative")
	}
	u.Message = Truncate(u.Message, MaxMessageLength)
	return nil
}

// FinalizePodRequest is the agent saying "container is gone, you can delete the record now".
type FinalizePodRequest struct {
	UID      string `json:"uid"`
	NodeName string `json:"nodeName"`
}

// Validate checks the request.
func (r *FinalizePodRequest) Validate() error {
	if r.UID == "" {
		return invalid("uid", "is required")
	}
	return ValidateName("nodeName", r.NodeName, MaxNameLength)
}

// DeleteResponse tells you whether the delete is done or still in progress.
type DeleteResponse struct {
	Name    string `json:"name"`
	Result  string `json:"result"` // "Deleted" or "Terminating"
	Message string `json:"message,omitempty"`
}

// LogsResponse is a log snapshot: the last TailLines lines, size-capped.
// It's a snapshot, not a stream, and we don't pretend otherwise.
type LogsResponse struct {
	PodName     string    `json:"podName,omitempty"`
	NodeName    string    `json:"nodeName,omitempty"`
	ContainerID string    `json:"containerID"`
	TailLines   int       `json:"tailLines"`
	Truncated   bool      `json:"truncated"`
	Logs        string    `json:"logs"`
	FetchedAt   time.Time `json:"fetchedAt"`
	Mode        string    `json:"mode"` // always "snapshot"
}

// Truncate cuts s to at most n bytes without slicing a UTF-8 character in half.
func Truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	cut := n
	for cut > 0 && cut < len(s) && (s[cut]&0xC0) == 0x80 {
		cut--
	}
	return s[:cut]
}
