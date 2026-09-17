"use client";

import { useEffect, useState, useCallback, useMemo } from "react";
import { useRouter } from "next/navigation";
import {
  Users,
  Search,
  RefreshCw,
  SlidersHorizontal,
  X,
  Plus,
} from "lucide-react";
import EmptyState from "@/components/EmptyState";
import { financeService } from "@/lib/services/financeService";
import type { PayrollPeriodRow, PayrollStatus } from "@/lib/types/finance";
import {
  formatDate,
  formatMinor,
  payrollStatusLabel,
  payrollStatusTone,
  timeAgo,
} from "@/lib/utils/financeLabels";

const STATUS_FILTERS: Array<{ value: "ALL" | PayrollStatus; label: string }> = [
  { value: "ALL", label: "All" },
  { value: "DRAFT", label: "Draft" },
  { value: "CALCULATING", label: "Calculating" },
  { value: "CALCULATED", label: "Calculated" },
  { value: "PENDING_APPROVAL", label: "Pending approval" },
  { value: "APPROVED", label: "Approved" },
  { value: "POSTING", label: "Posting" },
  { value: "POSTED", label: "Posted" },
  { value: "PARTIALLY_POSTED", label: "Partially posted" },
  { value: "FAILED", label: "Failed" },
  { value: "REVERSED", label: "Reversed" },
  { value: "CLOSED", label: "Closed" },
];

export default function PayrollPeriodsPage() {
  const router = useRouter();

  const [search, setSearch] = useState("");
  const [statusFilter, setStatusFilter] = useState<"ALL" | PayrollStatus>("ALL");
  const [showFilters, setShowFilters] = useState(false);

  const [data, setData] = useState<PayrollPeriodRow[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [refreshing, setRefreshing] = useState(false);

  const fetchPeriods = useCallback(async (opts: { silent?: boolean } = {}) => {
    if (opts.silent) setRefreshing(true);
    else setLoading(true);
    setError(null);
    try {
      const result = await financeService.payrollPeriods.list();
      setData(result.data);
    } catch (err: unknown) {
      const message =
        (err as { response?: { data?: { error?: { message?: string } } } })
          ?.response?.data?.error?.message ??
        (err as Error)?.message ??
        "Failed to load payroll periods.";
      setError(message);
    } finally {
      setLoading(false);
      setRefreshing(false);
    }
  }, []);

  useEffect(() => {
    fetchPeriods();
  }, [fetchPeriods]);

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase();
    return data.filter((item) => {
      if (statusFilter !== "ALL" && item.status !== statusFilter) return false;
      if (!q) return true;
      const haystack = [
        item.id,
        item.currency,
        item.branch_id ?? "",
        item.period_start,
        item.period_end,
      ]
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
    const total = data.length;
    const calculated = data.filter((p) => p.status === "CALCULATED").length;
    const approved = data.filter((p) => p.status === "APPROVED").length;
    const posted = data.filter((p) => p.status === "POSTED").length;
    return { total, calculated, approved, posted };
  }, [data]);

  if (loading && data.length === 0) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent" />
        <p className="mt-3 text-sm text-ink-600">Loading payroll periods…</p>
      </div>
    );
  }

  return (
    <div className="space-y-5">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
            Payroll Periods
          </h1>
          <p className="mt-1 text-sm text-ink-600">
            Payroll periods with calculation, approval and posting state.
          </p>
        </div>
        <div className="flex items-center gap-2">
          <button
            onClick={() => fetchPeriods({ silent: true })}
            disabled={refreshing}
            className="flex h-9 w-9 items-center justify-center rounded-xl border border-line bg-surface text-ink-500 transition hover:bg-surface-2 disabled:opacity-50"
            aria-label="Refresh"
          >
            <RefreshCw className={`h-3.5 w-3.5 ${refreshing ? "animate-spin" : ""}`} />
          </button>
          <button
            onClick={() => router.push("/finance/payroll/create")}
            className="flex h-9 items-center gap-1.5 rounded-xl bg-primary px-3.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90"
          >
            <Plus className="h-3.5 w-3.5" />
            Create Period
          </button>
        </div>
      </div>

      {error && (
        <div className="flex items-start justify-between gap-3 rounded-lg border border-red/30 bg-red-bg px-4 py-2.5 text-sm text-red">
          <span>{error}</span>
          <button
            onClick={() => fetchPeriods()}
            className="whitespace-nowrap text-xs font-semibold underline"
          >
            Retry
          </button>
        </div>
      )}

      <div className="grid grid-cols-2 gap-2.5 sm:grid-cols-4">
        <div className="rounded-xl border border-line bg-surface p-3.5">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">
            Total
          </div>
          <div className="text-lg font-bold text-ink-950">{stats.total}</div>
        </div>
        <div className="rounded-xl border border-line bg-surface p-3.5">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">
            Calculated
          </div>
          <div className="text-lg font-bold text-amber">{stats.calculated}</div>
        </div>
        <div className="rounded-xl border border-line bg-surface p-3.5">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">
            Approved
          </div>
          <div className="text-lg font-bold text-green">{stats.approved}</div>
        </div>
        <div className="rounded-xl border border-line bg-surface p-3.5">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">
            Posted
          </div>
          <div className="text-lg font-bold text-green">{stats.posted}</div>
        </div>
      </div>

      <div className="flex flex-wrap items-center gap-2">
        <div className="flex h-11 min-w-[200px] flex-1 items-center gap-2.5 rounded-xl border border-line bg-surface px-4">
          <Search className="h-4 w-4 text-ink-400" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search by period, branch, or currency…"
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
                ? "No payroll periods match your filters"
                : "No payroll periods yet"
            }
            description={
              hasActiveFilters
                ? "Try adjusting your search or clearing the filters."
                : "Create your first period to get started."
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
                  onClick={() => router.push("/finance/payroll/create")}
                  className="inline-flex items-center justify-center rounded-lg bg-primary-bg px-4 py-2 text-xs font-semibold text-primary transition-colors hover:bg-primary/10"
                >
                  Create period
                </button>
              )
            }
          />
        </div>
      ) : (
        <div className="overflow-hidden rounded-2xl border border-line bg-surface">
          <div className="overflow-x-auto">
            <table className="w-full min-w-[900px] border-collapse text-sm">
              <thead>
                <tr className="bg-surface-2/70">
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">
                    Period
                  </th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">
                    Branch
                  </th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">
                    Status
                  </th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">
                    Calculation
                  </th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">
                    Version
                  </th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">
                    Updated
                  </th>
                </tr>
              </thead>
              <tbody>
                {filtered.map((item) => {
                  const statusTone = payrollStatusTone(item.status);
                  return (
                    <tr
                      key={item.id}
                      onClick={() => router.push(`/finance/payroll/${item.id}`)}
                      className="group cursor-pointer border-b border-line transition-colors hover:bg-surface-2"
                    >
                      <td className="px-4 py-3">
                        <div className="font-medium text-ink-950">
                          {formatDate(item.period_start)} – {formatDate(item.period_end)}
                        </div>
                        <div className="font-mono text-[11px] text-ink-500">
                          {item.id}
                        </div>
                      </td>
                      <td className="px-4 py-3">
                        <span className="font-mono text-[12px] text-ink-600">
                          {item.branch_id ?? "All"}
                        </span>
                      </td>
                      <td className="px-4 py-3">
                        <span
                          className={`inline-flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${statusTone.bg} ${statusTone.text}`}
                        >
                          <span className={`h-1 w-1 rounded-full ${statusTone.dot}`} />
                          {payrollStatusLabel(item.status)}
                        </span>
                      </td>
                      <td className="px-4 py-3 font-mono text-[12px] text-ink-600">
                        v{item.calculation_version}
                      </td>
                      <td className="px-4 py-3 text-ink-500">v{item.version}</td>
                      <td className="px-4 py-3 text-[12px] text-ink-500">
                        {timeAgo(item.updated_at)}
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        </div>
      )}
    </div>
  );
}