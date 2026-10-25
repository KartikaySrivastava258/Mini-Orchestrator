# User guide

How to actually use Mini Orchestrator once it's running. If it isn't running yet, do the [Quick start](../README.md#quick-start) first.

You'll need the control plane, at least one agent, and the console open at **http://localhost:5173**.

---

## The console in 60 seconds

The sidebar has everything:

| Page | What it's for |
|---|---|
| **Overview** | One-glance health: pod counts, pod phases, nodes, ReplicaSets, recent events, and anything that needs attention |
| **Pods** | Every pod. Search, filter by phase or node, sort, delete, jump to logs |
| **ReplicaSets** | Create, scale and delete groups of identical pods |
| **Nodes** | Every registered node, its health, how full it is, when it last checked in |
| **Events** | What happened and why: scheduling, restarts, nodes going quiet |
| **Logs** | Pick a pod, read its container output |
| **About** | What this is, how it maps to Kubernetes, current control-plane settings |

The top bar shows whether the console can reach the API:

- **API ready**: the control plane is up and etcd answers.
- **etcd unavailable**: the control plane is up, but etcd isn't. Nothing can be read or saved.
- **API unreachable**: the control plane isn't answering at all.

"API ready" does **not** mean every node and pod is healthy. That's what the Overview is for.

Pages refresh themselves every 5 seconds while the tab is visible. If a refresh fails but older data is still on screen, you'll see a yellow banner saying so, so you never mistake stale data for live data.

---

## Guided demo (about 10 minutes)

Run two agents for this one, so the scheduler has a choice:

```bash
make run-agent NODE=worker-1 AGENT_PORT=9101
make run-agent NODE=worker-2 AGENT_PORT=9102
```

### 1. Meet your nodes

Open **Nodes**. You should see `worker-1` and `worker-2`, both **Ready**, with `0 / 10` pod slots used and a heartbeat a few seconds ago. Click one to see its details: agent address, when it registered, whether Docker is reachable.

### 2. Run a single pod

On **Pods**, click **Create pod**:

- Name: `hello`
- Image: `nginx:stable`
- When the container exits: **Restart it**

Watch it go **Pending → Scheduled → Running** within a few seconds. The first run may take longer while Docker pulls the image. Click the pod name to see its spec (what you asked for) next to its status (what actually happened), plus its events and logs.

### 3. Let a ReplicaSet do the work

On **ReplicaSets**, click **Create ReplicaSet**:

- Name: `web`
- Image: `nginx:stable`
- Replicas: `4`

Go to **Pods** and filter by node: two pods land on each worker. That's the `LeastPods` scheduler spreading the load. The ReplicaSets page shows a little row of blocks, one per desired replica, filling in as pods start running.

### 4. Scale down

Click **Scale** on `web` and set it to `1`. The dialog warns you that 3 pods will be terminated, and which ones go first (unscheduled, then the newest running). Confirm, then watch them go **Terminating** and disappear.

### 5. Break things on purpose

**Bad image.** Create a pod with image `this-image-does-not-exist:nope`. It fails with reason `ImagePullFailed`, and the pod page shows the Docker error.

**Crash loop.** Create a pod with image `busybox` and restart policy **Restart it**. `busybox` exits immediately, so the agent restarts it with growing delays (reason `RestartBackOff`). After 5 restarts it gives up: `Failed`, reason `RestartLimitExceeded`. It shows up under **Needs attention** on the Overview.

**Validation.** Try a pod named `Hello_World` or an image like `--privileged`. The form, and the API behind it, both refuse.

### 6. Lose a node

Scale `web` back up to `4`. Then press `Ctrl+C` on the `worker-2` agent.

- After about 15 seconds, `worker-2` turns **Not ready** and its pods go **Unknown**.
- About 60 seconds later, those pods become **Failed** with reason `NodeLost`, and the ReplicaSet creates replacements on `worker-1`.
- Check **Events** to see the whole story in order.

Start `worker-2` again and it re-registers. The containers it left running are removed as orphans on its first sync, because the pods they belonged to were replaced and no longer exist.

### 7. Cordon and remove a node

Open `worker-2` on **Nodes** and click **Cordon**. New pods won't be scheduled there, but existing ones keep running. Try **Remove node** while it still has pods: the API refuses and tells you why. Delete or scale away its pods, stop its agent, then remove it.

### 8. Clean up

Delete the `web` ReplicaSet; its pods go with it. Delete `hello` and the broken pods from **Pods**. To wipe everything, use `make etcd-reset`.

---

## Everyday tasks

| I want to... | Do this |
|---|---|
| Run one container | **Pods → Create pod** |
| Run N copies and keep them running | **ReplicaSets → Create ReplicaSet** |
| Change how many copies | **ReplicaSets → Scale** |
| See why a pod isn't starting | Click the pod; read the reason and message at the top, then its events |
| Read output | **Logs**, or the Logs section on the pod page |
| Stop new pods landing on a node | **Nodes → (node) → Cordon** |
| See everything that happened | **Events**, filter by type or object |

Deleting a pod that belongs to a ReplicaSet just makes the ReplicaSet create a new one. The delete dialog warns you about that. To have fewer pods, scale the ReplicaSet instead.

---

## What the statuses mean

### Pod phases

| Phase | Meaning |
|---|---|
| **Pending** | Accepted, waiting for a node |
| **Scheduled** | Assigned to a node; the agent hasn't started the container yet |
| **Running** | The agent started the container (check the reason if it's backing off) |
| **Succeeded** | Exited with code 0 and restart policy was "Leave it stopped" |
| **Failed** | Done for good: bad image, crash limit, non-zero exit with "Leave it stopped", or its node was lost |
| **Unknown** | Its node stopped reporting, so nobody can confirm what's happening |
| **Terminating** | Delete requested; waiting for the agent to remove the container |

### Common reasons

| Reason | What happened |
|---|---|
| `AwaitingScheduling` | Just created, the scheduler will pick it up in a moment |
| `NoEligibleNodes` | No ready, uncordoned node with a recent heartbeat |
| `InsufficientCapacity` | Every eligible node is full (`AGENT_MAX_PODS`) |
| `ContainerStarted` / `ContainerRestarted` | Running normally / running again after a crash |
| `RestartBackOff` | Crashed; waiting before the next restart |
| `RestartLimitExceeded` | Crashed too many times, gave up |
| `ImagePullFailed` / `ContainerStartFailed` | Docker couldn't get or start the image |
| `Completed` / `ContainerExited` | Finished with exit code 0 / non-zero |
| `NodeNotReady` / `NodeLost` | Its node went quiet / stayed quiet too long |

### Node health

| Badge | Meaning |
|---|---|
| **Ready** | Heartbeating and Docker is reachable |
| **Stale heartbeat** | Marked ready, but the last heartbeat is older than the timeout (the control plane will mark it not ready on its next check) |
| **Runtime unavailable** | Agent is alive, but it can't reach Docker |
| **Not ready** | No heartbeat within the timeout |
| **Cordoned** | Won't receive new pods |

---

## FAQ

**Why don't I see CPU or memory graphs?**
Because nothing measures them yet, and the console doesn't show numbers it doesn't have. Metrics are on the roadmap.

**Can I open nginx in my browser?**
Not yet. Containers don't publish ports. Port support is on the roadmap.

**Are logs live?**
They're snapshots. Turn on **Refresh every 5s** to re-fetch automatically. It's polling, and it's labelled that way.

**Where did my events go?**
Events are kept in control-plane memory (the last 500) and reset when it restarts. They're also written to the control-plane log.

**Can I use kubectl?**
No. The API is Kubernetes-inspired but not Kubernetes-compatible.
