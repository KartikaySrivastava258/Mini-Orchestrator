import { cn } from "@/lib/utils";
import type { LucideIcon } from "lucide-react";

interface Props {
  label: string;
  value: string | number;
  hint?: string;
  Icon: LucideIcon;
  tone?: "primary" | "success" | "warning" | "destructive" | "info" | "accent";
}

const tones: Record<NonNullable<Props["tone"]>, string> = {
  primary: "from-primary/30 to-primary/0 text-primary",
  success: "from-success/30 to-success/0 text-success",
  warning: "from-warning/30 to-warning/0 text-warning",
  destructive: "from-destructive/30 to-destructive/0 text-destructive",
  info: "from-info/30 to-info/0 text-info",
  accent: "from-accent/30 to-accent/0 text-accent",
};

export function StatCard({ label, value, hint, Icon, tone = "primary" }: Props) {
  return (
    <div className="glass relative overflow-hidden rounded-2xl p-5 animate-slide-up">
      <div
        className={cn(
          "pointer-events-none absolute -top-10 -right-10 h-32 w-32 rounded-full bg-gradient-to-br blur-2xl opacity-60",
          tones[tone],
        )}
      />
      <div className="flex items-start justify-between">
        <div>
          <p className="text-xs uppercase tracking-wider text-muted-foreground">
            {label}
          </p>
          <p className="mt-2 font-mono text-3xl font-semibold text-foreground">
            {value}
          </p>
          {hint && (
            <p className="mt-1 text-xs text-muted-foreground">{hint}</p>
          )}
        </div>
        <div
          className={cn(
            "rounded-xl p-2.5 ring-1 ring-border/60 bg-secondary/60",
            tones[tone].split(" ").pop(),
          )}
        >
          <Icon className="h-5 w-5" />
        </div>
      </div>
    </div>
  );
}
