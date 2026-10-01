"use client";

import { useEffect, useState, useCallback, useMemo } from "react";
import {
  RefreshCw,
  Search,
  SlidersHorizontal,
  X,
  Activity,
  Eye,
  AlertTriangle,
} from "lucide-react";
import EmptyState from "@/components/EmptyState";
import { financeService } from "@/lib/services/financeService";
import type {
  Projection,
  FreshnessState,
  ReconciliationStatus,
  ProjectionType,
} from "@/lib/types/finance";
import {
  formatDateTime,
  formatMinor,
  freshnessLabel,
  freshnessTone,
  projectionTypeLabel,
  reconciliationStatusLabel,
  reconciliationStatusTone,
  timeAgo,
} from "@/lib/utils/financeLabels";

const FRESHNESS_FILTERS: Array<{ value: "ALL" | FreshnessState; label: string }> = [
  { value: "ALL", label: "All" },
  { value: "CURRENT", label: "Current" },
  { value: "STALE", label: "Stale" },
  { value: "UNKNOWN", label: "Unknown" },
  { value: "GAP_DETECTED", label: "Gap detected" },
  { value: "RECONCILING", label: "Reconciling" },
];

const PROJECTION_TYPE_FILTERS: Array<{
  value: "ALL" | ProjectionType;
  label: string;
}> = [
  { value: "ALL", label: "All" },
  { value: "EARNING_ACCOUNT", label: "Earning account" },
  { value: "SUBSCRIPTION", label: "Subscription" },
  { value: "INVOICE", label: "Invoice" },
  { value: "HOLD", label: "Hold" },
  { value: "COMMISSION", label: "Commission" },
  { value: "EARNING", label: "Earning" },
  { value: "SETTLEMENT", label: "Settlement" },
  { value: "REFUND", label: "Refund" },
  { value: "REVERSAL", label: "Reversal" },
  { value: "WALLET", label: "Wallet" },
  { value: "PAYMENT_VERIFICATION", label: "Payment verification" },
];

export default function ReconciliationPage() {
  const [search, setSearch] = useState("");
  const [freshnessFilter, setFreshnessFilter] = useState<"ALL" | FreshnessState>(
    "ALL"
  );
  const [typeFilter, setTypeFilter] = useState<"ALL" | ProjectionType>("ALL");
  const [showFilters, setShowFilters] = useState(false);

  const [data, setData] = useState<Projection[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [refreshing, setRefreshing] = useState(false);
  const [busy, setBusy] = useState<string | null>(null);

  const fetchCases = useCallback(async (opts: { silent?: boolean } = {}) => {
    if (opts.silent) setRefreshing(true);
    else setLoading(true);
    setError(null);
    try {
      const result = await financeService.reconciliation.listCases({
        limit: 100,
      });
      setData(result.data);
    } catch (err: unknown) {
      const message =
        (err as { response?: { data?: { error?: { message?: string } } } })
          ?.response?.data?.error?.message ??
        (err as Error)?.message ??
        "Failed to load reconciliation cases.";
      setError(message);
    } finally {
      setLoading(false);
      setRefreshing(false);
    }
  }, []);

  useEffect(() => {
    fetchCases();
  }, [fetchCases]);

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase();
    return data.filter((item) => {
      if (freshnessFilter !== "ALL" && item.freshness_state !== freshnessFilter) {
        return false;
      }
      if (typeFilter !== "ALL" && item.projection_type !== typeFilter) {
        return false;
      }
      if (!q) return true;
      const haystack = [
        item.id,
        item.subject_id,
        item.wallet_reference,
        item.safe_status,
      ]
        .join(" ")
        .toLowerCase();
      return haystack.includes(q);
    });
  }, [data, search, freshnessFilter, typeFilter]);

  const hasActiveFilters =
    freshnessFilter !== "ALL" || typeFilter !== "ALL" || search.trim() !== "";

  const clearFilters = () => {
    setSearch("");
    setFreshnessFilter("ALL");
    setTypeFilter("ALL");
  };

  const handleReplay = async (projectionId: string) => {
    setBusy(projectionId);
    setError(null);
    try {
      await financeService.reconciliation.replayCase(projectionId);
      await fetchCases({ silent: true });
    } catch (err: unknown) {
      const message =
        (err as { response?: { data?: { error?: { message?: string } } } })
          ?.response?.data?.error?.message ??
        (err as Error)?.message ??
        "Failed to replay case.";
      setError(message);
    } finally {
      setBusy(null);
    }
  };

  const handleReadBack = async (projectionId: string) => {
    setBusy(projectionId);
    setError(null);
    try {
      await financeService.reconciliation.requestReadBack(projectionId);
      await fetchCases({ silent: true });
    } catch (err: unknown) {
      const message =
        (err as { response?: { data?: { error?: { message?: string } } } })
          ?.response?.data?.error?.message ??
        (err as Error)?.message ??
        "Failed to request read-back.";
      setError(message);
    } finally {
      setBusy(null);
    }
  };

  const stats = useMemo(() => {
    const total = data.length;
    const stale = data.filter(
      (c) => c.freshness_state === "STALE" || c.freshness_state === "GAP_DETECTED"
    ).length;
    const reconciling = data.filter(
      (c) =>
        c.reconciliation_status === "PENDING" ||
        c.reconciliation_status === "PROCESSING"
    ).length;
    const deadLetters = data.filter(
      (c) => c.reconciliation_status === "DEAD_LETTER"
    ).length;
    return { total, stale, reconciling, deadLetters };
  }, [data]);

  if (loading && data.length === 0) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent" />
        <p className="mt-3 text-sm text-ink-600">
          Loading reconciliation cases…
        </p>
      </div>
    );
  }

  return (
    <div className="space-y-5">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
            Reconciliation
          </h1>
          <p className="mt-1 text-sm text-ink-600">
            Projection freshness and reconciliation status against authoritative Wallet state.
          </p>
        </div>
        <button
          onClick={() => fetchCases({ silent: true })}
          disabled={refreshing}
          className="flex h-9 w-9 items-center justify-center rounded-xl border border-line bg-surface text-ink-500 transition hover:bg-surface-2 disabled:opacity-50"
          aria-label="Refresh"
        >
          <RefreshCw className={`h-3.5 w-3.5 ${refreshing ? "animate-spin" : ""}`} />
        </button>
      </div>

      {error && (
        <div className="flex items-start justify-between gap-3 rounded-lg border border-red/30 bg-red-bg px-4 py-2.5 text-sm text-red">
          <span>{error}</span>
          <button
            onClick={() => setError(null)}
            className="whitespace-nowrap text-xs font-semibold underline"
          >
            Dismiss
          </button>
        </div>
      )}

      <div className="grid grid-cols-2 gap-2.5 sm:grid-cols-4">
        <div className="rounded-xl border border-line bg-surface p-3.5">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">
            Total cases
          </div>
          <div className="text-lg font-bold text-ink-950">{stats.total}</div>
        </div>
        <div className="rounded-xl border border-line bg-surface p-3.5">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">
            Stale / gap
          </div>
          <div className="text-lg font-bold text-amber">{stats.stale}</div>
        </div>
        <div className="rounded-xl border border-line bg-surface p-3.5">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">
            Reconciling
          </div>
          <div className="text-lg font-bold text-primary">
            {stats.reconciling}
          </div>
        </div>
        <div className="rounded-xl border border-line bg-surface p-3.5">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">
            Dead letter
          </div>
          <div className="text-lg font-bold text-red">{stats.deadLetters}</div>
        </div>
      </div>

      <div className="flex flex-wrap items-center gap-2">
        <div className="flex h-11 min-w-[200px] flex-1 items-center gap-2.5 rounded-xl border border-line bg-surface px-4">
          <Search className="h-4 w-4 text-ink-400" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search by projection, subject, or wallet reference…"
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
              {(freshnessFilter !== "ALL" ? 1 : 0) +
                (typeFilter !== "ALL" ? 1 : 0)}
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
                Freshness
              </div>
              <div className="flex flex-wrap gap-1.5">
                {FRESHNESS_FILTERS.map((f) => (
                  <button
                    key={f.value}
                    onClick={() => setFreshnessFilter(f.value)}
                    className={`rounded-full px-3 py-1 text-[11px] font-semibold transition ${
                      freshnessFilter === f.value
                        ? "bg-primary-bg text-primary ring-2 ring-primary/30"
                        : "bg-surface-2 text-ink-600 hover:bg-surface-3"
                    }`}
                  >
                    {f.label}
                  </button>
                ))}
              </div>
            </div>
            <div>
              <div className="mb-1 text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">
                Projection type
              </div>
              <div className="flex flex-wrap gap-1.5">
                {PROJECTION_TYPE_FILTERS.map((t) => (
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
            image="/images/alerts-empty.png"
            title={
              hasActiveFilters
                ? "No cases match your filters"
                : "No reconciliation cases"
            }
            description={
              hasActiveFilters
                ? "Try adjusting your search or clearing the filters."
                : "All projections are current and idle."
            }
            action={
              hasActiveFilters ? (
                <button
                  onClick={clearFilters}
                  className="inline-flex items-center justify-center rounded-lg bg-primary-bg px-4 py-2 text-xs font-semibold text-primary transition-colors hover:bg-primary/10"
                >
                  Clear filters
                </button>
              ) : undefined
            }
          />
        </div>
      ) : (
        <div className="overflow-hidden rounded-2xl border border-line bg-surface">
          <div className="overflow-x-auto">
            <table className="w-full min-w-[1100px] border-collapse text-sm">
              <thead>
                <tr className="bg-surface-2/70">
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">
                    Projection
                  </th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">
                    Wallet reference
                  </th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">
                    Amount
                  </th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">
                    Freshness
                  </th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">
                    Reconciliation
                  </th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">
                    Last synced
                  </th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">
                  </th>
                </tr>
              </thead>
              <tbody>
                {filtered.map((item) => {
                  const freshTone = freshnessTone(item.freshness_state);
                  const reconTone = reconciliationStatusTone(
                    item.reconciliation_status
                  );
                  const isStale =
                    item.freshness_state === "STALE" ||
                    item.freshness_state === "GAP_DETECTED";
                  const isDeadLetter =
                    item.reconciliation_status === "DEAD_LETTER";
                  return (
                    <tr
                      key={item.id}
                      className="border-b border-line transition-colors hover:bg-surface-2"
                    >
                      <td className="px-4 py-3">
                        <div className="text-[13px] font-medium text-ink-950">
                          {projectionTypeLabel(item.projection_type)}
                        </div>
                        <div className="font-mono text-[11px] text-ink-500">
                          {item.id}
                        </div>
                      </td>
                      <td className="px-4 py-3">
                        <div className="font-mono text-[12px] text-ink-700">
                          {item.wallet_reference}
                        </div>
                        <div className="font-mono text-[11px] text-ink-400">
                          {item.subject_id}
                        </div>
                      </td>
                      <td className="px-4 py-3">
                        {item.amount_minor !== null ? (
                          <span className="font-semibold text-ink-950">
                            {formatMinor(item.amount_minor, item.currency)}
                          </span>
                        ) : (
                          <span className="text-ink-400">—</span>
                        )}
                        <div className="font-mono text-[11px] text-ink-400">
                          {item.safe_status}
                        </div>
                      </td>
                      <td className="px-4 py-3">
                        <span
                          className={`inline-flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${freshTone.bg} ${freshTone.text}`}
                        >
                          <span
                            className={`h-1 w-1 rounded-full ${freshTone.dot}`}
                          />
                          {freshnessLabel(item.freshness_state)}
                        </span>
                      </td>
                      <td className="px-4 py-3">
                        <span
                          className={`inline-flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${reconTone.bg} ${reconTone.text}`}
                        >
                          <span
                            className={`h-1 w-1 rounded-full ${reconTone.dot}`}
                          />
                          {reconciliationStatusLabel(item.reconciliation_status)}
                        </span>
                      </td>
                      <td className="px-4 py-3 text-[12px] text-ink-500">
                        {timeAgo(item.last_synced_at)}
                      </td>
                      <td className="px-4 py-3">
                        <div className="flex items-center justify-end gap-1.5">
                          {(isStale || isDeadLetter) && (
                            <button
                              onClick={() => handleReplay(item.id)}
                              disabled={busy !== null}
                              className="rounded-lg bg-primary px-2.5 py-1 text-[10px] font-medium text-white transition hover:bg-primary/90 disabled:opacity-50"
                            >
                              {busy === item.id ? "…" : "Replay"}
                            </button>
                          )}
                          {(isStale || isDeadLetter) && (
                            <button
                              onClick={() => handleReadBack(item.id)}
                              disabled={busy !== null}
                              className="rounded-lg border border-line bg-surface px-2.5 py-1 text-[10px] font-medium text-ink-600 transition hover:bg-surface-2 disabled:opacity-50"
                            >
                              Read back
                            </button>
                          )}
                          <AlertTriangle className="h-4 w-4 text-ink-300" />
                        </div>
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