// src/app/(dashboard)/operations/routes/page.tsx

"use client";

import { useEffect, useMemo, useState } from "react";
import Link from "next/link";
import {
  Search,
  SlidersHorizontal,
  X,
  ArrowRight,
  MapPin,
  Users,
  Truck,
  Clock,
} from "lucide-react";
import EmptyState from "@/components/EmptyState";
import { routeService } from "@/lib/services/routeService";
import type { RoutePlan } from "@/lib/types/route";

function statusStyle(status: string): { bg: string; text: string; dot: string } {
  switch ((status || "").toUpperCase()) {
    case "DRAFT":               return { bg: "bg-surface-2", text: "text-ink-600", dot: "bg-ink-400" };
    case "OPTIMIZATION_PENDING":return { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber" };
    case "OPTIMIZED":           return { bg: "bg-primary-bg", text: "text-primary", dot: "bg-primary" };
    case "READY":               return { bg: "bg-primary-bg", text: "text-primary", dot: "bg-primary" };
    case "APPROVED":            return { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber" };
    case "PUBLISHED":           return { bg: "bg-green-bg", text: "text-green", dot: "bg-green" };
    case "IN_PROGRESS":         return { bg: "bg-primary-bg", text: "text-primary", dot: "bg-primary" };
    case "COMPLETED":           return { bg: "bg-green-bg", text: "text-green", dot: "bg-green" };
    case "PARTIALLY_COMPLETED": return { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber" };
    case "FAILED":              return { bg: "bg-red-bg", text: "text-red", dot: "bg-red" };
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

function serviceLabel(sourceService: string): string {
  switch ((sourceService || "").toUpperCase()) {
    case "DISPATCH": return "Dispatch";
    case "HAULING":  return "Hauling";
    case "TAXI":     return "Taxi";
    default:         return sourceService || "—";
  }
}

function distanceDisplay(r: RoutePlan): string {
  if (!r.plannedDistance) return "—";
  const unit = r.distanceUnit || "km";
  return `${r.plannedDistance.toFixed(1)} ${unit}`;
}

function durationDisplay(r: RoutePlan): string {
  if (!r.plannedDurationSeconds) return "—";
  const s = r.plannedDurationSeconds;
  if (s < 60) return `${s}s`;
  const m = Math.floor(s / 60);
  if (m < 60) return `${m} min`;
  const h = Math.floor(m / 60);
  return `${h}h ${m % 60}m`;
}

export default function RoutesListPage() {
  const [routes, setRoutes] = useState<RoutePlan[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const [search, setSearch] = useState("");
  const [statusFilter, setStatusFilter] = useState<string>("All");
  const [showFilters, setShowFilters] = useState(false);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError(null);
    routeService
      .listRoutes({ limit: 200 })
      .then((data) => {
        if (!cancelled) setRoutes(data);
      })
      .catch((err) => {
        if (cancelled) return;
        const message =
          err?.response?.data?.error?.message ??
          err?.message ??
          "Failed to load routes.";
        setError(message);
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, []);

  const statuses = useMemo(() => {
    const set = new Set<string>();
    routes.forEach((r) => r.status && set.add(r.status));
    return ["All", ...Array.from(set)];
  }, [routes]);

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase();
    return routes.filter((r) => {
      if (statusFilter !== "All" && r.status !== statusFilter) return false;
      if (q === "") return true;
      const haystack = [
        r.id,
        r.routeReference,
        r.branchId,
        r.providerId ?? "",
        r.vehicleOrTruckReference ?? "",
      ]
        .join(" ")
        .toLowerCase();
      return haystack.includes(q);
    });
  }, [routes, search, statusFilter]);

  const counts = useMemo(() => {
    let draft = 0, approved = 0, published = 0;
    for (const r of routes) {
      const s = (r.status || "").toUpperCase();
      if (s === "DRAFT") draft++;
      else if (s === "APPROVED") approved++;
      else if (s === "PUBLISHED") published++;
    }
    return { draft, approved, published, total: routes.length };
  }, [routes]);

  const hasActiveFilters = statusFilter !== "All" || search !== "";
  const clearFilters = () => {
    setSearch("");
    setStatusFilter("All");
  };

  return (
    <div className="space-y-5">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1>Route plans</h1>
          <p className="mt-1.5 text-sm text-ink-600">
            Planned routes with assigned providers, vehicles, and stop sequences.
          </p>
        </div>
      </div>

      {error && (
        <div className="rounded-lg border border-red/20 bg-red-bg px-4 py-2 text-sm text-red">
          {error}
        </div>
      )}

      <div className="grid grid-cols-2 gap-3.5 sm:grid-cols-4">
        <div className="rounded-lg border border-line bg-surface p-4">
          <div className="text-xs font-semibold text-ink-600">Total</div>
          <div className="mt-2 font-display text-[26px] font-bold tracking-[-0.02em] text-ink-950">
            {counts.total}
          </div>
        </div>
        <div className="rounded-lg border border-line bg-surface p-4">
          <div className="text-xs font-semibold text-ink-600">Draft</div>
          <div className="mt-2 font-display text-[26px] font-bold tracking-[-0.02em] text-ink-950">
            {counts.draft}
          </div>
        </div>
        <div className="rounded-lg border border-line bg-surface p-4">
          <div className="text-xs font-semibold text-ink-600">Approved</div>
          <div className="mt-2 font-display text-[26px] font-bold tracking-[-0.02em] text-amber">
            {counts.approved}
          </div>
        </div>
        <div className="rounded-lg border border-line bg-surface p-4">
          <div className="text-xs font-semibold text-ink-600">Published</div>
          <div className="mt-2 font-display text-[26px] font-bold tracking-[-0.02em] text-green">
            {counts.published}
          </div>
        </div>
      </div>

      <div className="flex flex-wrap items-center gap-2.5">
        <div className="flex h-10 w-full items-center gap-2 rounded-lg border border-line bg-surface px-3 sm:max-w-xs">
          <Search className="h-3.5 w-3.5 flex-shrink-0 text-ink-500" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search routes..."
            className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-500"
          />
        </div>

        <button
          onClick={() => setShowFilters(!showFilters)}
          className={`flex h-10 items-center gap-1.5 rounded-lg border px-3 text-sm font-medium transition-colors whitespace-nowrap ${
            showFilters || hasActiveFilters
              ? "border-primary bg-primary-bg text-primary"
              : "border-line bg-surface text-ink-700 hover:bg-surface-2"
          }`}
        >
          <SlidersHorizontal className="h-3.5 w-3.5" />
          Filters
          {statusFilter !== "All" && (
            <span className="rounded-full bg-primary px-1.5 py-0.5 text-[9px] text-white">1</span>
          )}
        </button>

        {hasActiveFilters && (
          <button
            onClick={clearFilters}
            className="flex h-10 items-center gap-1 rounded-lg border border-line bg-surface px-3 text-sm font-medium text-ink-500 hover:bg-surface-2 transition-colors whitespace-nowrap"
          >
            <X className="h-3.5 w-3.5" />
            Clear
          </button>
        )}

        <span className="ml-auto text-sm text-ink-500 whitespace-nowrap">
          {filtered.length} routes
        </span>
      </div>

      {showFilters && (
        <div className="rounded-lg border border-line bg-surface p-4">
          <div className="flex flex-wrap items-center gap-6">
            <div>
              <span className="mb-1.5 block text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">
                Status
              </span>
              <div className="flex flex-wrap gap-1.5">
                {statuses.map((s) => (
                  <button
                    key={s}
                    onClick={() => setStatusFilter(s)}
                    className={`rounded-full px-3 py-1 text-xs font-semibold transition-colors whitespace-nowrap ${
                      statusFilter === s
                        ? "bg-primary text-white"
                        : "bg-surface-2 text-ink-600 hover:bg-surface-3"
                    }`}
                  >
                    {s === "All" ? "All" : statusLabel(s)}
                  </button>
                ))}
              </div>
            </div>
            <button
              onClick={clearFilters}
              className="rounded-md border border-line px-3.5 py-1.5 text-xs font-medium text-ink-600 hover:bg-surface-2 transition-colors whitespace-nowrap"
            >
              Clear all
            </button>
          </div>
        </div>
      )}

      {loading ? (
        <div className="rounded-lg border border-line bg-surface px-4 py-16 text-center text-sm text-ink-500">
          Loading routes…
        </div>
      ) : routes.length === 0 ? (
        <div className="rounded-lg border border-line bg-surface">
          <EmptyState
            image="/images/trips-empty.png"
            title="No route plans"
            description="Route plans will appear here once they are created."
          />
        </div>
      ) : filtered.length === 0 ? (
        <div className="rounded-lg border border-line bg-surface">
          <EmptyState
            image="/images/trips-empty.png"
            title="No routes match your filters"
            description="Try adjusting your search or clearing the filters."
            action={
              <button
                onClick={clearFilters}
                className="inline-flex items-center justify-center rounded-lg bg-primary-bg px-4 py-2 text-xs font-semibold text-primary transition-colors hover:bg-primary/10"
              >
                Clear filters
              </button>
            }
          />
        </div>
      ) : (
        <div className="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-3">
          {filtered.map((r) => {
            const pill = statusStyle(r.status);
            return (
              <Link
                key={r.id}
                href={`/operations/routes/${encodeURIComponent(r.id)}`}
                className="group flex flex-col rounded-lg border border-line bg-surface p-4 transition-colors hover:border-ink-400 hover:bg-surface-2"
              >
                <div className="flex items-start justify-between gap-2">
                  <div className="min-w-0 flex-1">
                    <div className="flex items-center gap-2">
                      <span className="font-semibold text-ink-950 truncate">
                        {r.routeReference || r.id}
                      </span>
                    </div>
                    <div className="mt-1 flex flex-wrap items-center gap-1.5">
                      <span
                        className={`flex items-center gap-1 rounded-full px-2 py-0.5 text-[10px] font-semibold ${pill.bg} ${pill.text}`}
                      >
                        <span className={`h-1.5 w-1.5 rounded-full ${pill.dot}`} />
                        {statusLabel(r.status)}
                      </span>
                      <span className="font-mono text-[10px] text-ink-400 truncate">
                        {r.id}
                      </span>
                    </div>
                  </div>
                  <div className="flex h-8 w-8 flex-shrink-0 items-center justify-center rounded-md bg-primary-bg text-primary">
                    {r.executionService === "HAULING" ? (
                      <Truck className="h-4 w-4" />
                    ) : (
                      <MapPin className="h-4 w-4" />
                    )}
                  </div>
                </div>

                <div className="mt-3 grid grid-cols-2 gap-1.5">
                  <div className="flex items-center gap-1.5 text-xs text-ink-500">
                    <MapPin className="h-3.5 w-3.5 flex-shrink-0" />
                    <span className="font-mono truncate">{r.branchId || "—"}</span>
                  </div>
                  <div className="flex items-center gap-1.5 text-xs text-ink-500">
                    <Clock className="h-3.5 w-3.5 flex-shrink-0" />
                    <span className="truncate">{durationDisplay(r)}</span>
                  </div>
                  <div className="flex items-center gap-1.5 text-xs text-ink-500">
                    <Users className="h-3.5 w-3.5 flex-shrink-0" />
                    <span className="font-mono truncate">
                      {r.providerId || "—"}
                    </span>
                  </div>
                  <div className="flex items-center gap-1.5 text-xs text-ink-500">
                    <Truck className="h-3.5 w-3.5 flex-shrink-0" />
                    <span className="font-mono text-[10px] truncate">
                      {r.vehicleOrTruckReference || "—"}
                    </span>
                  </div>
                </div>

                <div className="mt-3 flex items-center justify-between border-t border-line pt-2.5">
                  <span className="text-xs text-ink-500">
                    {serviceLabel(r.executionService)} · {distanceDisplay(r)}
                  </span>
                  <ArrowRight className="h-4 w-4 text-ink-400 transition-colors group-hover:text-primary" />
                </div>
              </Link>
            );
          })}
        </div>
      )}
    </div>
  );
}