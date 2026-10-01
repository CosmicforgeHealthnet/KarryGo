"use client";

import { useEffect, useState, useCallback, useMemo } from "react";
import { useRouter } from "next/navigation";
import {
  Users,
  Search,
  RefreshCw,
  SlidersHorizontal,
  X,
  Clock,
  ArrowRight,
  Building2,
  Target,
  Plus,
} from "lucide-react";
import EmptyState from "@/components/EmptyState";
import { crmService } from "@/lib/services/crmService";
import type { OpportunityView, OpportunityStage } from "@/lib/types/crm";
import {
  formatMoney,
  opportunityStageLabel,
  opportunityStageTone,
  probabilityPercent,
  timeAgo,
} from "@/lib/utils/crmLabels";

const LEAD_STAGES: OpportunityStage[] = [
  "LEAD",
  "QUALIFIED",
  "NEEDS_ANALYSIS",
  "PROPOSAL",
  "NEGOTIATION",
];

const STAGE_FILTERS: Array<{ value: "ALL" | OpportunityStage; label: string }> = [
  { value: "ALL", label: "All" },
  { value: "LEAD", label: "Lead" },
  { value: "QUALIFIED", label: "Qualified" },
  { value: "NEEDS_ANALYSIS", label: "Needs analysis" },
  { value: "PROPOSAL", label: "Proposal" },
  { value: "NEGOTIATION", label: "Negotiation" },
];

export default function LeadsPage() {
  const router = useRouter();

  const [search, setSearch] = useState("");
  const [stageFilter, setStageFilter] = useState<"ALL" | OpportunityStage>("ALL");
  const [showFilters, setShowFilters] = useState(false);

  const [data, setData] = useState<OpportunityView[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [refreshing, setRefreshing] = useState(false);

  const fetchLeads = useCallback(
    async (opts: { silent?: boolean } = {}) => {
      if (opts.silent) setRefreshing(true);
      else setLoading(true);
      setError(null);
      try {
        const result = await crmService.listOpportunities({ limit: 100 });
        const leads = result.data.filter((o) => LEAD_STAGES.includes(o.stage));
        setData(leads);
      } catch (err: unknown) {
        const message =
          (err as { response?: { data?: { error?: { message?: string } } } })
            ?.response?.data?.error?.message ??
          (err as Error)?.message ??
          "Failed to load leads.";
        setError(message);
      } finally {
        setLoading(false);
        setRefreshing(false);
      }
    },
    []
  );

  useEffect(() => {
    fetchLeads();
  }, [fetchLeads]);

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase();
    return data.filter((item) => {
      if (stageFilter !== "ALL" && item.stage !== stageFilter) return false;
      if (!q) return true;
      const haystack = [
        item.title,
        item.customerId,
        item.accountOwnerUserId ?? "",
      ]
        .join(" ")
        .toLowerCase();
      return haystack.includes(q);
    });
  }, [data, search, stageFilter]);

  const hasActiveFilters = stageFilter !== "ALL" || search.trim() !== "";

  const clearFilters = () => {
    setSearch("");
    setStageFilter("ALL");
  };

  const stats = useMemo(() => {
    const total = data.length;
    const lead = data.filter((d) => d.stage === "LEAD").length;
    const qualified = data.filter((d) => d.stage === "QUALIFIED").length;
    const pipeline = data.reduce(
      (sum, d) => sum + (d.expectedValue?.amountMinor ?? 0),
      0
    );
    return { total, lead, qualified, pipeline };
  }, [data]);

  if (loading && data.length === 0) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent" />
        <p className="mt-3 text-sm text-ink-600">Loading leads…</p>
      </div>
    );
  }

  const pipelineCurrency = data.find((d) => d.expectedValue)?.expectedValue?.currency;

  return (
    <div className="space-y-5">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
            Leads
          </h1>
          <p className="mt-1 text-sm text-ink-600">
            Early-stage sales opportunities in the pipeline.
          </p>
        </div>
        <div className="flex items-center gap-2">
          <button
            onClick={() => fetchLeads({ silent: true })}
            disabled={refreshing}
            className="flex h-9 w-9 items-center justify-center rounded-xl border border-line bg-surface text-ink-500 transition hover:bg-surface-2 disabled:opacity-50"
            aria-label="Refresh"
          >
            <RefreshCw className={`h-3.5 w-3.5 ${refreshing ? "animate-spin" : ""}`} />
          </button>
          <button
            onClick={() => router.push("/crm/opportunities/create")}
            className="flex h-9 items-center gap-1.5 rounded-xl bg-primary px-3.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90"
          >
            <Plus className="h-3.5 w-3.5" />
            Add Lead
          </button>
        </div>
      </div>

      {error && (
        <div className="flex items-start justify-between gap-3 rounded-lg border border-red/30 bg-red-bg px-4 py-2.5 text-sm text-red">
          <span>{error}</span>
          <button
            onClick={() => fetchLeads()}
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
            Lead
          </div>
          <div className="text-lg font-bold text-primary">{stats.lead}</div>
        </div>
        <div className="rounded-xl border border-line bg-surface p-3.5">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">
            Qualified
          </div>
          <div className="text-lg font-bold text-green">{stats.qualified}</div>
        </div>
        <div className="rounded-xl border border-line bg-surface p-3.5">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">
            Pipeline
          </div>
          <div className="text-lg font-bold text-ink-950">
            {pipelineCurrency
              ? formatMoney({ amountMinor: stats.pipeline, currency: pipelineCurrency })
              : "—"}
          </div>
        </div>
      </div>

      <div className="flex flex-wrap items-center gap-2">
        <div className="flex h-11 min-w-[200px] flex-1 items-center gap-2.5 rounded-xl border border-line bg-surface px-4">
          <Search className="h-4 w-4 text-ink-400" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search by title, customer, or owner…"
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
              {stageFilter !== "ALL" ? 1 : 0}
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
                Stage
              </div>
              <div className="flex flex-wrap gap-1.5">
                {STAGE_FILTERS.map((s) => (
                  <button
                    key={s.value}
                    onClick={() => setStageFilter(s.value)}
                    className={`rounded-full px-3 py-1 text-[11px] font-semibold transition ${
                      stageFilter === s.value
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
            image="/images/trips-empty.png"
            title={
              hasActiveFilters
                ? "No leads match your filters"
                : "No leads yet"
            }
            description={
              hasActiveFilters
                ? "Try adjusting your search or clearing the filters."
                : "Add your first lead to get started."
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
                  onClick={() => router.push("/crm/opportunities/create")}
                  className="inline-flex items-center justify-center rounded-lg bg-primary-bg px-4 py-2 text-xs font-semibold text-primary transition-colors hover:bg-primary/10"
                >
                  Add lead
                </button>
              )
            }
          />
        </div>
      ) : (
        <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
          {filtered.map((item) => {
            const stageTone = opportunityStageTone(item.stage);
            return (
              <div
                key={item.id}
                onClick={() => router.push(`/crm/opportunities/${item.id}`)}
                className="group cursor-pointer rounded-xl border border-line bg-surface p-4 transition hover:shadow-md"
              >
                <div className="flex items-start justify-between">
                  <div className="min-w-0 flex-1">
                    <div className="flex flex-wrap items-center gap-2">
                      <span className="truncate text-[15px] font-semibold text-ink-950">
                        {item.title}
                      </span>
                      <span
                        className={`flex items-center gap-1 rounded-full px-2 py-0.5 text-[10px] font-semibold ${stageTone.bg} ${stageTone.text}`}
                      >
                        <span className={`h-1 w-1 rounded-full ${stageTone.dot}`} />
                        {opportunityStageLabel(item.stage)}
                      </span>
                    </div>
                    <div className="mt-1 flex flex-wrap items-center gap-3 text-[12px] text-ink-500">
                      <span className="flex items-center gap-1.5">
                        <Building2 className="h-3.5 w-3.5" />
                        <span className="font-mono">{item.customerId}</span>
                      </span>
                      {item.accountOwnerUserId && (
                        <span className="flex items-center gap-1.5">
                          <Users className="h-3.5 w-3.5" />
                          {item.accountOwnerUserId}
                        </span>
                      )}
                    </div>
                  </div>
                </div>

                <div className="mt-3 grid grid-cols-2 gap-1.5 text-[12px] text-ink-500">
                  <div className="flex items-center gap-1.5">
                    <Target className="h-3.5 w-3.5 text-ink-400" />
                    <span className="font-semibold text-ink-950">
                      {probabilityPercent(item.probabilityBasisPoints)}
                    </span>
                  </div>
                  {item.expectedValue && (
                    <div className="text-right font-semibold text-ink-950">
                      {formatMoney(item.expectedValue)}
                    </div>
                  )}
                </div>

                {item.nextAction && (
                  <div className="mt-3 border-t border-line pt-2.5">
                    <div className="flex items-center gap-2 text-[12px]">
                      <span className="text-ink-500">Next:</span>
                      <span className="font-medium text-ink-950 truncate">
                        {item.nextAction}
                      </span>
                    </div>
                  </div>
                )}

                <div className="mt-2.5 flex items-center justify-between border-t border-line pt-2.5">
                  <div className="flex items-center gap-1.5 text-[10px] text-ink-400">
                    <Clock className="h-3.5 w-3.5" />
                    {timeAgo(item.createdAt)}
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