import { Link } from "react-router-dom";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { PageHeader } from "@/components/console/PageHeader";
import { EmptyState, ErrorPanel, StaleDataNotice, TableSkeleton } from "@/components/console/States";
import { Badge, NodeHealthBadge } from "@/components/console/StatusBadge";
import { RelativeTime } from "@/components/console/Time";
import { useInfo, useNodes, usePods } from "@/hooks/queries";
import { nodeHealth, occupiesNode } from "@/lib/format";

export default function Nodes() {
  const { data, error, isPending, refetch, dataUpdatedAt } = useNodes();
  const pods = usePods();
  const info = useInfo();
  const now = Date.now();
  const load = new Map<string, number>();
  for (const p of pods.data ?? []) if (occupiesNode(p)) load.set(p.status.nodeName as string, (load.get(p.status.nodeName as string) ?? 0) + 1);

  return (
    <>
      <PageHeader
        title="Nodes"
        description={
          <>
            Nodes appear when a node agent registers. A node is ready while its agent heartbeats
            {info.data ? ` (timeout ${info.data.nodeHeartbeatTimeoutSeconds}s)` : ""} and can reach Docker.
          </>
        }
      />
      <div className="panel">
        {error && data && <StaleDataNotice error={error} updatedAt={dataUpdatedAt} />}
        {isPending ? (
          <TableSkeleton columns={6} />
        ) : error && !data ? (
          <ErrorPanel error={error} what="nodes" onRetry={() => refetch()} />
        ) : !data || data.length === 0 ? (
          <EmptyState title="No nodes registered">
            Start a node agent: <code className="ident">make run-agent NODE=worker-1</code>. Agents register themselves; there is no manual
            registration.
          </EmptyState>
        ) : (
          <div className="overflow-x-auto">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Name</TableHead>
                  <TableHead>Health</TableHead>
                  <TableHead>Scheduling</TableHead>
                  <TableHead>Pod slots used</TableHead>
                  <TableHead>Last heartbeat</TableHead>
                  <TableHead>Registered</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {data.map((n) => (
                  <TableRow key={n.uid}>
                    <TableCell>
                      <Link className="ident font-medium text-primary hover:underline" to={`/nodes/${encodeURIComponent(n.name)}`}>
                        {n.name}
                      </Link>
                    </TableCell>
                    <TableCell>
                      <span title={n.status.message}>
                        <NodeHealthBadge health={nodeHealth(n, info.data?.nodeHeartbeatTimeoutSeconds, now)} />
                      </span>
                    </TableCell>
                    <TableCell>{n.spec.unschedulable ? <Badge tone="warning">Cordoned</Badge> : <span className="text-sm">Schedulable</span>}</TableCell>
                    <TableCell className="tabular-nums">
                      {pods.data ? `${load.get(n.name) ?? 0} / ${n.status.maxPods}` : "Not available"}
                    </TableCell>
                    <TableCell className="text-muted-foreground">
                      <RelativeTime value={n.status.lastHeartbeatTime} />
                    </TableCell>
                    <TableCell className="text-muted-foreground">
                      <RelativeTime value={n.registeredAt} />
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        )}
      </div>
      <p className="mt-3 text-xs text-muted-foreground">
        CPU and memory are not measured. Capacity is the pod-slot limit each agent declares (AGENT_MAX_PODS).
      </p>
    </>
  );
}
