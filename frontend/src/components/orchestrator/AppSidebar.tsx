import { LayoutDashboard, Boxes, ScrollText, Server, Hexagon } from "lucide-react";
import { NavLink, useLocation } from "react-router-dom";
import {
  Sidebar,
  SidebarContent,
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarHeader,
  SidebarFooter,
  useSidebar,
} from "@/components/ui/sidebar";
import { cn } from "@/lib/utils";
import { ORCHESTRATOR_BASE_URL } from "@/lib/orchestrator-api";

const items = [
  { title: "Dashboard", url: "/", icon: LayoutDashboard },
  { title: "Pods", url: "/pods", icon: Boxes },
  { title: "Nodes", url: "/nodes", icon: Server },
  { title: "Logs", url: "/logs", icon: ScrollText },
];

export function AppSidebar() {
  const { state } = useSidebar();
  const collapsed = state === "collapsed";
  const location = useLocation();

  return (
    <Sidebar collapsible="icon" className="border-r border-sidebar-border">
      <SidebarHeader className="border-b border-sidebar-border">
        <div className="flex items-center gap-2.5 px-2 py-2">
          <div className="relative grid h-9 w-9 place-items-center rounded-xl bg-gradient-primary shadow-glow">
            <Hexagon className="h-5 w-5 text-primary-foreground" strokeWidth={2.5} />
          </div>
          {!collapsed && (
            <div className="flex flex-col">
              <span className="text-sm font-bold tracking-tight">Obsidian</span>
              <span className="text-[10px] text-muted-foreground font-mono">
                kubernetes-companion
              </span>
            </div>
          )}
        </div>
      </SidebarHeader>

      <SidebarContent>
        <SidebarGroup>
          <SidebarGroupLabel>Cluster</SidebarGroupLabel>
          <SidebarGroupContent>
            <SidebarMenu>
              {items.map((item) => {
                const active =
                  item.url === "/"
                    ? location.pathname === "/"
                    : location.pathname.startsWith(item.url);
                return (
                  <SidebarMenuItem key={item.title}>
                    <SidebarMenuButton asChild isActive={active}>
                      <NavLink
                        to={item.url}
                        end={item.url === "/"}
                        className={cn(
                          "transition-colors",
                          active &&
                            "bg-sidebar-accent text-sidebar-accent-foreground",
                        )}
                      >
                        <item.icon className="h-4 w-4" />
                        {!collapsed && <span>{item.title}</span>}
                      </NavLink>
                    </SidebarMenuButton>
                  </SidebarMenuItem>
                );
              })}
            </SidebarMenu>
          </SidebarGroupContent>
        </SidebarGroup>
      </SidebarContent>

      <SidebarFooter className="border-t border-sidebar-border">
        {!collapsed && (
          <div className="px-2 py-2">
            <p className="text-[10px] uppercase tracking-wider text-muted-foreground">
              API endpoint
            </p>
            <p className="font-mono text-[11px] text-foreground/80 truncate">
              {ORCHESTRATOR_BASE_URL}
            </p>
          </div>
        )}
      </SidebarFooter>
    </Sidebar>
  );
}
