"use client";

import { useState } from "react";
import { Sidebar } from "@/components/layout/sidebar";
import { MobileTopBar } from "@/components/layout/mobile-topbar";
import { cn } from "@/lib/utils";

export default function DashboardLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  const [mobileOpen, setMobileOpen] = useState(false);
  const [collapsed, setCollapsed] = useState(false);

  return (
    <div className="min-h-screen bg-surface">
      <Sidebar
        mobileOpen={mobileOpen}
        onMobileClose={() => setMobileOpen(false)}
        collapsed={collapsed}
        onToggleCollapsed={() => setCollapsed((c) => !c)}
      />

      <div
        className={cn(
          "flex min-h-screen flex-col transition-[margin-left] duration-200",
          "min-[768px]:max-[1013px]:ml-[88px]",
          collapsed ? "content:ml-[88px]" : "content:ml-[264px]"
        )}
      >
        <MobileTopBar onMenuClick={() => setMobileOpen(true)} />
        <main className="flex-1 p-6 md:p-8">{children}</main>
      </div>
    </div>
  );
}