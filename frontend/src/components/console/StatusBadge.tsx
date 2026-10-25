import { cn } from "@/lib/utils";
import type { PodPhase } from "@/lib/api/types";
import type { NodeHealth } from "@/lib/format";

type Tone = "success" | "warning" | "danger" | "info" | "neutral";

const toneClasses: Record<Tone, { dot: string; text: string; bg: string }> = {
  success: { dot: "bg-success", text: "text-success", bg: "bg-success/10" },
  warning: { dot: "bg-warning", text: "text-warning", bg: "bg-warning/10" },
  danger: { dot: "bg-destructive", text: "text-destructive", bg: "bg-destructive/10" },
  info: { dot: "bg-info", text: "text-info", bg: "bg-info/10" },
  neutral: { dot: "bg-muted-foreground", text: "text-muted-foreground", bg: "bg-muted" },
};

export const phaseTone: Record<PodPhase, Tone> = {
  Running: "success",
  Succeeded: "success",
  Pending: "warning",
  Scheduled: "info",
  Failed: "danger",
  Unknown: "neutral",
  Terminating: "neutral",
};

export function Badge({ tone, children, title }: { tone: Tone; children: React.ReactNode; title?: string }) {
  const t = toneClasses[tone];
  return (
    <span
      title={title}
      className={cn("inline-flex items-center gap-1.5 whitespace-nowrap rounded px-2 py-0.5 text-xs font-medium", t.bg, t.text)}
    >
      <span aria-hidden className={cn("h-1.5 w-1.5 rounded-full", t.dot)} />
      {children}
    </span>
  );
}

/** Pod phase badge. Hover it for the reason, that's where the real story is. */
export function PhaseBadge({ phase, reason }: { phase: PodPhase; reason?: string }) {
  return (
    <Badge tone={phaseTone[phase] ?? "neutral"} title={reason ? `Reason: ${reason}` : undefined}>
      {phase}
    </Badge>
  );
}

const healthLabel: Record<NodeHealth, { label: string; tone: Tone }> = {
  Ready: { label: "Ready", tone: "success" },
  StaleHeartbeat: { label: "Stale heartbeat", tone: "warning" },
  RuntimeUnavailable: { label: "Runtime unavailable", tone: "danger" },
  NotReady: { label: "Not ready", tone: "danger" },
};

export function NodeHealthBadge({ health }: { health: NodeHealth }) {
  const h = healthLabel[health];
  return <Badge tone={h.tone}>{h.label}</Badge>;
}
