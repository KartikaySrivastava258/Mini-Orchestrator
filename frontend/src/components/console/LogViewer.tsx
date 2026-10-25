import { useState } from "react";
import { Check, Copy, RefreshCw } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { usePodLogs } from "@/hooks/queries";
import type { Pod } from "@/lib/api/types";
import { exactTime, shortId } from "@/lib/format";
import { ErrorPanel } from "./States";

const TAIL_OPTIONS = [100, 200, 500, 1000, 2000];

/**
 * Log snapshot for one pod, fetched from the node agent via the control plane.
 * "Refresh every 5s" is polling, not streaming, and we label it that way.
 * Logs render as plain text only. Never HTML. Never.
 */
export function LogViewer({ pod }: { pod: Pod }) {
  const [tail, setTail] = useState(200);
  const [auto, setAuto] = useState(false);
  const [wrap, setWrap] = useState(true);
  const [copied, setCopied] = useState(false);
  const hasContainer = !!pod.status.containerID;
  const { data, error, isFetching, isPending, refetch } = usePodLogs(hasContainer ? pod.name : undefined, tail, auto);

  if (!hasContainer) {
    return (
      <p className="px-4 py-6 text-sm text-muted-foreground">
        No container yet (phase {pod.status.phase}). Logs become available once the node agent starts the container.
      </p>
    );
  }

  return (
    <div>
      <div className="flex flex-wrap items-center gap-x-4 gap-y-2 border-b px-4 py-2 text-sm">
        <div className="flex items-center gap-2">
          <Label htmlFor="log-tail" className="text-muted-foreground">Last</Label>
          <Select value={String(tail)} onValueChange={(v) => setTail(Number(v))}>
            <SelectTrigger id="log-tail" className="h-8 w-24">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {TAIL_OPTIONS.map((n) => (
                <SelectItem key={n} value={String(n)}>
                  {n} lines
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        <div className="flex items-center gap-2">
          <Switch id="log-auto" checked={auto} onCheckedChange={setAuto} />
          <Label htmlFor="log-auto">Refresh every 5s</Label>
        </div>
        <div className="flex items-center gap-2">
          <Switch id="log-wrap" checked={wrap} onCheckedChange={setWrap} />
          <Label htmlFor="log-wrap">Wrap lines</Label>
        </div>
        <div className="ml-auto flex items-center gap-1">
          <Button
            variant="ghost"
            size="sm"
            disabled={!data?.logs}
            onClick={async () => {
              try {
                await navigator.clipboard.writeText(data?.logs ?? "");
                setCopied(true);
                setTimeout(() => setCopied(false), 1500);
              } catch {
                setCopied(false);
              }
            }}
          >
            {copied ? <Check aria-hidden className="mr-1.5 h-3.5 w-3.5" /> : <Copy aria-hidden className="mr-1.5 h-3.5 w-3.5" />}
            {copied ? "Copied" : "Copy"}
          </Button>
          <Button variant="ghost" size="sm" onClick={() => refetch()} disabled={isFetching}>
            <RefreshCw aria-hidden className={isFetching ? "mr-1.5 h-3.5 w-3.5 animate-spin" : "mr-1.5 h-3.5 w-3.5"} />
            Refresh
          </Button>
        </div>
      </div>

      {error && !data ? (
        <ErrorPanel error={error} what="logs" onRetry={() => refetch()} />
      ) : isPending ? (
        <p className="px-4 py-6 text-sm text-muted-foreground">Fetching logs from node {pod.status.nodeName}…</p>
      ) : data ? (
        <>
          <p className="border-b bg-muted/40 px-4 py-1.5 text-xs text-muted-foreground">
            Snapshot of container <span className="ident">{shortId(data.containerID)}</span> on {data.nodeName}, fetched{" "}
            {exactTime(data.fetchedAt)}
            {data.truncated && <span className="text-warning"> (truncated to the most recent output)</span>}
            {error ? <span className="text-destructive"> Latest refresh failed.</span> : null}
          </p>
          {data.logs ? (
            <pre
              tabIndex={0}
              aria-label={`Logs for ${pod.name}`}
              className={`max-h-[60vh] overflow-auto bg-[#0f1720] p-4 font-mono text-xs leading-relaxed text-slate-100 ${wrap ? "whitespace-pre-wrap break-all" : "whitespace-pre"}`}
            >
              {data.logs}
            </pre>
          ) : (
            <p className="px-4 py-6 text-sm text-muted-foreground">The container has not written any output.</p>
          )}
        </>
      ) : null}
    </div>
  );
}
