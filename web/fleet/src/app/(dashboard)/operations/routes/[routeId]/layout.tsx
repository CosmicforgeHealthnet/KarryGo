// src/app/(dashboard)/operations/routes/[routeId]/layout.tsx

"use client";

import Link from "next/link";
import { usePathname, useParams } from "next/navigation";
import {
  Edit3,
  GitBranch,
  Rocket,
  TrendingUp,
  Layers,
} from "lucide-react";
import { ROUTE_DETAIL } from "@/lib/mock-routes-data";

const TABS = [
  { label: "Overview", segment: "", icon: Layers },
  { label: "Builder", segment: "builder", icon: Edit3 },
  { label: "Optimize", segment: "optimize", icon: TrendingUp },
  { label: "Publish", segment: "publish", icon: Rocket },
  { label: "Progress", segment: "progress", icon: GitBranch },
];

const STATUS_STYLES: Record<string, { bg: string; text: string; dot: string }> = {
  Draft: { bg: "bg-surface-2", text: "text-ink-600", dot: "bg-ink-400" },
  Locked: { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber" },
  Published: { bg: "bg-green-bg", text: "text-green", dot: "bg-green" },
  Archived: { bg: "bg-surface-2", text: "text-ink-500", dot: "bg-ink-400" },
};

export default function RouteDetailLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  const pathname = usePathname();
  const params = useParams();
  const routeId = params.routeId as string;
  const route = ROUTE_DETAIL;

  const basePath = `/operations/routes/${routeId}`;
  const statusStyle = STATUS_STYLES[route.status] || STATUS_STYLES.Draft;

  const assetDisplay = typeof route.asset === 'string' 
    ? route.asset 
    : route.asset?.name || route.asset?.id || 'Unknown';

  return (
    <div className="space-y-5">
      {/* ── Header ── */}
      <div>
        <div className="flex items-center gap-3 flex-wrap">
          <h1>{routeId}</h1>
          <span className={`flex items-center gap-1.5 rounded-full px-3 py-0.5 text-xs font-semibold whitespace-nowrap ${statusStyle.bg} ${statusStyle.text}`}>
            <span className={`h-1.5 w-1.5 rounded-full ${statusStyle.dot}`} />
            {route.status}
          </span>
        </div>
        <p className="mt-0.5 text-sm text-ink-500">
          {route.service} route · {route.branch} branch · {route.stopCount} stops · {assetDisplay}
        </p>
      </div>

      {/* ── Tabs with horizontal scroll ── */}
      <div className="overflow-x-auto scrollbar-hide border-b border-line">
        <div className="flex gap-1 min-w-max">
          {TABS.map((tab) => {
            const href = tab.segment ? `${basePath}/${tab.segment}` : basePath;
            const active = pathname === href;
            const Icon = tab.icon;

            return (
              <Link
                key={tab.label}
                href={href}
                className={`flex items-center gap-1.5 px-3.5 py-2.5 text-sm font-medium whitespace-nowrap transition-colors ${
                  active
                    ? "text-primary border-b-2 border-primary"
                    : "text-ink-600 hover:text-ink-950"
                }`}
              >
                <Icon className="h-4 w-4 flex-shrink-0" />
                {tab.label}
              </Link>
            );
          })}
        </div>
      </div>

      {/* ── Children ── */}
      <div>{children}</div>
    </div>
  );
}