import { CheckCircle2, Clock, XCircle, AlertTriangle, Loader2 } from "lucide-react";
import { cn } from "@/lib/utils";
import type { PodStatus } from "@/lib/orchestrator-api";

const map: Record<
  string,
  { label: string; color: string; bg: string; ring: string; Icon: typeof Clock }
> = {
  Running: {
    label: "Running",
    color: "text-success",
    bg: "bg-success/10",
    ring: "ring-success/30",
    Icon: CheckCircle2,
  },
  Pending: {
    label: "Pending",
    color: "text-warning",
    bg: "bg-warning/10",
    ring: "ring-warning/30",
    Icon: Loader2,
  },
  Succeeded: {
    label: "Succeeded",
    color: "text-info",
    bg: "bg-info/10",
    ring: "ring-info/30",
    Icon: CheckCircle2,
  },
  Failed: {
    label: "Failed",
    color: "text-destructive",
    bg: "bg-destructive/10",
    ring: "ring-destructive/30",
    Icon: XCircle,
  },
  CrashLoopBackOff: {
    label: "CrashLoopBackOff",
    color: "text-destructive",
    bg: "bg-destructive/10",
    ring: "ring-destructive/30",
    Icon: AlertTriangle,
  },
};

export function StatusBadge({ status }: { status: PodStatus }) {
  const cfg = map[status] ?? {
    label: status || "Unknown",
    color: "text-muted-foreground",
    bg: "bg-muted/40",
    ring: "ring-border",
    Icon: Clock,
  };
  const { Icon } = cfg;
  const spinning = status === "Pending";
  return (
    <span
      className={cn(
        "inline-flex items-center gap-1.5 rounded-full px-2.5 py-1 text-xs font-medium ring-1",
        cfg.bg,
        cfg.color,
        cfg.ring,
      )}
    >
      <Icon className={cn("h-3.5 w-3.5", spinning && "animate-spin")} />
      {cfg.label}
    </span>
  );
}
