"use client";

import { useState } from "react";
import Link from "next/link";
import { usePathname } from "next/navigation";
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
} from "lucide-react";
import { cn } from "@/lib/utils";

const NAV_ITEMS = [
  { label: "Dashboard", href: "/dashboard", icon: Home },
  { label: "Deliveries", href: "/deliveries", icon: Car },
  { label: "Webhook Logs", href: "/webhook-logs", icon: Webhook },
  { label: "API Documentation", href: "/api-documentation", icon: FileText },
  { label: "Billing", href: "/billing", icon: CreditCard },
  { label: "Settings", href: "/settings", icon: Settings },
];

const PARTNER = {
  name: "Kingsley Noori",
  avatarInitial: "K",
};

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
  const [profileOpen, setProfileOpen] = useState(false);

  return (
    <>
      {mobileOpen && (
        <div
          className="fixed inset-0 z-30 bg-black/40 content:hidden"
          onClick={onMobileClose}
          aria-hidden
        />
      )}

      <aside
        className={cn(
          "fixed inset-y-0 left-0 z-40 flex h-full flex-col overflow-y-auto bg-brand-200 py-6 transition-[transform,width] duration-200",
          "w-[240px] px-4",
          mobileOpen ? "translate-x-0" : "-translate-x-full",
          "min-[768px]:max-[1013px]:translate-x-0 min-[768px]:max-[1013px]:w-[88px] min-[768px]:max-[1013px]:items-center min-[768px]:max-[1013px]:px-0",
          collapsed
            ? "content:translate-x-0 content:w-[88px] content:items-center content:px-0"
            : "content:translate-x-0 content:w-[264px] content:items-stretch content:px-5"
        )}
      >
        <button
          type="button"
          onClick={onMobileClose}
          aria-label="Close menu"
          className="absolute right-4 top-4 flex h-8 w-8 items-center justify-center rounded-lg text-ink-900 md:hidden"
        >
          <X className="h-5 w-5" />
        </button>

        <button
          type="button"
          onClick={onToggleCollapsed}
          aria-label={collapsed ? "Expand sidebar" : "Collapse sidebar"}
          className="absolute -right-3 top-8 hidden h-6 w-6 items-center justify-center rounded-full border border-ink-100 bg-white text-ink-700 shadow-sm content:flex"
        >
          {collapsed ? (
            <ChevronRight className="h-3.5 w-3.5" />
          ) : (
            <ChevronLeft className="h-3.5 w-3.5" />
          )}
        </button>

        {/* Width/gap collapse to the icon's own size when the wordmark is
            hidden, so there's no leftover gap space pushing it off-center. */}
        <div
          className={cn(
            "flex h-9 shrink-0 items-center gap-2.5",
            "min-[768px]:max-[1013px]:w-9 min-[768px]:max-[1013px]:justify-center min-[768px]:max-[1013px]:gap-0",
            collapsed
              ? "content:w-9 content:justify-center content:gap-0"
              : "content:w-auto content:justify-start"
          )}
        >
          <div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-brand-600 text-sm font-bold text-white">
            C
          </div>
          <div
            className={cn(
              "leading-tight",
              "min-[768px]:max-[1013px]:hidden",
              collapsed ? "content:hidden" : "content:block"
            )}
          >
            <p className="text-sm font-bold text-brand-700">COSMICFORGE</p>
            <p className="text-[10px] tracking-wide text-brand-600">
              HEALTHNET LIMITED
            </p>
          </div>
        </div>

        <nav className="mt-10 flex flex-1 flex-col gap-1">
          {NAV_ITEMS.map((item) => {
            const active = pathname.startsWith(item.href);
            const Icon = item.icon;
            const showLabel = !collapsed;
            return (
              <div key={item.href} className="relative">
                <Link
                  href={item.href}
                  onClick={onMobileClose}
                  className={cn(
                    "flex items-center gap-3 rounded-lg py-2.5 text-sm font-semibold text-ink-900 transition-colors hover:bg-white/40",
                    "px-3",
                    "min-[768px]:max-[1013px]:justify-center min-[768px]:max-[1013px]:px-0",
                    showLabel
                      ? "content:justify-start content:px-3"
                      : "content:justify-center content:px-0"
                  )}
                >
                  <Icon className="h-5 w-5 shrink-0" />
                  <span
                    className={cn(
                      "min-[768px]:max-[1013px]:hidden",
                      showLabel ? "content:inline" : "content:hidden"
                    )}
                  >
                    {item.label}
                  </span>
                </Link>
                {active && (
                  <span className="absolute right-0 top-1/2 h-6 w-1 -translate-y-1/2 rounded-l-full bg-brand-600 min-[768px]:max-[1013px]:right-[-1px] content:right-[-21px]" />
                )}
              </div>
            );
          })}
        </nav>

        <div className="relative mt-auto shrink-0">
          {profileOpen && (
            <div className="absolute bottom-14 left-0 w-56 rounded-xl border border-ink-100 bg-white p-4 shadow-lg">
              <div className="flex items-center gap-3">
                <div className="flex h-9 w-9 items-center justify-center rounded-full bg-brand-200 text-sm font-semibold text-brand-700">
                  {PARTNER.avatarInitial}
                </div>
                <div>
                  <p className="text-sm font-semibold text-ink-900">
                    {PARTNER.name}
                  </p>
                  <Link
                    href="/settings"
                    className="text-xs font-medium text-brand-600"
                  >
                    Profile
                  </Link>
                </div>
              </div>
              <button
                type="button"
                className="mt-4 flex items-center gap-2 text-sm font-semibold text-danger"
                onClick={() => setProfileOpen(false)}
              >
                <LogOut className="h-4 w-4" />
                Log Out
              </button>
            </div>
          )}

          <button
            type="button"
            onClick={() => setProfileOpen((o) => !o)}
            aria-label="Account menu"
            className="flex h-11 w-11 items-center justify-center overflow-hidden rounded-full border-2 border-white bg-brand-400 text-sm font-semibold text-white"
          >
            {PARTNER.avatarInitial}
          </button>
        </div>
      </aside>
    </>
  );
}