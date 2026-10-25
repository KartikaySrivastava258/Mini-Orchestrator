import { cn } from "@/lib/utils";
import { POD_PHASES, type PodPhase } from "@/lib/api/types";
import type { PhaseCounts } from "@/lib/format";
import { phaseTone } from "./StatusBadge";

const fill: Record<string, string> = {
  success: "bg-success",
  warning: "bg-warning",
  danger: "bg-destructive",
  info: "bg-info",
  neutral: "bg-muted-foreground/50",
};

/** One bar, split by pod phase. Real counts from GET /api/v1/pods, no made-up charts. */
export function PhaseBar({ counts }: { counts: PhaseCounts }) {
  const total = POD_PHASES.reduce((n, p) => n + counts[p], 0);
  const present = POD_PHASES.filter((p) => counts[p] > 0);
  if (total === 0) return <div className="h-2 rounded-full bg-muted" aria-label="No pods" />;
  return (
    <div>
      <div className="flex h-2 overflow-hidden rounded-full bg-muted" role="img" aria-label={present.map((p) => `${counts[p]} ${p}`).join(", ")}>
        {present.map((p) => (
          <div key={p} className={fill[phaseTone[p]]} style={{ width: `${(counts[p] / total) * 100}%` }} />
        ))}
      </div>
      <ul className="mt-2 flex flex-wrap gap-x-4 gap-y-1 text-xs text-muted-foreground">
        {present.map((p: PodPhase) => (
          <li key={p} className="flex items-center gap-1.5">
            <span aria-hidden className={cn("h-2 w-2 rounded-sm", fill[phaseTone[p]])} />
            {p} <span className="tabular-nums text-foreground">{counts[p]}</span>
          </li>
        ))}
      </ul>
    </div>
  );
}

/**
 * Desired vs observed, as little blocks: filled = running, outlined = alive but
 * not running yet, dashed = missing. Extra pods during a scale-down show up
 * after a small gap. Basically reconciliation you can see.
 */
export function ReconcileStrip({ desired, active, running }: { desired: number; active: number; running: number }) {
  const cells = Math.max(desired, active);
  const label = `${running} running, ${active} active, ${desired} desired`;
  if (cells === 0) return <span className="text-xs text-muted-foreground">Scaled to zero</span>;
  return (
    <div className="flex flex-wrap items-center gap-0.5" role="img" aria-label={label} title={label}>
      {Array.from({ length: Math.min(cells, 50) }, (_, i) => {
        const kind = i < running ? "running" : i < active ? "active" : "missing";
        return (
          <span
            key={i}
            className={cn(
              "h-3 w-2 rounded-[2px]",
              kind === "running" && "bg-primary",
              kind === "active" && "border border-primary bg-primary/15",
              kind === "missing" && "border border-dashed border-muted-foreground/60",
              i === desired && desired < active && "ml-1.5",
            )}
          />
        );
      })}
    </div>
  );
}
