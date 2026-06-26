import { useMemo } from "react";
import { Server, Cpu, Heart, AlertTriangle } from "lucide-react";
import { usePods } from "@/hooks/usePods";
import { useNodes } from "@/hooks/useNodes";
import { RegisterNodeDialog } from "@/components/orchestrator/RegisterNodeDialog";
import { deleteNode } from "@/lib/orchestrator-api"; // ✅ ADDED
import { cn } from "@/lib/utils";

function formatHeartbeatAge(ts: number): { label: string; stale: boolean } {
  if (!ts) return { label: "never", stale: true };
  const seconds = Math.max(0, Math.floor(Date.now() / 1000) - ts);
  const stale = seconds > 30;
  if (seconds < 60) return { label: `${seconds}s ago`, stale };
  const m = Math.floor(seconds / 60);
  if (m < 60) return { label: `${m}m ago`, stale };
  const h = Math.floor(m / 60);
  return { label: `${h}h ago`, stale };
}

export default function Nodes() {
  const { data: pods = [] } = usePods();
  const { data: nodes = [], isError, error } = useNodes();

  const podCountsByNode = useMemo(() => {
    const map = new Map<string, { pods: number; running: number; failed: number }>();
    pods.forEach((p) => {
      if (!p.node) return;
      const cur = map.get(p.node) ?? { pods: 0, running: 0, failed: 0 };
      cur.pods += 1;
      if (p.status === "Running") cur.running += 1;
      if (p.status === "Failed" || p.status === "CrashLoopBackOff") cur.failed += 1;
      map.set(p.node, cur);
    });
    return map;
  }, [pods]);

  const backendUnreachable =
    isError && (error as any)?.response?.status !== 404;

  return (
    <div className="space-y-6 animate-fade-in">
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <p className="text-xs uppercase tracking-[0.2em] text-primary font-mono">
            fleet
          </p>
          <h1 className="mt-1 text-3xl font-bold tracking-tight">Nodes</h1>
          <p className="mt-1 text-sm text-muted-foreground">
            Live worker node inventory from{" "}
            <code className="font-mono">GET /getnodes</code> {/* ✅ FIXED */}
          </p>
        </div>
        <RegisterNodeDialog />
      </div>

      {backendUnreachable && (
        <div className="glass flex items-start gap-3 rounded-2xl border border-warning/40 p-4">
          <AlertTriangle className="h-5 w-5 text-warning shrink-0 mt-0.5" />
          <div className="text-sm">
            <p className="font-semibold">
              Could not reach <code className="font-mono">/getnodes</code>
            </p>
            <p className="text-muted-foreground">
              Make sure your Go backend exposes{" "}
              <code className="font-mono">GET /getnodes</code>.
            </p>
          </div>
        </div>
      )}

      {nodes.length === 0 ? (
        <div className="glass grid place-items-center rounded-2xl py-20 text-center">
          <Server className="h-10 w-10 text-muted-foreground/40" />
          <p className="mt-3 text-sm text-muted-foreground">
            No nodes registered. Use{" "}
            <span className="font-semibold">Register Node</span>.
          </p>
        </div>
      ) : (
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {nodes.map((n) => {
            const counts = podCountsByNode.get(n.name) ?? {
              pods: 0,
              running: 0,
              failed: 0,
            };
            const hb = formatHeartbeatAge(n.last_heartbeat);
            const healthy = n.active && !hb.stale;
            const cpu = Math.max(0, Math.min(100, Math.round(n.cpu_usage ?? 0)));

            return (
              <div
                key={n.name}
                className="glass rounded-2xl p-5 animate-slide-up"
              >
                {/* Header */}
                <div className="flex items-start justify-between">
                  <div className="flex items-center gap-2">
                    <div className="grid h-10 w-10 place-items-center rounded-xl bg-gradient-primary shadow-glow">
                      <Server className="h-5 w-5 text-primary-foreground" />
                    </div>
                    <div>
                      <p className="font-mono text-sm font-semibold">{n.name}</p>
                      <p className="text-[11px] text-muted-foreground">
                        worker node
                      </p>
                    </div>
                  </div>
                  <span
                    className={cn(
                      "status-dot",
                      healthy
                        ? "bg-success animate-pulse-glow"
                        : "bg-destructive",
                    )}
                  />
                </div>

                {/* Heartbeat */}
                <div className="mt-4 flex items-center justify-between text-[11px] text-muted-foreground">
                  <span className="flex items-center gap-1">
                    <Heart className={cn("h-3.5 w-3.5", healthy ? "text-success" : "text-destructive")} />
                    last heartbeat
                  </span>
                  <span className={cn("font-mono", hb.stale && "text-destructive")}>
                    {hb.label}
                  </span>
                </div>

                {/* Pod Stats */}
                <div className="mt-3 grid grid-cols-3 gap-2 text-center">
                  <div className="rounded-lg bg-secondary/60 py-2">
                    <p className="text-[10px] uppercase text-muted-foreground">
                      Pods
                    </p>
                    <p className="font-mono text-lg font-semibold">{counts.pods}</p>
                  </div>
                  <div className="rounded-lg bg-success/10 py-2">
                    <p className="text-[10px] uppercase text-success">Running</p>
                    <p className="font-mono text-lg font-semibold text-success">
                      {counts.running}
                    </p>
                  </div>
                  <div className="rounded-lg bg-destructive/10 py-2">
                    <p className="text-[10px] uppercase text-destructive">
                      Failed
                    </p>
                    <p className="font-mono text-lg font-semibold text-destructive">
                      {counts.failed}
                    </p>
                  </div>
                </div>

                {/* CPU */}
                <div className="mt-4">
                  <div className="flex items-center justify-between text-xs">
                    <span className="flex items-center gap-1 text-muted-foreground">
                      <Cpu className="h-3.5 w-3.5" /> CPU usage
                    </span>
                    <span className="font-mono">{cpu}%</span>
                  </div>
                  <div className="mt-1.5 h-1.5 overflow-hidden rounded-full bg-secondary">
                    <div
                      className={cn(
                        "h-full",
                        cpu > 85
                          ? "bg-destructive"
                          : cpu > 65
                          ? "bg-warning"
                          : "bg-gradient-primary",
                      )}
                      style={{ width: `${cpu}%` }}
                    />
                  </div>
                </div>

                {/* 🔥 DELETE BUTTON */}
                <div className="mt-4 flex justify-end">
                  <button
                    onClick={async () => {
                      if (confirm(`Delete node ${n.name}?`)) {
                        try {
                          await deleteNode(n.name);
                          alert("Node deleted successfully");
                          window.location.reload();
                        } catch (err) {
                          console.error(err);
                          alert("Failed to delete node");
                        }
                      }
                    }}
                    className="text-xs px-3 py-1.5 rounded-lg bg-destructive text-white hover:opacity-90 transition"
                  >
                    Delete Node
                  </button>
                </div>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}