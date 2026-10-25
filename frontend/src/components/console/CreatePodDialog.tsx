import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate } from "react-router-dom";
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
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group";
import { ApiError, createPod, errorMessage } from "@/lib/api/client";
import type { RestartPolicy } from "@/lib/api/types";
import { queryKeys } from "@/hooks/queries";
import { validateImage, validateName } from "@/lib/validation";

export function CreatePodDialog() {
  const [open, setOpen] = useState(false);
  const [name, setName] = useState("");
  const [image, setImage] = useState("");
  const [policy, setPolicy] = useState<RestartPolicy>("Always");
  const [touched, setTouched] = useState(false);
  const [serverError, setServerError] = useState<{ field?: string; message: string } | null>(null);
  const qc = useQueryClient();
  const navigate = useNavigate();

  const nameError = validateName(name.trim(), 63);
  const imageError = validateImage(image.trim());

  const mutation = useMutation({
    mutationFn: () => createPod({ name: name.trim(), image: image.trim(), restartPolicy: policy }),
    onSuccess: (pod) => {
      qc.invalidateQueries({ queryKey: queryKeys.pods });
      qc.invalidateQueries({ queryKey: queryKeys.events });
      toast.success(`Created pod ${pod.name}`, {
        description: "It is Pending until the scheduler assigns a node.",
        action: { label: "View pod", onClick: () => navigate(`/pods/${pod.name}`) },
      });
      reset();
      setOpen(false);
    },
    onError: (err) => {
      setServerError({ field: err instanceof ApiError ? err.field : undefined, message: errorMessage(err) });
    },
  });

  function reset() {
    setName("");
    setImage("");
    setPolicy("Always");
    setTouched(false);
    setServerError(null);
  }

  const fieldError = (field: "name" | "image", local: string | null) =>
    (touched && local) || (serverError?.field === field ? serverError.message : null);

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
          <Plus aria-hidden className="mr-1.5 h-4 w-4" /> Create pod
        </Button>
      </DialogTrigger>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>Create pod</DialogTitle>
          <DialogDescription>
            A pod runs one container. The scheduler places it on a ready node; that node's agent starts it with Docker.
          </DialogDescription>
        </DialogHeader>
        <form
          noValidate
          className="space-y-4"
          onSubmit={(e) => {
            e.preventDefault();
            setTouched(true);
            setServerError(null);
            if (!nameError && !imageError) mutation.mutate();
          }}
        >
          <div className="space-y-1.5">
            <Label htmlFor="pod-name">Name</Label>
            <Input
              id="pod-name"
              className="ident"
              placeholder="web-1"
              value={name}
              autoComplete="off"
              aria-invalid={!!fieldError("name", nameError)}
              aria-describedby="pod-name-help"
              onChange={(e) => setName(e.target.value)}
            />
            <p id="pod-name-help" className={fieldError("name", nameError) ? "text-xs text-destructive" : "text-xs text-muted-foreground"}>
              {fieldError("name", nameError) ?? "Lowercase letters, digits and '-', up to 63 characters."}
            </p>
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="pod-image">Image</Label>
            <Input
              id="pod-image"
              className="ident"
              placeholder="nginx:stable"
              value={image}
              autoComplete="off"
              aria-invalid={!!fieldError("image", imageError)}
              aria-describedby="pod-image-help"
              onChange={(e) => setImage(e.target.value)}
            />
            <p id="pod-image-help" className={fieldError("image", imageError) ? "text-xs text-destructive" : "text-xs text-muted-foreground"}>
              {fieldError("image", imageError) ?? "Pulled by Docker on the assigned node. No ports are published and no volumes are mounted."}
            </p>
          </div>
          <fieldset className="space-y-1.5">
            <legend className="text-sm font-medium">When the container exits</legend>
            <RadioGroup value={policy} onValueChange={(v) => setPolicy(v as RestartPolicy)} className="gap-2">
              <label className="flex items-start gap-2 text-sm">
                <RadioGroupItem value="Always" className="mt-0.5" />
                <span>
                  Restart it <span className="text-muted-foreground">(Always). Gives up after the agent's restart limit.</span>
                </span>
              </label>
              <label className="flex items-start gap-2 text-sm">
                <RadioGroupItem value="Never" className="mt-0.5" />
                <span>
                  Leave it stopped <span className="text-muted-foreground">(Never). Exit code 0 means Succeeded.</span>
                </span>
              </label>
            </RadioGroup>
          </fieldset>
          {serverError && !serverError.field && (
            <p role="alert" className="text-sm text-destructive">
              {serverError.message}
            </p>
          )}
          <p className="text-xs text-muted-foreground">
            Need several identical pods? Use a <Link className="underline" to="/replicasets">ReplicaSet</Link>.
          </p>
          <DialogFooter>
            <Button type="button" variant="ghost" onClick={() => setOpen(false)} disabled={mutation.isPending}>
              Cancel
            </Button>
            <Button type="submit" disabled={mutation.isPending}>
              {mutation.isPending && <Loader2 aria-hidden className="mr-1.5 h-4 w-4 animate-spin" />}
              Create pod
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
