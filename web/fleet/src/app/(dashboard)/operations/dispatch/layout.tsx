// src/app/(dashboard)/operations/dispatch/layout.tsx

"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { LayoutGrid, UserX, CalendarClock } from "lucide-react";

const DISPATCH_TABS = [
  { label: "Board", href: "/operations/dispatch", icon: LayoutGrid },
  { label: "Unassigned", href: "/operations/dispatch/unassigned", icon: UserX },
  { label: "Plans", href: "/operations/dispatch/plans", icon: CalendarClock },
];

export default function DispatchLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  const pathname = usePathname();

  return (
    <div className="space-y-5">
      {/* Tabs — flat, no glass, no shadow */}
      <div className="flex gap-1 border-b border-line">
        {DISPATCH_TABS.map((tab) => {
          const active = pathname === tab.href;
          const Icon = tab.icon;
          return (
            <Link
              key={tab.href}
              href={tab.href}
              className={`flex items-center gap-1.5 px-3.5 py-2.5 text-sm font-medium transition-colors ${
                active
                  ? "text-primary border-b-2 border-primary"
                  : "text-ink-600 hover:text-ink-950"
              }`}
            >
              <Icon className={`h-3.5 w-3.5 ${active ? "text-primary" : "text-ink-500"}`} />
              {tab.label}
            </Link>
          );
        })}
      </div>

      {children}
    </div>
  );
}