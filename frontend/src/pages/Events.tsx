import { useMemo, useState } from "react";
import { Link } from "react-router-dom";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { PageHeader } from "@/components/console/PageHeader";
import { EmptyState, ErrorPanel, StaleDataNotice, TableSkeleton } from "@/components/console/States";
import { Badge } from "@/components/console/StatusBadge";
import { RelativeTime } from "@/components/console/Time";
import { useEvents, useInfo } from "@/hooks/queries";

const objectLink = (kind: string, name: string) =>
  kind === "Pod" ? `/pods/${encodeURIComponent(name)}` : kind === "Node" ? `/nodes/${encodeURIComponent(name)}` : `/pods?owner=${encodeURIComponent(name)}`;

export default function Events() {
  const { data, error, isPending, refetch, dataUpdatedAt } = useEvents();
  const info = useInfo();
  const [type, setType] = useState("all");
  const [kind, setKind] = useState("all");
  const rows = useMemo(
    () => (data ?? []).filter((e) => (type === "all" || e.type === type) && (kind === "all" || e.object.kind === kind)),
    [data, type, kind],
  );

  return (
    <>
      <PageHeader
        title="Events"
        description={`Newest first. The control plane keeps the last ${info.data?.eventsRetained ?? "few hundred"} events in memory; they are not persisted and are cleared on restart.`}
      />
      <div className="panel">
        <div className="flex flex-wrap gap-2 border-b p-3">
          <Select value={type} onValueChange={setType}>
            <SelectTrigger className="h-9 w-36" aria-label="Filter by type">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">All types</SelectItem>
              <SelectItem value="Normal">Normal</SelectItem>
              <SelectItem value="Warning">Warning</SelectItem>
            </SelectContent>
          </Select>
          <Select value={kind} onValueChange={setKind}>
            <SelectTrigger className="h-9 w-40" aria-label="Filter by object kind">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">All objects</SelectItem>
              <SelectItem value="Pod">Pods</SelectItem>
              <SelectItem value="Node">Nodes</SelectItem>
              <SelectItem value="ReplicaSet">ReplicaSets</SelectItem>
            </SelectContent>
          </Select>
        </div>
        {error && data && <StaleDataNotice error={error} updatedAt={dataUpdatedAt} />}
        {isPending ? (
          <TableSkeleton columns={5} />
        ) : error && !data ? (
          <ErrorPanel error={error} what="events" onRetry={() => refetch()} />
        ) : rows.length === 0 ? (
          <EmptyState title="No events">{data && data.length > 0 ? "No events match these filters." : "Nothing has happened since the control plane started."}</EmptyState>
        ) : (
          <div className="overflow-x-auto">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Time</TableHead>
                  <TableHead>Type</TableHead>
                  <TableHead>Object</TableHead>
                  <TableHead>Reason</TableHead>
                  <TableHead>Message</TableHead>
                  <TableHead>Source</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {rows.map((e) => (
                  <TableRow key={e.id}>
                    <TableCell className="text-muted-foreground">
                      <RelativeTime value={e.time} />
                    </TableCell>
                    <TableCell>{e.type === "Warning" ? <Badge tone="warning">Warning</Badge> : <Badge tone="neutral">Normal</Badge>}</TableCell>
                    <TableCell>
                      <Link className="ident text-primary hover:underline" to={objectLink(e.object.kind, e.object.name)}>
                        {e.object.kind}/{e.object.name}
                      </Link>
                    </TableCell>
                    <TableCell className="font-medium">{e.reason}</TableCell>
                    <TableCell className="min-w-[16rem] text-muted-foreground">{e.message}</TableCell>
                    <TableCell className="text-xs text-muted-foreground">{e.source}</TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        )}
      </div>
    </>
  );
}
