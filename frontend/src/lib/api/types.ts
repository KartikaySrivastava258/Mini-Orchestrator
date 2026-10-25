// The Go API contract, retyped in TypeScript (source of truth: backend/internal/domain).
// If you change a struct over there, change it here too, or the UI will gaslight you.
// Timestamps are RFC 3339 strings in UTC.

export type PodPhase =
  | "Pending"
  | "Scheduled"
  | "Running"
  | "Succeeded"
  | "Failed"
  | "Unknown"
  | "Terminating";

export const POD_PHASES: PodPhase[] = [
  "Pending",
  "Scheduled",
  "Running",
  "Succeeded",
  "Failed",
  "Unknown",
  "Terminating",
];

export type RestartPolicy = "Always" | "Never";

export interface OwnerReference {
  kind: string;
  name: string;
  uid: string;
}

export interface PodStatus {
  phase: PodPhase;
  nodeName?: string;
  reason?: string;
  message?: string;
  containerID?: string;
  restartCount: number;
  exitCode?: number;
  scheduledAt?: string;
  startedAt?: string;
  lastTransitionTime: string;
  updatedAt: string;
}

export interface Pod {
  name: string;
  uid: string;
  createdAt: string;
  deletionRequestedAt?: string;
  owner?: OwnerReference;
  spec: { image: string; restartPolicy: RestartPolicy };
  status: PodStatus;
}

export interface Node {
  name: string;
  uid: string;
  registeredAt: string;
  spec: { unschedulable: boolean };
  status: {
    ready: boolean;
    reason: string;
    message?: string;
    address: string;
    maxPods: number;
    runtimeReady: boolean;
    lastHeartbeatTime: string;
    lastTransitionTime: string;
  };
}

export interface ReplicaSet {
  name: string;
  uid: string;
  createdAt: string;
  spec: { replicas: number; template: { image: string } };
  status: { replicas: number; runningReplicas: number; observedAt?: string };
}

export interface ClusterEvent {
  id: number;
  time: string;
  type: "Normal" | "Warning";
  reason: string;
  object: { kind: string; name: string };
  message: string;
  source: string;
}

export interface ListResponse<T> {
  items: T[];
  count: number;
}

export interface DeleteResponse {
  name: string;
  result: "Deleted" | "Terminating";
  message?: string;
}

export interface LogsResponse {
  podName?: string;
  nodeName?: string;
  containerID: string;
  tailLines: number;
  truncated: boolean;
  logs: string;
  fetchedAt: string;
  mode: "snapshot";
}

export interface Readiness {
  status: "ready" | "not ready";
  checks: Record<string, string>;
}

export interface ClusterInfo {
  name: string;
  version: string;
  apiVersion: string;
  schedulerPolicy: string;
  nodeHeartbeatTimeoutSeconds: number;
  podEvictionTimeoutSeconds: number;
  eventsRetained: number;
  eventsPersistent: boolean;
  authentication: string;
}

export interface ApiErrorEnvelope {
  error: { code: string; message: string; field?: string };
  requestId?: string;
}

export interface CreatePodRequest {
  name: string;
  image: string;
  restartPolicy: RestartPolicy;
}

export interface CreateReplicaSetRequest {
  name: string;
  image: string;
  replicas: number;
}
