import axios from "axios";

const BASE_URL =
  import.meta.env.VITE_ORCHESTRATOR_URL?.trim() || "http://localhost:8080";

export const api = axios.create({
  baseURL: BASE_URL,
  timeout: 8000,
  headers: { "Content-Type": "application/json" },
});

// Optional debug (very useful)
api.interceptors.request.use((req) => {
  console.log("API CALL:", req.method?.toUpperCase(), req.baseURL + req.url);
  return req;
});

export type PodStatus =
  | "Pending"
  | "Running"
  | "Succeeded"
  | "Failed"
  | "CrashLoopBackOff"
  | string;

export interface Pod {
  name: string;
  image: string;
  node: string;
  status: PodStatus;
}

export interface Node {
  name: string;
  active: boolean;
  last_heartbeat: number;
  cpu_usage: number;
}

// ------- POD API -------
export async function listPods(): Promise<Pod[]> {
  const { data } = await api.get<Pod[]>("/getpods");
  return Array.isArray(data) ? data : [];
}

export async function createPod(name: string, image: string): Promise<void> {
  await api.post("/pods", { name, image });
}

export async function deletePod(name: string): Promise<void> {
  await api.delete(`/pods/${encodeURIComponent(name)}`);
}

export async function updatePodStatus(
  name: string,
  status: PodStatus,
): Promise<void> {
  await api.post("/updatePodStatus", { name, status });
}

export async function getLogs(podName: string): Promise<string> {
  const { data } = await api.get(`/logs/${encodeURIComponent(podName)}`, {
    transformResponse: [(d) => d], // raw text
  });
  return typeof data === "string" ? data : JSON.stringify(data, null, 2);
}

// ------- NODE API -------

// ✅ FIXED: now uses /getnodes (not /nodes)
export async function listNodes(): Promise<Node[]> {
  const { data } = await api.get<Node[]>("/getnodes");
  return Array.isArray(data) ? data : [];
}

// Register node (used by node agent)
export async function registerNode(node: Partial<Node>): Promise<void> {
  await api.post("/nodes", node);
}

// Heartbeat (CPU updates)
export async function sendHeartbeat(node: Partial<Node>): Promise<void> {
  await api.post("/heartbeat", node);
}

// ✅ NEW: Delete node
export async function deleteNode(name: string): Promise<void> {
  await api.delete(`/nodes/${encodeURIComponent(name)}`);
}

export const ORCHESTRATOR_BASE_URL = BASE_URL;