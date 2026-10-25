import { useQuery } from "@tanstack/react-query";
import * as api from "@/lib/api/client";

/**
 * One query per resource, shared by everyone who needs it. Five widgets on a
 * page still means one request per interval, not five. React Query also pauses
 * polling while the tab is hidden, so your laptop fan can chill.
 */
export const POLL_INTERVAL_MS = 5_000;

export const queryKeys = {
  pods: ["pods"] as const,
  pod: (name: string) => ["pods", "detail", name] as const,
  nodes: ["nodes"] as const,
  node: (name: string) => ["nodes", "detail", name] as const,
  replicaSets: ["replicasets"] as const,
  events: ["events"] as const,
  readiness: ["readiness"] as const,
  info: ["info"] as const,
  logs: (pod: string, tail: number) => ["logs", pod, tail] as const,
};

export const usePods = () =>
  useQuery({ queryKey: queryKeys.pods, queryFn: ({ signal }) => api.listPods(signal), refetchInterval: POLL_INTERVAL_MS });

export const usePod = (name: string) =>
  useQuery({ queryKey: queryKeys.pod(name), queryFn: ({ signal }) => api.getPod(name, signal), refetchInterval: POLL_INTERVAL_MS });

export const useNodes = () =>
  useQuery({ queryKey: queryKeys.nodes, queryFn: ({ signal }) => api.listNodes(signal), refetchInterval: POLL_INTERVAL_MS });

export const useNode = (name: string) =>
  useQuery({ queryKey: queryKeys.node(name), queryFn: ({ signal }) => api.getNode(name, signal), refetchInterval: POLL_INTERVAL_MS });

export const useReplicaSets = () =>
  useQuery({
    queryKey: queryKeys.replicaSets,
    queryFn: ({ signal }) => api.listReplicaSets(signal),
    refetchInterval: POLL_INTERVAL_MS,
  });

export const useEvents = () =>
  useQuery({ queryKey: queryKeys.events, queryFn: ({ signal }) => api.listEvents(signal), refetchInterval: POLL_INTERVAL_MS });

export const useReadiness = () =>
  useQuery({
    queryKey: queryKeys.readiness,
    queryFn: ({ signal }) => api.getReadiness(signal),
    refetchInterval: POLL_INTERVAL_MS,
    retry: false,
  });

/** Control-plane settings only change on restart, no need to keep asking. */
export const useInfo = () =>
  useQuery({ queryKey: queryKeys.info, queryFn: ({ signal }) => api.getInfo(signal), staleTime: 5 * 60_000 });

export const usePodLogs = (pod: string | undefined, tailLines: number, autoRefresh: boolean) =>
  useQuery({
    queryKey: queryKeys.logs(pod ?? "", tailLines),
    queryFn: ({ signal }) => api.getPodLogs(pod as string, tailLines, signal),
    enabled: !!pod,
    refetchInterval: autoRefresh ? POLL_INTERVAL_MS : false,
    retry: false,
  });
