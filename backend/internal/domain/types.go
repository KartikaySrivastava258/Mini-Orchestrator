// Package domain is the vocabulary of the whole project: pods, nodes,
// ReplicaSets and the states they move through.
//
// The rule that runs everywhere: Spec is what you asked for, Status is what
// actually happened. Keep those two apart and reconciliation basically writes
// itself. Timestamps are UTC and RFC 3339, no timezone drama.
package domain

import "time"

// PodPhase is the one-word answer to "so how is my pod doing?"
type PodPhase string

// Pod phases. See docs/ARCHITECTURE.md for the transition table and the
// component that owns each transition.
const (
	PodPending     PodPhase = "Pending"     // accepted, not yet placed (owner: API server)
	PodScheduled   PodPhase = "Scheduled"   // bound to a node, container not yet confirmed (owner: scheduler)
	PodRunning     PodPhase = "Running"     // agent confirmed a started container (owner: node agent)
	PodSucceeded   PodPhase = "Succeeded"   // container exited 0 with restartPolicy Never (owner: node agent)
	PodFailed      PodPhase = "Failed"      // terminal failure (owner: node agent or node monitor)
	PodUnknown     PodPhase = "Unknown"     // node stopped reporting (owner: node monitor)
	PodTerminating PodPhase = "Terminating" // deletion requested, waiting for agent cleanup (owner: API server)
)

// AllPodPhases lists every phase in the order the console shows them.
var AllPodPhases = []PodPhase{PodPending, PodScheduled, PodRunning, PodSucceeded, PodFailed, PodUnknown, PodTerminating}

// Valid says whether p is a real phase and not something someone made up.
func (p PodPhase) Valid() bool {
	for _, known := range AllPodPhases {
		if p == known {
			return true
		}
	}
	return false
}

// IsTerminal is true once a pod is done for good. No comeback arc from here.
func (p PodPhase) IsTerminal() bool { return p == PodSucceeded || p == PodFailed }

// IsActive is true for pods that still count as a live replica.
func (p PodPhase) IsActive() bool {
	switch p {
	case PodPending, PodScheduled, PodRunning, PodUnknown:
		return true
	}
	return false
}

// RestartPolicy decides what the agent does when a container exits:
// shake it off and restart (Always), or let it rest (Never).
type RestartPolicy string

const (
	RestartAlways RestartPolicy = "Always"
	RestartNever  RestartPolicy = "Never"
)

// KindReplicaSet is the only owner kind we have. Deployments are a future-us problem.
const KindReplicaSet = "ReplicaSet"

// Reasons are the short, greppable "why" behind every status change.
const (
	ReasonAwaitingScheduling   = "AwaitingScheduling"
	ReasonScheduled            = "Scheduled"
	ReasonNoEligibleNodes      = "NoEligibleNodes"
	ReasonInsufficientCapacity = "InsufficientCapacity"
	ReasonContainerStarted     = "ContainerStarted"
	ReasonContainerRestarted   = "ContainerRestarted"
	ReasonRestartBackOff       = "RestartBackOff"
	ReasonRestartFailed        = "RestartFailed"
	ReasonRestartLimitExceeded = "RestartLimitExceeded"
	ReasonImagePullFailed      = "ImagePullFailed"
	ReasonContainerStartFailed = "ContainerStartFailed"
	ReasonContainerMissing     = "ContainerMissing"
	ReasonCompleted            = "Completed"
	ReasonContainerExited      = "ContainerExited"
	ReasonNodeNotReady         = "NodeNotReady"
	ReasonNodeLost             = "NodeLost"
	ReasonDeletionRequested    = "DeletionRequested"
	ReasonNodeReady            = "Ready"
	ReasonHeartbeatTimeout     = "HeartbeatTimeout"
	ReasonRuntimeUnavailable   = "RuntimeUnavailable"
)

// OwnerReference points a pod at the ReplicaSet that made it. We match on
// name AND UID, so a re-created ReplicaSet with the same name can't adopt
// somebody else's pods.
type OwnerReference struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
	UID  string `json:"uid"`
}

// Pod is one container plus the bookkeeping around it. Kubernetes-flavoured,
// but way smaller: no volumes, no ports, no sidecars.
type Pod struct {
	Name                string          `json:"name"`
	UID                 string          `json:"uid"`
	CreatedAt           time.Time       `json:"createdAt"`
	DeletionRequestedAt *time.Time      `json:"deletionRequestedAt,omitempty"`
	Owner               *OwnerReference `json:"owner,omitempty"`
	Spec                PodSpec         `json:"spec"`
	Status              PodStatus       `json:"status"`
}

// PodSpec is the wishlist: which image to run and what to do when it exits.
type PodSpec struct {
	Image         string        `json:"image"`
	RestartPolicy RestartPolicy `json:"restartPolicy"`
}

// PodStatus is reality, written by the scheduler, the node agent and the
// node monitor. Users never write it.
type PodStatus struct {
	Phase              PodPhase   `json:"phase"`
	NodeName           string     `json:"nodeName,omitempty"`
	Reason             string     `json:"reason,omitempty"`
	Message            string     `json:"message,omitempty"`
	ContainerID        string     `json:"containerID,omitempty"`
	RestartCount       int        `json:"restartCount"`
	ExitCode           *int       `json:"exitCode,omitempty"`
	ScheduledAt        *time.Time `json:"scheduledAt,omitempty"`
	StartedAt          *time.Time `json:"startedAt,omitempty"`
	LastTransitionTime time.Time  `json:"lastTransitionTime"`
	UpdatedAt          time.Time  `json:"updatedAt"`
}

// OccupiesNode says whether the pod is taking up one of its node's slots.
// Terminating pods still count until the agent has actually cleaned up.
func (p Pod) OccupiesNode() bool {
	if p.Status.NodeName == "" {
		return false
	}
	switch p.Status.Phase {
	case PodScheduled, PodRunning, PodUnknown, PodTerminating:
		return true
	}
	return false
}

// IsTerminating is true once someone has hit delete.
func (p Pod) IsTerminating() bool {
	return p.DeletionRequestedAt != nil || p.Status.Phase == PodTerminating
}

// Node is a worker registered by an agent process. One agent, one node.
type Node struct {
	Name         string     `json:"name"`
	UID          string     `json:"uid"`
	RegisteredAt time.Time  `json:"registeredAt"`
	Spec         NodeSpec   `json:"spec"`
	Status       NodeStatus `json:"status"`
}

// NodeSpec holds the knobs an operator can turn.
type NodeSpec struct {
	// Cordoned nodes keep their current pods but get no new ones. Do-not-disturb mode.
	Unschedulable bool `json:"unschedulable"`
}

// NodeStatus comes from heartbeats and, when those stop, from the node monitor.
type NodeStatus struct {
	Ready              bool      `json:"ready"`
	Reason             string    `json:"reason"`
	Message            string    `json:"message,omitempty"`
	Address            string    `json:"address"`
	MaxPods            int       `json:"maxPods"`
	RuntimeReady       bool      `json:"runtimeReady"`
	LastHeartbeatTime  time.Time `json:"lastHeartbeatTime"`
	LastTransitionTime time.Time `json:"lastTransitionTime"`
}

// ReplicaSet keeps N identical pods alive. A pod dies, the ReplicaSet
// doesn't panic, it just makes another one.
type ReplicaSet struct {
	Name      string           `json:"name"`
	UID       string           `json:"uid"`
	CreatedAt time.Time        `json:"createdAt"`
	Spec      ReplicaSetSpec   `json:"spec"`
	Status    ReplicaSetStatus `json:"status"`
}

// ReplicaSetSpec is how many pods you want and what they look like.
type ReplicaSetSpec struct {
	Replicas int         `json:"replicas"`
	Template PodTemplate `json:"template"`
}

// PodTemplate gets stamped onto every pod a ReplicaSet creates. Those pods
// always use restartPolicy Always.
type PodTemplate struct {
	Image string `json:"image"`
}

// ReplicaSetStatus is the controller's latest headcount.
type ReplicaSetStatus struct {
	// Replicas is the number of active (non-terminal, non-terminating) owned pods observed.
	Replicas int `json:"replicas"`
	// RunningReplicas is the subset of Replicas whose phase is Running.
	RunningReplicas int        `json:"runningReplicas"`
	ObservedAt      *time.Time `json:"observedAt,omitempty"`
}
