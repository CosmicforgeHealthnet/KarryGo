"use client";

import { useEffect, useState, useCallback, useMemo } from "react";
import { useRouter } from "next/navigation";
import {
  Award,
  Search,
  RefreshCw,
  SlidersHorizontal,
  X,
  Clock,
  CheckCircle2,
  ArrowRight,
  Building2,
  Target,
  Calendar,
  Gauge,
  Plus,
} from "lucide-react";
import EmptyState from "@/components/EmptyState";
import { crmService } from "@/lib/services/crmService";
import type { SLAView, SLAStatus } from "@/lib/types/crm";
import {
  formatDate,
  slaMetricLabel,
  slaStatusLabel,
  slaStatusTone,
  slaUnitLabel,
  slaWindowLabel,
  timeAgo,
} from "@/lib/utils/crmLabels";

const STATUS_FILTERS: Array<{ value: "ALL" | SLAStatus; label: string }> = [
  { value: "ALL", label: "All" },
  { value: "DRAFT", label: "Draft" },
  { value: "ACTIVE", label: "Active" },
  { value: "SUSPENDED", label: "Suspended" },
  { value: "EXPIRED", label: "Expired" },
  { value: "TERMINATED", label: "Terminated" },
  { value: "SUPERSEDED", label: "Superseded" },
];

export default function SLAsPage() {
  const router = useRouter();

  const [search, setSearch] = useState("");
  const [statusFilter, setStatusFilter] = useState<"ALL" | SLAStatus>("ALL");
  const [showFilters, setShowFilters] = useState(false);

  const [data, setData] = useState<SLAView[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [refreshing, setRefreshing] = useState(false);

  const fetchSLAs = useCallback(
    async (opts: { silent?: boolean } = {}) => {
      if (opts.silent) setRefreshing(true);
      else setLoading(true);
      setError(null);
      try {
        const result = await crmService.listSLAs({
          status: statusFilter === "ALL" ? undefined : statusFilter,
          limit: 100,
        });
        setData(result.data);
      } catch (err: unknown) {
        const message =
          (err as { response?: { data?: { error?: { message?: string } } } })
            ?.response?.data?.error?.message ??
          (err as Error)?.message ??
          "Failed to load SLAs.";
        setError(message);
      } finally {
        setLoading(false);
        setRefreshing(false);
      }
    },
    [statusFilter]
  );

  useEffect(() => {
    fetchSLAs();
  }, [fetchSLAs]);

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase();
    if (!q) return data;
    return data.filter((item) => {
      const haystack = [
        item.customerId,
        item.contractId ?? "",
        slaMetricLabel(item.metricCode),
      ]
        .join(" ")
        .toLowerCase();
      return haystack.includes(q);
    });
  }, [data, search]);

  const hasActiveFilters = statusFilter !== "ALL" || search.trim() !== "";

  const clearFilters = () => {
    setSearch("");
    setStatusFilter("ALL");
  };

  const stats = useMemo(() => {
    const total = data.length;
    const active = data.filter((d) => d.status === "ACTIVE").length;
    const draft = data.filter((d) => d.status === "DRAFT").length;
    const withContract = data.filter((d) => d.contractId).length;
    return { total, active, draft, withContract };
  }, [data]);

  if (loading && data.length === 0) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent" />
        <p className="mt-3 text-sm text-ink-600">Loading SLAs…</p>
      </div>
    );
  }

  return (
    <div className="space-y-5">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
            Service-level Agreements
          </h1>
          <p className="mt-1 text-sm text-ink-600">
            Measurable service commitments linked to contracts and customers.
          </p>
        </div>
        <div className="flex items-center gap-2">
          <button
            onClick={() => fetchSLAs({ silent: true })}
            disabled={refreshing}
            className="flex h-9 w-9 items-center justify-center rounded-xl border border-line bg-surface text-ink-500 transition hover:bg-surface-2 disabled:opacity-50"
            aria-label="Refresh"
          >
            <RefreshCw className={`h-3.5 w-3.5 ${refreshing ? "animate-spin" : ""}`} />
          </button>
          <button
            onClick={() => router.push("/crm/slas/create")}
            className="flex h-9 items-center gap-1.5 rounded-xl bg-primary px-3.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90"
          >
            <Plus className="h-3.5 w-3.5" />
            Create SLA
          </button>
        </div>
      </div>

      {error && (
        <div className="flex items-start justify-between gap-3 rounded-lg border border-red/30 bg-red-bg px-4 py-2.5 text-sm text-red">
          <span>{error}</span>
          <button
            onClick={() => fetchSLAs()}
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
            With contract
          </div>
          <div className="text-lg font-bold text-ink-950">
            {stats.withContract}
          </div>
        </div>
      </div>

      <div className="flex flex-wrap items-center gap-2">
        <div className="flex h-11 min-w-[200px] flex-1 items-center gap-2.5 rounded-xl border border-line bg-surface px-4">
          <Search className="h-4 w-4 text-ink-400" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search by customer, contract, or metric…"
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
                ? "No SLAs match your filters"
                : "No SLAs yet"
            }
            description={
              hasActiveFilters
                ? "Try adjusting your search or clearing the filters."
                : "Create your first SLA to get started."
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
                  onClick={() => router.push("/crm/slas/create")}
                  className="inline-flex items-center justify-center rounded-lg bg-primary-bg px-4 py-2 text-xs font-semibold text-primary transition-colors hover:bg-primary/10"
                >
                  Create SLA
                </button>
              )
            }
          />
        </div>
      ) : (
        <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
          {filtered.map((item) => {
            const statusTone = slaStatusTone(item.status);
            return (
              <div
                key={item.id}
                onClick={() => router.push(`/crm/slas/${item.id}`)}
                className="group cursor-pointer rounded-xl border border-line bg-surface p-4 transition hover:shadow-md"
              >
                <div className="flex items-start justify-between">
                  <div className="flex items-center gap-3 min-w-0">
                    <div
                      className={`flex h-11 w-11 flex-shrink-0 items-center justify-center rounded-xl ${statusTone.bg} ${statusTone.text}`}
                    >
                      <Award className="h-5 w-5" />
                    </div>
                    <div className="min-w-0">
                      <div className="flex flex-wrap items-center gap-2">
                        <span className="truncate text-[15px] font-semibold text-ink-950">
                          {slaMetricLabel(item.metricCode)}
                        </span>
                        <span
                          className={`flex items-center gap-1 rounded-full px-2 py-0.5 text-[10px] font-semibold ${statusTone.bg} ${statusTone.text}`}
                        >
                          <span className={`h-1 w-1 rounded-full ${statusTone.dot}`} />
                          {slaStatusLabel(item.status)}
                        </span>
                      </div>
                      <div className="mt-0.5 flex flex-wrap items-center gap-1.5 text-[12px] text-ink-500">
                        <span className="flex items-center gap-1">
                          <Building2 className="h-3.5 w-3.5" />
                          <span className="font-mono">{item.customerId}</span>
                        </span>
                        {item.contractId && (
                          <>
                            <span className="text-ink-300">·</span>
                            <span className="font-mono text-[11px]">
                              {item.contractId}
                            </span>
                          </>
                        )}
                      </div>
                    </div>
                  </div>
                </div>

                <div className="mt-3 grid grid-cols-2 gap-1.5 text-[12px] text-ink-500">
                  <div className="flex items-center gap-1.5">
                    <Target className="h-3.5 w-3.5 text-ink-400" />
                    <span className="font-semibold text-ink-950">
                      {item.targetValue} {slaUnitLabel(item.measurementUnit)}
                    </span>
                  </div>
                  <div className="flex items-center gap-1.5">
                    <Gauge className="h-3.5 w-3.5 text-ink-400" />
                    <span>{slaWindowLabel(item.measurementWindow)}</span>
                  </div>
                  <div className="col-span-2 flex items-center gap-1.5">
                    <Calendar className="h-3.5 w-3.5 text-ink-400" />
                    <span>
                      {formatDate(item.effectiveFrom)}
                      {item.effectiveTo ? ` → ${formatDate(item.effectiveTo)}` : " → ongoing"}
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