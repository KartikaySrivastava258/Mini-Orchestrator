# Roadmap

Everything below is **planned, not implemented**. Implemented features are listed in the README feature matrix. Items are ordered by technical value within each group.

## Reliability

1. Opt-in integration tests (`-tags integration`) against real etcd (Compose) and Docker, using a unique key prefix per run: API ↔ etcd persistence, full pod lifecycle, node expiry.
2. etcd watches with resync-on-compaction to replace periodic full listing in the scheduler, controller and node monitor.
3. Leader election (etcd lease) so several control-plane replicas can run safely.
4. Scripted etcd snapshot and restore.
5. Pruning of exited containers for terminal pods after a retention period.

## Scheduling and workloads

1. Declared CPU/memory requests on pods and allocatable capacity on nodes, with tests proving the constraints are enforced.
2. Scoring plugins (spread, bin-pack) with documented tie-breaks.
3. Environment variables, command/args, and published ports in the pod spec.
4. Deployments with rolling updates on top of ReplicaSets.
5. Node drain (cordon + evict) as a single operation.

## Observability

1. Prometheus `/metrics`: HTTP request count/latency, loop duration and errors, scheduling attempts and failures, heartbeat age, pods by phase.
2. Persistent events with TTL (etcd leases) or an external sink.
3. Streaming logs (chunked HTTP or WebSocket), clearly separated from snapshots.
4. Time-series charts in the console only once metrics exist.

## Security

1. Token authentication for node agents (bootstrap token → per-node credential) and verification that an agent only reports on its own node.
2. API authentication for users, then simple role separation (read-only vs operator).
3. TLS between agents and the control plane.
4. etcd authentication and TLS in the Compose setup.
5. Image allow-list.

## Developer experience

1. Screenshots and a short recorded demo in the README.
2. Dockerfiles (multi-stage, non-root) for the control plane and console, kept separate from agents that need the Docker socket.
3. Prune unused frontend dependencies inherited from the template (for example `recharts`, `axios`, `embla-carousel-react`) and regenerate the lockfile.
4. OpenAPI description generated from or checked against the handlers.
5. Optional dark theme.

## Optional Kubernetes deployment / integration

1. Helm chart or manifests to run the *control plane* inside a Kubernetes cluster as an ordinary application (it would still not be a Kubernetes implementation).
2. A comparison lab: run the same workload on Mini Orchestrator and on kind, and document the differences.
