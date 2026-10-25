import type {
  ApiErrorEnvelope,
  ClusterEvent,
  ClusterInfo,
  CreatePodRequest,
  CreateReplicaSetRequest,
  DeleteResponse,
  ListResponse,
  LogsResponse,
  Node,
  Pod,
  Readiness,
  ReplicaSet,
} from "./types";

const configured = (import.meta.env.VITE_ORCHESTRATOR_URL as string | undefined)?.trim();

/** Where the control-plane API lives. No trailing slash, ever. */
export const API_BASE_URL = (configured || "http://127.0.0.1:8080").replace(/\/+$/, "");

const DEFAULT_TIMEOUT_MS = 10_000;

/** Every failed request becomes an ApiError with a message a human can actually read. */
export class ApiError extends Error {
  readonly status: number;
  readonly code: string;
  readonly field?: string;
  readonly requestId?: string;

  constructor(status: number, code: string, message: string, field?: string, requestId?: string) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.code = code;
    this.field = field;
    this.requestId = requestId;
  }

  /** True when we never even reached the API (vs. the API answering "no"). */
  get isNetworkError(): boolean {
    return this.status === 0;
  }
}

interface RequestOptions {
  method?: "GET" | "POST" | "PUT" | "PATCH" | "DELETE";
  body?: unknown;
  signal?: AbortSignal;
  timeoutMs?: number;
}

function isEnvelope(value: unknown): value is ApiErrorEnvelope {
  if (typeof value !== "object" || value === null || !("error" in value)) return false;
  const err = (value as { error: unknown }).error;
  return typeof err === "object" && err !== null && "message" in err && "code" in err;
}

async function send(path: string, opts: RequestOptions): Promise<{ response: Response; data: unknown }> {
  const controller = new AbortController();
  let timedOut = false;
  const timer = setTimeout(() => {
    timedOut = true;
    controller.abort();
  }, opts.timeoutMs ?? DEFAULT_TIMEOUT_MS);
  const forwardAbort = () => controller.abort();
  opts.signal?.addEventListener("abort", forwardAbort, { once: true });

  let response: Response;
  try {
    response = await fetch(`${API_BASE_URL}${path}`, {
      method: opts.method ?? "GET",
      headers: opts.body === undefined ? undefined : { "Content-Type": "application/json" },
      body: opts.body === undefined ? undefined : JSON.stringify(opts.body),
      signal: controller.signal,
    });
  } catch (err) {
    if (opts.signal?.aborted) throw err; // React Query cancelled it on purpose, that's not the API's fault
    if (timedOut) {
      throw new ApiError(0, "TIMEOUT", `The API at ${API_BASE_URL} did not respond within ${(opts.timeoutMs ?? DEFAULT_TIMEOUT_MS) / 1000}s.`);
    }
    throw new ApiError(0, "NETWORK_ERROR", `Cannot reach the API at ${API_BASE_URL}. Check that the control plane is running.`);
  } finally {
    clearTimeout(timer);
    opts.signal?.removeEventListener("abort", forwardAbort);
  }

  const text = await response.text();
  let data: unknown = undefined;
  if (text) {
    try {
      data = JSON.parse(text);
    } catch {
      data = undefined;
    }
  }
  return { response, data };
}

/** Sends a request and hands back the JSON body, or throws an ApiError. No silent failures. */
export async function request<T>(path: string, opts: RequestOptions = {}): Promise<T> {
  const { response, data } = await send(path, opts);
  if (!response.ok) {
    if (isEnvelope(data)) {
      throw new ApiError(response.status, data.error.code, data.error.message, data.error.field, data.requestId);
    }
    throw new ApiError(response.status, `HTTP_${response.status}`, `The API returned HTTP ${response.status}.`);
  }
  if (data === undefined) {
    throw new ApiError(response.status, "INVALID_RESPONSE", "The API returned a response that is not JSON.");
  }
  return data as T;
}

const enc = encodeURIComponent;

// --- pods ---
export const listPods = (signal?: AbortSignal) =>
  request<ListResponse<Pod>>("/api/v1/pods", { signal }).then((r) => r.items);
export const getPod = (name: string, signal?: AbortSignal) => request<Pod>(`/api/v1/pods/${enc(name)}`, { signal });
export const createPod = (body: CreatePodRequest) => request<Pod>("/api/v1/pods", { method: "POST", body });
export const deletePod = (name: string) => request<DeleteResponse>(`/api/v1/pods/${enc(name)}`, { method: "DELETE" });
export const getPodLogs = (name: string, tailLines: number, signal?: AbortSignal) =>
  request<LogsResponse>(`/api/v1/pods/${enc(name)}/logs?tailLines=${tailLines}`, { signal, timeoutMs: 15_000 });

// --- replica sets ---
export const listReplicaSets = (signal?: AbortSignal) =>
  request<ListResponse<ReplicaSet>>("/api/v1/replicasets", { signal }).then((r) => r.items);
export const createReplicaSet = (body: CreateReplicaSetRequest) =>
  request<ReplicaSet>("/api/v1/replicasets", { method: "POST", body });
export const scaleReplicaSet = (name: string, replicas: number) =>
  request<ReplicaSet>(`/api/v1/replicasets/${enc(name)}`, { method: "PATCH", body: { replicas } });
export const deleteReplicaSet = (name: string) =>
  request<DeleteResponse>(`/api/v1/replicasets/${enc(name)}`, { method: "DELETE" });

// --- nodes ---
export const listNodes = (signal?: AbortSignal) =>
  request<ListResponse<Node>>("/api/v1/nodes", { signal }).then((r) => r.items);
export const getNode = (name: string, signal?: AbortSignal) => request<Node>(`/api/v1/nodes/${enc(name)}`, { signal });
export const setNodeSchedulable = (name: string, unschedulable: boolean) =>
  request<Node>(`/api/v1/nodes/${enc(name)}`, { method: "PATCH", body: { unschedulable } });
export const deleteNode = (name: string) => request<DeleteResponse>(`/api/v1/nodes/${enc(name)}`, { method: "DELETE" });

// --- events, health, info ---
export const listEvents = (signal?: AbortSignal, limit = 200) =>
  request<ListResponse<ClusterEvent>>(`/api/v1/events?limit=${limit}`, { signal }).then((r) => r.items);
export const getInfo = (signal?: AbortSignal) => request<ClusterInfo>("/api/v1/info", { signal });

/**
 * /readyz answers 503 (with a JSON body) when etcd is down. That's a real
 * answer, not a crash, so we return it instead of throwing.
 */
export async function getReadiness(signal?: AbortSignal): Promise<Readiness> {
  const { response, data } = await send("/readyz", { signal, timeoutMs: 5_000 });
  if ((response.ok || response.status === 503) && typeof data === "object" && data !== null && "status" in data) {
    return data as Readiness;
  }
  throw new ApiError(response.status, `HTTP_${response.status}`, `Unexpected readiness response (HTTP ${response.status}).`);
}

/** Turns whatever got thrown into a sentence. */
export function errorMessage(err: unknown): string {
  if (err instanceof ApiError) return err.message;
  if (err instanceof Error) return err.message;
  return "Unexpected error";
}
