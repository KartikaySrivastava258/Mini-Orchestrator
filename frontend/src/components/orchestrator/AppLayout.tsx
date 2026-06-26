import { Outlet } from "react-router-dom";
import { SidebarProvider, SidebarTrigger } from "@/components/ui/sidebar";
import { AppSidebar } from "./AppSidebar";
import { ConnectionPill } from "./ConnectionPill";

export function AppLayout() {
  return (
    <SidebarProvider>
      <div className="min-h-screen flex w-full bg-background">
        <AppSidebar />
        <div className="flex-1 flex flex-col min-w-0">
          <header className="sticky top-0 z-30 flex h-14 items-center justify-between gap-3 border-b border-border/60 bg-background/70 backdrop-blur px-4">
            <div className="flex items-center gap-2">
              <SidebarTrigger />
              <div className="hidden md:flex items-center gap-2 text-xs text-muted-foreground">
                <span className="font-mono">helmsman</span>
                <span>/</span>
                <span className="text-foreground">cluster</span>
              </div>
            </div>
            <ConnectionPill />
          </header>
          <main className="flex-1 p-4 md:p-6 lg:p-8 max-w-[1600px] w-full mx-auto">
            <Outlet />
          </main>
        </div>
      </div>
    </SidebarProvider>
  );
}
