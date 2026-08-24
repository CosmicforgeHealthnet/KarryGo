// src/app/(dashboard)/wallet/layout.tsx

"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import {
  LayoutDashboard,
  Wallet,
  ArrowLeftRight,
  Users,
  CreditCard,
  RefreshCw,
  FileText,
  Settings,
} from "lucide-react";

const TABS = [
  { label: "Overview", segment: "", icon: LayoutDashboard },
  { label: "Transactions", segment: "transactions", icon: ArrowLeftRight },
  { label: "Allocations", segment: "allocations", icon: CreditCard },
  { label: "Providers", segment: "providers", icon: Users },
  { label: "Withdrawals", segment: "withdrawals", icon: Wallet },
  { label: "Reconciliation", segment: "reconciliation", icon: RefreshCw },
  { label: "Policy", segment: "withdrawal-policy", icon: Settings },
];

export default function WalletLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  const pathname = usePathname();
  const basePath = "/wallet";

  return (
    <div className="mx-auto max-w-[1400px] space-y-5 pb-8">
      {/* ===== TABS ===== */}
      <div className="flex gap-1 border-b border-line overflow-x-auto scrollbar-hide">
        {TABS.map((tab) => {
          const href = tab.segment ? `${basePath}/${tab.segment}` : basePath;
          const active = pathname === href;
          const Icon = tab.icon;

          return (
            <Link
              key={tab.label}
              href={href}
              className={`flex items-center gap-1.5 px-3.5 py-2.5 text-[12.5px] font-medium transition whitespace-nowrap border-b-2 ${
                active
                  ? "text-primary border-primary"
                  : "text-ink-600 hover:text-ink-950 border-transparent hover:border-ink-300"
              }`}
            >
              <Icon className="h-4 w-4" />
              {tab.label}
            </Link>
          );
        })}
      </div>

      {/* ===== CHILDREN ===== */}
      <div>{children}</div>
    </div>
  );
}