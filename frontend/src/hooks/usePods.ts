import { useQuery } from "@tanstack/react-query";
import { listPods } from "@/lib/orchestrator-api";

export function usePods(refetchMs = 4000) {
  return useQuery({
    queryKey: ["pods"],
    queryFn: listPods,
    refetchInterval: refetchMs,
    refetchOnWindowFocus: false,
    retry: 1,
  });
}
