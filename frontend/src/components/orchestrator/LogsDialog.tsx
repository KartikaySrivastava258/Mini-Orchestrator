import { useEffect, useState } from "react";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Loader2, RefreshCw, ScrollText, Copy, Check } from "lucide-react";
import { getLogs } from "@/lib/orchestrator-api";
import { toast } from "sonner";

interface Props {
  podName: string | null;
  onOpenChange: (o: boolean) => void;
}

export function LogsDialog({ podName, onOpenChange }: Props) {
  const [logs, setLogs] = useState<string>("");
  const [loading, setLoading] = useState(false);
  const [copied, setCopied] = useState(false);

  const fetchLogs = async (name: string) => {
    setLoading(true);
    try {
      const data = await getLogs(name);
      setLogs(data || "(no log output)");
    } catch (err: any) {
      setLogs(`Error fetching logs: ${err?.message ?? "unknown"}`);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    if (podName) {
      setLogs("");
      fetchLogs(podName);
    }
  }, [podName]);

  return (
    <Dialog open={!!podName} onOpenChange={onOpenChange}>
      <DialogContent className="glass max-w-3xl border-glass-border">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <ScrollText className="h-4 w-4 text-primary" />
            Logs · <span className="font-mono text-primary">{podName}</span>
          </DialogTitle>
          <DialogDescription>
            Streamed from the node agent via <code className="font-mono">/logs/{podName}</code>
          </DialogDescription>
        </DialogHeader>

        <div className="relative rounded-lg border border-border bg-[hsl(222_40%_4%)] overflow-hidden">
          <div className="flex items-center justify-between border-b border-border/80 bg-secondary/40 px-3 py-1.5">
            <div className="flex items-center gap-1.5">
              <span className="h-2.5 w-2.5 rounded-full bg-destructive/70" />
              <span className="h-2.5 w-2.5 rounded-full bg-warning/70" />
              <span className="h-2.5 w-2.5 rounded-full bg-success/70" />
              <span className="ml-3 font-mono text-[11px] text-muted-foreground">
                pod://{podName}
              </span>
            </div>
            <div className="flex items-center gap-1">
              <Button
                size="sm"
                variant="ghost"
                className="h-7 px-2"
                onClick={() => {
                  navigator.clipboard.writeText(logs);
                  setCopied(true);
                  setTimeout(() => setCopied(false), 1500);
                  toast.success("Logs copied");
                }}
              >
                {copied ? (
                  <Check className="h-3.5 w-3.5" />
                ) : (
                  <Copy className="h-3.5 w-3.5" />
                )}
              </Button>
              <Button
                size="sm"
                variant="ghost"
                className="h-7 px-2"
                onClick={() => podName && fetchLogs(podName)}
                disabled={loading}
              >
                <RefreshCw
                  className={`h-3.5 w-3.5 ${loading ? "animate-spin" : ""}`}
                />
              </Button>
            </div>
          </div>
          <pre className="scrollbar-thin h-[420px] overflow-auto p-4 text-xs font-mono leading-relaxed text-foreground/90 whitespace-pre-wrap">
            {loading ? (
              <span className="flex items-center gap-2 text-muted-foreground">
                <Loader2 className="h-3.5 w-3.5 animate-spin" /> Fetching logs…
              </span>
            ) : (
              logs
            )}
          </pre>
        </div>
      </DialogContent>
    </Dialog>
  );
}
