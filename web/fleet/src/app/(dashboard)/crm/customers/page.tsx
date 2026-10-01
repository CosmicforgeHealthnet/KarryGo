"use client";

import { useState, useEffect, useMemo, useCallback } from "react";
import { useRouter } from "next/navigation";
import {
  Users,
  Search,
  SlidersHorizontal,
  X,
  Clock,
  CheckCircle2,
  AlertCircle,
  ArrowRight,
  Building2,
  Plus,
  RefreshCw,
} from "lucide-react";
import EmptyState from "@/components/EmptyState";
import { crmService } from "@/lib/services/crmService";
import type {
  CustomerView,
  CustomerStatus,
  CustomerType,
} from "@/lib/types/crm";
import {
  customerStatusLabel,
  customerStatusTone,
  customerTypeLabel,
  riskStatusLabel,
  riskStatusTone,
  initialsOf,
  timeAgo,
} from "@/lib/utils/crmLabels";

const STATUS_FILTERS: Array<{ value: "ALL" | CustomerStatus; label: string }> = [
  { value: "ALL", label: "All" },
  { value: "PROSPECT", label: "Prospect" },
  { value: "ACTIVE", label: "Active" },
  { value: "ON_HOLD", label: "On hold" },
  { value: "SUSPENDED", label: "Suspended" },
  { value: "INACTIVE", label: "Inactive" },
  { value: "ARCHIVED", label: "Archived" },
];

const TYPE_FILTERS: Array<{ value: "ALL" | CustomerType; label: string }> = [
  { value: "ALL", label: "All" },
  { value: "BUSINESS", label: "Business" },
  { value: "INDIVIDUAL", label: "Individual" },
  { value: "GOVERNMENT", label: "Government" },
  { value: "NONPROFIT", label: "Non-profit" },
  { value: "INTERNAL_BUSINESS_UNIT", label: "Internal unit" },
  { value: "OTHER", label: "Other" },
];

export default function CustomerDirectoryPage() {
  const router = useRouter();

  const [search, setSearch] = useState("");
  const [statusFilter, setStatusFilter] = useState<"ALL" | CustomerStatus>("ALL");
  const [typeFilter, setTypeFilter] = useState<"ALL" | CustomerType>("ALL");
  const [showFilters, setShowFilters] = useState(false);

  const [data, setData] = useState<CustomerView[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [refreshing, setRefreshing] = useState(false);

  const fetchCustomers = useCallback(
    async (opts: { silent?: boolean } = {}) => {
      if (opts.silent) setRefreshing(true);
      else setLoading(true);
      setError(null);
      try {
        const result = await crmService.listCustomers({
          status: statusFilter === "ALL" ? undefined : statusFilter,
          customerType: typeFilter === "ALL" ? undefined : typeFilter,
          search: search.trim() || undefined,
          limit: 100,
        });
        setData(result.data);
      } catch (err: unknown) {
        const message =
          (err as { response?: { data?: { error?: { message?: string } } } })
            ?.response?.data?.error?.message ??
          (err as Error)?.message ??
          "Failed to load customers.";
        setError(message);
      } finally {
        setLoading(false);
        setRefreshing(false);
      }
    },
    [statusFilter, typeFilter, search]
  );

  useEffect(() => {
    const t = setTimeout(() => {
      fetchCustomers();
    }, 250);
    return () => clearTimeout(t);
  }, [fetchCustomers]);

  const hasActiveFilters =
    statusFilter !== "ALL" || typeFilter !== "ALL" || search.trim() !== "";

  const clearFilters = () => {
    setSearch("");
    setStatusFilter("ALL");
    setTypeFilter("ALL");
  };

  const stats = useMemo(() => {
    const total = data.length;
    const active = data.filter((d) => d.status === "ACTIVE").length;
    const atRisk = data.filter(
      (d) => d.riskStatus === "HIGH" || d.riskStatus === "RESTRICTED"
    ).length;
    return { total, active, atRisk };
  }, [data]);

  if (loading && data.length === 0) {
    return (
      <div className="py-12">
        <div className="flex items-center justify-center">
          <div className="h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent" />
          <span className="ml-3 text-sm text-ink-600">Loading customers…</span>
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-5">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1>Customer Directory</h1>
          <p className="mt-1.5">
            Customer profiles, status, risk, account ownership, and recent activity.
          </p>
        </div>
        <div className="flex items-center gap-2.5">
          <button
            onClick={() => fetchCustomers({ silent: true })}
            disabled={refreshing}
            className="flex h-10 w-10 items-center justify-center rounded-lg border border-line bg-surface text-ink-600 hover:bg-surface-2 transition-colors disabled:opacity-50"
            aria-label="Refresh"
          >
            <RefreshCw className={`h-3.5 w-3.5 ${refreshing ? "animate-spin" : ""}`} />
          </button>
          <button
            onClick={() => router.push("/crm/customers/create")}
            className="flex h-10 items-center gap-1.5 rounded-lg bg-primary px-3.5 text-sm font-semibold text-white transition-colors hover:bg-primary-hover"
          >
            <Plus className="h-3.5 w-3.5" />
            Add Customer
          </button>
        </div>
      </div>

      {error && (
        <div className="rounded-lg border border-red/30 bg-red-bg px-4 py-2.5 text-sm text-red flex items-start justify-between gap-3">
          <span>{error}</span>
          <button
            onClick={() => fetchCustomers()}
            className="text-xs font-semibold underline whitespace-nowrap"
          >
            Retry
          </button>
        </div>
      )}

      <div className="grid grid-cols-2 gap-3.5 sm:grid-cols-4">
        <div className="rounded-lg border border-line bg-surface p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-primary-bg text-primary">
              <Users className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">Total</span>
          </div>
          <div className="mt-3 font-display text-[26px] font-bold tracking-[-0.02em] text-ink-950">
            {stats.total}
          </div>
        </div>
        <div className="rounded-lg border border-line bg-surface p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-green-bg text-green">
              <CheckCircle2 className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">Active</span>
          </div>
          <div className="mt-3 font-display text-[26px] font-bold tracking-[-0.02em] text-ink-950">
            {stats.active}
          </div>
        </div>
        <div className="rounded-lg border border-line bg-surface p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-red-bg text-red">
              <AlertCircle className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">At Risk</span>
          </div>
          <div className="mt-3 font-display text-[26px] font-bold tracking-[-0.02em] text-ink-950">
            {stats.atRisk}
          </div>
        </div>
        <div className="rounded-lg border border-line bg-surface p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-primary-bg text-primary">
              <Building2 className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">Filtered</span>
          </div>
          <div className="mt-3 font-display text-[26px] font-bold tracking-[-0.02em] text-ink-950">
            {data.length}
          </div>
        </div>
      </div>

      <div className="flex flex-wrap items-center gap-2.5">
        <div className="flex h-10 w-full items-center gap-2 rounded-lg border border-line bg-surface px-3 sm:max-w-xs">
          <Search className="h-3.5 w-3.5 flex-shrink-0 text-ink-500" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search by name…"
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
              {(statusFilter !== "ALL" ? 1 : 0) + (typeFilter !== "ALL" ? 1 : 0)}
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
          {data.length} {data.length === 1 ? "customer" : "customers"}
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
                {STATUS_FILTERS.map((s) => (
                  <button
                    key={s.value}
                    onClick={() => setStatusFilter(s.value)}
                    className={`rounded-full px-3 py-1 text-xs font-semibold transition-colors whitespace-nowrap ${
                      statusFilter === s.value
                        ? "bg-primary text-white"
                        : "bg-surface-2 text-ink-600 hover:bg-surface-3"
                    }`}
                  >
                    {s.label}
                  </button>
                ))}
              </div>
            </div>
            <div>
              <span className="mb-1.5 block text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">
                Type
              </span>
              <div className="flex flex-wrap gap-1.5">
                {TYPE_FILTERS.map((t) => (
                  <button
                    key={t.value}
                    onClick={() => setTypeFilter(t.value)}
                    className={`rounded-full px-3 py-1 text-xs font-semibold transition-colors whitespace-nowrap ${
                      typeFilter === t.value
                        ? "bg-primary text-white"
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
              className="rounded-md border border-line px-3.5 py-1.5 text-xs font-medium text-ink-600 hover:bg-surface-2 transition-colors whitespace-nowrap"
            >
              Clear all
            </button>
          </div>
        </div>
      )}

      {data.length === 0 ? (
        <div className="rounded-lg border border-line bg-surface">
          <EmptyState
            image="/images/vehicles-empty.png"
            title={
              hasActiveFilters
                ? "No customers match your filters"
                : "No customers yet"
            }
            description={
              hasActiveFilters
                ? "Try adjusting your search or clearing the filters."
                : "Add your first customer to start building your CRM."
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
                  onClick={() => router.push("/crm/customers/create")}
                  className="inline-flex items-center justify-center rounded-lg bg-primary-bg px-4 py-2 text-xs font-semibold text-primary transition-colors hover:bg-primary/10"
                >
                  Add customer
                </button>
              )
            }
          />
        </div>
      ) : (
        <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
          {data.map((item) => {
            const statusTone = customerStatusTone(item.status);
            const riskTone = riskStatusTone(item.riskStatus);
            return (
              <div
                key={item.id}
                onClick={() => router.push(`/crm/customers/${item.id}`)}
                className="group rounded-lg border border-line bg-surface p-4 transition-colors cursor-pointer hover:border-ink-400 hover:bg-surface-2"
              >
                <div className="flex items-start justify-between">
                  <div className="flex items-center gap-3 min-w-0">
                    <div className="flex h-11 w-11 flex-shrink-0 items-center justify-center rounded-full bg-primary text-base font-bold text-white">
                      {initialsOf(item.displayName || item.legalName)}
                    </div>
                    <div className="min-w-0">
                      <div className="flex items-center gap-2 min-w-0">
                        <span className="text-sm font-semibold text-ink-950 truncate">
                          {item.displayName || item.legalName}
                        </span>
                        <span className="font-mono text-[10px] text-ink-400 flex-shrink-0">
                          {item.id}
                        </span>
                      </div>
                      <div className="flex flex-wrap items-center gap-1.5 mt-1">
                        <span
                          className={`flex items-center gap-1 rounded-full px-2 py-0.5 text-[10px] font-semibold ${statusTone.bg} ${statusTone.text}`}
                        >
                          <span className={`h-1 w-1 rounded-full ${statusTone.dot}`} />
                          {customerStatusLabel(item.status)}
                        </span>
                        <span className="rounded-full bg-surface-2 px-2 py-0.5 text-[10px] font-medium text-ink-600">
                          {customerTypeLabel(item.customerType)}
                        </span>
                        <span
                          className={`flex items-center gap-1 rounded-full px-2 py-0.5 text-[9px] font-medium ${riskTone.bg} ${riskTone.text}`}
                        >
                          <span className={`h-1 w-1 rounded-full ${riskTone.dot}`} />
                          {riskStatusLabel(item.riskStatus)}
                        </span>
                      </div>
                    </div>
                  </div>
                </div>

                <div className="mt-3 grid grid-cols-2 gap-1.5 text-xs text-ink-500">
                  {item.branchId && (
                    <div className="flex items-center gap-1.5">
                      <Building2 className="h-3.5 w-3.5 flex-shrink-0 text-ink-400" />
                      <span className="truncate">{item.branchId}</span>
                    </div>
                  )}
                  <div className="flex items-center gap-1.5">
                    <span className="text-ink-400">Currency</span>
                    <span className="font-mono font-medium text-ink-700">
                      {item.defaultCurrency}
                    </span>
                  </div>
                </div>

                <div className="mt-2.5 flex items-center justify-between border-t border-line pt-2.5">
                  <div className="flex items-center gap-1.5 text-[10px] text-ink-400">
                    <Clock className="h-3.5 w-3.5" />
                    Created {timeAgo(item.createdAt)}
                  </div>
                  <div className="flex items-center gap-1 text-xs font-medium text-ink-400 transition-colors group-hover:text-primary">
                    View <ArrowRight className="h-3.5 w-3.5" />
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