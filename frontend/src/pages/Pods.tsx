import { PodsTable } from "@/components/orchestrator/PodsTable";
import { CreatePodDialog } from "@/components/orchestrator/CreatePodDialog";

export default function Pods() {
  return (
    <div className="space-y-6 animate-fade-in">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <p className="text-xs uppercase tracking-[0.2em] text-primary font-mono">
            workloads
          </p>
          <h1 className="mt-1 text-3xl font-bold tracking-tight">Pods</h1>
          <p className="mt-1 text-sm text-muted-foreground">
            Deploy, inspect, and terminate workloads scheduled by the controller.
          </p>
        </div>
        <CreatePodDialog />
      </div>

      <PodsTable />
    </div>
  );
}
