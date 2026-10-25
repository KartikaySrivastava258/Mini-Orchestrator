import { Link, useNavigate, useParams } from "react-router-dom";
import { useState } from "react";
import { Trash2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { ConfirmDialog } from "@/components/console/ConfirmDialog";
import { LogViewer } from "@/components/console/LogViewer";
import { DetailList, PageHeader, Section } from "@/components/console/PageHeader";
import { deleteConsequences, useDeletePod } from "@/components/console/PodTable";
import { EmptyState, ErrorPanel, TableSkeleton } from "@/components/console/States";
import { PhaseBadge } from "@/components/console/StatusBadge";
import { RelativeTime } from "@/components/console/Time";
import { useEvents, usePod } from "@/hooks/queries";
import { ApiError } from "@/lib/api/client";
import { exactTime } from "@/lib/format";

export default function PodDetail() {
  const { name = "" } = useParams();
  const navigate = useNavigate();
  const { data: pod, error, isPending, refetch } = usePod(name);
  const events = useEvents();
  const [confirm, setConfirm] = useState(false);
  const del = useDeletePod(() => {
    setConfirm(false);
    navigate("/pods");
  });

  if (isPending) return <TableSkeleton rows={6} columns={2} />;
  if (error instanceof ApiError && error.status === 404) {
    return (
      <EmptyState title={`Pod ${name} does not exist`} action={<Link className="text-primary underline" to="/pods">Back to pods</Link>}>
        It may have been deleted, or replaced by its ReplicaSet under a new name.
      </EmptyState>
    );
  }
  if (error || !pod) return <ErrorPanel error={error} what={`pod ${name}`} onRetry={() => refetch()} />;

  const podEvents = (events.data ?? []).filter((e) => e.object.kind === "Pod" && e.object.name === pod.name);
  const s = pod.status;

  return (
    <>
      <PageHeader
        title={pod.name}
        description={<PhaseBadge phase={s.phase} reason={s.reason} />}
        actions={
          <Button variant="outline" size="sm" className="text-destructive" disabled={s.phase === "Terminating"} onClick={() => setConfirm(true)}>
            <Trash2 aria-hidden className="mr-1.5 h-4 w-4" /> Delete pod
          </Button>
        }
      />
      {s.message && (
        <p className="mb-4 rounded border bg-card px-4 py-2.5 text-sm">
          <span className="font-medium">{s.reason}</span>
          <span className="text-muted-foreground"> {s.message}</span>
        </p>
      )}
      <div className="grid gap-4 lg:grid-cols-2">
        <Section title="Spec (desired)">
          <div className="p-4">
            <DetailList
              items={[
                { label: "Image", value: <span className="ident">{pod.spec.image}</span> },
                { label: "Restart policy", value: pod.spec.restartPolicy },
                {
                  label: "Owner",
                  value: pod.owner ? (
                    <Link className="text-primary hover:underline" to={`/pods?owner=${encodeURIComponent(pod.owner.name)}`}>
                      ReplicaSet {pod.owner.name}
                    </Link>
                  ) : (
                    "None (created directly)"
                  ),
                },
                { label: "UID", value: <span className="ident break-all">{pod.uid}</span> },
                { label: "Created", value: exactTime(pod.createdAt) },
              ]}
            />
          </div>
        </Section>
        <Section title="Status (observed)">
          <div className="p-4">
            <DetailList
              items={[
                {
                  label: "Node",
                  value: s.nodeName ? (
                    <Link className="ident text-primary hover:underline" to={`/nodes/${encodeURIComponent(s.nodeName)}`}>
                      {s.nodeName}
                    </Link>
                  ) : (
                    "Not scheduled"
                  ),
                },
                { label: "Container ID", value: s.containerID ? <span className="ident break-all">{s.containerID}</span> : "None yet" },
                { label: "Restarts", value: s.restartCount },
                { label: "Last exit code", value: s.exitCode ?? "—" },
                { label: "Scheduled", value: exactTime(s.scheduledAt) },
                { label: "Started", value: exactTime(s.startedAt) },
                { label: "Last phase change", value: exactTime(s.lastTransitionTime) },
                ...(pod.deletionRequestedAt ? [{ label: "Deletion requested", value: exactTime(pod.deletionRequestedAt) }] : []),
              ]}
            />
          </div>
        </Section>
        <div className="lg:col-span-2">
          <Section title="Events">
            {podEvents.length === 0 ? (
              <p className="px-4 py-6 text-sm text-muted-foreground">
                No events recorded for this pod. Events are kept in control-plane memory and are lost on restart.
              </p>
            ) : (
              <ul className="divide-y">
                {podEvents.map((e) => (
                  <li key={e.id} className="grid gap-1 px-4 py-2 text-sm sm:grid-cols-[7rem_10rem_1fr]">
                    <span className="text-muted-foreground"><RelativeTime value={e.time} /></span>
                    <span className={e.type === "Warning" ? "font-medium text-warning" : "font-medium"}>{e.reason}</span>
                    <span className="text-muted-foreground">{e.message}</span>
                  </li>
                ))}
              </ul>
            )}
          </Section>
        </div>
        <div className="lg:col-span-2">
          <Section title="Logs">
            <LogViewer pod={pod} />
          </Section>
        </div>
      </div>
      <ConfirmDialog
        open={confirm}
        onOpenChange={setConfirm}
        title={`Delete pod ${pod.name}?`}
        consequences={deleteConsequences(pod)}
        confirmLabel="Delete pod"
        pending={del.isPending}
        onConfirm={() => del.mutate(pod.name)}
      />
    </>
  );
}
