"use client";

import { useEffect, useState, useCallback, useMemo } from "react";
import { useRouter } from "next/navigation";
import {
  PieChart,
  Search,
  RefreshCw,
  SlidersHorizontal,
  X,
  Plus,
} from "lucide-react";
import EmptyState from "@/components/EmptyState";
import { financeService } from "@/lib/services/financeService";
import type { BudgetRow, BudgetStatus } from "@/lib/types/finance";
import {
  budgetStatusLabel,
  budgetStatusTone,
  formatMinor,
  formatDate,
} from "@/lib/utils/financeLabels";

const STATUS_FILTERS: Array<{ value: "ALL" | BudgetStatus; label: string }> = [
  { value: "ALL", label: "All" },
  { value: "DRAFT", label: "Draft" },
  { value: "PENDING_APPROVAL", label: "Pending approval" },
  { value: "APPROVED", label: "Approved" },
  { value: "ACTIVE", label: "Active" },
  { value: "CLOSED", label: "Closed" },
  { value: "CANCELLED", label: "Cancelled" },
];

export default function BudgetsPage() {
  const router = useRouter();

  const [search, setSearch] = useState("");
  const [statusFilter, setStatusFilter] = useState<"ALL" | BudgetStatus>("ALL");
  const [showFilters, setShowFilters] = useState(false);

  const [data, setData] = useState<BudgetRow[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [refreshing, setRefreshing] = useState(false);

  const fetchBudgets = useCallback(async (opts: { silent?: boolean } = {}) => {
    if (opts.silent) setRefreshing(true);
    else setLoading(true);
    setError(null);
    try {
      const result = await financeService.budgets.list();
      setData(result.data);
    } catch (err: unknown) {
      const message =
        (err as { response?: { data?: { error?: { message?: string } } } })
          ?.response?.data?.error?.message ??
        (err as Error)?.message ??
        "Failed to load budgets.";
      setError(message);
    } finally {
      setLoading(false);
      setRefreshing(false);
    }
  }, []);

  useEffect(() => {
    fetchBudgets();
  }, [fetchBudgets]);

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase();
    return data.filter((item) => {
      if (statusFilter !== "ALL" && item.status !== statusFilter) return false;
      if (!q) return true;
      const haystack = [item.id, item.category, item.cost_centre, item.currency]
        .join(" ")
        .toLowerCase();
      return haystack.includes(q);
    });
  }, [data, search, statusFilter]);

  const hasActiveFilters = statusFilter !== "ALL" || search.trim() !== "";

  const clearFilters = () => {
    setSearch("");
    setStatusFilter("ALL");
  };

  const stats = useMemo(() => {
    const approved = data.filter(
      (b) => b.status === "APPROVED" || b.status === "ACTIVE"
    );
    const totalApproved = approved.reduce(
      (sum, b) => sum + b.approved_amount_minor,
      0
    );
    const totalActual = approved.reduce(
      (sum, b) => sum + b.actual_amount_projection,
      0
    );
    const totalCommitted = approved.reduce(
      (sum, b) => sum + b.committed_amount_projection,
      0
    );
    const utilization =
      totalApproved > 0 ? Math.round((totalActual / totalApproved) * 100) : 0;
    return { totalApproved, totalActual, totalCommitted, utilization };
  }, [data]);

  const currency = data[0]?.currency ?? "NGN";

  if (loading && data.length === 0) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent" />
        <p className="mt-3 text-sm text-ink-600">Loading budgets…</p>
      </div>
    );
  }

  return (
    <div className="space-y-5">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
            Budgets
          </h1>
          <p className="mt-1 text-sm text-ink-600">
            Cost-centre and category budgets with approved, committed and actual projections.
          </p>
        </div>
        <div className="flex items-center gap-2">
          <button
            onClick={() => fetchBudgets({ silent: true })}
            disabled={refreshing}
            className="flex h-9 w-9 items-center justify-center rounded-xl border border-line bg-surface text-ink-500 transition hover:bg-surface-2 disabled:opacity-50"
            aria-label="Refresh"
          >
            <RefreshCw className={`h-3.5 w-3.5 ${refreshing ? "animate-spin" : ""}`} />
          </button>
          <button
            onClick={() => router.push("/finance/budgets/create")}
            className="flex h-9 items-center gap-1.5 rounded-xl bg-primary px-3.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90"
          >
            <Plus className="h-3.5 w-3.5" />
            Create Budget
          </button>
        </div>
      </div>

      {error && (
        <div className="flex items-start justify-between gap-3 rounded-lg border border-red/30 bg-red-bg px-4 py-2.5 text-sm text-red">
          <span>{error}</span>
          <button
            onClick={() => fetchBudgets()}
            className="whitespace-nowrap text-xs font-semibold underline"
          >
            Retry
          </button>
        </div>
      )}

      <div className="grid grid-cols-2 gap-2.5 sm:grid-cols-4">
        <div className="rounded-xl border border-line bg-surface p-3.5">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">
            Approved
          </div>
          <div className="text-lg font-bold text-ink-950">
            {formatMinor(stats.totalApproved, currency)}
          </div>
        </div>
        <div className="rounded-xl border border-line bg-surface p-3.5">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">
            Committed
          </div>
          <div className="text-lg font-bold text-amber">
            {formatMinor(stats.totalCommitted, currency)}
          </div>
        </div>
        <div className="rounded-xl border border-line bg-surface p-3.5">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">
            Actual
          </div>
          <div className="text-lg font-bold text-ink-950">
            {formatMinor(stats.totalActual, currency)}
          </div>
        </div>
        <div className="rounded-xl border border-line bg-surface p-3.5">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">
            Utilization
          </div>
          <div className="text-lg font-bold text-primary">{stats.utilization}%</div>
        </div>
      </div>

      <div className="flex flex-wrap items-center gap-2">
        <div className="flex h-11 min-w-[200px] flex-1 items-center gap-2.5 rounded-xl border border-line bg-surface px-4">
          <Search className="h-4 w-4 text-ink-400" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search by category, cost centre, or currency…"
            className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-400"
          />
        </div>

        <button
          onClick={() => setShowFilters(!showFilters)}
          className={`flex h-11 items-center gap-1.5 rounded-xl border px-3.5 text-sm font-medium transition ${
            showFilters || hasActiveFilters
              ? "border-primary bg-primary-bg text-primary"
              : "border-line bg-surface text-ink-600 hover:bg-surface-2"
          }`}
        >
          <SlidersHorizontal className="h-4 w-4" />
          Filters
          {hasActiveFilters && (
            <span className="rounded-full bg-primary px-1.5 py-0.5 text-[9px] text-white">
              {statusFilter !== "ALL" ? 1 : 0}
            </span>
          )}
        </button>

        {hasActiveFilters && (
          <button
            onClick={clearFilters}
            className="flex h-11 items-center gap-1 rounded-xl border border-line bg-surface px-3 text-sm font-medium text-ink-500 transition hover:bg-surface-2"
          >
            <X className="h-4 w-4" />
            Clear
          </button>
        )}
      </div>

      {showFilters && (
        <div className="rounded-xl border border-line bg-surface p-3.5">
          <div className="flex flex-wrap items-center gap-6">
            <div>
              <div className="mb-1 text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">
                Status
              </div>
              <div className="flex flex-wrap gap-1.5">
                {STATUS_FILTERS.map((s) => (
                  <button
                    key={s.value}
                    onClick={() => setStatusFilter(s.value)}
                    className={`rounded-full px-3 py-1 text-[11px] font-semibold transition ${
                      statusFilter === s.value
                        ? "bg-primary-bg text-primary ring-2 ring-primary/30"
                        : "bg-surface-2 text-ink-600 hover:bg-surface-3"
                    }`}
                  >
                    {s.label}
                  </button>
                ))}
              </div>
            </div>
            <button
              onClick={clearFilters}
              className="rounded-lg border border-line px-3.5 py-1.5 text-[11px] font-medium text-ink-600 transition hover:bg-surface-2"
            >
              Clear all
            </button>
          </div>
        </div>
      )}

      {filtered.length === 0 ? (
        <div className="rounded-lg border border-line bg-surface">
          <EmptyState
            image="/images/reports-empty.png"
            title={
              hasActiveFilters
                ? "No budgets match your filters"
                : "No budgets yet"
            }
            description={
              hasActiveFilters
                ? "Try adjusting your search or clearing the filters."
                : "Create your first budget to get started."
            }
            action={
              hasActiveFilters ? (
                <button
                  onClick={clearFilters}
                  className="inline-flex items-center justify-center rounded-lg bg-primary-bg px-4 py-2 text-xs font-semibold text-primary transition-colors hover:bg-primary/10"
                >
                  Clear filters
                </button>
              ) : (
                <button
                  onClick={() => router.push("/finance/budgets/create")}
                  className="inline-flex items-center justify-center rounded-lg bg-primary-bg px-4 py-2 text-xs font-semibold text-primary transition-colors hover:bg-primary/10"
                >
                  Create budget
                </button>
              )
            }
          />
        </div>
      ) : (
        <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
          {filtered.map((item) => {
            const statusTone = budgetStatusTone(item.status);
            const utilization =
              item.approved_amount_minor > 0
                ? Math.round(
                    (item.actual_amount_projection /
                      item.approved_amount_minor) *
                      100
                  )
                : 0;
            return (
              <div
                key={item.id}
                onClick={() => router.push(`/finance/budgets/${item.id}`)}
                className="group cursor-pointer rounded-xl border border-line bg-surface p-4 transition hover:shadow-md"
              >
                <div className="flex items-start justify-between">
                  <div className="flex items-center gap-3">
                    <div
                      className={`flex h-11 w-11 flex-shrink-0 items-center justify-center rounded-xl ${statusTone.bg} ${statusTone.text}`}
                    >
                      <PieChart className="h-5 w-5" />
                    </div>
                    <div>
                      <div className="flex items-center gap-2">
                        <span className="text-[15px] font-semibold text-ink-950">
                          {item.category}
                        </span>
                        <span
                          className={`flex items-center gap-1 rounded-full px-2 py-0.5 text-[10px] font-semibold ${statusTone.bg} ${statusTone.text}`}
                        >
                          <span
                            className={`h-1 w-1 rounded-full ${statusTone.dot}`}
                          />
                          {budgetStatusLabel(item.status)}
                        </span>
                      </div>
                      <div className="mt-0.5 flex items-center gap-1.5 text-[12px] text-ink-500">
                        <span>{item.cost_centre}</span>
                        <span className="text-ink-300">·</span>
                        <span className="font-mono">
                          {formatDate(item.period_start)} – {formatDate(item.period_end)}
                        </span>
                      </div>
                    </div>
                  </div>
                </div>

                <div className="mt-3 grid grid-cols-2 gap-1.5 text-[12px] text-ink-500">
                  <div>
                    <span className="text-ink-400">Approved: </span>
                    <span className="font-semibold text-ink-950">
                      {formatMinor(item.approved_amount_minor, item.currency)}
                    </span>
                  </div>
                  <div className="text-right">
                    <span className="text-ink-400">Actual: </span>
                    <span className="font-semibold text-ink-950">
                      {formatMinor(item.actual_amount_projection, item.currency)}
                    </span>
                  </div>
                  <div className="col-span-2">
                    <span className="text-ink-400">Committed: </span>
                    <span className="font-semibold text-amber">
                      {formatMinor(
                        item.committed_amount_projection,
                        item.currency
                      )}
                    </span>
                  </div>
                </div>

                <div className="mt-3 border-t border-line pt-2.5">
                  <div className="flex items-center justify-between text-[12px]">
                    <span className="text-ink-500">Utilization</span>
                    <span
                      className={`font-semibold ${
                        utilization >= 90
                          ? "text-red"
                          : utilization >= 70
                            ? "text-amber"
                            : "text-green"
                      }`}
                    >
                      {utilization}%
                    </span>
                  </div>
                  <div className="mt-1 h-1.5 w-full overflow-hidden rounded-full bg-surface-3">
                    <div
                      className={`h-full rounded-full transition-all ${
                        utilization >= 90
                          ? "bg-red"
                          : utilization >= 70
                            ? "bg-amber"
                            : "bg-green"
                      }`}
                      style={{ width: `${Math.min(utilization, 100)}%` }}
                    />
                  </div>
                </div>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}