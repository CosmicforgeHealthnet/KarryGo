// src/app/(dashboard)/operations/dispatch/unassigned/page.tsx

"use client";

import { useEffect, useMemo, useState } from "react";
import { Search, X, AlertCircle } from "lucide-react";
import EmptyState from "@/components/EmptyState";
import { controlTowerService } from "@/lib/services/controlTowerService";
import type { Board, UnassignedWork } from "@/lib/types/controlTower";

function sourceLabel(sourceType: string): string {
  switch ((sourceType || "").toUpperCase()) {
    case "ORDER":
      return "Order";
    case "SHIPMENT":
      return "Shipment";
    case "ROUTE":
      return "Route";
    default:
      return sourceType || "—";
  }
}

function reasonLabel(reason: string): string {
  switch ((reason || "").toUpperCase()) {
    case "CONFIRMED_WITHOUT_ROUTE":
      return "Confirmed without a route";
    case "ORDER_REQUIRES_ACTION":
      return "Order requires action";
    case "READY_WITHOUT_ACTIVE_ASSIGNMENT":
      return "Ready without active assignment";
    case "PROVIDER_REPLACEMENT_REQUIRED":
      return "Provider replacement required";
    default:
      return reason || "—";
  }
}

export default function UnassignedWorkPage() {
  const [board, setBoard] = useState<Board | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const [search, setSearch] = useState("");
  const [kindFilter, setKindFilter] = useState<string>("All");

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
          "Failed to load unassigned work.";
        setError(message);
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, []);

  const kinds = useMemo(() => {
    const set = new Set<string>();
    (board?.unassigned ?? []).forEach((u) => {
      if (u.sourceType) set.add(u.sourceType);
    });
    return ["All", ...Array.from(set)];
  }, [board]);

  const filtered = useMemo(() => {
    const list: UnassignedWork[] = board?.unassigned ?? [];
    const q = search.trim().toLowerCase();
    return list.filter((u) => {
      if (kindFilter !== "All" && u.sourceType !== kindFilter) return false;
      if (q === "") return true;
      return [u.id, u.sourceType, u.actionReason, u.branchId]
        .join(" ")
        .toLowerCase()
        .includes(q);
    });
  }, [board, search, kindFilter]);

  const hasFilters = kindFilter !== "All" || search !== "";
  const clearFilters = () => {
    setSearch("");
    setKindFilter("All");
  };

  return (
    <div className="space-y-5">
      <div>
        <h1>Unassigned work</h1>
        <p className="mt-1.5 text-sm text-ink-600">
          Work that has not yet been attached to a route or active assignment.
        </p>
      </div>

      {error && (
        <div className="rounded-lg border border-red/20 bg-red-bg px-4 py-2 text-sm text-red">
          {error}
        </div>
      )}

      <div className="flex flex-wrap items-center gap-2.5">
        <div className="flex h-10 w-full items-center gap-2 rounded-lg border border-line bg-surface px-3 sm:max-w-xs">
          <Search className="h-3.5 w-3.5 flex-shrink-0 text-ink-500" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search by ID or reason..."
            className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-500"
          />
        </div>
        {kinds.length > 1 && (
          <div className="flex flex-wrap gap-1.5">
            {kinds.map((k) => (
              <button
                key={k}
                onClick={() => setKindFilter(k)}
                className={`rounded-full px-3 py-1 text-xs font-semibold transition-colors ${
                  kindFilter === k
                    ? "bg-primary text-white"
                    : "bg-surface-2 text-ink-600 hover:bg-surface-3"
                }`}
              >
                {k === "All" ? "All types" : sourceLabel(k)}
              </button>
            ))}
          </div>
        )}
        {hasFilters && (
          <button
            onClick={clearFilters}
            className="flex h-10 items-center gap-1 rounded-lg border border-line bg-surface px-3 text-sm font-medium text-ink-500 hover:bg-surface-2"
          >
            <X className="h-3.5 w-3.5" />
            Clear
          </button>
        )}
        <span className="ml-auto text-sm text-ink-500">
          {filtered.length} items
        </span>
      </div>

      <div className="overflow-x-auto rounded-lg border border-line bg-surface">
        {loading ? (
          <div className="px-4 py-16 text-center text-sm text-ink-500">
            Loading unassigned work…
          </div>
        ) : !board ? (
          <EmptyState
            image="/images/live-operations-empty.png"
            title="Unassigned work unavailable"
            description="The dispatch board is not currently responding."
          />
        ) : (board.unassigned ?? []).length === 0 ? (
          <EmptyState
            image="/images/trips-empty.png"
            title="No unassigned work"
            description="Everything is currently routed or assigned."
          />
        ) : filtered.length === 0 ? (
          <EmptyState
            image="/images/trips-empty.png"
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
          <table className="w-full min-w-[700px] border-collapse text-left">
            <thead>
              <tr className="border-b border-line bg-surface-2">
                <th className="px-4 py-3 text-[10px] font-semibold uppercase tracking-[0.06em] text-ink-500">
                  ID
                </th>
                <th className="px-4 py-3 text-[10px] font-semibold uppercase tracking-[0.06em] text-ink-500">
                  Type
                </th>
                <th className="px-4 py-3 text-[10px] font-semibold uppercase tracking-[0.06em] text-ink-500">
                  Branch
                </th>
                <th className="px-4 py-3 text-[10px] font-semibold uppercase tracking-[0.06em] text-ink-500">
                  Status
                </th>
                <th className="px-4 py-3 text-[10px] font-semibold uppercase tracking-[0.06em] text-ink-500">
                  Action needed
                </th>
              </tr>
            </thead>
            <tbody>
              {filtered.map((u) => (
                <tr
                  key={u.id}
                  className="border-b border-line transition-colors hover:bg-surface-2"
                >
                  <td className="px-4 py-3.5 font-mono text-sm font-semibold text-ink-950 whitespace-nowrap">
                    {u.id}
                  </td>
                  <td className="px-4 py-3.5 text-sm text-ink-700 whitespace-nowrap">
                    {sourceLabel(u.sourceType)}
                  </td>
                  <td className="px-4 py-3.5 font-mono text-xs text-ink-600 whitespace-nowrap">
                    {u.branchId || "—"}
                  </td>
                  <td className="px-4 py-3.5 text-sm text-ink-700 whitespace-nowrap">
                    {u.status || "—"}
                  </td>
                  <td className="px-4 py-3.5 whitespace-nowrap">
                    <span className="inline-flex items-center gap-1.5 rounded-full bg-amber-bg px-2.5 py-1 text-[11px] font-semibold text-amber">
                      <AlertCircle className="h-3 w-3" />
                      {reasonLabel(u.actionReason)}
                    </span>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </div>
  );
}