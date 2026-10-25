import { useMemo } from "react";
import { useSearchParams } from "react-router-dom";
import { RefreshCw, Search } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { CreatePodDialog } from "@/components/console/CreatePodDialog";
import { PageHeader } from "@/components/console/PageHeader";
import { PodTable } from "@/components/console/PodTable";
import { ErrorPanel, StaleDataNotice, TableSkeleton } from "@/components/console/States";
import { usePods } from "@/hooks/queries";
import { POD_PHASES } from "@/lib/api/types";

const ALL = "all";

export default function Pods() {
  const { data, error, isPending, isFetching, refetch, dataUpdatedAt } = usePods();
  const [params, setParams] = useSearchParams();
  const q = params.get("q") ?? "";
  const phase = params.get("phase") ?? ALL;
  const node = params.get("node") ?? ALL;
  const owner = params.get("owner") ?? "";

  const setParam = (key: string, value: string) => {
    const next = new URLSearchParams(params);
    if (!value || value === ALL) next.delete(key);
    else next.set(key, value);
    setParams(next, { replace: true });
  };

  const nodes = useMemo(() => [...new Set((data ?? []).map((p) => p.status.nodeName).filter((n): n is string => !!n))].sort(), [data]);
  const filtered = useMemo(() => {
    const needle = q.trim().toLowerCase();
    return (data ?? []).filter(
      (p) =>
        (!needle || p.name.includes(needle)) &&
        (phase === ALL || p.status.phase === phase) &&
        (node === ALL || p.status.nodeName === node) &&
        (!owner || p.owner?.name === owner),
    );
  }, [data, q, phase, node, owner]);

  const filtering = !!q || phase !== ALL || node !== ALL || !!owner;

  return (
    <>
      <PageHeader
        title="Pods"
        description="Single-container workloads. Pods created by a ReplicaSet are named after it."
        actions={<CreatePodDialog />}
      />
      <div className="panel">
        <div className="flex flex-wrap items-center gap-2 border-b p-3">
          <div className="relative">
            <Search aria-hidden className="absolute left-2.5 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
            <Input
              aria-label="Search pods by name"
              placeholder="Search by name"
              value={q}
              onChange={(e) => setParam("q", e.target.value)}
              className="h-9 w-56 pl-8"
            />
          </div>
          <Select value={phase} onValueChange={(v) => setParam("phase", v)}>
            <SelectTrigger className="h-9 w-40" aria-label="Filter by phase">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value={ALL}>All phases</SelectItem>
              {POD_PHASES.map((p) => (
                <SelectItem key={p} value={p}>
                  {p}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <Select value={node} onValueChange={(v) => setParam("node", v)}>
            <SelectTrigger className="h-9 w-40" aria-label="Filter by node">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value={ALL}>All nodes</SelectItem>
              {nodes.map((n) => (
                <SelectItem key={n} value={n}>
                  {n}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          {owner && (
            <Button variant="outline" size="sm" onClick={() => setParam("owner", "")}>
              ReplicaSet {owner} ✕
            </Button>
          )}
          <div className="ml-auto flex items-center gap-2 text-xs text-muted-foreground">
            {data && (
              <span>
                {filtered.length} of {data.length}
              </span>
            )}
            <Button variant="outline" size="sm" onClick={() => refetch()} disabled={isFetching}>
              <RefreshCw aria-hidden className={isFetching ? "mr-1.5 h-3.5 w-3.5 animate-spin" : "mr-1.5 h-3.5 w-3.5"} /> Refresh
            </Button>
          </div>
        </div>
        {error && data && <StaleDataNotice error={error} updatedAt={dataUpdatedAt} />}
        {isPending ? (
          <TableSkeleton columns={7} />
        ) : error && !data ? (
          <ErrorPanel error={error} what="pods" onRetry={() => refetch()} />
        ) : (
          <PodTable
            pods={filtered}
            emptyText={
              filtering ? (
                <>
                  No pods match these filters.{" "}
                  <button type="button" className="text-primary underline" onClick={() => setParams(new URLSearchParams(), { replace: true })}>
                    Clear filters
                  </button>
                </>
              ) : (
                "No pods yet. Create one, or create a ReplicaSet to run several."
              )
            }
          />
        )}
      </div>
    </>
  );
}
