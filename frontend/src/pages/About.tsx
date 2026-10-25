import { PageHeader, Section } from "@/components/console/PageHeader";
import { API_BASE_URL } from "@/lib/api/client";
import { useInfo } from "@/hooks/queries";

const mapping = [
  ["API server", "kube-apiserver", "Small custom REST API; not Kubernetes API compatible"],
  ["etcd state", "etcd", "Project-specific key schema under /mini-orchestrator/v1"],
  ["Scheduler", "kube-scheduler", "LeastPods policy over declared pod slots; no CPU or memory awareness"],
  ["ReplicaSet controller and node monitor", "kube-controller-manager", "Custom polling loops, not Kubernetes controllers"],
  ["Node agent", "kubelet (conceptually)", "Polls the API and drives the Docker CLI; no CRI"],
  ["Pod", "Pod (conceptually)", "Exactly one container; no volumes, ports, probes, namespaces or resource requests"],
  ["Console", "Dashboard-like UI", "Custom UI, not the Kubernetes Dashboard"],
];

export default function About() {
  const { data: info } = useInfo();
  return (
    <>
      <PageHeader title="About" />
      <div className="grid max-w-4xl gap-4">
        <Section title="What this is">
          <div className="space-y-2 p-4 text-sm leading-relaxed">
            <p>
              Mini Orchestrator is a small, Kubernetes-inspired orchestration platform built to teach and demonstrate control-plane concepts using
              Go, etcd, Docker and a React console.
            </p>
            <p>
              It is not a Kubernetes replacement. It does not implement the Kubernetes API, CRI, CNI, CSI, RBAC, admission control or multi-tenant
              isolation.
            </p>
            <p className="text-destructive">
              The API has no authentication. Run it only on a trusted local machine and never expose it, or the node agents, to an untrusted
              network.
            </p>
          </div>
        </Section>
        <Section title="How concepts map to Kubernetes">
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead className="text-left text-muted-foreground">
                <tr className="border-b">
                  <th className="px-4 py-2 font-medium">Here</th>
                  <th className="px-4 py-2 font-medium">Kubernetes analogue</th>
                  <th className="px-4 py-2 font-medium">Important difference</th>
                </tr>
              </thead>
              <tbody className="divide-y">
                {mapping.map(([here, k8s, diff]) => (
                  <tr key={here}>
                    <td className="px-4 py-2 font-medium">{here}</td>
                    <td className="px-4 py-2">{k8s}</td>
                    <td className="px-4 py-2 text-muted-foreground">{diff}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </Section>
        <Section title="This control plane">
          <dl className="grid grid-cols-[minmax(10rem,auto)_1fr] gap-x-6 gap-y-2 p-4 text-sm">
            <dt className="text-muted-foreground">API endpoint</dt>
            <dd className="ident">{API_BASE_URL}</dd>
            <dt className="text-muted-foreground">Version</dt>
            <dd>{info?.version ?? "Not available"}</dd>
            <dt className="text-muted-foreground">Scheduler policy</dt>
            <dd>{info?.schedulerPolicy ?? "Not available"}</dd>
            <dt className="text-muted-foreground">Heartbeat timeout</dt>
            <dd>{info ? `${info.nodeHeartbeatTimeoutSeconds}s` : "Not available"}</dd>
            <dt className="text-muted-foreground">Pod eviction timeout</dt>
            <dd>{info ? `${info.podEvictionTimeoutSeconds}s` : "Not available"}</dd>
            <dt className="text-muted-foreground">Metrics</dt>
            <dd>Not implemented. The console shows only state reported by the API.</dd>
          </dl>
        </Section>
      </div>
    </>
  );
}
