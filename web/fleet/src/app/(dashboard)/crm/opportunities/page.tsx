"use client";

import { useEffect, useState, useCallback, useMemo } from "react";
import { useRouter } from "next/navigation";
import {
  Target,
  Search,
  RefreshCw,
  SlidersHorizontal,
  X,
  Clock,
  CheckCircle2,
  AlertCircle,
  ArrowRight,
  Building2,
  Users,
  Zap,
  Plus,
} from "lucide-react";
import EmptyState from "@/components/EmptyState";
import { crmService } from "@/lib/services/crmService";
import type { OpportunityView, OpportunityStage } from "@/lib/types/crm";
import {
  formatDate,
  formatMoney,
  opportunityStageLabel,
  opportunityStageTone,
  probabilityPercent,
  timeAgo,
} from "@/lib/utils/crmLabels";

const STAGE_FILTERS: Array<{ value: "ALL" | OpportunityStage; label: string }> = [
  { value: "ALL", label: "All" },
  { value: "LEAD", label: "Lead" },
  { value: "QUALIFIED", label: "Qualified" },
  { value: "NEEDS_ANALYSIS", label: "Needs analysis" },
  { value: "PROPOSAL", label: "Proposal" },
  { value: "NEGOTIATION", label: "Negotiation" },
  { value: "WON", label: "Won" },
  { value: "LOST", label: "Lost" },
  { value: "ON_HOLD", label: "On hold" },
];

export default function OpportunitiesPage() {
  const router = useRouter();

  const [search, setSearch] = useState("");
  const [stageFilter, setStageFilter] = useState<"ALL" | OpportunityStage>("ALL");
  const [showFilters, setShowFilters] = useState(false);

  const [data, setData] = useState<OpportunityView[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [refreshing, setRefreshing] = useState(false);

  const fetchOpportunities = useCallback(
    async (opts: { silent?: boolean } = {}) => {
      if (opts.silent) setRefreshing(true);
      else setLoading(true);
      setError(null);
      try {
        const result = await crmService.listOpportunities({
          stage: stageFilter === "ALL" ? undefined : stageFilter,
          limit: 100,
        });
        setData(result.data);
      } catch (err: unknown) {
        const message =
          (err as { response?: { data?: { error?: { message?: string } } } })
            ?.response?.data?.error?.message ??
          (err as Error)?.message ??
          "Failed to load opportunities.";
        setError(message);
      } finally {
        setLoading(false);
        setRefreshing(false);
      }
    },
    [stageFilter]
  );

  useEffect(() => {
    fetchOpportunities();
  }, [fetchOpportunities]);

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase();
    if (!q) return data;
    return data.filter((item) => {
      const haystack = [
        item.title,
        item.customerId,
        item.accountOwnerUserId ?? "",
      ]
        .join(" ")
        .toLowerCase();
      return haystack.includes(q);
    });
  }, [data, search]);

  const hasActiveFilters = stageFilter !== "ALL" || search.trim() !== "";

  const clearFilters = () => {
    setSearch("");
    setStageFilter("ALL");
  };

  const stats = useMemo(() => {
    const total = data.length;
    const won = data.filter((d) => d.stage === "WON").length;
    const lost = data.filter((d) => d.stage === "LOST").length;
    const open = data.filter(
      (d) => d.stage !== "WON" && d.stage !== "LOST"
    ).length;
    const weightedMinor = data.reduce(
      (sum, d) =>
        sum +
        Math.round(
          ((d.expectedValue?.amountMinor ?? 0) * d.probabilityBasisPoints) / 10000
        ),
      0
    );
    return { total, won, lost, open, weightedMinor };
  }, [data]);

  if (loading && data.length === 0) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent" />
        <p className="mt-3 text-sm text-ink-600">Loading opportunities…</p>
      </div>
    );
  }

  const pipelineCurrency = data.find((d) => d.expectedValue)?.expectedValue?.currency;

  return (
    <div className="space-y-5">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
            Sales Opportunities
          </h1>
          <p className="mt-1 text-sm text-ink-600">
            Pipeline stages, expected value, probability, and ownership.
          </p>
        </div>
        <div className="flex items-center gap-2">
          <button
            onClick={() => fetchOpportunities({ silent: true })}
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
            Add Opportunity
          </button>
        </div>
      </div>

      {error && (
        <div className="flex items-start justify-between gap-3 rounded-lg border border-red/30 bg-red-bg px-4 py-2.5 text-sm text-red">
          <span>{error}</span>
          <button
            onClick={() => fetchOpportunities()}
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
            Open
          </div>
          <div className="text-lg font-bold text-primary">{stats.open}</div>
        </div>
        <div className="rounded-xl border border-line bg-surface p-3.5">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">
            Won
          </div>
          <div className="text-lg font-bold text-green">{stats.won}</div>
        </div>
        <div className="rounded-xl border border-line bg-surface p-3.5">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">
            Weighted value
          </div>
          <div className="text-lg font-bold text-ink-950 truncate">
            {pipelineCurrency
              ? formatMoney({
                  amountMinor: stats.weightedMinor,
                  currency: pipelineCurrency,
                })
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
                ? "No opportunities match your filters"
                : "No opportunities yet"
            }
            description={
              hasActiveFilters
                ? "Try adjusting your search or clearing the filters."
                : "Create your first opportunity to get started."
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
                  Add opportunity
                </button>
              )
            }
          />
        </div>
      ) : (
        <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
          {filtered.map((item) => {
            const stageTone = opportunityStageTone(item.stage);
            const isWon = item.stage === "WON";
            const isLost = item.stage === "LOST";
            const isOnHold = item.stage === "ON_HOLD";
            return (
              <div
                key={item.id}
                onClick={() => router.push(`/crm/opportunities/${item.id}`)}
                className={`group cursor-pointer rounded-xl border p-4 transition hover:shadow-md ${
                  isWon
                    ? "border-green/20 bg-surface"
                    : isLost
                      ? "border-red/20 bg-red-bg/5"
                      : isOnHold
                        ? "border-amber/20 bg-amber-bg/5"
                        : "border-line bg-surface"
                }`}
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
                        {isWon ? (
                          <CheckCircle2 className="h-3 w-3" />
                        ) : isLost ? (
                          <AlertCircle className="h-3 w-3" />
                        ) : (
                          <span className={`h-1 w-1 rounded-full ${stageTone.dot}`} />
                        )}
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
                  <div>
                    <span className="text-ink-400">Probability: </span>
                    <span className="font-semibold text-ink-950">
                      {probabilityPercent(item.probabilityBasisPoints)}
                    </span>
                  </div>
                  {item.expectedValue && (
                    <div className="text-right">
                      <span className="font-semibold text-ink-950">
                        {formatMoney(item.expectedValue)}
                      </span>
                    </div>
                  )}
                  {item.expectedCloseDate && (
                    <div className="col-span-2">
                      <span className="text-ink-400">Expected close: </span>
                      <span className="text-ink-700">
                        {formatDate(item.expectedCloseDate)}
                      </span>
                    </div>
                  )}
                </div>

                {item.nextAction && !isWon && !isLost && (
                  <div className="mt-3 border-t border-line pt-2.5">
                    <div className="flex items-center gap-2 text-[12px]">
                      <Zap className="h-3.5 w-3.5 text-amber" />
                      <span className="font-medium text-ink-950 truncate">
                        {item.nextAction}
                      </span>
                    </div>
                  </div>
                )}

                {item.lossReason && isLost && (
                  <div className="mt-3 rounded-lg bg-red-bg/40 px-3 py-2 text-[12px] text-red">
                    Lost: {item.lossReason}
                  </div>
                )}

                {item.holdReason && isOnHold && (
                  <div className="mt-3 rounded-lg bg-amber-bg/40 px-3 py-2 text-[12px] text-amber">
                    On hold: {item.holdReason}
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