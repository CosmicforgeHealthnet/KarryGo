"use client";

import { useEffect, useState, useCallback, useMemo } from "react";
import { useRouter } from "next/navigation";
import {
  Scale,
  Search,
  RefreshCw,
  SlidersHorizontal,
  X,
  Plus,
} from "lucide-react";
import EmptyState from "@/components/EmptyState";
import { financeService } from "@/lib/services/financeService";
import type {
  CompensationRuleRow,
  CompensationRuleStatus,
  CompensationRuleType,
} from "@/lib/types/finance";
import {
  compensationRuleStatusLabel,
  compensationRuleStatusTone,
  compensationRuleTypeLabel,
  compensationRuleTypeTone,
  formatDate,
  formatMinor,
  timeAgo,
} from "@/lib/utils/financeLabels";

const STATUS_FILTERS: Array<{
  value: "ALL" | CompensationRuleStatus;
  label: string;
}> = [
  { value: "ALL", label: "All" },
  { value: "DRAFT", label: "Draft" },
  { value: "PENDING_APPROVAL", label: "Pending approval" },
  { value: "APPROVED", label: "Approved" },
  { value: "ACTIVE", label: "Active" },
  { value: "INACTIVE", label: "Inactive" },
  { value: "ARCHIVED", label: "Archived" },
];

const TYPE_FILTERS: Array<{
  value: "ALL" | CompensationRuleType;
  label: string;
}> = [
  { value: "ALL", label: "All" },
  { value: "SALARY", label: "Salary" },
  { value: "PER_JOB", label: "Per job" },
  { value: "PER_SHIFT", label: "Per shift" },
  { value: "COMMISSION", label: "Commission" },
  { value: "ALLOWANCE", label: "Allowance" },
  { value: "OVERTIME", label: "Overtime" },
  { value: "BONUS", label: "Bonus" },
  { value: "MANUAL_ADJUSTMENT", label: "Manual" },
  { value: "DEDUCTION", label: "Deduction" },
];

export default function CompensationRulesPage() {
  const router = useRouter();

  const [search, setSearch] = useState("");
  const [statusFilter, setStatusFilter] = useState<
    "ALL" | CompensationRuleStatus
  >("ALL");
  const [typeFilter, setTypeFilter] = useState<"ALL" | CompensationRuleType>(
    "ALL"
  );
  const [showFilters, setShowFilters] = useState(false);

  const [data, setData] = useState<CompensationRuleRow[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [refreshing, setRefreshing] = useState(false);

  const fetchRules = useCallback(async (opts: { silent?: boolean } = {}) => {
    if (opts.silent) setRefreshing(true);
    else setLoading(true);
    setError(null);
    try {
      const result = await financeService.compensationRules.list();
      setData(result.data);
    } catch (err: unknown) {
      const message =
        (err as { response?: { data?: { error?: { message?: string } } } })
          ?.response?.data?.error?.message ??
        (err as Error)?.message ??
        "Failed to load compensation rules.";
      setError(message);
    } finally {
      setLoading(false);
      setRefreshing(false);
    }
  }, []);

  useEffect(() => {
    fetchRules();
  }, [fetchRules]);

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase();
    return data.filter((item) => {
      if (statusFilter !== "ALL" && item.status !== statusFilter) return false;
      if (typeFilter !== "ALL" && item.rule_type !== typeFilter) return false;
      if (!q) return true;
      const haystack = [item.id, item.name, item.currency]
        .join(" ")
        .toLowerCase();
      return haystack.includes(q);
    });
  }, [data, search, statusFilter, typeFilter]);

  const hasActiveFilters =
    statusFilter !== "ALL" || typeFilter !== "ALL" || search.trim() !== "";

  const clearFilters = () => {
    setSearch("");
    setStatusFilter("ALL");
    setTypeFilter("ALL");
  };

  const stats = useMemo(() => {
    const total = data.length;
    const active = data.filter(
      (r) => r.status === "ACTIVE" || r.status === "APPROVED"
    ).length;
    const draft = data.filter((r) => r.status === "DRAFT").length;
    const inactive = data.filter((r) => r.status === "INACTIVE").length;
    return { total, active, draft, inactive };
  }, [data]);

  if (loading && data.length === 0) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent" />
        <p className="mt-3 text-sm text-ink-600">
          Loading compensation rules…
        </p>
      </div>
    );
  }

  return (
    <div className="space-y-5">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
            Compensation Rules
          </h1>
          <p className="mt-1 text-sm text-ink-600">
            Deterministic rule definitions that drive payroll computation.
          </p>
        </div>
        <div className="flex items-center gap-2">
          <button
            onClick={() => fetchRules({ silent: true })}
            disabled={refreshing}
            className="flex h-9 w-9 items-center justify-center rounded-xl border border-line bg-surface text-ink-500 transition hover:bg-surface-2 disabled:opacity-50"
            aria-label="Refresh"
          >
            <RefreshCw className={`h-3.5 w-3.5 ${refreshing ? "animate-spin" : ""}`} />
          </button>
          <button
            onClick={() => router.push("/finance/compensation-rules/create")}
            className="flex h-9 items-center gap-1.5 rounded-xl bg-primary px-3.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90"
          >
            <Plus className="h-3.5 w-3.5" />
            Create Rule
          </button>
        </div>
      </div>

      {error && (
        <div className="flex items-start justify-between gap-3 rounded-lg border border-red/30 bg-red-bg px-4 py-2.5 text-sm text-red">
          <span>{error}</span>
          <button
            onClick={() => fetchRules()}
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
            Active
          </div>
          <div className="text-lg font-bold text-green">{stats.active}</div>
        </div>
        <div className="rounded-xl border border-line bg-surface p-3.5">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">
            Draft
          </div>
          <div className="text-lg font-bold text-primary">{stats.draft}</div>
        </div>
        <div className="rounded-xl border border-line bg-surface p-3.5">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">
            Inactive
          </div>
          <div className="text-lg font-bold text-ink-500">{stats.inactive}</div>
        </div>
      </div>

      <div className="flex flex-wrap items-center gap-2">
        <div className="flex h-11 min-w-[200px] flex-1 items-center gap-2.5 rounded-xl border border-line bg-surface px-4">
          <Search className="h-4 w-4 text-ink-400" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search by name or currency…"
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
              {(statusFilter !== "ALL" ? 1 : 0) + (typeFilter !== "ALL" ? 1 : 0)}
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
            <div>
              <div className="mb-1 text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">
                Type
              </div>
              <div className="flex flex-wrap gap-1.5">
                {TYPE_FILTERS.map((t) => (
                  <button
                    key={t.value}
                    onClick={() => setTypeFilter(t.value)}
                    className={`rounded-full px-3 py-1 text-[11px] font-semibold transition ${
                      typeFilter === t.value
                        ? "bg-primary-bg text-primary ring-2 ring-primary/30"
                        : "bg-surface-2 text-ink-600 hover:bg-surface-3"
                    }`}
                  >
                    {t.label}
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
                ? "No rules match your filters"
                : "No compensation rules yet"
            }
            description={
              hasActiveFilters
                ? "Try adjusting your search or clearing the filters."
                : "Create your first rule to get started."
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
                  onClick={() =>
                    router.push("/finance/compensation-rules/create")
                  }
                  className="inline-flex items-center justify-center rounded-lg bg-primary-bg px-4 py-2 text-xs font-semibold text-primary transition-colors hover:bg-primary/10"
                >
                  Create rule
                </button>
              )
            }
          />
        </div>
      ) : (
        <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
          {filtered.map((item) => {
            const statusTone = compensationRuleStatusTone(item.status);
            const typeTone = compensationRuleTypeTone(item.rule_type);
            const isDeduction = item.rule_type === "DEDUCTION";
            return (
              <div
                key={item.id}
                onClick={() =>
                  router.push(`/finance/compensation-rules/${item.id}`)
                }
                className="group cursor-pointer rounded-xl border border-line bg-surface p-4 transition hover:shadow-md"
              >
                <div className="flex items-start justify-between">
                  <div className="flex min-w-0 items-center gap-3">
                    <div
                      className={`flex h-11 w-11 flex-shrink-0 items-center justify-center rounded-xl ${typeTone.bg} ${typeTone.text}`}
                    >
                      <Scale className="h-5 w-5" />
                    </div>
                    <div className="min-w-0">
                      <div className="flex flex-wrap items-center gap-2">
                        <span className="truncate text-[15px] font-semibold text-ink-950">
                          {item.name}
                        </span>
                        <span
                          className={`flex items-center gap-1 rounded-full px-2 py-0.5 text-[10px] font-semibold ${statusTone.bg} ${statusTone.text}`}
                        >
                          <span
                            className={`h-1 w-1 rounded-full ${statusTone.dot}`}
                          />
                          {compensationRuleStatusLabel(item.status)}
                        </span>
                      </div>
                      <div className="mt-0.5 flex flex-wrap items-center gap-1.5 text-[12px] text-ink-500">
                        <span
                          className={`rounded-full px-2 py-0.5 text-[10px] font-medium ${typeTone.bg} ${typeTone.text}`}
                        >
                          {compensationRuleTypeLabel(item.rule_type)}
                        </span>
                        <span className="text-ink-300">·</span>
                        <span className="font-mono">{item.currency}</span>
                        <span className="text-ink-300">·</span>
                        <span>v{item.version}</span>
                      </div>
                    </div>
                  </div>
                </div>

                <div className="mt-3 grid grid-cols-2 gap-1.5 text-[12px] text-ink-500">
                  <div>
                    <span className="text-ink-400">Amount: </span>
                    <span
                      className={`font-semibold ${
                        isDeduction ? "text-red" : "text-ink-950"
                      }`}
                    >
                      {formatMinor(
                        item.calculation_configuration.amountMinor,
                        item.currency
                      )}
                    </span>
                  </div>
                  <div className="text-right">
                    <span className="text-ink-400">From: </span>
                    <span className="font-semibold text-ink-950">
                      {formatDate(item.effective_from)}
                    </span>
                  </div>
                  <div className="col-span-2">
                    <span className="text-ink-400">Until: </span>
                    <span className="font-semibold text-ink-950">
                      {item.effective_until
                        ? formatDate(item.effective_until)
                        : "Open-ended"}
                    </span>
                  </div>
                </div>

                <div className="mt-2.5 flex items-center justify-between border-t border-line pt-2.5">
                  <div className="flex items-center gap-1.5 text-[10px] text-ink-400">
                    <span>Created {timeAgo(item.created_at)}</span>
                  </div>
                  <div className="flex items-center gap-1 text-xs font-medium text-ink-400 transition-colors group-hover:text-primary">
                    View →
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