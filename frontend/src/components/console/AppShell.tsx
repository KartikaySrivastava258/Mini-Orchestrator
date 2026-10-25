import { Activity, Boxes, Info, Layers, LayoutGrid, ScrollText, Server } from "lucide-react";
import { NavLink, Outlet, useLocation } from "react-router-dom";
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarProvider,
  SidebarTrigger,
} from "@/components/ui/sidebar";
import { API_BASE_URL } from "@/lib/api/client";
import { useReadiness } from "@/hooks/queries";
import { cn } from "@/lib/utils";

const groups = [
  { label: null, items: [{ title: "Overview", url: "/", icon: LayoutGrid }] },
  {
    label: "Workloads",
    items: [
      { title: "Pods", url: "/pods", icon: Boxes },
      { title: "ReplicaSets", url: "/replicasets", icon: Layers },
    ],
  },
  {
    label: "Cluster",
    items: [
      { title: "Nodes", url: "/nodes", icon: Server },
      { title: "Events", url: "/events", icon: Activity },
    ],
  },
  { label: "Observability", items: [{ title: "Logs", url: "/logs", icon: ScrollText }] },
];

function isActive(pathname: string, url: string) {
  return url === "/" ? pathname === "/" : pathname === url || pathname.startsWith(url + "/");
}

function Brand() {
  return (
    <div className="flex items-center gap-2 px-2 py-1.5">
      <svg aria-hidden viewBox="0 0 24 24" className="h-6 w-6 shrink-0 text-primary" fill="none" stroke="currentColor" strokeWidth="2">
        <rect x="3" y="3" width="7" height="7" rx="1" />
        <rect x="14" y="3" width="7" height="7" rx="1" />
        <rect x="3" y="14" width="7" height="7" rx="1" />
        <path d="M17.5 14v7M14 17.5h7" />
      </svg>
      <span className="truncate text-sm font-semibold group-data-[collapsible=icon]:hidden">Mini Orchestrator</span>
    </div>
  );
}

function AppSidebar() {
  const { pathname } = useLocation();
  return (
    <Sidebar collapsible="icon">
      <SidebarHeader className="border-b">
        <Brand />
      </SidebarHeader>
      <SidebarContent>
        {groups.map((g, i) => (
          <SidebarGroup key={g.label ?? i}>
            {g.label && <SidebarGroupLabel>{g.label}</SidebarGroupLabel>}
            <SidebarGroupContent>
              <SidebarMenu>
                {g.items.map((item) => (
                  <SidebarMenuItem key={item.url}>
                    <SidebarMenuButton asChild isActive={isActive(pathname, item.url)} tooltip={item.title}>
                      <NavLink to={item.url} end={item.url === "/"}>
                        <item.icon aria-hidden className="h-4 w-4" />
                        <span>{item.title}</span>
                      </NavLink>
                    </SidebarMenuButton>
                  </SidebarMenuItem>
                ))}
              </SidebarMenu>
            </SidebarGroupContent>
          </SidebarGroup>
        ))}
      </SidebarContent>
      <SidebarFooter className="border-t">
        <SidebarMenu>
          <SidebarMenuItem>
            <SidebarMenuButton asChild isActive={isActive(pathname, "/about")} tooltip="About">
              <NavLink to="/about">
                <Info aria-hidden className="h-4 w-4" />
                <span>About</span>
              </NavLink>
            </SidebarMenuButton>
          </SidebarMenuItem>
        </SidebarMenu>
      </SidebarFooter>
    </Sidebar>
  );
}

/** Mirrors /readyz (API up + etcd reachable). It does NOT mean the whole cluster is healthy. */
function ReadinessIndicator() {
  const { data, isPending, isError, dataUpdatedAt } = useReadiness();
  let tone = "bg-muted-foreground";
  let label = "Checking API";
  let detail = `API ${API_BASE_URL}`;
  if (isError) {
    tone = "bg-destructive";
    label = "API unreachable";
  } else if (data?.status === "ready") {
    tone = "bg-success";
    label = "API ready";
    detail = "Control plane is up and etcd answers";
  } else if (data) {
    tone = "bg-destructive";
    label = "etcd unavailable";
    detail = "The API is running but cannot reach etcd";
  }
  return (
    <div className="flex items-center gap-3 text-xs" role="status" aria-live="polite">
      <span className="flex items-center gap-1.5" title={detail}>
        <span aria-hidden className={cn("h-2 w-2 rounded-full", tone)} />
        <span className="font-medium">{label}</span>
      </span>
      {!isPending && dataUpdatedAt > 0 && (
        <span className="hidden text-muted-foreground sm:inline">Checked {new Date(dataUpdatedAt).toLocaleTimeString()}</span>
      )}
    </div>
  );
}

export function AppShell() {
  return (
    <SidebarProvider>
      <a href="#main" className="sr-only focus:not-sr-only focus:absolute focus:left-2 focus:top-2 focus:z-50 focus:rounded focus:bg-card focus:px-3 focus:py-2">
        Skip to content
      </a>
      <AppSidebar />
      <div className="flex min-w-0 flex-1 flex-col">
        <header className="sticky top-0 z-20 flex h-12 items-center justify-between gap-3 border-b bg-card/95 px-3 backdrop-blur supports-[backdrop-filter]:bg-card/80">
          <SidebarTrigger aria-label="Toggle navigation" />
          <ReadinessIndicator />
        </header>
        <main id="main" className="mx-auto w-full max-w-[1400px] flex-1 p-4 md:p-6">
          <Outlet />
        </main>
      </div>
    </SidebarProvider>
  );
}
