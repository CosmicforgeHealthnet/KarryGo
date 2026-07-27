"use client";

import { useState, useEffect } from "react";
import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import {
  Home,
  Car,
  Webhook,
  FileText,
  CreditCard,
  Settings,
  ChevronLeft,
  ChevronRight,
  LogOut,
  X,
  User,
  ChevronDown,
  LayoutDashboard,
  Truck,
  HelpCircle,
} from "lucide-react";
import { cn } from "@/lib/utils";
import { authService } from "@/services/auth-service";
import { useAuthStore } from "@/store/auth-store";

const NAV_ITEMS = [
  { label: "Dashboard", href: "/dashboard", icon: LayoutDashboard },
  { label: "Deliveries", href: "/deliveries", icon: Truck },
  { label: "Webhook Logs", href: "/webhook-logs", icon: Webhook },
  { label: "API Documentation", href: "/api-documentation", icon: FileText },
  { label: "Billing", href: "/billing", icon: CreditCard },
  { label: "Settings", href: "/settings", icon: Settings },
];

interface SidebarProps {
  mobileOpen: boolean;
  onMobileClose: () => void;
  collapsed: boolean;
  onToggleCollapsed: () => void;
}

export function Sidebar({
  mobileOpen,
  onMobileClose,
  collapsed,
  onToggleCollapsed,
}: SidebarProps) {
  const pathname = usePathname();
  const router = useRouter();
  const [profileOpen, setProfileOpen] = useState(false);
  const [loggingOut, setLoggingOut] = useState(false);
  const [isMediumScreen, setIsMediumScreen] = useState(false);

  const token = useAuthStore((s) => s.token);
  const account = useAuthStore((s) => s.account);
  const clearSession = useAuthStore((s) => s.clearSession);

  const partnerName = account?.business_name || "Partner";
  const avatarInitial = partnerName.charAt(0).toUpperCase();

  // Auto-collapse on medium screens (768px - 1013px)
  useEffect(() => {
    const handleResize = () => {
      const width = window.innerWidth;
      setIsMediumScreen(width >= 768 && width <= 1013);
    };

    handleResize();
    window.addEventListener("resize", handleResize);
    return () => window.removeEventListener("resize", handleResize);
  }, []);

  // Auto-collapse when on medium screen
  useEffect(() => {
    if (isMediumScreen && !collapsed) {
      onToggleCollapsed();
    }
  }, [isMediumScreen]);

  async function handleLogout() {
    setLoggingOut(true);
    try {
      if (token) {
        await authService.logout(token);
      }
    } catch {
      // Best-effort logout
    } finally {
      clearSession();
      setLoggingOut(false);
      setProfileOpen(false);
      router.push("/login");
    }
  }

  const isActive = (href: string) => {
    if (href === "/dashboard") {
      return pathname === href;
    }
    return pathname.startsWith(href);
  };

  return (
    <>
      {/* Mobile overlay */}
      {mobileOpen && (
        <div
          className="fixed inset-0 z-30 bg-black/50 backdrop-blur-sm md:hidden"
          onClick={onMobileClose}
          aria-hidden
        />
      )}

      <aside
        className={cn(
          "fixed inset-y-0 left-0 z-40 flex h-full flex-col bg-white transition-all duration-300 ease-in-out",
          "border-r border-gray-100 shadow-sm",
          // Mobile
          "w-[280px]",
          mobileOpen ? "translate-x-0" : "-translate-x-full",
          // Desktop
          "md:translate-x-0",
          collapsed ? "md:w-[72px]" : "md:w-[260px]"
        )}
      >
        {/* Toggle collapse button - desktop only */}
        <button
          type="button"
          onClick={onToggleCollapsed}
          aria-label={collapsed ? "Expand sidebar" : "Collapse sidebar"}
          className={cn(
            "absolute -right-3 top-8 z-10 hidden h-6 w-6 items-center justify-center rounded-full border border-gray-200 bg-white text-gray-600 shadow-md hover:bg-gray-50 hover:text-gray-900 transition-all duration-200",
            "md:flex",
            isMediumScreen && "hidden"
          )}
        >
          {collapsed ? (
            <ChevronRight className="h-3.5 w-3.5" />
          ) : (
            <ChevronLeft className="h-3.5 w-3.5" />
          )}
        </button>

        {/* Mobile close button */}
        <button
          type="button"
          onClick={onMobileClose}
          aria-label="Close menu"
          className="absolute right-4 top-4 flex h-8 w-8 items-center justify-center rounded-lg text-gray-500 hover:bg-gray-100 md:hidden"
        >
          <X className="h-5 w-5" />
        </button>

        {/* Logo - with proper spacing */}
        <div
          className={cn(
            "flex h-20 shrink-0 items-center gap-3 border-b border-gray-100/50 transition-all duration-300",
            collapsed ? "md:justify-center md:px-2" : "md:px-6",
            "px-4"
          )}
        >
          <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-xl bg-gradient-to-br from-brand-600 to-brand-700 text-base font-bold text-white shadow-lg shadow-brand-200/50">
            C
          </div>
          <div
            className={cn(
              "transition-all duration-300 overflow-hidden",
              collapsed ? "md:w-0 md:opacity-0" : "md:w-auto md:opacity-100"
            )}
          >
            <p className="text-sm font-bold text-gray-900 leading-tight tracking-tight">
              CosmicForge
            </p>
            <p className="text-[10px] font-medium text-gray-500 tracking-[0.15em] uppercase">
              Logistics
            </p>
          </div>
        </div>

        {/* Navigation - with consistent spacing */}
        <nav className="flex-1 overflow-y-auto px-3 py-4">
          <div className="space-y-1">
            {NAV_ITEMS.map((item) => {
              const active = isActive(item.href);
              const Icon = item.icon;
              return (
                <Link
                  key={item.href}
                  href={item.href}
                  onClick={onMobileClose}
                  className={cn(
                    "group relative flex items-center gap-3 rounded-xl px-3 py-2.5 text-sm font-medium transition-all duration-200",
                    active
                      ? "bg-brand-50 text-brand-700 shadow-sm"
                      : "text-gray-600 hover:bg-gray-50 hover:text-gray-900",
                    collapsed
                      ? "md:justify-center md:px-2"
                      : "md:px-4"
                  )}
                >
                  <Icon
                    className={cn(
                      "h-5 w-5 shrink-0 transition-colors",
                      active ? "text-brand-600" : "text-gray-400 group-hover:text-gray-600"
                    )}
                  />
                  <span
                    className={cn(
                      "transition-all duration-200",
                      collapsed ? "md:hidden" : "md:inline"
                    )}
                  >
                    {item.label}
                  </span>
                  {active && (
                    <span className="absolute left-0 top-1/2 h-6 w-1 -translate-y-1/2 rounded-r-full bg-brand-600" />
                  )}
                </Link>
              );
            })}
          </div>
        </nav>

        {/* Bottom section */}
        <div className="shrink-0 border-t border-gray-100 p-3">
          {/* Help */}
          <button
            type="button"
            className={cn(
              "flex w-full items-center gap-3 rounded-xl px-3 py-2.5 text-sm font-medium text-gray-500 transition-all hover:bg-gray-50 hover:text-gray-700",
              collapsed ? "md:justify-center md:px-2" : "md:px-4"
            )}
          >
            <HelpCircle className="h-5 w-5 shrink-0" />
            <span className={cn(collapsed ? "md:hidden" : "md:inline")}>
              Help & Support
            </span>
          </button>

          {/* Profile */}
          <div className="relative mt-1">
            <button
              type="button"
              onClick={() => setProfileOpen((o) => !o)}
              className={cn(
                "flex w-full items-center gap-3 rounded-xl px-3 py-2.5 transition-all hover:bg-gray-50",
                collapsed ? "md:justify-center md:px-2" : "md:px-4"
              )}
            >
              <div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-full bg-gradient-to-br from-brand-500 to-brand-600 text-sm font-semibold text-white shadow-md shadow-brand-200/50">
                {avatarInitial}
              </div>
              <div
                className={cn(
                  "flex-1 text-left transition-all duration-200",
                  collapsed ? "md:hidden" : "md:block"
                )}
              >
                <p className="text-sm font-semibold text-gray-900 truncate">
                  {partnerName}
                </p>
                <p className="text-xs text-gray-500">Partner Account</p>
              </div>
              <ChevronDown
                className={cn(
                  "h-4 w-4 text-gray-400 transition-transform duration-200",
                  profileOpen && "rotate-180",
                  collapsed ? "md:hidden" : "md:block"
                )}
              />
            </button>

            {/* Dropdown */}
            {profileOpen && (
              <div className="absolute bottom-full left-0 mb-2 w-full rounded-xl border border-gray-100 bg-white p-2 shadow-lg shadow-gray-200/50">
                <Link
                  href="/settings"
                  onClick={() => setProfileOpen(false)}
                  className="flex items-center gap-3 rounded-lg px-3 py-2.5 text-sm text-gray-700 transition-colors hover:bg-gray-50"
                >
                  <User className="h-4 w-4" />
                  My Profile
                </Link>
                <button
                  type="button"
                  disabled={loggingOut}
                  className="flex w-full items-center gap-3 rounded-lg px-3 py-2.5 text-sm text-red-600 transition-colors hover:bg-red-50 disabled:opacity-50"
                  onClick={handleLogout}
                >
                  <LogOut className="h-4 w-4" />
                  {loggingOut ? "Logging out..." : "Log Out"}
                </button>
              </div>
            )}
          </div>
        </div>
      </aside>
    </>
  );
}