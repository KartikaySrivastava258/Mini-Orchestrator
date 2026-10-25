import { useEffect, useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Loader2, Plus } from "lucide-react";
import { toast } from "sonner";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { ApiError, createReplicaSet, errorMessage, scaleReplicaSet } from "@/lib/api/client";
import type { ReplicaSet } from "@/lib/api/types";
import { queryKeys } from "@/hooks/queries";
import { validateImage, validateName, validateReplicas } from "@/lib/validation";

function FieldHelp({ id, error, children }: { id: string; error: string | null; children: React.ReactNode }) {
  return (
    <p id={id} className={error ? "text-xs text-destructive" : "text-xs text-muted-foreground"}>
      {error ?? children}
    </p>
  );
}

export function CreateReplicaSetDialog() {
  const [open, setOpen] = useState(false);
  const [name, setName] = useState("");
  const [image, setImage] = useState("");
  const [replicas, setReplicas] = useState("2");
  const [touched, setTouched] = useState(false);
  const [serverError, setServerError] = useState<{ field?: string; message: string } | null>(null);
  const qc = useQueryClient();

  const errors = {
    name: validateName(name.trim(), 57),
    image: validateImage(image.trim()),
    replicas: validateReplicas(replicas),
  };
  const shown = (field: keyof typeof errors) =>
    (touched && errors[field]) || (serverError?.field === field ? serverError.message : null);

  const reset = () => {
    setName("");
    setImage("");
    setReplicas("2");
    setTouched(false);
    setServerError(null);
  };

  const mutation = useMutation({
    mutationFn: () => createReplicaSet({ name: name.trim(), image: image.trim(), replicas: Number(replicas) }),
    onSuccess: (rs) => {
      qc.invalidateQueries({ queryKey: queryKeys.replicaSets });
      qc.invalidateQueries({ queryKey: queryKeys.events });
      toast.success(`Created ReplicaSet ${rs.name}`, { description: "The controller creates its pods on its next pass." });
      reset();
      setOpen(false);
    },
    onError: (err) => setServerError({ field: err instanceof ApiError ? err.field : undefined, message: errorMessage(err) }),
  });

  return (
    <Dialog
      open={open}
      onOpenChange={(o) => {
        if (mutation.isPending) return;
        setOpen(o);
        if (!o) reset();
      }}
    >
      <DialogTrigger asChild>
        <Button size="sm">
          <Plus aria-hidden className="mr-1.5 h-4 w-4" /> Create ReplicaSet
        </Button>
      </DialogTrigger>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>Create ReplicaSet</DialogTitle>
          <DialogDescription>
            The ReplicaSet controller keeps this many identical pods active, replacing pods that fail. Its pods always restart on exit.
          </DialogDescription>
        </DialogHeader>
        <form
          noValidate
          className="space-y-4"
          onSubmit={(e) => {
            e.preventDefault();
            setTouched(true);
            setServerError(null);
            if (!errors.name && !errors.image && !errors.replicas) mutation.mutate();
          }}
        >
          <div className="space-y-1.5">
            <Label htmlFor="rs-name">Name</Label>
            <Input id="rs-name" className="ident" placeholder="web" value={name} autoComplete="off"
              aria-invalid={!!shown("name")} aria-describedby="rs-name-help" onChange={(e) => setName(e.target.value)} />
            <FieldHelp id="rs-name-help" error={shown("name")}>
              Up to 57 characters; pods are named after it with a random suffix.
            </FieldHelp>
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="rs-image">Image</Label>
            <Input id="rs-image" className="ident" placeholder="nginx:stable" value={image} autoComplete="off"
              aria-invalid={!!shown("image")} aria-describedby="rs-image-help" onChange={(e) => setImage(e.target.value)} />
            <FieldHelp id="rs-image-help" error={shown("image")}>
              The image cannot be changed later; there are no rolling updates.
            </FieldHelp>
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="rs-replicas">Replicas</Label>
            <Input id="rs-replicas" type="number" min={0} max={50} inputMode="numeric" className="w-28 tabular-nums" value={replicas}
              aria-invalid={!!shown("replicas")} aria-describedby="rs-replicas-help" onChange={(e) => setReplicas(e.target.value)} />
            <FieldHelp id="rs-replicas-help" error={shown("replicas")}>
              0 to 50.
            </FieldHelp>
          </div>
          {serverError && !serverError.field && (
            <p role="alert" className="text-sm text-destructive">
              {serverError.message}
            </p>
          )}
          <DialogFooter>
            <Button type="button" variant="ghost" onClick={() => setOpen(false)} disabled={mutation.isPending}>
              Cancel
            </Button>
            <Button type="submit" disabled={mutation.isPending}>
              {mutation.isPending && <Loader2 aria-hidden className="mr-1.5 h-4 w-4 animate-spin" />}
              Create ReplicaSet
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

/** Scaling down kills pods, so the dialog tells you exactly which ones go first. */
export function ScaleDialog({ rs, open, onOpenChange }: { rs: ReplicaSet | null; open: boolean; onOpenChange: (o: boolean) => void }) {
  const [value, setValue] = useState("");
  const qc = useQueryClient();
  const current = rs?.spec.replicas ?? 0;
  const error = validateReplicas(value);
  const target = Number(value);

  useEffect(() => {
    if (open) setValue(String(current));
  }, [open, current]);

  const mutation = useMutation({
    mutationFn: () => scaleReplicaSet((rs as ReplicaSet).name, target),
    onSuccess: (updated) => {
      qc.invalidateQueries({ queryKey: queryKeys.replicaSets });
      qc.invalidateQueries({ queryKey: queryKeys.events });
      toast.success(`Scaled ${updated.name} to ${updated.spec.replicas}`);
      onOpenChange(false);
    },
    onError: (err) => toast.error("Scaling failed", { description: errorMessage(err) }),
  });

  return (
    <Dialog
      open={open}
      onOpenChange={(o) => {
        if (mutation.isPending) return;
        onOpenChange(o);
      }}
    >
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Scale {rs?.name}</DialogTitle>
          <DialogDescription>Currently {current} desired replica{current === 1 ? "" : "s"}.</DialogDescription>
        </DialogHeader>
        <form
          noValidate
          className="space-y-4"
          onSubmit={(e) => {
            e.preventDefault();
            if (!error && rs) mutation.mutate();
          }}
        >
          <div className="space-y-1.5">
            <Label htmlFor="scale-replicas">Desired replicas</Label>
            <Input id="scale-replicas" type="number" min={0} max={50} inputMode="numeric" className="w-28 tabular-nums"
              value={value} aria-invalid={!!error} onChange={(e) => setValue(e.target.value)} />
            {error && <p className="text-xs text-destructive">{error}</p>}
          </div>
          {!error && target < current && (
            <p role="note" className="rounded border border-warning/40 bg-warning/10 px-3 py-2 text-sm">
              {current - target} pod{current - target === 1 ? "" : "s"} will be terminated: unscheduled pods first, then the newest
              running ones.
            </p>
          )}
          <DialogFooter>
            <Button type="button" variant="ghost" onClick={() => onOpenChange(false)} disabled={mutation.isPending}>
              Cancel
            </Button>
            <Button type="submit" disabled={mutation.isPending || !!error || target === current}>
              {mutation.isPending && <Loader2 aria-hidden className="mr-1.5 h-4 w-4 animate-spin" />}
              Scale
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
