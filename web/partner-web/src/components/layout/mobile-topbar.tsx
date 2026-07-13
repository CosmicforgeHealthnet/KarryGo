"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { Bell, Menu, Search } from "lucide-react";

const NAV_TITLES: Record<string, string> = {
  "/dashboard": "Dashboard",
  "/deliveries": "Deliveries",
  "/webhook-logs": "Webhook Logs",
  "/api-documentation": "API Documentation",
  "/billing": "Billing",
  "/settings": "Settings",
};

const PARTNER = {
  name: "Kingsley Noori",
  avatarInitial: "K",
};

export function MobileTopBar({ onMenuClick }: { onMenuClick: () => void }) {
  const pathname = usePathname();
  const title =
    Object.entries(NAV_TITLES).find(([href]) => pathname.startsWith(href))?.[1] ??
    "Dashboard";

  return (
    <header className="sticky top-0 z-20 flex items-center justify-between gap-4 border-b border-ink-100 bg-surface/90 px-5 py-4 backdrop-blur-sm md:px-8">
      {/* Mobile only: logo + hamburger, since the sidebar is a hidden drawer here */}
      <div className="flex items-center gap-2 md:hidden">
        <div className="flex h-8 w-8 items-center justify-center rounded-lg bg-brand-600 text-sm font-bold text-white">
          C
        </div>
        <div className="leading-tight">
          <p className="text-xs font-bold text-brand-700">COSMICFORGE</p>
          <p className="text-[8px] tracking-wide text-brand-600">
            HEALTHNET LIMITED
          </p>
        </div>
      </div>
      <button
        type="button"
        onClick={onMenuClick}
        aria-label="Open menu"
        className="flex h-9 w-9 items-center justify-center rounded-lg text-ink-900 md:hidden"
      >
        <Menu className="h-5 w-5" />
      </button>

      {/* Tablet & desktop: sidebar already shows the logo, so the bar carries page context */}
      <h1 className="hidden text-lg font-bold text-ink-900 md:block">
        {title}
      </h1>

      <div className="hidden items-center gap-3 md:flex">
        <label className="hidden items-center gap-2 rounded-full border border-ink-100 bg-white px-3.5 py-2 text-sm text-ink-500 content:flex">
          <Search className="h-4 w-4 shrink-0" />
          <input
            type="text"
            placeholder="Search"
            className="w-40 bg-transparent text-ink-900 placeholder:text-ink-300 focus:outline-none lg:w-56"
          />
        </label>

        <button
          type="button"
          aria-label="Notifications"
          className="relative flex h-9 w-9 items-center justify-center rounded-full text-ink-700 hover:bg-ink-100"
        >
          <Bell className="h-[18px] w-[18px]" />
          <span className="absolute right-2 top-2 h-1.5 w-1.5 rounded-full bg-danger" />
        </button>

        <span className="h-6 w-px bg-ink-100" aria-hidden />

        <Link
          href="/settings"
          className="flex items-center gap-2 rounded-full py-1 pl-1 pr-3 hover:bg-ink-100"
        >
          <span className="flex h-8 w-8 items-center justify-center rounded-full bg-brand-400 text-sm font-semibold text-white">
            {PARTNER.avatarInitial}
          </span>
          <span className="hidden text-sm font-semibold text-ink-900 content:inline">
            {PARTNER.name}
          </span>
        </Link>
      </div>
    </header>
  );
}