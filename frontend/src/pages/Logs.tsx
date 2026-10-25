import { useSearchParams } from "react-router-dom";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Label } from "@/components/ui/label";
import { LogViewer } from "@/components/console/LogViewer";
import { PageHeader } from "@/components/console/PageHeader";
import { EmptyState, ErrorPanel, TableSkeleton } from "@/components/console/States";
import { usePods } from "@/hooks/queries";

export default function Logs() {
  const { data, error, isPending, refetch } = usePods();
  const [params, setParams] = useSearchParams();
  const selected = params.get("pod") ?? "";
  const pod = data?.find((p) => p.name === selected);
  const withContainers = (data ?? []).filter((p) => p.status.containerID);

  return (
    <>
      <PageHeader
        title="Logs"
        description="Container output fetched on demand from the node agent through the control plane. Logs are snapshots, not a live stream, and are capped in size."
      />
      <div className="panel">
        <div className="flex flex-wrap items-center gap-2 border-b p-3">
          <Label htmlFor="log-pod">Pod</Label>
          <Select value={selected || undefined} onValueChange={(v) => setParams({ pod: v }, { replace: true })}>
            <SelectTrigger id="log-pod" className="h-9 w-72">
              <SelectValue placeholder={withContainers.length ? "Choose a pod with a container" : "No pods with containers"} />
            </SelectTrigger>
            <SelectContent>
              {withContainers.map((p) => (
                <SelectItem key={p.uid} value={p.name}>
                  {p.name} ({p.status.phase})
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        {isPending ? (
          <TableSkeleton rows={3} columns={1} />
        ) : error && !data ? (
          <ErrorPanel error={error} what="pods" onRetry={() => refetch()} />
        ) : !selected ? (
          <EmptyState title="Choose a pod">Only pods whose container has been started have logs.</EmptyState>
        ) : !pod ? (
          <EmptyState title={`Pod ${selected} does not exist`}>It may have been deleted.</EmptyState>
        ) : (
          <LogViewer key={pod.uid} pod={pod} />
        )}
      </div>
    </>
  );
}
