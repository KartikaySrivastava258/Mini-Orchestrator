# Architecture decision records

Short records of significant choices. Each lists the decision, why, and what it costs.

## ADR-001: etcd is the single source of truth for cluster objects

**Decision.** Pods, nodes and ReplicaSets are stored only in etcd under `/mini-orchestrator/v1/`. Components keep no long-lived in-memory copies; every loop pass re-reads.

**Why.** etcd offers linearizable reads and multi-key transactions (`Txn` with `Compare`), which give compare-and-swap updates for free. It is also what Kubernetes uses, which is the point of the project.

**Cost.** Each pass lists whole prefixes. Acceptable at lab scale; watches are on the roadmap. Running etcd adds a dependency, handled with one Compose service.

## ADR-002: Optimistic concurrency on every write

**Decision.** All updates are read-modify-write guarded by the key's `ModRevision`; creates require `CreateRevision == 0`; deletes are conditional on the revision that was checked. Conflicts retry (bounded) or surface as `409`.

**Why.** The scheduler, controllers, API and agents all write pods. Without CAS, the original prototype could overwrite an agent's status with a scheduler write, or schedule a pod twice.

**Cost.** Slightly more code per write; centralised in `store.updateObj` so callers only provide a mutate function.

## ADR-003: Scheduling and reconciliation are separate loops

**Decision.** The ReplicaSet controller only creates and deletes Pending pods; the scheduler only binds Pending pods to nodes; the agent only runs what is bound to its node.

**Why.** Each loop has one question to answer, can be tested alone (`scheduler.Place` is a pure function), and fails independently. It mirrors the Kubernetes split, which makes the mapping easy to explain.

**Cost.** An extra hop of latency between "pod created" and "pod placed", reduced by triggering the scheduler right after API and controller writes.

## ADR-004: A simple, deterministic scheduler (LeastPods over declared slots)

**Decision.** Choose the eligible node with the fewest occupied pod slots; break ties by name. Capacity is `maxPods`, declared by the agent.

**Why.** It is explainable ("0/3 nodes available: 1 cordoned, 2 at pod capacity") and testable with tables. The prototype filtered on live CPU from `top`, which is noisy, host-wide when agents share a machine, and not a capacity model.

**Cost.** No resource awareness; a heavy pod counts the same as a light one. Resource requests are the next step on the roadmap.

## ADR-005: Events are kept in memory, not in etcd

**Decision.** A mutex-protected ring buffer (default 500) in the control plane; every event is also logged.

**Why.** Events are high-churn, append-only and only useful recently. Storing them in etcd would need leases/TTLs and would compete with real state for etcd write bandwidth.

**Cost.** Lost on restart; not shared between control-plane replicas. Stated in the UI and API (`eventsPersistent: false`).

## ADR-006: The container runtime is behind an interface; Docker is driven through its CLI

**Decision.** `runtime.Runtime` with a `Docker` implementation that runs the Docker CLI with argv slices, and a `Fake` for tests.

**Why.** Agent logic (restart policy, back-off, orphan cleanup, error classification) is the interesting part and is now unit-tested without a daemon. The CLI avoids adding the large Docker SDK dependency tree and works with any Docker context the user has configured.

**Cost.** Parsing CLI output is more brittle than an SDK, and each call forks a process. Parsing is isolated in two tested functions. Swapping in the Docker Engine API or containerd later only touches `internal/runtime`.

## ADR-007: Agents talk to the API, not to etcd, and poll

**Decision.** Agents register, heartbeat, list their pods, report status and finalize deletions over the REST API, polling every few seconds.

**Why.** Worker hosts never hold etcd access; the API enforces transition rules in one place (`lifecycle.ApplyAgentStatus`); stale reports are rejected by UID and node checks.

**Cost.** Polling latency (default 3 s) and load proportional to the number of agents. A watch or long-poll endpoint is a later optimisation.

## ADR-008: Deletion goes through Terminating and agent finalization

**Decision.** Deleting a scheduled pod marks it Terminating; the agent removes the container and then calls `/finalize`. Unscheduled pods (or pods on nodes that no longer exist) are deleted immediately. The node monitor force-deletes terminating pods on lost nodes.

**Why.** Deleting the record first would leave the container running with nothing tracking it. Two-phase deletion keeps the record until cleanup is confirmed, and is idempotent at each step.

**Cost.** Deletion is asynchronous (`202 Accepted`), which the console explains.

## ADR-009: What is deliberately out of scope compared with Kubernetes

Not implemented, by design for this project size: the Kubernetes API and object model (namespaces, labels/selectors, resourceVersion semantics), CRI, CNI (networking, services, DNS), CSI (volumes), probes, Deployments and rolling updates, RBAC and admission control, multi-tenant isolation, high availability. Some appear on the roadmap; none are implied by the current code or UI.
