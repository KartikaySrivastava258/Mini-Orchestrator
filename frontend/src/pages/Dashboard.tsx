import { useMemo } from "react";
import {
  Boxes,
  CheckCircle2,
  XCircle,
  Server,
  Activity,
  Clock,
} from "lucide-react";
import {
  PieChart,
  Pie,
  Cell,
  ResponsiveContainer,
  Tooltip,
  AreaChart,
  Area,
  XAxis,
  YAxis,
  CartesianGrid,
} from "recharts";
import { StatCard } from "@/components/orchestrator/StatCard";
import { PodsTable } from "@/components/orchestrator/PodsTable";
import { CreatePodDialog } from "@/components/orchestrator/CreatePodDialog";
import { usePods } from "@/hooks/usePods";
import { useNodes } from "@/hooks/useNodes";
import { useEffect, useRef, useState } from "react";

const STATUS_COLORS: Record<string, string> = {
  Running: "hsl(var(--success))",
  Pending: "hsl(var(--warning))",
  Succeeded: "hsl(var(--info))",
  Failed: "hsl(var(--destructive))",
  CrashLoopBackOff: "hsl(var(--destructive))",
  Unknown: "hsl(var(--muted-foreground))",
};

export default function Dashboard() {
  const { data: pods = [] } = usePods();
  const { data: nodes = [], isError: nodesError } = useNodes();

  const stats = useMemo(() => {
    const total = pods.length;
    const running = pods.filter((p) => p.status === "Running").length;
    const pending = pods.filter((p) => p.status === "Pending").length;
    const failed = pods.filter(
      (p) => p.status === "Failed" || p.status === "CrashLoopBackOff",
    ).length;
    const activeNodes = nodes.filter((n) => {
      const ageSec = Math.floor(Date.now() / 1000) - (n.last_heartbeat ?? 0);
      return n.active && ageSec <= 30;
    }).length;
    return { total, running, pending, failed, nodes: activeNodes };
  }, [pods, nodes]);

  const distribution = useMemo(() => {
    const counts: Record<string, number> = {};
    pods.forEach((p) => {
      const k = p.status || "Unknown";
      counts[k] = (counts[k] ?? 0) + 1;
    });
    
    return Object.entries(counts).map(([name, value]) => ({ name, value }));
  }, [pods]);

  // Real cluster CPU = average cpu_usage across nodes, accumulated into a rolling timeseries.
  const avgCpu = useMemo(() => {
    if (!nodes.length) return 0;
    const sum = nodes.reduce((acc, n) => acc + (Number(n.cpu_usage) || 0), 0);
    return Math.round(sum / nodes.length);
  }, [nodes]);

  const MAX_POINTS = 24;
  const [cpuSeries, setCpuSeries] = useState<{ t: string; cpu: number }[]>([]);
  const tickRef = useRef(0);

  useEffect(() => {
    if (!nodes.length) return;
    setCpuSeries((prev) => {
      const next = [...prev, { t: String(tickRef.current++), cpu: avgCpu }];
      return next.slice(-MAX_POINTS);
    });
  }, [avgCpu, nodes.length]);

  return (
    <div className="space-y-6 animate-fade-in">
      <div className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <p className="text-xs uppercase tracking-[0.2em] text-primary font-mono">
            Cluster overview
          </p>
          <h1 className="mt-1 text-3xl md:text-4xl font-bold tracking-tight">
            Mission <span className="gradient-text">Control</span>
          </h1>
          <p className="mt-1 text-sm text-muted-foreground">
            Real-time view of your Mini Orchestrator workloads, pulled live from
            etcd via the Go control plane.
          </p>
        </div>
        <CreatePodDialog />
      </div>

      <div className="grid grid-cols-2 lg:grid-cols-5 gap-4">
        <StatCard label="Total pods" value={stats.total} Icon={Boxes} tone="primary" />
        <StatCard
          label="Running"
          value={stats.running}
          Icon={CheckCircle2}
          tone="success"
        />
        <StatCard
          label="Pending"
          value={stats.pending}
          Icon={Clock}
          tone="warning"
        />
        <StatCard
          label="Failed"
          value={stats.failed}
          Icon={XCircle}
          tone="destructive"
        />
        <StatCard
          label="Active nodes"
          value={stats.nodes || "—"}
          Icon={Server}
          tone="info"
          hint={nodesError ? "GET /nodes unreachable" : "live heartbeats"}
        />
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-4">
        <div className="glass rounded-2xl p-5 lg:col-span-2">
          <div className="flex items-center justify-between mb-2">
            <div className="flex items-center gap-2">
              <Activity className="h-4 w-4 text-primary" />
              <h3 className="text-sm font-semibold">Cluster CPU pressure</h3>
            </div>
            <span className="text-[11px] font-mono text-muted-foreground">
              {nodes.length > 0
                ? `avg ${avgCpu}% · ${nodes.length} node${nodes.length === 1 ? "" : "s"}`
                : nodesError
                  ? "GET /nodes unreachable"
                  : "waiting for nodes…"}
            </span>
          </div>
          {cpuSeries.length === 0 ? (
            <div className="grid h-[240px] place-items-center text-xs text-muted-foreground">
              No node telemetry yet — register a node to start sampling CPU.
            </div>
          ) : (
          <div className="h-[240px]">
            <ResponsiveContainer width="100%" height="100%">
              <AreaChart data={cpuSeries} margin={{ left: -20, right: 8, top: 8 }}>
                <defs>
                  <linearGradient id="cpuFill" x1="0" y1="0" x2="0" y2="1">
                    <stop offset="0%" stopColor="hsl(var(--primary))" stopOpacity={0.5} />
                    <stop offset="100%" stopColor="hsl(var(--primary))" stopOpacity={0} />
                  </linearGradient>
                </defs>
                <CartesianGrid stroke="hsl(var(--border))" strokeDasharray="3 3" vertical={false} />
                <XAxis dataKey="t" stroke="hsl(var(--muted-foreground))" fontSize={10} tickLine={false} axisLine={false} />
                <YAxis stroke="hsl(var(--muted-foreground))" fontSize={10} tickLine={false} axisLine={false} domain={[0, 100]} unit="%" />
                <Tooltip
                  contentStyle={{
                    background: "hsl(var(--popover))",
                    border: "1px solid hsl(var(--border))",
                    borderRadius: 8,
                    fontSize: 12,
                  }}
                />
                <Area
                  type="monotone"
                  dataKey="cpu"
                  stroke="hsl(var(--primary))"
                  strokeWidth={2}
                  fill="url(#cpuFill)"
                />
              </AreaChart>
            </ResponsiveContainer>
          </div>
          )}
        </div>

        <div className="glass rounded-2xl p-5">
          <h3 className="text-sm font-semibold mb-2">Status distribution</h3>
          {distribution.length === 0 ? (
            <div className="grid h-[240px] place-items-center text-xs text-muted-foreground">
              No pods yet
            </div>
          ) : (
            <div className="h-[240px]">
              <ResponsiveContainer width="100%" height="100%">
                <PieChart>
                  <Pie
                    data={distribution}
                    dataKey="value"
                    nameKey="name"
                    innerRadius={50}
                    outerRadius={80}
                    paddingAngle={3}
                    stroke="hsl(var(--background))"
                  >
                    {distribution.map((entry) => (
                      <Cell
                        key={entry.name}
                        fill={STATUS_COLORS[entry.name] ?? STATUS_COLORS.Unknown}
                      />
                    ))}
                  </Pie>
                  <Tooltip
                    contentStyle={{
                      background: "hsl(var(--popover))",
                      border: "1px solid hsl(var(--border))",
                      borderRadius: 8,
                      fontSize: 12,
                    }}
                  />
                </PieChart>
              </ResponsiveContainer>
            </div>
          )}
          <ul className="mt-2 space-y-1">
            {distribution.map((d) => (
              <li
                key={d.name}
                className="flex items-center justify-between text-xs"
              >
                <span className="flex items-center gap-2">
                  <span
                    className="h-2 w-2 rounded-full"
                    style={{
                      background:
                        STATUS_COLORS[d.name] ?? STATUS_COLORS.Unknown,
                    }}
                  />
                  {d.name}
                </span>
                <span className="font-mono text-muted-foreground">
                  {d.value}
                </span>
              </li>
            ))}
          </ul>
        </div>
      </div>

      <PodsTable />
    </div>
  );
}
