import { useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { ConfirmDialog } from "@/components/console/ConfirmDialog";
import { DetailList, PageHeader, Section } from "@/components/console/PageHeader";
import { PodTable } from "@/components/console/PodTable";
import { EmptyState, ErrorPanel, TableSkeleton } from "@/components/console/States";
import { Badge, NodeHealthBadge } from "@/components/console/StatusBadge";
import { queryKeys, useInfo, useNode, usePods } from "@/hooks/queries";
import { ApiError, deleteNode, errorMessage, setNodeSchedulable } from "@/lib/api/client";
import { exactTime, nodeHealth, occupiesNode, relativeTime } from "@/lib/format";

export default function NodeDetail() {
  const { name = "" } = useParams();
  const navigate = useNavigate();
  const qc = useQueryClient();
  const { data: node, error, isPending, refetch } = useNode(name);
  const pods = usePods();
  const info = useInfo();
  const [confirmDelete, setConfirmDelete] = useState(false);

  const cordon = useMutation({
    mutationFn: (unschedulable: boolean) => setNodeSchedulable(name, unschedulable),
    onSuccess: (n) => {
      qc.invalidateQueries({ queryKey: queryKeys.nodes });
      toast.success(n.spec.unschedulable ? `Cordoned ${n.name}` : `Uncordoned ${n.name}`);
    },
    onError: (err) => toast.error("Could not change scheduling", { description: errorMessage(err) }),
  });
  const remove = useMutation({
    mutationFn: () => deleteNode(name),
    onSuccess: (res) => {
      qc.invalidateQueries({ queryKey: queryKeys.nodes });
      toast.success(`Removed node ${res.name}`, { description: res.message });
      setConfirmDelete(false);
      navigate("/nodes");
    },
    onError: (err) => toast.error("Could not remove node", { description: errorMessage(err) }),
  });

  if (isPending) return <TableSkeleton rows={6} columns={2} />;
  if (error instanceof ApiError && error.status === 404) {
    return (
      <EmptyState title={`Node ${name} is not registered`} action={<Link className="text-primary underline" to="/nodes">Back to nodes</Link>}>
        If its agent is still running it will register again on its next heartbeat.
      </EmptyState>
    );
  }
  if (error || !node) return <ErrorPanel error={error} what={`node ${name}`} onRetry={() => refetch()} />;

  const nodePods = (pods.data ?? []).filter((p) => p.status.nodeName === node.name);
  const occupying = nodePods.filter(occupiesNode).length;

  return (
    <>
      <PageHeader
        title={node.name}
        description={
          <span className="inline-flex flex-wrap items-center gap-2">
            <NodeHealthBadge health={nodeHealth(node, info.data?.nodeHeartbeatTimeoutSeconds)} />
            {node.spec.unschedulable && <Badge tone="warning">Cordoned</Badge>}
          </span>
        }
        actions={
          <>
            <Button variant="outline" size="sm" disabled={cordon.isPending} onClick={() => cordon.mutate(!node.spec.unschedulable)}>
              {node.spec.unschedulable ? "Uncordon" : "Cordon"}
            </Button>
            <Button variant="outline" size="sm" className="text-destructive" onClick={() => setConfirmDelete(true)}>
              Remove node
            </Button>
          </>
        }
      />
      <div className="grid gap-4">
        <Section title="Details">
          <div className="p-4">
            <DetailList
              items={[
                { label: "Condition", value: `${node.status.reason}${node.status.message ? `: ${node.status.message}` : ""}` },
                { label: "Last heartbeat", value: `${relativeTime(node.status.lastHeartbeatTime)} (${exactTime(node.status.lastHeartbeatTime)})` },
                { label: "Ready since / not ready since", value: exactTime(node.status.lastTransitionTime) },
                { label: "Container runtime", value: node.status.runtimeReady ? "Reachable" : "Unreachable" },
                { label: "Pod slots", value: `${occupying} used of ${node.status.maxPods} (declared by the agent)` },
                { label: "Agent address", value: <span className="ident">{node.status.address}</span> },
                { label: "Registered", value: exactTime(node.registeredAt) },
                { label: "UID", value: <span className="ident break-all">{node.uid}</span> },
              ]}
            />
          </div>
        </Section>
        <Section title={`Pods on ${node.name}`}>
          {pods.isPending ? <TableSkeleton columns={6} /> : <PodTable pods={nodePods} hideNode emptyText="No pods are assigned to this node." />}
        </Section>
      </div>
      <ConfirmDialog
        open={confirmDelete}
        onOpenChange={setConfirmDelete}
        title={`Remove node ${node.name}?`}
        consequences={
          occupying > 0 ? (
            <p>
              This node still has {occupying} pod{occupying === 1 ? "" : "s"}, so the API will refuse. Cordon it, then delete or scale away its pods
              first.
            </p>
          ) : (
            <>
              <p>The node record is deleted. No pods are assigned to it.</p>
              <p>If its agent is still running, it registers again on its next heartbeat. Stop the agent first to remove the node permanently.</p>
            </>
          )
        }
        confirmLabel="Remove node"
        pending={remove.isPending}
        onConfirm={() => remove.mutate()}
      />
    </>
  );
}
