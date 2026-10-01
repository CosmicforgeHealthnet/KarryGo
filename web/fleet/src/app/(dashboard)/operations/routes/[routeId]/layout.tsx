// src/app/(dashboard)/operations/routes/[routeId]/layout.tsx

"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { usePathname, useParams } from "next/navigation";
import { Edit3, GitBranch, Rocket, TrendingUp, Layers } from "lucide-react";
import { routeService } from "@/lib/services/routeService";
import type { RoutePlan } from "@/lib/types/route";

const TABS = [
  { label: "Overview", segment: "", icon: Layers },
  { label: "Builder", segment: "builder", icon: Edit3 },
  { label: "Optimize", segment: "optimize", icon: TrendingUp },
  { label: "Publish", segment: "publish", icon: Rocket },
  { label: "Progress", segment: "progress", icon: GitBranch },
];

function statusStyle(status: string): { bg: string; text: string; dot: string } {
  switch ((status || "").toUpperCase()) {
    case "DRAFT":               return { bg: "bg-surface-2", text: "text-ink-600", dot: "bg-ink-400" };
    case "OPTIMIZATION_PENDING":return { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber" };
    case "OPTIMIZED":
    case "READY":
    case "IN_PROGRESS":         return { bg: "bg-primary-bg", text: "text-primary", dot: "bg-primary" };
    case "APPROVED":
    case "PARTIALLY_COMPLETED": return { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber" };
    case "PUBLISHED":
    case "COMPLETED":           return { bg: "bg-green-bg", text: "text-green", dot: "bg-green" };
    case "FAILED":
    case "CANCELLED":           return { bg: "bg-red-bg", text: "text-red", dot: "bg-red" };
    default:                    return { bg: "bg-surface-2", text: "text-ink-500", dot: "bg-ink-400" };
  }
}

function statusLabel(status: string): string {
  switch ((status || "").toUpperCase()) {
    case "DRAFT":                return "Draft";
    case "OPTIMIZATION_PENDING": return "Optimizing";
    case "OPTIMIZED":            return "Optimized";
    case "READY":                return "Ready";
    case "APPROVED":             return "Approved";
    case "PUBLISHED":            return "Published";
    case "IN_PROGRESS":          return "In progress";
    case "COMPLETED":            return "Completed";
    case "PARTIALLY_COMPLETED":  return "Partial";
    case "FAILED":               return "Failed";
    case "CANCELLED":            return "Cancelled";
    default:                     return status || "—";
  }
}

export default function RouteDetailLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  const pathname = usePathname();
  const params = useParams();
  const routeId = typeof params.routeId === "string" ? params.routeId : "";

  const [route, setRoute] = useState<RoutePlan | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!routeId) return;
    let cancelled = false;
    setLoading(true);
    setError(null);
    routeService
      .getRoute(routeId)
      .then((data) => {
        if (!cancelled) setRoute(data);
      })
      .catch((err) => {
        if (cancelled) return;
        const status = err?.response?.status;
        if (status === 404) setError("Route not found.");
        else
          setError(
            err?.response?.data?.error?.message ??
              err?.message ??
              "Failed to load route.",
          );
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [routeId]);

  const basePath = `/operations/routes/${routeId}`;
  const pill = route ? statusStyle(route.status) : null;

  return (
    <div className="space-y-5">
      <div>
        <div className="flex flex-wrap items-center gap-3">
          <h1>{route?.routeReference || routeId}</h1>
          {loading ? (
            <span className="h-6 w-24 rounded-full bg-surface-2" />
          ) : route ? (
            <span
              className={`flex items-center gap-1.5 rounded-full px-3 py-0.5 text-xs font-semibold whitespace-nowrap ${pill!.bg} ${pill!.text}`}
            >
              <span className={`h-1.5 w-1.5 rounded-full ${pill!.dot}`} />
              {statusLabel(route.status)}
            </span>
          ) : null}
        </div>
        {route && (
          <p className="mt-0.5 text-sm text-ink-500">
            {route.executionService} route · {route.branchId || "no branch"}
            {route.vehicleOrTruckReference
              ? ` · ${route.vehicleOrTruckReference}`
              : ""}
          </p>
        )}
        {!route && !loading && error && (
          <p className="mt-0.5 text-sm text-ink-500">{error}</p>
        )}
      </div>

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

      <div>{children}</div>
    </div>
  );
}