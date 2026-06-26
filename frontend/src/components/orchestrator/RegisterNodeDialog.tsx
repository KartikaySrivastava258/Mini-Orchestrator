import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Plus, Loader2, Server } from "lucide-react";
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
import { registerNode, sendHeartbeat } from "@/lib/orchestrator-api";
import { toast } from "sonner";

export function RegisterNodeDialog() {
  const [open, setOpen] = useState(false);
  const [name, setName] = useState("");
  const [cpu, setCpu] = useState("20");
  const qc = useQueryClient();

  const mutation = useMutation({
    mutationFn: async () => {
      const payload = {
        name: name.trim(),
        active: true,
        last_heartbeat: Math.floor(Date.now() / 1000),
        cpu_usage: Number(cpu) || 0,
      };
      await registerNode(payload);
      // Send an initial heartbeat so the controller marks it healthy immediately.
      try {
        await sendHeartbeat(payload);
      } catch {
        /* heartbeat is best-effort */
      }
    },
    onSuccess: () => {
      toast.success(`Node "${name}" registered`, {
        description: "It will start receiving scheduled pods shortly.",
      });
      qc.invalidateQueries({ queryKey: ["pods"] });
      setName("");
      setCpu("20");
      setOpen(false);
    },
    onError: (err: any) => {
      toast.error("Failed to register node", {
        description: err?.response?.data ?? err?.message ?? "Unknown error",
      });
    },
  });

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <Button className="bg-gradient-primary text-primary-foreground hover:opacity-90 shadow-glow">
          <Plus className="h-4 w-4 mr-1.5" /> Register Node
        </Button>
      </DialogTrigger>
      <DialogContent className="glass border-glass-border sm:max-w-md">
        <DialogHeader>
          <div className="mx-auto mb-2 grid h-11 w-11 place-items-center rounded-xl bg-gradient-primary shadow-glow">
            <Server className="h-5 w-5 text-primary-foreground" />
          </div>
          <DialogTitle className="text-center">Register a worker node</DialogTitle>
          <DialogDescription className="text-center">
            Adds the node to the cluster via <code className="font-mono">POST /nodes</code>.
            The agent process should still be running on that host.
          </DialogDescription>
        </DialogHeader>

        <form
          onSubmit={(e) => {
            e.preventDefault();
            if (!name.trim()) {
              toast.error("Node name is required");
              return;
            }
            mutation.mutate();
          }}
          className="space-y-4 py-2"
        >
          <div className="space-y-1.5">
            <Label htmlFor="node-name" className="text-xs uppercase tracking-wider">
              Node name
            </Label>
            <Input
              id="node-name"
              placeholder="worker-1"
              value={name}
              onChange={(e) => setName(e.target.value)}
              className="font-mono bg-secondary/60 border-border/80"
              autoFocus
            />
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="node-cpu" className="text-xs uppercase tracking-wider">
              Initial CPU usage (%)
            </Label>
            <Input
              id="node-cpu"
              type="number"
              min={0}
              max={100}
              placeholder="20"
              value={cpu}
              onChange={(e) => setCpu(e.target.value)}
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
                  <Loader2 className="h-4 w-4 mr-1.5 animate-spin" /> Registering…
                </>
              ) : (
                <>Register</>
              )}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
