import { usePods } from "@/hooks/usePods";
import { cn } from "@/lib/utils";
import { Wifi, WifiOff } from "lucide-react";

export function ConnectionPill() {
  const { isError, isLoading, dataUpdatedAt } = usePods(5000);
  const online = !isError && !isLoading;
  return (
    <div
      className={cn(
        "flex items-center gap-2 rounded-full border px-3 py-1.5 text-xs",
        online
          ? "border-success/30 bg-success/10 text-success"
          : "border-destructive/30 bg-destructive/10 text-destructive",
      )}
    >
      <span
        className={cn(
          "status-dot",
          online ? "bg-success animate-pulse-glow" : "bg-destructive",
        )}
      />
      {online ? (
        <>
          <Wifi className="h-3.5 w-3.5" />
          <span className="font-mono">
            cluster online · {dataUpdatedAt ? new Date(dataUpdatedAt).toLocaleTimeString() : "—"}
          </span>
        </>
      ) : (
        <>
          <WifiOff className="h-3.5 w-3.5" />
          <span className="font-mono">cluster unreachable</span>
        </>
      )}
    </div>
  );
}
