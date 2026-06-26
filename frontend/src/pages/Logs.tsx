import { useState } from "react";
import { ScrollText, Search } from "lucide-react";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { StatusBadge } from "@/components/orchestrator/StatusBadge";
import { LogsDialog } from "@/components/orchestrator/LogsDialog";
import { usePods } from "@/hooks/usePods";

export default function Logs() {
  const { data: pods = [] } = usePods();
  const [q, setQ] = useState("");
  const [logsFor, setLogsFor] = useState<string | null>(null);

  const filtered = pods.filter((p) =>
    !q ? true : p.name.toLowerCase().includes(q.toLowerCase()),
  );

  return (
    <div className="space-y-6 animate-fade-in">
      <div>
        <p className="text-xs uppercase tracking-[0.2em] text-primary font-mono">
          observability
        </p>
        <h1 className="mt-1 text-3xl font-bold tracking-tight">Logs</h1>
        <p className="mt-1 text-sm text-muted-foreground">
          Tap any pod to stream container output from the node agent.
        </p>
      </div>

      <div className="glass rounded-2xl p-4">
        <div className="relative max-w-sm">
          <Search className="absolute left-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground" />
          <Input
            placeholder="Filter pods…"
            value={q}
            onChange={(e) => setQ(e.target.value)}
            className="pl-8 bg-secondary/60 border-border/80 font-mono text-xs"
          />
        </div>

        <div className="mt-4 grid gap-2 sm:grid-cols-2 lg:grid-cols-3">
          {filtered.length === 0 ? (
            <p className="text-sm text-muted-foreground col-span-full text-center py-8">
              No pods to show.
            </p>
          ) : (
            filtered.map((p) => (
              <button
                key={p.name}
                onClick={() => setLogsFor(p.name)}
                className="group flex items-center justify-between rounded-xl border border-border/60 bg-secondary/40 p-3 text-left transition-all hover:border-primary/50 hover:bg-secondary/70 hover:shadow-glow"
              >
                <div className="min-w-0">
                  <p className="font-mono text-sm font-medium truncate">
                    {p.name}
                  </p>
                  <p className="font-mono text-[11px] text-muted-foreground truncate">
                    {p.image}
                  </p>
                </div>
                <div className="flex items-center gap-2 shrink-0">
                  <StatusBadge status={p.status} />
                  <ScrollText className="h-4 w-4 text-muted-foreground group-hover:text-primary transition-colors" />
                </div>
              </button>
            ))
          )}
        </div>
      </div>

      <LogsDialog podName={logsFor} onOpenChange={(o) => !o && setLogsFor(null)} />
    </div>
  );
}
