import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Plus, Loader2, Box } from "lucide-react";
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
import { createPod } from "@/lib/orchestrator-api";
import { toast } from "sonner";

export function CreatePodDialog() {
  const [open, setOpen] = useState(false);
  const [name, setName] = useState("");
  const [image, setImage] = useState("nginx");
  const qc = useQueryClient();

  const mutation = useMutation({
    mutationFn: () => createPod(name.trim(), image.trim()),
    onSuccess: () => {
      toast.success(`Pod "${name}" scheduled`, {
        description: "The scheduler will assign it to a node shortly.",
      });
      qc.invalidateQueries({ queryKey: ["pods"] });
      setName("");
      setOpen(false);
    },
    onError: (err: any) => {
      toast.error("Failed to create pod", {
        description: err?.response?.data ?? err?.message ?? "Unknown error",
      });
    },
  });

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <Button className="bg-gradient-primary text-primary-foreground hover:opacity-90 shadow-glow">
          <Plus className="h-4 w-4 mr-1.5" /> Create Pod
        </Button>
      </DialogTrigger>
      <DialogContent className="glass border-glass-border sm:max-w-md">
        <DialogHeader>
          <div className="mx-auto mb-2 grid h-11 w-11 place-items-center rounded-xl bg-gradient-primary shadow-glow">
            <Box className="h-5 w-5 text-primary-foreground" />
          </div>
          <DialogTitle className="text-center">Deploy a new pod</DialogTitle>
          <DialogDescription className="text-center">
            Submit a pod spec — the controller will schedule it onto an active node.
          </DialogDescription>
        </DialogHeader>

        <form
          onSubmit={(e) => {
            e.preventDefault();
            if (!name.trim() || !image.trim()) {
              toast.error("Name and image are required");
              return;
            }
            mutation.mutate();
          }}
          className="space-y-4 py-2"
        >
          <div className="space-y-1.5">
            <Label htmlFor="pod-name" className="text-xs uppercase tracking-wider">
              Pod name
            </Label>
            <Input
              id="pod-name"
              placeholder="my-nginx-1"
              value={name}
              onChange={(e) => setName(e.target.value)}
              className="font-mono bg-secondary/60 border-border/80"
              autoFocus
            />
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="pod-image" className="text-xs uppercase tracking-wider">
              Container image
            </Label>
            <Input
              id="pod-image"
              placeholder="nginx:latest"
              value={image}
              onChange={(e) => setImage(e.target.value)}
              className="font-mono bg-secondary/60 border-border/80"
            />
          </div>

          <DialogFooter className="gap-2">
            <Button
              type="button"
              variant="ghost"
              onClick={() => setOpen(false)}
              disabled={mutation.isPending}
            >
              Cancel
            </Button>
            <Button
              type="submit"
              disabled={mutation.isPending}
              className="bg-gradient-primary text-primary-foreground hover:opacity-90"
            >
              {mutation.isPending ? (
                <>
                  <Loader2 className="h-4 w-4 mr-1.5 animate-spin" /> Scheduling…
                </>
              ) : (
                <>Deploy</>
              )}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
