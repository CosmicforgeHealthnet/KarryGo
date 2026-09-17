// src/app/(dashboard)/operations/control-tower/page.tsx

"use client";

import React, { useEffect, useMemo, useState } from "react";
import Link from "next/link";
import {
  Truck,
  Package,
  Clock,
  AlertCircle,
  Navigation,
  Users,
  Eye,
  SlidersHorizontal,
  X,
  Search,
  Gauge,
  Activity,
  Wifi,
  WifiOff,
  ArrowRight,
  AlertTriangle,
} from "lucide-react";
import EmptyState from "@/components/EmptyState";
import { controlTowerService } from "@/lib/services/controlTowerService";
import type {
  Board,
  RoutePlan,
  TrackingProjection,
  UnassignedWork,
  PendingAssignmentRow,
} from "@/lib/types/controlTower";

// ---------------------------------------------------------------------------
// Row model — one shape across the five arrays
//
// The backend returns five different arrays; the page shows one list. Each
// item keeps its array of origin in `type` so the dispatcher can tell them
// apart, and each carries only the fields the backend provides for that
// array.
// ---------------------------------------------------------------------------

type RowKind = "Trip" | "Route" | "Assignment" | "Unassigned";

type Row = {
  id: string;
  kind: RowKind;
  /** Free-form sub-label from the source array. */
  subtitle: string;
  /** The provider or owner id, when the source has one. */
  providerId?: string;
  vehicleOrTruckReference?: string;
  /** Status enum, rendered as a pill. */
  status: string;
  /** Where in the workflow, when the source has a status-like field. */
  statusLabel?: string;
  /** When this row was last updated, when the source carries it. */
  updatedAt?: string;
  /** Freshness / availability signal, when the source carries it. */
  signal?: "fresh" | "aging" | "stale" | "unknown";
  /** Detail link, when the row has a page to go to. */
  href?: string;
};

function freshnessToSignal(freshness: string): Row["signal"] {
  switch ((freshness || "").toUpperCase()) {
    case "FRESH":
      return "fresh";
    case "AGING":
      return "aging";
    case "STALE":
      return "stale";
    default:
      return "unknown";
  }
}

function tripToRow(t: TrackingProjection): Row {
  return {
    id: t.canonicalTripReference,
    kind: "Trip",
    subtitle: t.sourceService,
    providerId: t.providerId || undefined,
    vehicleOrTruckReference: t.vehicleOrTruckReference || undefined,
    status: t.operationalStatus || "UNKNOWN",
    statusLabel: t.operationalStatus || "Unknown",
    updatedAt: t.receivedAt,
    signal: freshnessToSignal(t.freshnessStatus),
    href: `/operations/trips/${encodeURIComponent(t.canonicalTripReference)}`,
  };
}

function routeToRow(r: RoutePlan): Row {
  return {
    id: r.routeReference || r.id,
    kind: "Route",
    subtitle: r.executionService,
    providerId: r.providerId || undefined,
    vehicleOrTruckReference: r.vehicleOrTruckReference || undefined,
    status: r.status,
    statusLabel: r.status,
    updatedAt: r.updatedAt,
    href: `/operations/routes/${encodeURIComponent(r.id)}`,
  };
}

function assignmentToRow(a: PendingAssignmentRow): Row {
  return {
    id: a.id,
    kind: "Assignment",
    subtitle: a.executionService,
    providerId: a.providerId || undefined,
    vehicleOrTruckReference: a.vehicleOrTruckReference || undefined,
    status: a.status,
    statusLabel: a.status,
    updatedAt: a.responseDeadline,
    href: `/operations/assignments/${encodeURIComponent(a.id)}`,
  };
}

function unassignedToRow(u: UnassignedWork): Row {
  return {
    id: u.id,
    kind: "Unassigned",
    subtitle: u.sourceType,
    status: u.status,
    statusLabel: u.status,
    href: undefined, // no page for the source record yet
  };
}

// ---------------------------------------------------------------------------
// Style mappings
// ---------------------------------------------------------------------------

type Tone = "blue" | "green" | "amber" | "red" | "gray";

const TONE_BG: Record<Tone, string> = {
  blue: "bg-primary-bg text-primary",
  green: "bg-green-bg text-green",
  amber: "bg-amber-bg text-amber",
  red: "bg-red-bg text-red",
  gray: "bg-surface-2 text-ink-500",
};

function statusTone(status: string): Tone {
  const s = (status || "").toUpperCase();
  if (
    s.includes("COMPLETED") ||
    s === "ACCEPTED" ||
    s === "READY_FOR_EXECUTION" ||
    s === "IN_PROGRESS" ||
    s === "PUBLISHED" ||
    s === "FRESH"
  ) {
    return "green";
  }
  if (s === "PENDING_PROVIDER_RESPONSE" || s === "PENDING" || s === "AGING") {
    return "amber";
  }
  if (
    s === "DECLINED" ||
    s === "EXPIRED" ||
    s === "CANCELLED" ||
    s === "FAILED" ||
    s === "STALE" ||
    s === "BLOCKED"
  ) {
    return "red";
  }
  if (s === "EN_ROUTE" || s === "ASSIGNED") return "blue";
  return "gray";
}

const KIND_BG: Record<RowKind, string> = {
  Trip: "bg-primary-bg text-primary",
  Route: "bg-amber-bg text-amber",
  Assignment: "bg-green-bg text-green",
  Unassigned: "bg-red-bg text-red",
};

const SIGNAL_ICON = {
  fresh: { Icon: Wifi, color: "text-green" },
  aging: { Icon: Wifi, color: "text-amber" },
  stale: { Icon: WifiOff, color: "text-red" },
  unknown: { Icon: Wifi, color: "text-ink-400" },
} as const;

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

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------

export default function ControlTowerPage() {
  const [board, setBoard] = useState<Board | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const [search, setSearch] = useState("");
  const [kindFilter, setKindFilter] = useState<"All" | RowKind>("All");
  const [showFilters, setShowFilters] = useState(false);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError(null);

    controlTowerService
      .getBoard({ limit: 200 })
      .then((data) => {
        if (!cancelled) setBoard(data);
      })
      .catch((err) => {
        if (cancelled) return;
        const message =
          err?.response?.data?.error?.message ??
          err?.message ??
          "Failed to load control tower.";
        setError(message);
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });

    return () => {
      cancelled = true;
    };
  }, []);

  const rows: Row[] = useMemo(() => {
    if (!board) return [];
    return [
      ...board.activeTrips.map(tripToRow),
      ...board.activeRoutes.map(routeToRow),
      ...board.pendingAssignments.map(assignmentToRow),
      ...board.acceptedAssignments.map(assignmentToRow),
      ...board.unassigned.map(unassignedToRow),
    ];
  }, [board]);

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase();
    return rows.filter((r) => {
      if (kindFilter !== "All" && r.kind !== kindFilter) return false;
      if (q === "") return true;
      const haystack = [
        r.id,
        r.subtitle,
        r.providerId ?? "",
        r.vehicleOrTruckReference ?? "",
        r.status,
      ]
        .join(" ")
        .toLowerCase();
      return haystack.includes(q);
    });
  }, [rows, search, kindFilter]);

  // Real counts from real arrays. No invented percentages.
  const counts = useMemo(() => {
    if (!board) {
      return { active: 0, idle: 0, exceptions: 0, unassigned: 0 };
    }
    const active = board.activeTrips.length + board.activeRoutes.length;
    const idle = board.providerAvailability.length;
    const exceptions = board.alerts.filter(
      (a) =>
        a.status === "OPEN" ||
        a.status === "ACKNOWLEDGED" ||
        a.status === "IN_PROGRESS",
    ).length;
    const unassigned = board.unassigned.length;
    return { active, idle, exceptions, unassigned };
  }, [board]);

  const hasActiveFilters = kindFilter !== "All" || search !== "";
  const clearFilters = () => {
    setSearch("");
    setKindFilter("All");
  };

  return (
    <div className="space-y-5">
      {/* ── Header ── */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1>Control Tower</h1>
          <p className="mt-1.5">
            Live operational view of active work, provider availability, and
            exceptions.
          </p>
        </div>
      </div>

      {error && (
        <div className="rounded-lg border border-red/20 bg-red-bg px-4 py-2 text-sm text-red">
          {error}
        </div>
      )}

      {/* ── Stats ── */}
      <div className="grid grid-cols-2 gap-3.5 sm:grid-cols-4">
        <div className="rounded-lg border border-line bg-surface p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-green-bg text-green">
              <Activity className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">Active</span>
          </div>
          <div className="mt-3 font-display text-[26px] font-bold tracking-[-0.02em] text-ink-950">
            {counts.active}
          </div>
        </div>
        <div className="rounded-lg border border-line bg-surface p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-primary-bg text-primary">
              <Clock className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">
              Available providers
            </span>
          </div>
          <div className="mt-3 font-display text-[26px] font-bold tracking-[-0.02em] text-ink-950">
            {counts.idle}
          </div>
        </div>
        <div className="rounded-lg border border-line bg-surface p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-red-bg text-red">
              <AlertCircle className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">
              Exceptions
            </span>
          </div>
          <div className="mt-3 font-display text-[26px] font-bold tracking-[-0.02em] text-red">
            {counts.exceptions}
          </div>
        </div>
        <div className="rounded-lg border border-line bg-surface p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-amber-bg text-amber">
              <AlertTriangle className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">
              Unassigned work
            </span>
          </div>
          <div className="mt-3 font-display text-[26px] font-bold tracking-[-0.02em] text-amber">
            {counts.unassigned}
          </div>
        </div>
      </div>

      {/* ── Toolbar ── */}
      <div className="flex flex-wrap items-center gap-2.5">
        <div className="flex h-10 w-full items-center gap-2 rounded-lg border border-line bg-surface px-3 sm:max-w-xs">
          <Search className="h-3.5 w-3.5 flex-shrink-0 text-ink-500" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search by ID, provider, or vehicle..."
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
          {kindFilter !== "All" && (
            <span className="rounded-full bg-primary px-1.5 py-0.5 text-[9px] text-white">
              1
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
          {filtered.length} items
        </span>
      </div>

      {/* ── Filter drawer ── */}
      {showFilters && (
        <div className="rounded-lg border border-line bg-surface p-4">
          <div className="flex flex-wrap items-center gap-6">
            <div>
              <span className="mb-1.5 block text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">
                Type
              </span>
              <div className="flex flex-wrap gap-1.5">
                {(["All", "Trip", "Route", "Assignment", "Unassigned"] as const).map(
                  (k) => (
                    <button
                      key={k}
                      onClick={() => setKindFilter(k)}
                      className={`rounded-full px-3 py-1 text-xs font-semibold transition-colors whitespace-nowrap ${
                        kindFilter === k
                          ? "bg-primary text-white"
                          : "bg-surface-2 text-ink-600 hover:bg-surface-3"
                      }`}
                    >
                      {k}
                    </button>
                  ),
                )}
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

      {/* ── List ── */}
      <div className="overflow-x-auto rounded-lg border border-line bg-surface">
        {loading ? (
          <div className="px-4 py-16 text-center text-sm text-ink-500">
            Loading control tower…
          </div>
        ) : !board ? (
          <EmptyState
            image="/images/live-operations-empty.png"
            title="Control tower unavailable"
            description="The control tower endpoint is not currently responding."
          />
        ) : rows.length === 0 ? (
          <EmptyState
            image="/images/live-operations-empty.png"
            title="No active work"
            description="Live work will appear here as trips, routes, and assignments start."
          />
        ) : filtered.length === 0 ? (
          <EmptyState
            image="/images/live-operations-empty.png"
            title="No items match your filters"
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
        ) : (
          <table className="w-full min-w-[900px] border-collapse text-left">
            <thead>
              <tr className="border-b border-line bg-surface-2">
                <th className="px-4 py-3 text-[10px] font-semibold uppercase tracking-[0.06em] text-ink-500 whitespace-nowrap">
                  Item
                </th>
                <th className="px-4 py-3 text-[10px] font-semibold uppercase tracking-[0.06em] text-ink-500 whitespace-nowrap">
                  Type
                </th>
                <th className="px-4 py-3 text-[10px] font-semibold uppercase tracking-[0.06em] text-ink-500 whitespace-nowrap">
                  Provider
                </th>
                <th className="px-4 py-3 text-[10px] font-semibold uppercase tracking-[0.06em] text-ink-500 whitespace-nowrap">
                  Vehicle
                </th>
                <th className="px-4 py-3 text-[10px] font-semibold uppercase tracking-[0.06em] text-ink-500 whitespace-nowrap">
                  Status
                </th>
                <th className="px-4 py-3 text-[10px] font-semibold uppercase tracking-[0.06em] text-ink-500 whitespace-nowrap">
                  Updated
                </th>
                <th className="px-4 py-3 text-[10px] font-semibold uppercase tracking-[0.06em] text-ink-500 text-right whitespace-nowrap">
                  Action
                </th>
              </tr>
            </thead>
            <tbody>
              {filtered.map((row) => {
                const tone = statusTone(row.status);
                const signal = row.signal ? SIGNAL_ICON[row.signal] : null;
                return (
                  <tr
                    key={`${row.kind}-${row.id}`}
                    className="border-b border-line transition-colors hover:bg-surface-2"
                  >
                    <td className="px-4 py-3.5 whitespace-nowrap">
                      <div className="flex items-center gap-2">
                        {signal && (
                          <signal.Icon
                            className={`h-3.5 w-3.5 ${signal.color}`}
                          />
                        )}
                        <span className="font-mono text-sm font-semibold text-ink-950">
                          {row.id}
                        </span>
                      </div>
                      <div className="text-[10px] text-ink-400">
                        {row.subtitle}
                      </div>
                    </td>
                    <td className="px-4 py-3.5 whitespace-nowrap">
                      <span
                        className={`rounded-full px-2.5 py-1 text-[11px] font-semibold ${KIND_BG[row.kind]}`}
                      >
                        {row.kind}
                      </span>
                    </td>
                    <td className="px-4 py-3.5 font-mono text-xs text-ink-700 whitespace-nowrap">
                      {row.providerId || "—"}
                    </td>
                    <td className="px-4 py-3.5 font-mono text-xs text-ink-700 whitespace-nowrap">
                      {row.vehicleOrTruckReference || "—"}
                    </td>
                    <td className="px-4 py-3.5 whitespace-nowrap">
                      <span
                        className={`inline-flex items-center gap-1.5 rounded-full px-2.5 py-1 text-[11px] font-semibold ${TONE_BG[tone]}`}
                      >
                        {row.statusLabel || row.status}
                      </span>
                    </td>
                    <td className="px-4 py-3.5 text-xs text-ink-500 whitespace-nowrap">
                      {timeAgo(row.updatedAt)}
                    </td>
                    <td className="px-4 py-3.5 text-right whitespace-nowrap">
                      {row.href ? (
                        <Link
                          href={row.href}
                          className="inline-flex items-center gap-1 rounded-md border border-line px-3 py-1.5 text-xs font-medium text-ink-600 hover:bg-surface-2 transition-colors"
                        >
                          <Eye className="h-3.5 w-3.5" />
                          View
                        </Link>
                      ) : (
                        <span className="text-xs text-ink-400">—</span>
                      )}
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        )}
      </div>

      {/* ── Legend ── */}
      <div className="rounded-lg border border-line bg-surface p-3">
        <div className="flex flex-wrap items-center gap-4 text-xs text-ink-600">
          <span className="font-medium text-ink-700">Signal:</span>
          <span className="flex items-center gap-1.5">
            <Wifi className="h-3.5 w-3.5 text-green" />
            Fresh
          </span>
          <span className="flex items-center gap-1.5">
            <Wifi className="h-3.5 w-3.5 text-amber" />
            Aging
          </span>
          <span className="flex items-center gap-1.5">
            <WifiOff className="h-3.5 w-3.5 text-red" />
            Stale
          </span>
        </div>
      </div>
    </div>
  );
}