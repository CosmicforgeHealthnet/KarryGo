// src/app/(dashboard)/operations/geofences/page.tsx

"use client";

import { useEffect, useMemo, useState } from "react";
import Link from "next/link";
import {
  Search,
  SlidersHorizontal,
  X,
  MapPin,
  Clock,
  ArrowRight,
  CheckCircle2,
  AlertCircle,
  ShieldAlert,
  Circle,
} from "lucide-react";
import EmptyState from "@/components/EmptyState";
import { geofenceService } from "@/lib/services/geofenceService";
import type { Geofence } from "@/lib/types/geofence";

// Derive a display "type" from the real event rules. ROUTE_DEVIATION is
// excluded because every geofence is evaluated for it.
function displayType(g: Geofence): { label: string; tone: "red" | "primary" | "ink" } {
  if ((g.eventRules ?? []).includes("RESTRICTED_AREA")) {
    return { label: "Restricted", tone: "red" };
  }
  if ((g.eventRules ?? []).includes("DWELL")) {
    return { label: "Dwell zone", tone: "primary" };
  }
  return { label: "Standard", tone: "ink" };
}

function statusTone(status: string): {
  label: string;
  bg: string;
  text: string;
  dot: string;
} {
  if ((status || "").toUpperCase() === "ACTIVE") {
    return {
      label: "Active",
      bg: "bg-green-bg",
      text: "text-green",
      dot: "bg-green",
    };
  }
  return {
    label: "Inactive",
    bg: "bg-surface-2",
    text: "text-ink-500",
    dot: "bg-ink-400",
  };
}

function geometryDescription(g: Geofence): string {
  const type = (g.geometryType || "").toUpperCase();
  if (type === "CIRCLE") {
    const r = g.geometry?.radiusMeters;
    return r ? `Circle · ${Math.round(r)} m radius` : "Circle";
  }
  if (type === "POLYGON") {
    const pts = g.geometry?.points?.length ?? 0;
    return `Polygon · ${Math.max(0, pts - 1)} vertices`;
  }
  return g.geometryType || "—";
}

function formatTimestamp(iso: string | undefined): string {
  if (!iso) return "—";
  const t = new Date(iso).getTime();
  if (!Number.isFinite(t)) return "—";
  return new Date(t).toLocaleString(undefined, {
    year: "numeric",
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
}

function timeAgo(iso: string | undefined): string {
  if (!iso) return "—";
  const t = new Date(iso).getTime();
  if (!Number.isFinite(t)) return "—";
  const sec = Math.max(1, Math.floor((Date.now() - t) / 1000));
  if (sec < 60) return `${sec}s ago`;
  const min = Math.floor(sec / 60);
  if (min < 60) return `${min}m ago`;
  const hr = Math.floor(min / 60);
  if (hr < 24) return `${hr}h ago`;
  const day = Math.floor(hr / 24);
  return `${day}d ago`;
}

export default function GeofencesPage() {
  const [geofences, setGeofences] = useState<Geofence[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const [search, setSearch] = useState("");
  const [statusFilter, setStatusFilter] = useState<"All" | "ACTIVE" | "INACTIVE">("All");
  const [typeFilter, setTypeFilter] = useState<"All" | "Standard" | "Restricted" | "Dwell zone">("All");
  const [showFilters, setShowFilters] = useState(false);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError(null);
    geofenceService
      .listGeofences({ limit: 200 })
      .then((data) => {
        if (!cancelled) setGeofences(data);
      })
      .catch((err) => {
        if (cancelled) return;
        const message =
          err?.response?.data?.error?.message ??
          err?.message ??
          "Failed to load geofences.";
        setError(message);
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, []);

  const counts = useMemo(() => {
    let active = 0;
    let inactive = 0;
    for (const g of geofences) {
      if ((g.activeStatus || "").toUpperCase() === "ACTIVE") active++;
      else inactive++;
    }
    return { active, inactive, total: geofences.length };
  }, [geofences]);

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase();
    return geofences.filter((g) => {
      if (statusFilter !== "All" && (g.activeStatus || "").toUpperCase() !== statusFilter) {
        return false;
      }
      if (typeFilter !== "All" && displayType(g).label !== typeFilter) return false;
      if (q === "") return true;
      const haystack = [
        g.id,
        g.name,
        g.branchId ?? "",
        g.geometryReference ?? "",
      ]
        .join(" ")
        .toLowerCase();
      return haystack.includes(q);
    });
  }, [geofences, search, statusFilter, typeFilter]);

  const hasActiveFilters =
    statusFilter !== "All" || typeFilter !== "All" || search !== "";
  const clearFilters = () => {
    setSearch("");
    setStatusFilter("All");
    setTypeFilter("All");
  };

  return (
    <div className="space-y-5">
      <div>
        <h1>Geofences</h1>
        <p className="mt-1.5 text-sm text-ink-600">
          Geographic boundaries for branches and route events.
        </p>
      </div>

      {error && (
        <div className="rounded-lg border border-red/20 bg-red-bg px-4 py-2 text-sm text-red">
          {error}
        </div>
      )}

      <div className="grid grid-cols-1 gap-3.5 sm:grid-cols-3">
        <div className="rounded-lg border border-line bg-surface p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-green-bg text-green">
              <CheckCircle2 className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">Active</span>
          </div>
          <div className="mt-3 font-display text-[26px] font-bold tracking-[-0.02em] text-ink-950">
            {counts.active}
          </div>
        </div>
        <div className="rounded-lg border border-line bg-surface p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-surface-2 text-ink-500">
              <AlertCircle className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">Inactive</span>
          </div>
          <div className="mt-3 font-display text-[26px] font-bold tracking-[-0.02em] text-ink-950">
            {counts.inactive}
          </div>
        </div>
        <div className="rounded-lg border border-line bg-surface p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-primary-bg text-primary">
              <MapPin className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">Total</span>
          </div>
          <div className="mt-3 font-display text-[26px] font-bold tracking-[-0.02em] text-ink-950">
            {counts.total}
          </div>
        </div>
      </div>

      <div className="flex flex-wrap items-center gap-2.5">
        <div className="flex h-10 w-full items-center gap-2 rounded-lg border border-line bg-surface px-3 sm:max-w-xs">
          <Search className="h-3.5 w-3.5 flex-shrink-0 text-ink-500" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search geofences..."
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
          {hasActiveFilters && (
            <span className="rounded-full bg-primary px-1.5 py-0.5 text-[9px] text-white">
              {(statusFilter !== "All" ? 1 : 0) + (typeFilter !== "All" ? 1 : 0)}
            </span>
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
          {filtered.length} geofences
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
                {(["All", "ACTIVE", "INACTIVE"] as const).map((s) => (
                  <button
                    key={s}
                    onClick={() => setStatusFilter(s)}
                    className={`rounded-full px-3 py-1 text-xs font-semibold transition-colors whitespace-nowrap ${
                      statusFilter === s
                        ? "bg-primary text-white"
                        : "bg-surface-2 text-ink-600 hover:bg-surface-3"
                    }`}
                  >
                    {s === "All" ? "All" : s === "ACTIVE" ? "Active" : "Inactive"}
                  </button>
                ))}
              </div>
            </div>
            <div>
              <span className="mb-1.5 block text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">
                Type
              </span>
              <div className="flex flex-wrap gap-1.5">
                {(["All", "Standard", "Restricted", "Dwell zone"] as const).map((t) => (
                  <button
                    key={t}
                    onClick={() => setTypeFilter(t)}
                    className={`rounded-full px-3 py-1 text-xs font-semibold transition-colors whitespace-nowrap ${
                      typeFilter === t
                        ? "bg-primary text-white"
                        : "bg-surface-2 text-ink-600 hover:bg-surface-3"
                    }`}
                  >
                    {t}
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
          Loading geofences…
        </div>
      ) : geofences.length === 0 ? (
        <div className="rounded-lg border border-line bg-surface">
          <EmptyState
            image="/images/live-operations-empty.png"
            title="No geofences"
            description="Geofences define geographic boundaries that drive route events and area restrictions."
          />
        </div>
      ) : filtered.length === 0 ? (
        <div className="rounded-lg border border-line bg-surface">
          <EmptyState
            image="/images/live-operations-empty.png"
            title="No geofences match your filters"
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
          {filtered.map((g) => {
            const status = statusTone(g.activeStatus);
            const type = displayType(g);
            const typeBg =
              type.tone === "red"
                ? "bg-red-bg text-red"
                : type.tone === "primary"
                ? "bg-primary-bg text-primary"
                : "bg-surface-2 text-ink-500";
            const TypeIcon = type.tone === "red" ? ShieldAlert : Circle;

            return (
              <Link
                key={g.id}
                href={`/operations/geofences/${encodeURIComponent(g.id)}`}
                className="group rounded-lg border border-line bg-surface p-4 transition-colors hover:border-ink-400 hover:bg-surface-2"
              >
                <div className="flex items-start justify-between gap-3">
                  <div className="flex min-w-0 items-center gap-2.5">
                    <div
                      className={`flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-md ${
                        status.label === "Active"
                          ? "bg-green-bg text-green"
                          : "bg-surface-2 text-ink-400"
                      }`}
                    >
                      <MapPin className="h-[18px] w-[18px]" />
                    </div>
                    <div className="min-w-0">
                      <div className="flex flex-wrap items-center gap-2">
                        <span className="text-sm font-semibold text-ink-950 truncate">
                          {g.name}
                        </span>
                        <span className="font-mono text-[10px] text-ink-400 truncate">
                          {g.id}
                        </span>
                      </div>
                      <div className="mt-0.5 flex flex-wrap items-center gap-1.5">
                        <span
                          className={`flex items-center gap-1 rounded-full px-2 py-0.5 text-[10px] font-semibold ${typeBg}`}
                        >
                          <TypeIcon className="h-3 w-3" />
                          {type.label}
                        </span>
                        <span
                          className={`flex items-center gap-1 rounded-full px-2 py-0.5 text-[10px] font-semibold ${status.bg} ${status.text}`}
                        >
                          <span className={`h-1.5 w-1.5 rounded-full ${status.dot}`} />
                          {status.label}
                        </span>
                      </div>
                    </div>
                  </div>
                </div>

                <div className="mt-3 grid grid-cols-2 gap-1.5">
                  <div className="flex items-center gap-1.5 text-xs text-ink-500">
                    <MapPin className="h-3.5 w-3.5 flex-shrink-0" />
                    <span className="font-mono truncate">
                      {g.branchId || "—"}
                    </span>
                  </div>
                  <div className="flex items-center gap-1.5 text-xs text-ink-500">
                    <Circle className="h-3.5 w-3.5 flex-shrink-0" />
                    <span className="truncate">{geometryDescription(g)}</span>
                  </div>
                </div>

                <div className="mt-3 flex items-center justify-between border-t border-line pt-2.5">
                  <div className="flex items-center gap-1.5 text-[10px] text-ink-400">
                    <Clock className="h-3.5 w-3.5" />
                    {timeAgo(g.updatedAt)}
                  </div>
                  <div className="flex items-center gap-1 text-xs font-medium text-ink-400 transition-colors group-hover:text-primary">
                    View <ArrowRight className="h-3.5 w-3.5" />
                  </div>
                </div>
              </Link>
            );
          })}
        </div>
      )}
    </div>
  );
}