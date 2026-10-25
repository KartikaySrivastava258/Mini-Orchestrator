# Development guide

## Prerequisites

| Tool | Version | Used for |
|---|---|---|
| Go | 1.25+ | Backend (`backend/go.mod`) |
| Node.js / npm | 20+ | Console |
| Docker Engine + Compose v2 | recent | etcd container; node agents run pods through Docker |
| GNU Make, Bash, curl | any | `Makefile`, `scripts/smoke.sh` |

## First-time setup

```bash
make etcd-up                 # etcd on 127.0.0.1:2379 (named volume etcd-data)
cd backend && go mod tidy    # downloads modules and checks go.sum; commit any changes
cd .. && make frontend-install
```

Then follow the Quick start in the README (control plane, one or more agents, console).

## Commands

| Command | What it does |
|---|---|
| `make test` | Go unit tests. No etcd or Docker needed. |
| `make test-race` | Go tests with the race detector |
| `make vet` / `make fmt` / `make fmt-check` | `go vet`, format, check formatting |
| `make build` | Build `backend/bin/controlplane` and `backend/bin/agent` |
| `make frontend-lint` / `frontend-typecheck` / `frontend-test` / `frontend-build` | ESLint, `tsc --noEmit`, Vitest, Vite build |
| `make lint` | vet, gofmt check, ESLint, TypeScript |
| `make ci` | Everything CI runs |
| `make smoke` | `scripts/smoke.sh` against `API=http://127.0.0.1:8080` |
| `make compose-config` | Validate `docker-compose.yml` |
| `make clean-containers` | Remove every container the agents created |

Single packages or tests: `cd backend && go test ./internal/scheduler -run TestPlace -v`.

## Code layout and conventions

- Go: `gofmt`, `go vet`, standard library first. Packages under `backend/internal/` follow the responsibilities in ARCHITECTURE.md. Domain types and the request contract are in `internal/domain`; state-transition rules in `internal/lifecycle`. New background work goes through `loop.Run`.
- Every function that does I/O takes a `context.Context` and respects deadlines.
- Errors: wrap with `%w`; storage errors are the sentinels in `internal/store`; the API maps them in `writeStoreError` and never returns raw error text for 5xx.
- Logs: `log/slog` with `component`, and fields such as `pod`, `node`, `request_id`, `error`.
- Comments: short and human. Say why, not what. A bit of personality is welcome; a paragraph restating the code is not.
- Frontend: typed calls only through `src/lib/api/client.ts`; data through the hooks in `src/hooks/queries.ts` (one query key per resource). Keep TypeScript types in `src/lib/api/types.ts` in sync with `backend/internal/domain`.
- Do not display a value the API does not provide; show "Not available" instead.

## Tests

- **Store:** `store.NewMemoryKV()` reproduces etcd revision semantics; `SetFailure` simulates an outage; `PutRaw` injects malformed data.
- **Agent:** `runtime.NewFake()` plus an in-memory control plane (`agent_test.go`) that applies the real transition rules.
- **Time:** components expose `SetClock` so tests control "now" instead of sleeping.
- **Frontend:** Vitest with jsdom; `fetch` is stubbed with `vi.stubGlobal`.

Integration tests against real etcd and Docker do not exist yet (roadmap). When added, they must be opt-in (for example a build tag `integration`) and use a unique `ORCH_ETCD_PREFIX` per run so they never touch a developer's data.

## Debugging

- `ORCH_LOG_LEVEL=debug` shows every request and loop pass; `ORCH_LOG_FORMAT=json` for machine-readable logs.
- Inspect etcd directly: `docker compose exec etcd etcdctl get --prefix /mini-orchestrator/v1/ --keys-only`.
- Inspect agent containers: `docker ps -a --filter label=io.mini-orchestrator.managed=true`.
- Send `X-Request-ID: something` with a request and search the control-plane log for it.

## Resetting local state

```bash
make etcd-reset   # deletes the etcd volume: all pods, nodes and ReplicaSets
make clean-containers   # leftover containers
```

Stop agents before resetting etcd; otherwise they re-register immediately (which is harmless).

## Contribution workflow

1. Create a branch.
2. Make a focused change with tests; update docs that describe the behaviour you changed (API.md for any contract change, ARCHITECTURE.md for lifecycle changes, DECISIONS.md for new trade-offs).
3. Run `make ci` (and `make smoke` against a running stack for runtime changes).
4. Open a pull request; CI runs the same checks.
