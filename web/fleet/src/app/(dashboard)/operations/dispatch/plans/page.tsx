// src/app/(dashboard)/operations/dispatch/plans/page.tsx

"use client";

import { useEffect, useMemo, useState } from "react";
import Link from "next/link";
import { Plus, Search, SlidersHorizontal, X, Eye } from "lucide-react";
import EmptyState from "@/components/EmptyState";
import { dispatchService } from "@/lib/services/dispatchService";
import type { DispatchPlan } from "@/lib/types/dispatch";

function statusStyle(status: string): { bg: string; text: string; dot: string } {
  switch ((status || "").toUpperCase()) {
    case "DRAFT":
      return { bg: "bg-surface-2", text: "text-ink-600", dot: "bg-ink-400" };
    case "READY":
      return { bg: "bg-primary-bg", text: "text-primary", dot: "bg-primary" };
    case "APPROVED":
      return { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber" };
    case "PUBLISHED":
      return { bg: "bg-green-bg", text: "text-green", dot: "bg-green" };
    case "IN_PROGRESS":
      return { bg: "bg-primary-bg", text: "text-primary", dot: "bg-primary" };
    case "COMPLETED":
      return { bg: "bg-green-bg", text: "text-green", dot: "bg-green" };
    case "CANCELLED":
      return { bg: "bg-red-bg", text: "text-red", dot: "bg-red" };
    default:
      return { bg: "bg-surface-2", text: "text-ink-500", dot: "bg-ink-400" };
  }
}

function statusLabel(status: string): string {
  switch ((status || "").toUpperCase()) {
    case "DRAFT":       return "Draft";
    case "READY":       return "Ready";
    case "APPROVED":    return "Approved";
    case "PUBLISHED":   return "Published";
    case "IN_PROGRESS": return "In progress";
    case "COMPLETED":   return "Completed";
    case "CANCELLED":   return "Cancelled";
    default:            return status || "—";
  }
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

export default function DispatchPlansListPage() {
  const [plans, setPlans] = useState<DispatchPlan[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const [search, setSearch] = useState("");
  const [statusFilter, setStatusFilter] = useState<string>("All");
  const [showFilters, setShowFilters] = useState(false);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError(null);
    dispatchService
      .listPlans({ limit: 200 })
      .then((data) => {
        if (!cancelled) setPlans(data);
      })
      .catch((err) => {
        if (cancelled) return;
        const message =
          err?.response?.data?.error?.message ??
          err?.message ??
          "Failed to load dispatch plans.";
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
    plans.forEach((p) => p.status && set.add(p.status));
    return ["All", ...Array.from(set)];
  }, [plans]);

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase();
    return plans.filter((p) => {
      if (statusFilter !== "All" && p.status !== statusFilter) return false;
      if (q === "") return true;
      return [p.id, p.branchId, p.plannerUserId].join(" ").toLowerCase().includes(q);
    });
  }, [plans, search, statusFilter]);

  const hasActiveFilters = statusFilter !== "All" || search !== "";
  const clearFilters = () => {
    setSearch("");
    setStatusFilter("All");
  };

  return (
    <div className="space-y-5">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1>Dispatch plans</h1>
          <p className="mt-1.5 text-sm text-ink-600">
            Planning windows for branch operations. Plans group work into a
            date and time slot before it is dispatched.
          </p>
        </div>
        <Link
          href="/operations/dispatch/plans/create"
          className="inline-flex h-10 items-center gap-1.5 rounded-lg bg-primary px-3.5 text-sm font-semibold text-white transition-colors hover:bg-primary-hover"
        >
          <Plus className="h-3.5 w-3.5" />
          New plan
        </Link>
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
            placeholder="Search plans..."
            className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-500"
          />
        </div>
        <button
          onClick={() => setShowFilters(!showFilters)}
          className={`flex h-10 items-center gap-1.5 rounded-lg border px-3 text-sm font-medium transition-colors whitespace-nowrap ${
            showFilters || statusFilter !== "All"
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
            className="flex h-10 items-center gap-1 rounded-lg border border-line bg-surface px-3 text-sm font-medium text-ink-500 hover:bg-surface-2"
          >
            <X className="h-3.5 w-3.5" />
            Clear
          </button>
        )}
        <span className="ml-auto text-sm text-ink-500 whitespace-nowrap">
          {filtered.length} plans
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
                    className={`rounded-full px-3 py-1 text-xs font-semibold transition-colors ${
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
              className="rounded-md border border-line px-3.5 py-1.5 text-xs font-medium text-ink-600 hover:bg-surface-2"
            >
              Clear all
            </button>
          </div>
        </div>
      )}

      <div className="overflow-x-auto rounded-lg border border-line bg-surface">
        {loading ? (
          <div className="px-4 py-16 text-center text-sm text-ink-500">
            Loading plans…
          </div>
        ) : plans.length === 0 ? (
          <EmptyState
            image="/images/reports-empty.png"
            title="No dispatch plans yet"
            description="Create a plan to organise a branch's work into a date and time window."
            actionHref="/operations/dispatch/plans/create"
            actionLabel="Create plan"
          />
        ) : filtered.length === 0 ? (
          <EmptyState
            image="/images/reports-empty.png"
            title="No plans match your filters"
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
                <th className="px-4 py-3 text-[10px] font-semibold uppercase tracking-[0.06em] text-ink-500">Plan</th>
                <th className="px-4 py-3 text-[10px] font-semibold uppercase tracking-[0.06em] text-ink-500">Branch</th>
                <th className="px-4 py-3 text-[10px] font-semibold uppercase tracking-[0.06em] text-ink-500">Plan date</th>
                <th className="px-4 py-3 text-[10px] font-semibold uppercase tracking-[0.06em] text-ink-500">Work window</th>
                <th className="px-4 py-3 text-[10px] font-semibold uppercase tracking-[0.06em] text-ink-500">Status</th>
                <th className="px-4 py-3 text-[10px] font-semibold uppercase tracking-[0.06em] text-ink-500">Planner</th>
                <th className="px-4 py-3 text-[10px] font-semibold uppercase tracking-[0.06em] text-ink-500 text-right">Action</th>
              </tr>
            </thead>
            <tbody>
              {filtered.map((p) => {
                const pill = statusStyle(p.status);
                return (
                  <tr
                    key={p.id}
                    className="border-b border-line transition-colors hover:bg-surface-2"
                  >
                    <td className="px-4 py-3.5 whitespace-nowrap">
                      <Link
                        href={`/operations/dispatch/plans/${p.id}`}
                        className="font-mono text-sm font-semibold text-ink-950 hover:text-primary"
                      >
                        {p.id}
                      </Link>
                    </td>
                    <td className="px-4 py-3.5 font-mono text-xs text-ink-700 whitespace-nowrap">
                      {p.branchId || "—"}
                    </td>
                    <td className="px-4 py-3.5 text-sm text-ink-700 whitespace-nowrap">
                      {formatTimestamp(p.planDate)}
                    </td>
                    <td className="px-4 py-3.5 text-xs text-ink-600 whitespace-nowrap">
                      {formatTimestamp(p.workWindowStart)} → {formatTimestamp(p.workWindowEnd)}
                    </td>
                    <td className="px-4 py-3.5 whitespace-nowrap">
                      <span className={`inline-flex items-center gap-1.5 rounded-full px-2.5 py-1 text-[11px] font-semibold ${pill.bg} ${pill.text}`}>
                        <span className={`h-1.5 w-1.5 rounded-full ${pill.dot}`} />
                        {statusLabel(p.status)}
                      </span>
                    </td>
                    <td className="px-4 py-3.5 font-mono text-xs text-ink-600 whitespace-nowrap">
                      {p.plannerUserId || "—"}
                    </td>
                    <td className="px-4 py-3.5 text-right whitespace-nowrap">
                      <Link
                        href={`/operations/dispatch/plans/${p.id}`}
                        className="inline-flex items-center gap-1 rounded-md border border-line px-3 py-1.5 text-xs font-medium text-ink-600 hover:bg-surface-2"
                      >
                        <Eye className="h-3.5 w-3.5" />
                        View
                      </Link>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        )}
      </div>
    </div>
  );
}