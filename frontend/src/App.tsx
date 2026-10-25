import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { BrowserRouter, Route, Routes } from "react-router-dom";
import { Toaster } from "@/components/ui/sonner";
import { TooltipProvider } from "@/components/ui/tooltip";
import { AppShell } from "@/components/console/AppShell";
import { ApiError } from "@/lib/api/client";
import About from "./pages/About";
import Events from "./pages/Events";
import Logs from "./pages/Logs";
import NodeDetail from "./pages/NodeDetail";
import Nodes from "./pages/Nodes";
import NotFound from "./pages/NotFound";
import Overview from "./pages/Overview";
import PodDetail from "./pages/PodDetail";
import Pods from "./pages/Pods";
import ReplicaSets from "./pages/ReplicaSets";

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 2_000,
      refetchOnWindowFocus: true,
      // retry hiccups once; a 4xx won't magically fix itself, so don't bother
      retry: (count, err) => count < 1 && !(err instanceof ApiError && err.status >= 400 && err.status < 500),
    },
  },
});

const App = () => (
  <QueryClientProvider client={queryClient}>
    <TooltipProvider>
      <Toaster theme="light" position="bottom-right" />
      <BrowserRouter>
        <Routes>
          <Route element={<AppShell />}>
            <Route path="/" element={<Overview />} />
            <Route path="/pods" element={<Pods />} />
            <Route path="/pods/:name" element={<PodDetail />} />
            <Route path="/replicasets" element={<ReplicaSets />} />
            <Route path="/nodes" element={<Nodes />} />
            <Route path="/nodes/:name" element={<NodeDetail />} />
            <Route path="/events" element={<Events />} />
            <Route path="/logs" element={<Logs />} />
            <Route path="/about" element={<About />} />
            <Route path="*" element={<NotFound />} />
          </Route>
        </Routes>
      </BrowserRouter>
    </TooltipProvider>
  </QueryClientProvider>
);

export default App;
