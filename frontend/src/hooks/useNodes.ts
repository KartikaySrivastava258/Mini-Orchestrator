import { useQuery } from "@tanstack/react-query";
import { listNodes, type Node } from "@/lib/orchestrator-api";

export function useNodes(refetchMs = 4000) {
  return useQuery<Node[]>({
    queryKey: ["nodes"],
    queryFn: listNodes,
    refetchInterval: refetchMs,
    refetchOnWindowFocus: false,
    retry: 1,
  });
}
