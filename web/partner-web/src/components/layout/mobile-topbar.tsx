"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { Bell, Menu, Search, X } from "lucide-react";
import { useAuthStore } from "@/store/auth-store";
import { useState, useEffect } from "react";
import { cn } from "@/lib/utils";

const NAV_TITLES: Record<string, string> = {
  "/dashboard": "Dashboard",
  "/deliveries": "Deliveries",
  "/webhook-logs": "Webhook Logs",
  "/api-documentation": "API Documentation",
  "/billing": "Billing",
  "/settings": "Settings",
};

interface MobileTopBarProps {
  onMenuClick: () => void;
  collapsed: boolean;
}

export function MobileTopBar({ onMenuClick, collapsed }: MobileTopBarProps) {
  const pathname = usePathname();
  const account = useAuthStore((s) => s.account);
  const [searchOpen, setSearchOpen] = useState(false);
  const [scrolled, setScrolled] = useState(false);

  useEffect(() => {
    const handleScroll = () => {
      setScrolled(window.scrollY > 10);
    };
    window.addEventListener("scroll", handleScroll);
    return () => window.removeEventListener("scroll", handleScroll);
  }, []);

  const title =
    Object.entries(NAV_TITLES).find(([href]) => pathname.startsWith(href))?.[1] ??
    "Dashboard";

  const partnerName = account?.business_name || "Partner";
  const avatarInitial = partnerName.charAt(0).toUpperCase();

  return (
    <>
      <header
        className={cn(
          "fixed top-0 right-0 z-20 border-b border-gray-100/50 bg-white/80 backdrop-blur-xl transition-all duration-300",
          "left-0",
          "min-[768px]:max-[1013px]:left-[72px]",
          collapsed ? "md:left-[72px]" : "md:left-[260px]",
          scrolled && "shadow-sm"
        )}
      >
        <div className="flex h-16 items-center justify-between px-4 md:px-6">
          <div className="flex flex-1 items-center gap-3">
            <button
              type="button"
              onClick={onMenuClick}
              aria-label="Open menu"
              className="flex h-9 w-9 items-center justify-center rounded-lg text-gray-600 transition-colors hover:bg-gray-100 hover:text-gray-900 md:hidden"
            >
              <Menu className="h-5 w-5" />
            </button>

            <div className="flex items-center gap-2.5 md:hidden">
              <div className="flex h-8 w-8 items-center justify-center rounded-lg bg-gradient-to-br from-brand-600 to-brand-700 text-xs font-bold text-white shadow-sm shadow-brand-200/50">
                C
              </div>
              <div className="leading-tight">
                <p className="text-xs font-bold text-gray-900">CosmicForge</p>
                <p className="text-[8px] font-medium text-gray-500 tracking-[0.1em]">
                  LOGISTICS
                </p>
              </div>
            </div>

            <h1 className="hidden text-lg font-bold text-gray-900 md:block">
              {title}
            </h1>
          </div>

          <div className="flex items-center gap-1 md:gap-2">
            <div className="hidden items-center gap-2 rounded-full border border-gray-200 bg-gray-50 px-3.5 py-2 text-sm text-gray-500 transition-all duration-200 focus-within:border-brand-300 focus-within:bg-white focus-within:ring-2 focus-within:ring-brand-100 lg:flex">
              <Search className="h-4 w-4 shrink-0 text-gray-400" />
              <input
                type="text"
                placeholder="Search..."
                className="w-40 bg-transparent text-gray-900 placeholder:text-gray-400 focus:outline-none lg:w-56"
              />
              <kbd className="hidden rounded bg-gray-200 px-1.5 py-0.5 text-xs font-medium text-gray-500 lg:block">
                ⌘K
              </kbd>
            </div>

            <button
              type="button"
              onClick={() => setSearchOpen(!searchOpen)}
              aria-label="Search"
              className="flex h-9 w-9 items-center justify-center rounded-lg text-gray-600 transition-colors hover:bg-gray-100 hover:text-gray-900 lg:hidden"
            >
              {searchOpen ? <X className="h-4 w-4" /> : <Search className="h-4 w-4" />}
            </button>

            <button
              type="button"
              aria-label="Notifications"
              className="relative flex h-9 w-9 items-center justify-center rounded-lg text-gray-600 transition-colors hover:bg-gray-100 hover:text-gray-900"
            >
              <Bell className="h-[18px] w-[18px]" />
              <span className="absolute right-2 top-2 h-2 w-2 rounded-full bg-red-500 ring-2 ring-white" />
            </button>

            <span className="hidden h-6 w-px bg-gray-200 md:block" />

            <Link
              href="/settings"
              className="hidden items-center gap-2.5 rounded-full py-1 pr-3 pl-1 transition-colors hover:bg-gray-100 md:flex"
            >
              <span className="flex h-8 w-8 items-center justify-center rounded-full bg-gradient-to-br from-brand-500 to-brand-600 text-sm font-semibold text-white shadow-sm shadow-brand-200/50">
                {avatarInitial}
              </span>
              <span className="hidden text-sm font-medium text-gray-700 lg:inline">
                {partnerName}
              </span>
            </Link>

            <Link
              href="/settings"
              className="flex h-9 w-9 items-center justify-center rounded-full bg-gradient-to-br from-brand-500 to-brand-600 text-sm font-semibold text-white shadow-sm shadow-brand-200/50 md:hidden"
            >
              {avatarInitial}
            </Link>
          </div>
        </div>

        <div
          className={cn(
            "overflow-hidden transition-all duration-300 ease-in-out",
            searchOpen ? "max-h-20 border-t border-gray-100/50" : "max-h-0"
          )}
        >
          <div className="p-3">
            <div className="flex items-center gap-2 rounded-full border border-gray-200 bg-gray-50 px-3.5 py-2 transition-all focus-within:border-brand-300 focus-within:bg-white focus-within:ring-2 focus-within:ring-brand-100">
              <Search className="h-4 w-4 shrink-0 text-gray-400" />
              <input
                type="text"
                placeholder="Search..."
                className="w-full bg-transparent text-gray-900 placeholder:text-gray-400 focus:outline-none"
                autoFocus
              />
            </div>
          </div>
        </div>
      </header>

      <div className="h-16" />
    </>
  );
}