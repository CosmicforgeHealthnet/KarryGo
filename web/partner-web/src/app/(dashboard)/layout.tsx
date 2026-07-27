"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { Sidebar } from "@/components/layout/sidebar";
import { MobileTopBar } from "@/components/layout/mobile-topbar";
import { cn } from "@/lib/utils";
import { useAuthStore } from "@/store/auth-store";

export default function DashboardLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  const router = useRouter();
  const token = useAuthStore((s) => s.token);
  const hasHydrated = useAuthStore((s) => s._hasHydrated);
  const [mobileOpen, setMobileOpen] = useState(false);
  const [collapsed, setCollapsed] = useState(false);
  const [isTablet, setIsTablet] = useState(false);

  useEffect(() => {
    if (!hasHydrated) return;
    if (!token) {
      router.replace("/login");
    }
  }, [token, router, hasHydrated]);

  // Track tablet breakpoint only, independent of collapsed state
  useEffect(() => {
    const handleResize = () => {
      const width = window.innerWidth;
      setIsTablet(width >= 768 && width <= 1013);
    };

    handleResize();
    window.addEventListener("resize", handleResize);
    return () => window.removeEventListener("resize", handleResize);
  }, []);

  // Force-collapse only when entering tablet range, don't fight manual toggle otherwise
  useEffect(() => {
    if (isTablet) {
      setCollapsed(true);
    }
  }, [isTablet]);

  if (!hasHydrated) {
    return null;
  }

  if (!token) {
    return null;
  }

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
          "flex min-h-screen flex-col transition-all duration-300 ease-in-out",
          "ml-0",
          "min-[768px]:max-[1013px]:ml-[72px]",
          collapsed ? "md:ml-[72px]" : "md:ml-[260px]"
        )}
      >
        <MobileTopBar onMenuClick={() => setMobileOpen(true)} collapsed={collapsed} />
        <main className="flex-1 p-6 md:p-8">{children}</main>
      </div>
    </div>
  );
}