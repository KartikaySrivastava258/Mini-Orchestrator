import type { Node, Pod, PodPhase } from "./api/types";

/** Go's zero time ("0001-01-01T00:00:00Z") and garbage strings both mean "no value". */
export function parseTime(iso?: string): Date | null {
  if (!iso) return null;
  const d = new Date(iso);
  if (Number.isNaN(d.getTime()) || d.getUTCFullYear() < 2000) return null;
  return d;
}

/** Short relative time: "now", "42s ago", "5m ago", "3h ago", "2d ago". */
export function relativeTime(iso: string | undefined, now: number = Date.now()): string {
  const d = parseTime(iso);
  if (!d) return "—";
  const s = Math.round((now - d.getTime()) / 1000);
  if (s < 0) return "just now";
  if (s < 5) return "now";
  if (s < 60) return `${s}s ago`;
  const m = Math.floor(s / 60);
  if (m < 60) return `${m}m ago`;
  const h = Math.floor(m / 60);
  if (h < 48) return `${h}h ago`;
  return `${Math.floor(h / 24)}d ago`;
}

/** Age without the "ago" suffix, as used in resource tables ("5m", "3h"). */
export function age(iso: string | undefined, now: number = Date.now()): string {
  const r = relativeTime(iso, now);
  if (r === "—" || r === "now" || r === "just now") return r === "—" ? r : "0s";
  return r.replace(" ago", "");
}

export function exactTime(iso?: string): string {
  const d = parseTime(iso);
  return d ? d.toLocaleString(undefined, { dateStyle: "medium", timeStyle: "medium" }) : "Not available";
}

export type NodeHealth = "Ready" | "StaleHeartbeat" | "RuntimeUnavailable" | "NotReady";

/**
 * Works out a node's health from the API fields. The control plane marks a
 * node NotReady on its next monitor pass, which can lag a few seconds, so the
 * console also calls out an old heartbeat as stale right away.
 */
export function nodeHealth(node: Node, heartbeatTimeoutSeconds: number | undefined, now: number = Date.now()): NodeHealth {
  if (!node.status.ready) {
    return node.status.reason === "RuntimeUnavailable" ? "RuntimeUnavailable" : "NotReady";
  }
  const hb = parseTime(node.status.lastHeartbeatTime);
  if (!hb) return "StaleHeartbeat";
  if (heartbeatTimeoutSeconds && now - hb.getTime() > heartbeatTimeoutSeconds * 1000) return "StaleHeartbeat";
  return "Ready";
}

export type PhaseCounts = Record<PodPhase, number>;

export function countPhases(pods: Pod[]): PhaseCounts {
  const counts: PhaseCounts = { Pending: 0, Scheduled: 0, Running: 0, Succeeded: 0, Failed: 0, Unknown: 0, Terminating: 0 };
  for (const p of pods) counts[p.status.phase] = (counts[p.status.phase] ?? 0) + 1;
  return counts;
}

/** Pods worth a second look: failed, unknown, backing off, or stuck Pending for a real reason. */
export function needsAttention(p: Pod): boolean {
  if (p.status.phase === "Failed" || p.status.phase === "Unknown") return true;
  if (p.status.phase === "Pending" && p.status.reason && p.status.reason !== "AwaitingScheduling") return true;
  return p.status.reason === "RestartBackOff" || p.status.reason === "RestartFailed";
}

/** Pods holding a slot on their node. Same rule as Pod.OccupiesNode in Go. */
export function occupiesNode(p: Pod): boolean {
  return !!p.status.nodeName && ["Scheduled", "Running", "Unknown", "Terminating"].includes(p.status.phase);
}

export function shortId(id?: string): string {
  return id ? id.slice(0, 12) : "—";
}
