import { AlertTriangle, RefreshCw } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { ApiError, errorMessage } from "@/lib/api/client";

export function TableSkeleton({ rows = 4, columns = 5 }: { rows?: number; columns?: number }) {
  return (
    <div className="divide-y" aria-busy="true" aria-label="Loading">
      {Array.from({ length: rows }, (_, r) => (
        <div key={r} className="flex gap-4 px-4 py-3">
          {Array.from({ length: columns }, (_, c) => (
            <Skeleton key={c} className="h-4 flex-1" />
          ))}
        </div>
      ))}
    </div>
  );
}

export function EmptyState({ title, children, action }: { title: string; children?: React.ReactNode; action?: React.ReactNode }) {
  return (
    <div className="px-6 py-12 text-center">
      <p className="font-medium">{title}</p>
      {children && <div className="mx-auto mt-1 max-w-prose text-sm text-muted-foreground">{children}</div>}
      {action && <div className="mt-4">{action}</div>}
    </div>
  );
}

/** Says what broke, hints at the fix, and offers a retry button. */
export function ErrorPanel({ error, onRetry, what }: { error: unknown; onRetry?: () => void; what: string }) {
  const network = error instanceof ApiError && error.isNetworkError;
  const storeDown = error instanceof ApiError && error.code === "STORE_UNAVAILABLE";
  return (
    <div role="alert" className="flex flex-col gap-3 border-l-4 border-destructive bg-destructive/5 px-4 py-3 sm:flex-row sm:items-start">
      <AlertTriangle aria-hidden className="mt-0.5 h-4 w-4 shrink-0 text-destructive" />
      <div className="flex-1 text-sm">
        <p className="font-medium">Could not load {what}.</p>
        <p className="text-muted-foreground">{errorMessage(error)}</p>
        {network && <p className="mt-1 text-muted-foreground">Start it with <code className="ident">make run-control-plane</code>.</p>}
        {storeDown && <p className="mt-1 text-muted-foreground">Start etcd with <code className="ident">make etcd-up</code>.</p>}
        {error instanceof ApiError && error.requestId && (
          <p className="mt-1 text-xs text-muted-foreground">
            Request ID <span className="ident">{error.requestId}</span>
          </p>
        )}
      </div>
      {onRetry && (
        <Button variant="outline" size="sm" onClick={onRetry}>
          <RefreshCw aria-hidden className="mr-1.5 h-3.5 w-3.5" /> Retry
        </Button>
      )}
    </div>
  );
}

/** Shown when a refresh failed but we still have older data on screen. Honesty banner. */
export function StaleDataNotice({ error, updatedAt }: { error: unknown; updatedAt: number }) {
  return (
    <p role="status" className="border-b bg-warning/10 px-4 py-2 text-xs text-warning">
      Showing data from {new Date(updatedAt).toLocaleTimeString()}; the latest refresh failed: {errorMessage(error)}
    </p>
  );
}
