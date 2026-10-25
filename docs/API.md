# API reference

Base URL: `http://127.0.0.1:8080` (see `ORCH_LISTEN_ADDR`). All bodies are JSON. Timestamps are RFC 3339 UTC.

This is a small custom REST API. It is **not** compatible with the Kubernetes API.

**Status column:** *Unit-tested* means covered by `backend/internal/api/api_test.go` (handlers run against the in-memory store). No endpoint has an automated test against a real etcd; `scripts/smoke.sh` exercises the main ones against a running stack. Whether those tests have been executed is recorded in [IMPLEMENTATION_AUDIT.md](IMPLEMENTATION_AUDIT.md).

## Conventions

- Requests with a body must use `Content-Type: application/json` (`415` otherwise). Bodies are limited to `ORCH_MAX_BODY_BYTES` (1 MiB; `413` beyond). Unknown fields, wrong types, malformed JSON and trailing data are rejected with `400 INVALID_JSON`.
- Names are DNS-label style: lowercase letters, digits and `-`, starting and ending alphanumeric, at most 63 characters (57 for ReplicaSets).
- Lists return `{"items":[...],"count":N}`, sorted by name (events: newest first). There is no pagination; lists are bounded only by cluster size.
- Every response carries `X-Request-ID` (an incoming valid `X-Request-ID` is reused). It appears in server logs and error bodies.
- CORS: only origins in `ORCH_CORS_ALLOWED_ORIGINS` are allowed; credentials are never allowed. Disallowed preflights get `403 ORIGIN_NOT_ALLOWED`.
- Unknown routes, including unsupported methods on known paths, return `404 NOT_FOUND` in the error format.

### Error format

```json
{"error":{"code":"VALIDATION_ERROR","message":"image is required","field":"image"},"requestId":"9f2c1a7be04d3c11"}
```

| HTTP | Code | Meaning |
|---|---|---|
| 400 | `VALIDATION_ERROR` | A field is missing or invalid (`field` names it) |
| 400 | `INVALID_JSON` | Body is malformed, has unknown fields or wrong types |
| 403 | `ORIGIN_NOT_ALLOWED` | CORS preflight from an unlisted origin |
| 404 | `NOT_FOUND` | Object or route does not exist |
| 409 | `ALREADY_EXISTS` | Name already in use |
| 409 | `CONFLICT` | Transition not allowed, stale agent report, node still has pods, or concurrent modification |
| 413 | `REQUEST_TOO_LARGE` | Body over the limit |
| 415 | `UNSUPPORTED_MEDIA_TYPE` | Body is not JSON |
| 502 | `AGENT_ERROR` | Node agent answered with an error or invalid data (logs) |
| 503 | `STORE_UNAVAILABLE` | etcd unreachable or timed out |
| 503 | `AGENT_UNREACHABLE` | Node agent did not respond, or its node is gone (logs) |
| 500 | `INTERNAL_ERROR` | Unexpected failure; details only in server logs |

Internal error text, etcd endpoints and file paths are never returned to clients.

## Health

| Method and path | Purpose | Responses | Status |
|---|---|---|---|
| `GET /healthz` | Process liveness; never touches dependencies | `200 {"status":"ok"}` | Unit-tested |
| `GET /readyz` | Readiness: etcd answers within 2 s | `200 {"status":"ready","checks":{"etcd":"ok"}}` or `503 {"status":"not ready","checks":{"etcd":"unavailable"}}` | Unit-tested |
| `GET /api/v1/info` | Control-plane settings for display | `200` (below) | Implemented |

```json
{"name":"mini-orchestrator","version":"dev","apiVersion":"v1","schedulerPolicy":"LeastPods",
 "nodeHeartbeatTimeoutSeconds":15,"podEvictionTimeoutSeconds":60,"eventsRetained":500,
 "eventsPersistent":false,"authentication":"none"}
```

## Pods

### `GET /api/v1/pods`
Query (all optional, combinable): `node`, `phase` (`Pending`, `Scheduled`, `Running`, `Succeeded`, `Failed`, `Unknown`, `Terminating`), `owner` (ReplicaSet name). `400` for an unknown phase, `503` if etcd is down. Unit-tested.

### `POST /api/v1/pods`
```json
{"name":"web","image":"nginx:stable","restartPolicy":"Always"}
```
`restartPolicy` is optional (`Always` default, or `Never`). Responses: `201` with the pod, `400`, `409 ALREADY_EXISTS`, `503`. Unit-tested.

```json
{
  "name": "web",
  "uid": "3f1c0d6e9a2b4c5d8e7f6a5b4c3d2e1f",
  "createdAt": "2026-03-01T12:00:00Z",
  "spec": {"image": "nginx:stable", "restartPolicy": "Always"},
  "status": {
    "phase": "Pending",
    "reason": "AwaitingScheduling",
    "message": "waiting for the scheduler",
    "restartCount": 0,
    "lastTransitionTime": "2026-03-01T12:00:00Z",
    "updatedAt": "2026-03-01T12:00:00Z"
  }
}
```

Once running, `status` also contains `nodeName`, `containerID`, `scheduledAt`, `startedAt`, and after an exit `exitCode`. Pods created by a ReplicaSet have `"owner":{"kind":"ReplicaSet","name":"api","uid":"..."}`. A deleting pod has `deletionRequestedAt`.

### `GET /api/v1/pods/{name}`
`200` pod, `404`, `503`. Unit-tested.

### `DELETE /api/v1/pods/{name}`
- No node assigned (or node no longer registered): record deleted, `200 {"name":"web","result":"Deleted"}`.
- Assigned: `202 {"name":"web","result":"Terminating","message":"the node agent will remove the container and then delete the record"}`. Repeating the request returns `202` again.
- If the pod belongs to a ReplicaSet, the controller creates a replacement; scale the ReplicaSet to reduce pods.

`404`, `503`. Unit-tested.

### `GET /api/v1/pods/{name}/logs?tailLines=200`
`tailLines` 1–2000 (default 200). The control plane fetches a snapshot from the node agent at the node's registered address (timeout `ORCH_AGENT_REQUEST_TIMEOUT`, response capped at 1 MiB).

```json
{"podName":"web","nodeName":"worker-1","containerID":"4b1e...","tailLines":200,
 "truncated":false,"logs":"...","fetchedAt":"2026-03-01T12:05:00Z","mode":"snapshot"}
```

`400` bad `tailLines`; `404` pod; `409` no container yet; `502 AGENT_ERROR`; `503 AGENT_UNREACHABLE`. Unit-tested (with a stub agent).

### Agent-only pod endpoints

These are called by node agents. They are unauthenticated (see OPERATIONS.md, "Security boundaries").

`PUT /api/v1/pods/{name}/status`
```json
{"uid":"3f1c...","nodeName":"worker-1","phase":"Running","reason":"ContainerStarted",
 "message":"container started","containerID":"4b1e...","restartCount":0}
```
`phase` must be `Running`, `Succeeded` or `Failed`; `exitCode` is optional. `200` updated pod; `400`; `404`; `409 CONFLICT` if the UID or node does not match, the pod is terminating, or its phase cannot change. Unit-tested.

`POST /api/v1/pods/{name}/finalize`
```json
{"uid":"3f1c...","nodeName":"worker-1"}
```
Deletes a terminating pod after its container is gone. `200` (also if already deleted); `409` if the pod is not terminating or does not match. Unit-tested.

## ReplicaSets

| Method and path | Body | Responses | Status |
|---|---|---|---|
| `GET /api/v1/replicasets` | | `200` list | Implemented |
| `POST /api/v1/replicasets` | `{"name":"api","image":"nginx:stable","replicas":3}` (`replicas` required, 0–50) | `201`, `400`, `409`, `503` | Unit-tested |
| `GET /api/v1/replicasets/{name}` | | `200`, `404` | Unit-tested (404 path) |
| `PATCH /api/v1/replicasets/{name}` | `{"replicas":1}` | `200`, `400`, `404` | Unit-tested |
| `DELETE /api/v1/replicasets/{name}` | | `200 {"name":"api","result":"Deleted","message":"owned pods will be terminated by the ReplicaSet controller"}`, `404` | Unit-tested |

```json
{"name":"api","uid":"...","createdAt":"2026-03-01T12:00:00Z",
 "spec":{"replicas":3,"template":{"image":"nginx:stable"}},
 "status":{"replicas":3,"runningReplicas":2,"observedAt":"2026-03-01T12:00:10Z"}}
```

`status.replicas` counts active owned pods (Pending, Scheduled, Running, Unknown); `runningReplicas` those in Running. Pods of a ReplicaSet always use `restartPolicy: Always`. The image cannot be changed after creation.

## Nodes

| Method and path | Body | Responses | Status |
|---|---|---|---|
| `GET /api/v1/nodes` | | `200` list | Implemented |
| `GET /api/v1/nodes/{name}` | | `200`, `404` | Implemented |
| `POST /api/v1/nodes` (agent) | `{"name":"worker-1","address":"http://127.0.0.1:9101","maxPods":10,"runtimeReady":true}` | `201` registered, `200` re-registered, `400` | Unit-tested |
| `POST /api/v1/nodes/{name}/heartbeat` (agent) | `{"runtimeReady":true}` or `{"runtimeReady":false,"runtimeMessage":"..."}` | `200`, `404` (agent re-registers) | Unit-tested |
| `PATCH /api/v1/nodes/{name}` | `{"unschedulable":true}` (cordon) or `false` | `200`, `400`, `404` | Unit-tested |
| `DELETE /api/v1/nodes/{name}` | | `200`; `409` while pods occupy the node | Unit-tested |

`address` must be an `http`/`https` URL with a host and no path, query or credentials.

```json
{"name":"worker-1","uid":"...","registeredAt":"2026-03-01T11:58:00Z",
 "spec":{"unschedulable":false},
 "status":{"ready":true,"reason":"Ready","message":"node agent is heartbeating and the container runtime is reachable",
  "address":"http://127.0.0.1:9101","maxPods":10,"runtimeReady":true,
  "lastHeartbeatTime":"2026-03-01T12:00:00Z","lastTransitionTime":"2026-03-01T11:58:00Z"}}
```

`status.reason` is `Ready`, `RuntimeUnavailable` or `HeartbeatTimeout`.

## Events

`GET /api/v1/events?kind=Pod&name=web&type=Warning&limit=200` (all optional; `limit` 1–1000, default 200). Newest first. `400` for a bad `limit` or `type`. Implemented; filtering and the ring buffer are unit-tested in `internal/events`.

```json
{"items":[{"id":42,"time":"2026-03-01T12:00:02Z","type":"Normal","reason":"Scheduled",
  "object":{"kind":"Pod","name":"web"},
  "message":"assigned to node worker-1 by policy LeastPods (0/10 pod slots in use before placement)",
  "source":"scheduler"}],"count":1}
```

Events live in control-plane memory (`ORCH_EVENTS_CAPACITY`, default 500) and are lost on restart.

## Pod and node reasons

| Reason | Set by | Meaning |
|---|---|---|
| `AwaitingScheduling` | API, controller | Just created |
| `NoEligibleNodes` / `InsufficientCapacity` | Scheduler | Pending; message lists why nodes were excluded |
| `Scheduled` | Scheduler | Bound to a node |
| `ContainerStarted` / `ContainerRestarted` | Agent | Running |
| `RestartBackOff` / `RestartFailed` | Agent | Exited, waiting to restart / restart attempt failed (phase stays Running) |
| `RestartLimitExceeded` | Agent | Failed after `AGENT_MAX_RESTARTS` |
| `ImagePullFailed` / `ContainerStartFailed` | Agent | Failed to start |
| `Completed` / `ContainerExited` | Agent | `restartPolicy: Never` exited 0 / non-zero |
| `ContainerMissing` | Agent | Container removed outside the orchestrator |
| `NodeNotReady` / `NodeLost` | Node monitor | Pod Unknown / Failed because its node stopped reporting |
| `DeletionRequested` | API | Terminating |

## Removed routes

The original prototype exposed `/getpods`, `/getnodes`, `/heartbeat`, `/updatePodStatus`, `/pods/{name}` (delete) and `/logs/{pod}`. They were replaced by the routes above; all callers (console, agent) were migrated, and no compatibility aliases are provided.
