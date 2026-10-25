# Project audit

Part 1 is the state of the original prototype. Part 2 is what changed, how it was verified, and what is still open.

---

## Part 1: The original prototype

### Repository contents

`backend/main.go`, `backend/controller.go`, `backend/node_agent.go`, `go.mod`/`go.sum`; a Vite + React + shadcn/ui frontend generated from a template (package name `obsidiank8s`); a README. No tests (except a placeholder `expect(true)`), no CI, no Compose file, no LICENSE file.

### Intended architecture and actual data flow

The three Go files were meant to be run individually (`go run main.go`, etc.):

- `main.go`: HTTP API on `:8080` plus a scheduler goroutine watching `/pods/` in etcd.
- `controller.go`: a replica controller keeping `DESIRED_REPLICAS` pods named `nginx-pod-N`, talking to the API over HTTP.
- `node_agent.go`: registers a node, heartbeats CPU usage from `top`, watches etcd directly and shells out to `docker`.

Intended pod flow: `POST /pods` → etcd `Pending` → scheduler picks the node with lowest reported CPU and writes **`Running`** immediately → the agent sees `Running` for its node and runs `docker run` → status updates via `/updatePodStatus`.

### Build status

**The backend did not compile.** `go vet ./...` on the original code stops with:

```text
controller.go:24:17: string literal not terminated
main.go:427:14: string literal not terminated
node_agent.go:31:17: string literal not terminated
```

Root causes found while reading the code:

- All three files are `package main` in one directory, each declaring `main`, `Pod`, `client` and `initEtcd` → duplicate declarations when building the module.
- Calls to non-existent functions `length(...)`, `size(...)`, `count(...)` instead of `len`.
- Truncated string literals: `var serverURL = "http:` and `fmt.Println("API Server running at http:` (the rest of each line was lost).
- `\n` replaced by `\total` in format strings; mixed spaces/tabs; not gofmt-formatted.
- `go.etcd.io/etcd/client/v3` marked `// indirect` although imported directly.

### Correctness and reliability issues (ranked)

**P0**
- Does not compile (above).
- Scheduler marks pods `Running` at bind time, before any container exists; status could not distinguish "assigned" from "running".
- Read-modify-write without compare-and-swap everywhere (pods, nodes, status); the scheduler, agent and API could overwrite each other; a pod could be bound twice.
- Errors ignored: `json.Unmarshal`, `client.Put` results, `json.Marshal`; failed etcd writes still returned 200.
- `GET /logs/{name}` passed the raw URL path to `docker logs` and returned unbounded output.

**P1**
- No timeouts on etcd (`context.Background()`), Docker commands, or the HTTP server; `http.Post` without a client timeout in the agent.
- Scheduler skips any node reporting exactly 0% CPU and any ≥ 80%; CPU from `top | awk` is host-wide and not a capacity model.
- Heartbeat freshness hard-coded (10 s) in the scheduler; nodes were never marked stale; pods on dead nodes stayed `Running` forever.
- Agent kept `runningPods`/`restartCount` in unsynchronised global maps (single goroutine in practice, but restart counts reset on agent restart).
- Watch errors were `continue`d forever (scheduler) without re-listing; compaction or cancellation would stop scheduling silently.
- Replica controller `shouldReconcile` used unsynchronised global state from two goroutines (data race).
- Deleting a pod record before its container was removed; containers keyed only by pod name (collisions after re-creation).
- `CORS: *`; no body size limits; `panic` on etcd client creation.
- Frontend dev server and API both on port 8080.

**P2**
- Inconsistent routes (`/pods` create vs `/getpods` list, `/nodes` register vs `/getnodes` list, `/updatePodStatus`); frontend called `DELETE /nodes/{name}`, which did not exist.
- Plain-text errors; no request IDs; `fmt.Println` logging.
- UI: CPU area chart accumulated client-side from `cpu_usage` (unreliable source presented as telemetry); "cluster online" derived from a successful pod list; "Register Node" dialog created nodes with no agent and a user-typed CPU value; `alert()`/`window.location.reload()` for node deletion; logs dialog claimed to "stream".
- README described a package layout, `docker-compose.yml`, ReplicaSet and Event APIs, and an MIT license that did not exist in the repository.

### API inventory (baseline)

| Route | Method check | Used by |
|---|---|---|
| `/` | any | — |
| `/pods` | POST create | UI, controller |
| `/getpods` | GET | UI, controller, agent |
| `/pods/{name}` | DELETE | UI, controller |
| `/nodes` | POST register | UI, agent |
| `/getnodes` | GET | UI |
| `/heartbeat` | POST | UI, agent |
| `/updatePodStatus` | POST | UI client (unused), agent |
| `/logs/{name}` | any | UI |
| `DELETE /nodes/{name}` | — (missing) | UI |

### Baseline commands

| Command | Result |
|---|---|
| `go vet ./...`, `go test ./...` | Fail to compile (errors above) |
| Frontend `npm run lint` | Not meaningful: the UI depended on routes and fields that the backend could not serve |

### Implementation sequence chosen

1. Domain types, validation and the store (with an in-memory test double) so everything else is testable.
2. Lifecycle rules, scheduler, controllers, loop runner.
3. Runtime interface and agent.
4. API, then entry points.
5. Console rebuilt on the new typed API.
6. Compose, Makefile, CI, docs.

---

## Part 2: Changes, verification and open items

### What changed

**Backend.** Rewritten as two binaries (`cmd/controlplane`, `cmd/agent`) over focused packages: `domain`, `store`, `lifecycle`, `scheduler`, `controller`, `api`, `agent`, `runtime`, `loop`, `events`, `config`, `logging`, `httpjson`. The prototype's ideas all survive: etcd state, a scheduler, a replica controller, a Docker-based agent with restart counting and a crash limit, heartbeats and logs. What's new is correctness: compare-and-swap writes, real pod phases, deadlines everywhere, stale-node handling, and 85 unit tests.

**Frontend.** The template dashboard became an operator console (Overview, Pods, Pod detail, ReplicaSets, Nodes, Node detail, Events, Logs, About) on a typed API client. No fabricated metrics, confirmation for every destructive action, light theme, dev server on port 5173. Template lint problems were fixed (empty interfaces, `require` in the Tailwind config) and the unused `chart.tsx` wrapper was removed.

**DevOps.** `docker-compose.yml` (etcd on loopback with a health check), `Makefile`, GitHub Actions CI, `scripts/smoke.sh`, `.gitignore`, `frontend/.env.example`.

**Docs.** README, User guide, Architecture, API, Development, Operations, Decisions, Roadmap and this audit.

### Verification

Run on Ubuntu 24.04 with Go 1.25.7, Node 22, Docker Engine 29 and Compose v5:

| Check | Result |
|---|---|
| `go mod tidy` | OK |
| `go vet ./...` | Clean |
| `go test ./...` | All 11 packages with tests pass |
| `go test -race -count=1 ./...` | All pass, no races reported |
| `gofmt -l .` | Two files needed formatting (`config.go`, `store.go`); fixed |
| `npm ci` | OK (npm reports advisories in template dependencies, see open items) |
| `npm run lint` | 3 errors, all in the unused template file `components/ui/chart.tsx`; file removed. Remaining output is `react-refresh` warnings only |
| `npm run typecheck` | 2 errors in `client.test.ts` (an `unknown` catch value); fixed |
| `npm test` | 14 tests pass |
| `npm run build` | OK, about 141 kB gzipped JavaScript |
| Control plane against etcd | Starts, connects, serves `/readyz` |

Found during that run and fixed afterwards:

- After a long etcd outage, the etcd client's default gRPC reconnect backoff could keep `/readyz` failing for up to two minutes after etcd came back. The reconnect backoff is now capped at 5 seconds, and the client library's own warnings are muted in favour of the store's structured logs.
- On machines with Ubuntu's `etcd` package installed, `make etcd-up` fails because port 2379 is taken. Documented in the README and OPERATIONS.md, with both ways to resolve it.

Re-run `make ci` and `make smoke` after pulling these final changes; they touch the etcd client setup and the test file mentioned above.

### Acceptance checklist

| Criterion | Status |
|---|---|
| Validation and consistent errors | Done, tested |
| Testable storage and runtime boundaries | Done (`KV` + `MemoryKV`, `Runtime` + `Fake`) |
| Deterministic, explainable scheduler | Done, table tests |
| Idempotent, cancellation-aware reconciliation | Done, tested |
| Explicit heartbeat and stale-node behaviour | Done, tested |
| Shared state checked for races | `go test -race` passes |
| Deadlines on every dependency call | Done (etcd, Docker, HTTP client and server) |
| Health vs readiness | Done |
| Console on real data with loading, empty, error, stale and retry states | Done |
| Lint, typecheck, build | Pass after the fixes above |
| One documented local setup, config reference, CI, docs | Done |
| No secrets or runtime artefacts in the repo | Checked |

### Open items

1. No automated integration tests against real etcd and Docker yet; `make smoke` is the end-to-end check.
2. No authentication or TLS; node identity is trusted.
3. Polling instead of etcd watches; no leader election.
4. No metrics endpoint; events are in memory only; logs are snapshots.
5. `npm audit` reports advisories in template dependencies, and several template packages are unused. Pruning them needs a lockfile regeneration and a careful upgrade pass.
6. No screenshots in the README yet.
