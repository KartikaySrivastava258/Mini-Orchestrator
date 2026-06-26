import { useMemo, useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import {
  Boxes,
  Loader2,
  RefreshCw,
  Search,
  ScrollText,
  Trash2,
  Server as ServerIcon,
} from "lucide-react";
import { StatusBadge } from "./StatusBadge";
import { LogsDialog } from "./LogsDialog";
import { usePods } from "@/hooks/usePods";
import { deletePod, type Pod } from "@/lib/orchestrator-api";
import { toast } from "sonner";

const statusOptions = [
  "All",
  "Running",
  "Pending",
  "Succeeded",
  "Failed",
  "CrashLoopBackOff",
];

export function PodsTable({ headerActions }: { headerActions?: React.ReactNode }) {
  const { data: pods = [], isLoading, isFetching, refetch } = usePods();
  const [query, setQuery] = useState("");
  const [statusFilter, setStatusFilter] = useState("All");
  const [logsFor, setLogsFor] = useState<string | null>(null);
  const [confirmDelete, setConfirmDelete] = useState<string | null>(null);
  const qc = useQueryClient();

  const delMutation = useMutation({
    mutationFn: (name: string) => deletePod(name),
    onSuccess: (_, name) => {
      toast.success(`Pod "${name}" deleted`);
      qc.invalidateQueries({ queryKey: ["pods"] });
    },
    onError: (err: any, name) => {
      toast.error(`Failed to delete "${name}"`, {
        description: err?.response?.data ?? err?.message,
      });
    },
    onSettled: () => setConfirmDelete(null),
  });

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase();
    return pods
      .filter((p) =>
        statusFilter === "All" ? true : p.status === statusFilter,
      )
      .filter((p) =>
        !q
          ? true
          : p.name.toLowerCase().includes(q) ||
            p.image.toLowerCase().includes(q) ||
            (p.node ?? "").toLowerCase().includes(q),
      )
      .sort((a, b) => a.name.localeCompare(b.name));
  }, [pods, query, statusFilter]);

  return (
    <div className="glass rounded-2xl overflow-hidden">
      <div className="flex flex-wrap items-center gap-3 border-b border-border/60 p-4">
        <div className="flex items-center gap-2">
          <div className="grid h-8 w-8 place-items-center rounded-lg bg-secondary text-primary">
            <Boxes className="h-4 w-4" />
          </div>
          <div>
            <h3 className="text-sm font-semibold">Workloads</h3>
            <p className="text-[11px] text-muted-foreground font-mono">
              {filtered.length} of {pods.length} pods
            </p>
          </div>
        </div>

        <div className="flex flex-1 flex-wrap items-center justify-end gap-2">
          <div className="relative">
            <Search className="absolute left-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground" />
            <Input
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder="Search name, image, node…"
              className="pl-8 h-9 w-[240px] bg-secondary/60 border-border/80 font-mono text-xs"
            />
          </div>
          <Select value={statusFilter} onValueChange={setStatusFilter}>
            <SelectTrigger className="h-9 w-[170px] bg-secondary/60 border-border/80 text-xs">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {statusOptions.map((s) => (
                <SelectItem key={s} value={s} className="text-xs">
                  {s}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <Button
            variant="outline"
            size="sm"
            className="h-9 border-border/80 bg-secondary/60"
            onClick={() => refetch()}
          >
            <RefreshCw
              className={`h-3.5 w-3.5 ${isFetching ? "animate-spin" : ""}`}
            />
          </Button>
          {headerActions}
        </div>
      </div>

      <div className="overflow-x-auto scrollbar-thin">
        <Table>
          <TableHeader>
            <TableRow className="border-border/60 hover:bg-transparent">
              <TableHead className="text-[11px] uppercase tracking-wider">
                Name
              </TableHead>
              <TableHead className="text-[11px] uppercase tracking-wider">
                Image
              </TableHead>
              <TableHead className="text-[11px] uppercase tracking-wider">
                Node
              </TableHead>
              <TableHead className="text-[11px] uppercase tracking-wider">
                Status
              </TableHead>
              <TableHead className="text-right text-[11px] uppercase tracking-wider">
                Actions
              </TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {isLoading ? (
              <TableRow>
                <TableCell colSpan={5} className="py-12 text-center">
                  <Loader2 className="mx-auto h-5 w-5 animate-spin text-muted-foreground" />
                </TableCell>
              </TableRow>
            ) : filtered.length === 0 ? (
              <TableRow>
                <TableCell colSpan={5} className="py-16 text-center">
                  <Boxes className="mx-auto h-8 w-8 text-muted-foreground/50" />
                  <p className="mt-2 text-sm text-muted-foreground">
                    No pods match your filters.
                  </p>
                </TableCell>
              </TableRow>
            ) : (
              filtered.map((pod: Pod) => (
                <TableRow
                  key={pod.name}
                  className="border-border/40 hover:bg-secondary/40 transition-colors"
                >
                  <TableCell className="font-mono text-sm font-medium">
                    {pod.name}
                  </TableCell>
                  <TableCell className="font-mono text-xs text-muted-foreground">
                    {pod.image}
                  </TableCell>
                  <TableCell>
                    {pod.node ? (
                      <span className="inline-flex items-center gap-1.5 font-mono text-xs">
                        <ServerIcon className="h-3.5 w-3.5 text-info" />
                        {pod.node}
                      </span>
                    ) : (
                      <span className="text-xs text-muted-foreground italic">
                        unscheduled
                      </span>
                    )}
                  </TableCell>
                  <TableCell>
                    <StatusBadge status={pod.status} />
                  </TableCell>
                  <TableCell className="text-right">
                    <div className="flex justify-end gap-1">
                      <Button
                        size="sm"
                        variant="ghost"
                        className="h-8 px-2 text-info hover:text-info hover:bg-info/10"
                        onClick={() => setLogsFor(pod.name)}
                      >
                        <ScrollText className="h-3.5 w-3.5 mr-1" /> Logs
                      </Button>
                      <Button
                        size="sm"
                        variant="ghost"
                        className="h-8 px-2 text-destructive hover:text-destructive hover:bg-destructive/10"
                        onClick={() => setConfirmDelete(pod.name)}
                      >
                        <Trash2 className="h-3.5 w-3.5" />
                      </Button>
                    </div>
                  </TableCell>
                </TableRow>
              ))
            )}
          </TableBody>
        </Table>
      </div>

      <LogsDialog podName={logsFor} onOpenChange={(o) => !o && setLogsFor(null)} />

      <AlertDialog
        open={!!confirmDelete}
        onOpenChange={(o) => !o && setConfirmDelete(null)}
      >
        <AlertDialogContent className="glass border-glass-border">
          <AlertDialogHeader>
            <AlertDialogTitle>Delete pod?</AlertDialogTitle>
            <AlertDialogDescription>
              This will permanently terminate{" "}
              <span className="font-mono text-foreground">{confirmDelete}</span>{" "}
              and remove it from etcd.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={delMutation.isPending}>
              Cancel
            </AlertDialogCancel>
            <AlertDialogAction
              className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
              onClick={() => confirmDelete && delMutation.mutate(confirmDelete)}
            >
              {delMutation.isPending ? (
                <Loader2 className="h-4 w-4 animate-spin" />
              ) : (
                "Delete"
              )}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}
