import { useMemo, useState } from "react";
import { Link } from "react-router-dom";
import { ArrowDown, ArrowUp, ScrollText, Trash2 } from "lucide-react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Button } from "@/components/ui/button";
import { deletePod, errorMessage } from "@/lib/api/client";
import type { Pod } from "@/lib/api/types";
import { POD_PHASES } from "@/lib/api/types";
import { parseTime } from "@/lib/format";
import { queryKeys } from "@/hooks/queries";
import { ConfirmDialog } from "./ConfirmDialog";
import { PhaseBadge } from "./StatusBadge";
import { RelativeTime } from "./Time";

type SortKey = "name" | "phase" | "node" | "age" | "restarts";

function compare(a: Pod, b: Pod, key: SortKey): number {
  switch (key) {
    case "phase":
      return POD_PHASES.indexOf(a.status.phase) - POD_PHASES.indexOf(b.status.phase);
    case "node":
      return (a.status.nodeName ?? "").localeCompare(b.status.nodeName ?? "");
    case "age":
      // ascending = oldest first
      return (parseTime(a.createdAt)?.getTime() ?? 0) - (parseTime(b.createdAt)?.getTime() ?? 0);
    case "restarts":
      return a.status.restartCount - b.status.restartCount;
    default:
      return a.name.localeCompare(b.name);
  }
}

function SortHeader({ label, k, sort, onSort }: { label: string; k: SortKey; sort: { key: SortKey; asc: boolean }; onSort: (k: SortKey) => void }) {
  const active = sort.key === k;
  return (
    <TableHead aria-sort={active ? (sort.asc ? "ascending" : "descending") : "none"}>
      <button type="button" className="inline-flex items-center gap-1 font-medium hover:text-foreground" onClick={() => onSort(k)}>
        {label}
        {active && (sort.asc ? <ArrowUp aria-hidden className="h-3 w-3" /> : <ArrowDown aria-hidden className="h-3 w-3" />)}
      </button>
    </TableHead>
  );
}

/** Spells out what deleting this pod actually does, before anyone clicks. */
export function deleteConsequences(pod: Pod): React.ReactNode {
  return (
    <>
      <p>
        {pod.status.nodeName
          ? `The pod is marked Terminating. The agent on ${pod.status.nodeName} removes its container, then the record is deleted. Container logs are lost.`
          : "The pod has no node yet, so its record is deleted immediately."}
      </p>
      {pod.owner && (
        <p>
          It belongs to ReplicaSet <span className="ident text-foreground">{pod.owner.name}</span>, which will create a replacement. Scale the
          ReplicaSet instead to reduce the number of pods.
        </p>
      )}
    </>
  );
}

export function useDeletePod(onDone?: () => void) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (name: string) => deletePod(name),
    onSuccess: (res) => {
      qc.invalidateQueries({ queryKey: queryKeys.pods });
      qc.invalidateQueries({ queryKey: queryKeys.events });
      toast.success(res.result === "Deleted" ? `Deleted pod ${res.name}` : `Terminating pod ${res.name}`, {
        description: res.message,
      });
      onDone?.();
    },
    onError: (err, name) => toast.error(`Could not delete pod ${name}`, { description: errorMessage(err) }),
  });
}

export function PodTable({ pods, hideNode = false, emptyText }: { pods: Pod[]; hideNode?: boolean; emptyText?: React.ReactNode }) {
  const [sort, setSort] = useState<{ key: SortKey; asc: boolean }>({ key: "name", asc: true });
  const [pendingDelete, setPendingDelete] = useState<Pod | null>(null);
  const del = useDeletePod(() => setPendingDelete(null));

  const sorted = useMemo(() => {
    const out = [...pods].sort((a, b) => compare(a, b, sort.key) || a.name.localeCompare(b.name));
    return sort.asc ? out : out.reverse();
  }, [pods, sort]);

  const onSort = (key: SortKey) => setSort((s) => ({ key, asc: s.key === key ? !s.asc : true }));

  if (pods.length === 0) {
    return <div className="px-6 py-10 text-center text-sm text-muted-foreground">{emptyText ?? "No pods."}</div>;
  }

  return (
    <>
      <div className="overflow-x-auto">
        <Table>
          <TableHeader>
            <TableRow>
              <SortHeader label="Name" k="name" sort={sort} onSort={onSort} />
              <TableHead>Image</TableHead>
              <SortHeader label="Phase" k="phase" sort={sort} onSort={onSort} />
              <TableHead>Reason</TableHead>
              {!hideNode && <SortHeader label="Node" k="node" sort={sort} onSort={onSort} />}
              <SortHeader label="Restarts" k="restarts" sort={sort} onSort={onSort} />
              <SortHeader label="Age" k="age" sort={sort} onSort={onSort} />
              <TableHead className="text-right">
                <span className="sr-only">Actions</span>
              </TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {sorted.map((p) => (
              <TableRow key={p.uid || p.name}>
                <TableCell>
                  <Link to={`/pods/${encodeURIComponent(p.name)}`} className="ident font-medium text-primary hover:underline">
                    {p.name}
                  </Link>
                  {p.owner && <div className="text-xs text-muted-foreground">from {p.owner.name}</div>}
                </TableCell>
                <TableCell className="ident max-w-[16rem] truncate text-muted-foreground" title={p.spec.image}>
                  {p.spec.image}
                </TableCell>
                <TableCell>
                  <PhaseBadge phase={p.status.phase} reason={p.status.reason} />
                </TableCell>
                <TableCell className="max-w-[14rem] truncate text-xs text-muted-foreground" title={p.status.message}>
                  {p.status.reason ?? "—"}
                </TableCell>
                {!hideNode && (
                  <TableCell>
                    {p.status.nodeName ? (
                      <Link to={`/nodes/${encodeURIComponent(p.status.nodeName)}`} className="ident hover:underline">
                        {p.status.nodeName}
                      </Link>
                    ) : (
                      <span className="text-xs text-muted-foreground">Unassigned</span>
                    )}
                  </TableCell>
                )}
                <TableCell className="tabular-nums">{p.status.restartCount}</TableCell>
                <TableCell className="text-muted-foreground">
                  <RelativeTime value={p.createdAt} short />
                </TableCell>
                <TableCell className="text-right">
                  <div className="flex justify-end gap-1">
                    <Button asChild variant="ghost" size="sm" className="h-8 px-2">
                      <Link to={`/logs?pod=${encodeURIComponent(p.name)}`} aria-label={`Logs for ${p.name}`}>
                        <ScrollText aria-hidden className="h-4 w-4" />
                      </Link>
                    </Button>
                    <Button
                      variant="ghost"
                      size="sm"
                      className="h-8 px-2 text-destructive hover:bg-destructive/10 hover:text-destructive"
                      aria-label={`Delete ${p.name}`}
                      disabled={p.status.phase === "Terminating"}
                      onClick={() => setPendingDelete(p)}
                    >
                      <Trash2 aria-hidden className="h-4 w-4" />
                    </Button>
                  </div>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </div>
      <ConfirmDialog
        open={!!pendingDelete}
        onOpenChange={(o) => !o && setPendingDelete(null)}
        title={`Delete pod ${pendingDelete?.name ?? ""}?`}
        consequences={pendingDelete ? deleteConsequences(pendingDelete) : null}
        confirmLabel="Delete pod"
        pending={del.isPending}
        onConfirm={() => pendingDelete && del.mutate(pendingDelete.name)}
      />
    </>
  );
}
