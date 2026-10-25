import { Link } from "react-router-dom";
import { CreatePodDialog } from "@/components/console/CreatePodDialog";
import { PageHeader, Section } from "@/components/console/PageHeader";
import { PhaseBar, ReconcileStrip } from "@/components/console/PhaseBar";
import { ErrorPanel, TableSkeleton } from "@/components/console/States";
import { NodeHealthBadge, PhaseBadge } from "@/components/console/StatusBadge";
import { RelativeTime } from "@/components/console/Time";
import { useEvents, useInfo, useNodes, usePods, useReadiness, useReplicaSets } from "@/hooks/queries";
import { countPhases, needsAttention, nodeHealth } from "@/lib/format";
import { cn } from "@/lib/utils";

function Figure({ label, value, hint, tone }: { label: string; value: React.ReactNode; hint?: string; tone?: "danger" | "warning" }) {
  return (
    <div className="px-4 py-3">
      <dt className="text-sm text-muted-foreground">{label}</dt>
      <dd className={cn("mt-1 text-2xl font-semibold tabular-nums", tone === "danger" && "text-destructive", tone === "warning" && "text-warning")}>
        {value}
      </dd>
      {hint && <p className="mt-0.5 text-xs text-muted-foreground">{hint}</p>}
    </div>
  );
}

export default function Overview() {
  const pods = usePods();
  const nodes = useNodes();
  const sets = useReplicaSets();
  const events = useEvents();
  const ready = useReadiness();
  const info = useInfo();
  const now = Date.now();

  const counts = countPhases(pods.data ?? []);
  const attention = (pods.data ?? []).filter(needsAttention);
  const health = (nodes.data ?? []).map((n) => nodeHealth(n, info.data?.nodeHeartbeatTimeoutSeconds, now));
  const readyNodes = health.filter((h) => h === "Ready").length;
  const unavailable = "Not available";

  return (
    <>
      <PageHeader
        title="Overview"
        description="Current state of this Mini Orchestrator cluster, read from the control-plane API every 5 seconds."
        actions={<CreatePodDialog />}
      />

      {ready.data?.status === "not ready" && (
        <div role="alert" className="mb-4 border-l-4 border-destructive bg-destructive/5 px-4 py-3 text-sm">
          The API is running but etcd is unavailable, so no cluster state can be read or written. Start etcd with{" "}
          <code className="ident">make etcd-up</code>.
        </div>
      )}
      {pods.error && !pods.data && (
        <div className="mb-4">
          <ErrorPanel error={pods.error} what="cluster state" onRetry={() => pods.refetch()} />
        </div>
      )}

      <div className="panel mb-4">
        <dl className="grid grid-cols-2 divide-x divide-y sm:grid-cols-3 lg:grid-cols-6 lg:divide-y-0">
          <Figure label="Pods" value={pods.data ? pods.data.length : unavailable} />
          <Figure label="Running" value={pods.data ? counts.Running : unavailable} />
          <Figure label="Pending" value={pods.data ? counts.Pending + counts.Scheduled : unavailable} hint="includes Scheduled" />
          <Figure label="Failed" value={pods.data ? counts.Failed : unavailable} tone={counts.Failed ? "danger" : undefined} />
          <Figure
            label="Nodes ready"
            value={nodes.data ? `${readyNodes} / ${nodes.data.length}` : unavailable}
            tone={nodes.data && readyNodes < nodes.data.length ? "warning" : undefined}
          />
          <Figure label="ReplicaSets" value={sets.data ? sets.data.length : unavailable} />
        </dl>
        <div className="border-t px-4 py-3">
          <p className="mb-2 text-sm font-medium">Pod phases</p>
          {pods.data ? <PhaseBar counts={counts} /> : <p className="text-sm text-muted-foreground">{unavailable}</p>}
        </div>
      </div>

      <div className="grid gap-4 xl:grid-cols-2">
        <Section title="Needs attention" actions={<Link to="/pods" className="text-sm text-primary hover:underline">All pods</Link>}>
          {pods.isPending ? (
            <TableSkeleton rows={2} columns={3} />
          ) : attention.length === 0 ? (
            <p className="px-4 py-6 text-sm text-muted-foreground">No failed, unknown, back-off or unschedulable pods.</p>
          ) : (
            <ul className="divide-y">
              {attention.slice(0, 8).map((p) => (
                <li key={p.uid} className="flex items-start justify-between gap-3 px-4 py-2.5">
                  <div className="min-w-0">
                    <Link to={`/pods/${encodeURIComponent(p.name)}`} className="ident font-medium text-primary hover:underline">
                      {p.name}
                    </Link>
                    <p className="truncate text-xs text-muted-foreground" title={p.status.message}>
                      {p.status.reason}: {p.status.message}
                    </p>
                  </div>
                  <PhaseBadge phase={p.status.phase} reason={p.status.reason} />
                </li>
              ))}
            </ul>
          )}
        </Section>

        <Section title="Nodes" actions={<Link to="/nodes" className="text-sm text-primary hover:underline">All nodes</Link>}>
          {nodes.isPending ? (
            <TableSkeleton rows={2} columns={3} />
          ) : !nodes.data || nodes.data.length === 0 ? (
            <p className="px-4 py-6 text-sm text-muted-foreground">
              No nodes registered. Start an agent with <code className="ident">make run-agent NODE=worker-1</code>.
            </p>
          ) : (
            <ul className="divide-y">
              {nodes.data.map((n, i) => (
                <li key={n.uid} className="flex items-center justify-between gap-3 px-4 py-2.5 text-sm">
                  <Link to={`/nodes/${encodeURIComponent(n.name)}`} className="ident text-primary hover:underline">
                    {n.name}
                  </Link>
                  <span className="flex items-center gap-3 text-muted-foreground">
                    <span className="hidden sm:inline">
                      heartbeat <RelativeTime value={n.status.lastHeartbeatTime} />
                    </span>
                    <NodeHealthBadge health={health[i]} />
                  </span>
                </li>
              ))}
            </ul>
          )}
        </Section>

        <Section title="ReplicaSets" actions={<Link to="/replicasets" className="text-sm text-primary hover:underline">Manage</Link>}>
          {sets.isPending ? (
            <TableSkeleton rows={2} columns={3} />
          ) : !sets.data || sets.data.length === 0 ? (
            <p className="px-4 py-6 text-sm text-muted-foreground">No ReplicaSets.</p>
          ) : (
            <ul className="divide-y">
              {sets.data.map((rs) => (
                <li key={rs.uid} className="grid grid-cols-[1fr_auto] items-center gap-3 px-4 py-2.5 text-sm">
                  <div className="min-w-0">
                    <span className="ident font-medium">{rs.name}</span>
                    <div className="mt-1">
                      <ReconcileStrip desired={rs.spec.replicas} active={rs.status.replicas} running={rs.status.runningReplicas} />
                    </div>
                  </div>
                  <span className="tabular-nums text-muted-foreground">
                    {rs.status.runningReplicas}/{rs.spec.replicas} running
                  </span>
                </li>
              ))}
            </ul>
          )}
        </Section>

        <Section title="Recent events" actions={<Link to="/events" className="text-sm text-primary hover:underline">All events</Link>}>
          {events.isPending ? (
            <TableSkeleton rows={3} columns={3} />
          ) : !events.data || events.data.length === 0 ? (
            <p className="px-4 py-6 text-sm text-muted-foreground">No events since the control plane started.</p>
          ) : (
            <ul className="divide-y">
              {events.data.slice(0, 6).map((e) => (
                <li key={e.id} className="px-4 py-2 text-sm">
                  <div className="flex items-baseline justify-between gap-3">
                    <span className={cn("font-medium", e.type === "Warning" && "text-warning")}>
                      {e.reason} <span className="ident font-normal text-muted-foreground">{e.object.kind}/{e.object.name}</span>
                    </span>
                    <span className="shrink-0 text-xs text-muted-foreground">
                      <RelativeTime value={e.time} />
                    </span>
                  </div>
                  {e.message && <p className="truncate text-xs text-muted-foreground" title={e.message}>{e.message}</p>}
                </li>
              ))}
            </ul>
          )}
        </Section>
      </div>
      {pods.dataUpdatedAt > 0 && (
        <p className="mt-4 text-xs text-muted-foreground">Last successful refresh {new Date(pods.dataUpdatedAt).toLocaleTimeString()}.</p>
      )}
    </>
  );
}
