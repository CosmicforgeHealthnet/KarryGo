// src/app/(dashboard)/warehouse/[warehouseId]/layout.tsx

"use client";

import Link from "next/link";
import { usePathname, useParams } from "next/navigation";
import {
  LayoutDashboard,
  Layers,
  Package,
  ArrowDownRight,
  ArrowUpRight,
  Truck,
  Dock,
  Warehouse as WarehouseIcon,
  ArrowLeft,
} from "lucide-react";

const TABS = [
  { label: "Overview", segment: "", icon: LayoutDashboard },
  { label: "Storage", segment: "storage", icon: Layers },
  { label: "Inventory", segment: "inventory", icon: Package },
  { label: "Inbound", segment: "inbound", icon: ArrowDownRight },
  { label: "Outbound", segment: "outbound", icon: ArrowUpRight },
  { label: "Docks", segment: "docks", icon: Truck },
  { label: "Yard", segment: "yard", icon: Dock },
  { label: "Transfers", segment: "transfers", icon: WarehouseIcon },
  { label: "Returns", segment: "returns", icon: ArrowLeft },
];

export default function WarehouseDetailLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  const pathname = usePathname();
  const params = useParams();
  const warehouseId = params.warehouseId as string;

  const basePath = `/warehouse/${warehouseId}`;

  return (
    <div className="mx-auto max-w-[1400px] space-y-5 pb-8">
      {/* ===== TABS ONLY ===== */}
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