# Operations runbook

This is a local learning platform. These notes explain how to start it, read its health signals, and diagnose common failures.

## Startup order

1. **etcd** — `make etcd-up` (waits for the Compose health check).
2. **Control plane** — `make run-control-plane`. It starts even if etcd is down; `/readyz` stays `503` until etcd answers.
3. **Node agents** — `make run-agent NODE=worker-1 AGENT_PORT=9101`. An agent retries registration (1 s → 30 s back-off) until the API answers.
4. **Console** — `make frontend-dev`.

Shutdown in reverse. Agents leave their containers running; they are re-adopted by label on the next start.

## Health signals

| Check | Healthy | Meaning when not |
|---|---|---|
| `curl -s 127.0.0.1:8080/healthz` | `200 {"status":"ok"}` | Control-plane process is down or not listening on `ORCH_LISTEN_ADDR` |
| `curl -s 127.0.0.1:8080/readyz` | `200`, `etcd: ok` | `503`: etcd unreachable or slower than 2 s; all reads and writes fail with `STORE_UNAVAILABLE` |
| `curl -s 127.0.0.1:9101/healthz` | `200` with the node name | Agent down or listening elsewhere |
| `GET /api/v1/nodes` → `status.ready` | `true` | See `status.reason`: `HeartbeatTimeout` (agent stopped heartbeating) or `RuntimeUnavailable` (agent cannot reach Docker) |
| Console top bar | "API ready" | "API unreachable" (no response) or "etcd unavailable" (`/readyz` 503) |

## Reading logs

Logs are `slog` text by default (`ORCH_LOG_FORMAT=json` for JSON). Useful fields: `component` (`api`, `scheduler`, `replicaset-controller`, `node-monitor`, `node-agent`, `store`, `events`), `loop`, `pod`, `node`, `request_id`, `error`.

- `msg="pass failed" loop=scheduler consecutive_failures=3 retry_in=8s` — a loop is backing off; the `error` field says why (usually etcd).
- `msg=event reason=FailedScheduling` — a pod cannot be placed; the message lists excluded nodes.
- `msg="request failed" ... error=...` — a 5xx response; match `request_id` with the client's error body.
- Successful GETs, heartbeats and health probes log at `debug` to keep `info` readable.

Every event is also logged, so events lost on restart can still be found in the log.

## Diagnosing etcd failures

Symptoms: `/readyz` 503, console banner "etcd unavailable", API `503 STORE_UNAVAILABLE`, loops logging `pass failed` with `state store unavailable`.

1. `docker compose ps etcd` — is it running and healthy?
2. `docker compose logs --tail 50 etcd`.
3. `docker compose exec etcd etcdctl endpoint health`.
4. Check `ORCH_ETCD_ENDPOINTS` (default `127.0.0.1:2379`) and that the port is bound on loopback.

Nothing is lost while etcd is down: loops back off and resume. Agents keep running existing containers but cannot report status.

**Reconnect time.** The etcd client's reconnect backoff is capped at 5 seconds, so once etcd is back the control plane recovers within a few seconds without a restart. If it ever doesn't, restarting the control plane is always safe: all state lives in etcd.

**Port 2379 already in use.** On Ubuntu, the `etcd` package (`apt install etcd-server`) runs its own etcd as a systemd service on the same port, and `make etcd-up` fails with *address already in use*. Find it with `sudo ss -ltnp | grep 2379`. Either stop it (`sudo systemctl stop etcd`, and `sudo systemctl disable etcd` to keep it off after reboots) and use the Compose etcd, or keep using it: this project only writes under `/mini-orchestrator/v1/`, so it won't collide with other data. With the system etcd, reset state with `etcdctl del --prefix /mini-orchestrator/v1/` instead of `make etcd-reset`.

## Diagnosing Docker failures

Symptoms: node `reason: RuntimeUnavailable`; agent log `container runtime unavailable`; pods on that node become `Unknown`, then `Failed/NodeLost` after the eviction timeout.

1. `docker version` as the user running the agent. "permission denied" on `/var/run/docker.sock` means the user is not in the `docker` group (or use rootless Docker).
2. Set `AGENT_DOCKER_BINARY` if `docker` is not on `PATH`.
3. Docker Desktop: make sure it is running.

Image problems are reported per pod: `ImagePullFailed` (wrong name, private registry, rate limit, no network) or `ContainerStartFailed`. The pod's `status.message` holds the (truncated) Docker error. ReplicaSets replace such pods, which repeats the failure every few seconds until the image is fixed; delete or scale down the ReplicaSet to stop it.

## Pending pods

`GET /api/v1/pods/{name}` → `status.reason`/`message`:

| Reason | Fix |
|---|---|
| `AwaitingScheduling` | Normal for up to `ORCH_SCHEDULER_INTERVAL` |
| `NoEligibleNodes` | No Ready, schedulable node with a fresh heartbeat: start an agent, uncordon a node, or fix Docker |
| `InsufficientCapacity` | All eligible nodes use `AGENT_MAX_PODS` slots: add an agent or raise `AGENT_MAX_PODS` (restart the agent) |

## Stale or lost nodes

- **Heartbeat older than the timeout** → node NotReady (`HeartbeatTimeout`); its Scheduled/Running pods become `Unknown`. Restart the agent: it re-registers, and running containers are reported `Running` again.
- **NotReady longer than `ORCH_POD_EVICTION_TIMEOUT`** → pods become `Failed` (`NodeLost`); ReplicaSets create replacements elsewhere. If the old agent returns, it stops the old containers because their pods are terminal.
- **Terminating pods on a lost node** are force-deleted by the node monitor (event `ForceDeleted`). The container may still exist on that host; remove it with `docker rm -f` using the `io.mini-orchestrator.pod-uid` label.
- **Removing a node:** cordon it (`PATCH /api/v1/nodes/{name} {"unschedulable":true}`), delete or scale away its pods, stop the agent, then `DELETE /api/v1/nodes/{name}`. The API refuses while pods remain. A still-running agent re-registers.

## Restarts and crash loops

With `restartPolicy: Always` the agent restarts an exited container immediately the first time, then waits 5 s, 10 s, 20 s ... up to 2 min between attempts (`AGENT_RESTART_BACKOFF_*`). During back-off the phase stays `Running` with reason `RestartBackOff`. After `AGENT_MAX_RESTARTS` (5) the pod is `Failed` (`RestartLimitExceeded`). Inspect with the Logs page or `docker logs mo-{pod}-{uid8}`.

## Data persistence and reset

- Cluster state lives in the Compose volume `etcd-data` and survives `make etcd-down` and host restarts.
- `make etcd-reset` deletes it permanently. There is no confirmation prompt; it is a development command.
- Events are in control-plane memory only. Logs are read from Docker on demand; deleting a pod removes its container and its logs.

## Backup and restore (limitations)

There is no built-in backup. For a manual snapshot of the development etcd:

```bash
docker compose exec etcd etcdctl snapshot save /etcd-data/backup.db
docker compose cp etcd:/etcd-data/backup.db ./etcd-backup.db
```

Restoring requires `etcdutl snapshot restore` into a fresh data directory and is not scripted. Restoring old state while agents run newer containers will be reconciled by the agents (orphans removed, missing containers started), which may restart workloads.

## Security boundaries

- **No authentication, no TLS.** Anyone who can reach the API can create pods that run arbitrary images, delete workloads, and register nodes. Registered node addresses are fetched by the control plane for logs (validated to be plain http(s) URLs with no path), so an untrusted client could point it at another local service. Keep `ORCH_LISTEN_ADDR` and `AGENT_LISTEN_ADDR` on loopback or a trusted private network.
- **Agent identity is not verified.** An agent claims a node name; the API trusts it.
- **Docker socket access is root-equivalent.** Containers are started without `--privileged`, mounts or published ports, but the agent itself can do anything Docker can.
- **etcd** in Compose has no authentication and is bound to `127.0.0.1` only.
- **CORS** allows only the configured origins and never credentials. CORS is a browser protection, not access control.
- Secrets: none are required. Do not commit `.env` files; `.gitignore` excludes them.
