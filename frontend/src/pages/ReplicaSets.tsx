import { useState } from "react";
import { Link } from "react-router-dom";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Button } from "@/components/ui/button";
import { ConfirmDialog } from "@/components/console/ConfirmDialog";
import { PageHeader } from "@/components/console/PageHeader";
import { ReconcileStrip } from "@/components/console/PhaseBar";
import { CreateReplicaSetDialog, ScaleDialog } from "@/components/console/ReplicaSetDialogs";
import { EmptyState, ErrorPanel, StaleDataNotice, TableSkeleton } from "@/components/console/States";
import { RelativeTime } from "@/components/console/Time";
import { queryKeys, useReplicaSets } from "@/hooks/queries";
import { deleteReplicaSet, errorMessage } from "@/lib/api/client";
import type { ReplicaSet } from "@/lib/api/types";

export default function ReplicaSets() {
  const { data, error, isPending, refetch, dataUpdatedAt } = useReplicaSets();
  const [scaling, setScaling] = useState<ReplicaSet | null>(null);
  const [deleting, setDeleting] = useState<ReplicaSet | null>(null);
  const qc = useQueryClient();
  const del = useMutation({
    mutationFn: (name: string) => deleteReplicaSet(name),
    onSuccess: (res) => {
      qc.invalidateQueries({ queryKey: queryKeys.replicaSets });
      qc.invalidateQueries({ queryKey: queryKeys.pods });
      qc.invalidateQueries({ queryKey: queryKeys.events });
      toast.success(`Deleted ReplicaSet ${res.name}`, { description: res.message });
      setDeleting(null);
    },
    onError: (err, name) => toast.error(`Could not delete ReplicaSet ${name}`, { description: errorMessage(err) }),
  });

  return (
    <>
      <PageHeader
        title="ReplicaSets"
        description="Each ReplicaSet keeps a fixed number of identical pods active. The controller compares desired and observed counts every few seconds."
        actions={<CreateReplicaSetDialog />}
      />
      <div className="panel">
        {error && data && <StaleDataNotice error={error} updatedAt={dataUpdatedAt} />}
        {isPending ? (
          <TableSkeleton columns={5} />
        ) : error && !data ? (
          <ErrorPanel error={error} what="ReplicaSets" onRetry={() => refetch()} />
        ) : !data || data.length === 0 ? (
          <EmptyState title="No ReplicaSets">Create one to run several copies of the same image and have failed copies replaced.</EmptyState>
        ) : (
          <div className="overflow-x-auto">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Name</TableHead>
                  <TableHead>Image</TableHead>
                  <TableHead>Desired vs observed</TableHead>
                  <TableHead>Running / active / desired</TableHead>
                  <TableHead>Age</TableHead>
                  <TableHead className="text-right">
                    <span className="sr-only">Actions</span>
                  </TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {data.map((rs) => (
                  <TableRow key={rs.uid}>
                    <TableCell>
                      <Link className="ident font-medium text-primary hover:underline" to={`/pods?owner=${encodeURIComponent(rs.name)}`}>
                        {rs.name}
                      </Link>
                    </TableCell>
                    <TableCell className="ident text-muted-foreground">{rs.spec.template.image}</TableCell>
                    <TableCell>
                      <ReconcileStrip desired={rs.spec.replicas} active={rs.status.replicas} running={rs.status.runningReplicas} />
                    </TableCell>
                    <TableCell className="tabular-nums">
                      {rs.status.runningReplicas} / {rs.status.replicas} / {rs.spec.replicas}
                    </TableCell>
                    <TableCell className="text-muted-foreground">
                      <RelativeTime value={rs.createdAt} short />
                    </TableCell>
                    <TableCell className="text-right">
                      <div className="flex justify-end gap-1">
                        <Button variant="outline" size="sm" onClick={() => setScaling(rs)}>
                          Scale
                        </Button>
                        <Button variant="ghost" size="sm" className="text-destructive hover:bg-destructive/10" onClick={() => setDeleting(rs)}>
                          Delete
                        </Button>
                      </div>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        )}
      </div>
      <ScaleDialog rs={scaling} open={!!scaling} onOpenChange={(o) => !o && setScaling(null)} />
      <ConfirmDialog
        open={!!deleting}
        onOpenChange={(o) => !o && setDeleting(null)}
        title={`Delete ReplicaSet ${deleting?.name ?? ""}?`}
        consequences={
          <p>
            All {deleting?.status.replicas ?? 0} active pods it owns will be terminated by the controller on its next pass, and their containers
            removed. Pods you created directly are not affected.
          </p>
        }
        confirmLabel="Delete ReplicaSet and its pods"
        pending={del.isPending}
        onConfirm={() => deleting && del.mutate(deleting.name)}
      />
    </>
  );
}
