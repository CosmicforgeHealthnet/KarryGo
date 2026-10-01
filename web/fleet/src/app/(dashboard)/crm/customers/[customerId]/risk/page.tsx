"use client";

import { useEffect, useState, useCallback } from "react";
import { useParams } from "next/navigation";
import {
  Shield,
  RefreshCw,
  X,
  Clock,
  CheckCircle2,
  AlertCircle,
  ArrowRight,
  DollarSign,
  CreditCard,
  AlertTriangle,
  Lock,
  Unlock,
  Save,
} from "lucide-react";
import { crmService } from "@/lib/services/crmService";
import type {
  CreditProfileView,
  CreditHistoryView,
  CreditHoldStatus,
  RiskStatus,
} from "@/lib/types/crm";
import {
  formatMoney,
  formatDateTime,
  holdStatusLabel,
  holdStatusTone,
  riskStatusLabel,
  riskStatusTone,
  timeAgo,
} from "@/lib/utils/crmLabels";

const HOLD_OPTIONS: Array<{ value: CreditHoldStatus; label: string }> = [
  { value: "REVIEW_REQUIRED", label: "Review required" },
  { value: "ON_HOLD", label: "On hold" },
  { value: "RESTRICTED", label: "Restricted" },
];

export default function RiskAndCreditPage() {
  const params = useParams();
  const customerId = params.customerId as string;

  const [profile, setProfile] = useState<CreditProfileView | null>(null);
  const [history, setHistory] = useState<CreditHistoryView[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [refreshing, setRefreshing] = useState(false);

  const [savingLimit, setSavingLimit] = useState(false);
  const [savingHold, setSavingHold] = useState(false);

  const [showLimitModal, setShowLimitModal] = useState(false);
  const [showHoldModal, setShowHoldModal] = useState(false);
  const [newLimit, setNewLimit] = useState("");
  const [holdReason, setHoldReason] = useState("");
  const [holdTarget, setHoldTarget] = useState<CreditHoldStatus>("ON_HOLD");

  const fetchAll = useCallback(
    async (opts: { silent?: boolean } = {}) => {
      if (opts.silent) setRefreshing(true);
      else setLoading(true);
      setError(null);
      try {
        const [profileResult, historyResult] = await Promise.all([
          crmService.getCreditProfile(customerId),
          crmService.getCreditHistory(customerId, { limit: 10 }),
        ]);
        setProfile(profileResult);
        setHistory(historyResult.data);
      } catch (err: unknown) {
        const message =
          (err as { response?: { data?: { error?: { message?: string } } } })
            ?.response?.data?.error?.message ??
          (err as Error)?.message ??
          "Failed to load credit profile.";
        setError(message);
      } finally {
        setLoading(false);
        setRefreshing(false);
      }
    },
    [customerId]
  );

  useEffect(() => {
    if (customerId) fetchAll();
  }, [customerId, fetchAll]);

  const handleUpdateLimit = async () => {
    if (!profile) return;
    const amountMinor = Math.round(Number(newLimit) * 100);
    if (!Number.isFinite(amountMinor) || amountMinor < 0) return;

    setSavingLimit(true);
    try {
      const updated = await crmService.setCreditProfile(
        customerId,
        {
          creditLimit: { amountMinor, currency: profile.currency },
          currency: profile.currency,
          paymentTermsDays: profile.paymentTermsDays,
          reasonCode: "credit_limit_updated_by_staff",
        },
        profile.version
      );
      setProfile(updated);
      setShowLimitModal(false);
      setNewLimit("");
      await fetchAll({ silent: true });
    } catch (err: unknown) {
      const message =
        (err as { response?: { data?: { error?: { message?: string } } } })
          ?.response?.data?.error?.message ??
        (err as Error)?.message ??
        "Failed to update credit limit.";
      setError(message);
    } finally {
      setSavingLimit(false);
    }
  };

  const handleApplyHold = async () => {
    if (!profile) return;
    if (!holdReason.trim()) return;
    setSavingHold(true);
    try {
      const updated = await crmService.applyCreditHold(
        customerId,
        { holdStatus: holdTarget, reasonCode: holdReason.trim() },
        profile.version
      );
      setProfile(updated);
      setShowHoldModal(false);
      setHoldReason("");
      await fetchAll({ silent: true });
    } catch (err: unknown) {
      const message =
        (err as { response?: { data?: { error?: { message?: string } } } })
          ?.response?.data?.error?.message ??
        (err as Error)?.message ??
        "Failed to apply credit hold.";
      setError(message);
    } finally {
      setSavingHold(false);
    }
  };

  const handleReleaseHold = async () => {
    if (!profile) return;
    setSavingHold(true);
    try {
      const updated = await crmService.releaseCreditHold(
        customerId,
        profile.version,
        "hold_released_by_staff"
      );
      setProfile(updated);
      await fetchAll({ silent: true });
    } catch (err: unknown) {
      const message =
        (err as { response?: { data?: { error?: { message?: string } } } })
          ?.response?.data?.error?.message ??
        (err as Error)?.message ??
        "Failed to release credit hold.";
      setError(message);
    } finally {
      setSavingHold(false);
    }
  };

  if (loading && !profile) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent" />
        <p className="mt-3 text-sm text-ink-600">Loading credit profile…</p>
      </div>
    );
  }

  if (error && !profile) {
    return (
      <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-surface py-14">
        <div className="rounded-full bg-red-bg p-4">
          <AlertTriangle className="h-8 w-8 text-red" />
        </div>
        <h3 className="mt-3 text-[15px] font-semibold text-ink-950">
          Could not load credit profile
        </h3>
        <p className="mt-1 max-w-sm px-4 text-center text-sm text-ink-500">
          {error}
        </p>
        <button
          onClick={() => fetchAll()}
          className="mt-3.5 rounded-lg bg-primary px-4 py-1.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90"
        >
          Retry
        </button>
      </div>
    );
  }

  if (!profile) return null;

  const riskTone = riskStatusTone(profile.riskStatusSnapshot);
  const holdTone = holdStatusTone(profile.holdStatus);
  const isHoldActive = profile.blocking;

  return (
    <div className="space-y-5">
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

      <div className="flex items-center justify-between">
        <h2 className="text-[15px] font-semibold text-ink-950">
          Risk & Credit Controls
        </h2>
        <button
          onClick={() => fetchAll({ silent: true })}
          disabled={refreshing}
          className="flex h-9 w-9 items-center justify-center rounded-xl border border-line bg-surface text-ink-500 transition hover:bg-surface-2 disabled:opacity-50"
          aria-label="Refresh"
        >
          <RefreshCw className={`h-3.5 w-3.5 ${refreshing ? "animate-spin" : ""}`} />
        </button>
      </div>

      <div className="grid grid-cols-1 gap-5 lg:grid-cols-[2fr_1fr]">
        <div className="space-y-5">
          <div className="rounded-2xl border border-line bg-surface p-5">
            <div className="mb-3 flex items-center gap-2.5">
              <Shield className="h-5 w-5 text-ink-400" />
              <h3 className="text-[15px] font-semibold text-ink-950">
                Risk Status
              </h3>
            </div>
            <div className="flex flex-wrap items-center gap-4">
              <span
                className={`flex items-center gap-2 rounded-full px-4 py-2 text-sm font-semibold ${riskTone.bg} ${riskTone.text}`}
              >
                <span className={`h-1.5 w-1.5 rounded-full ${riskTone.dot}`} />
                {riskStatusLabel(profile.riskStatusSnapshot)}
              </span>
              <span className="text-sm text-ink-500">
                Mirrored from the customer's current risk state.
              </span>
            </div>
          </div>

          <div className="rounded-2xl border border-line bg-surface p-5">
            <div className="mb-3 flex items-center gap-2.5">
              <CreditCard className="h-5 w-5 text-ink-400" />
              <h3 className="text-[15px] font-semibold text-ink-950">
                Credit Information
              </h3>
            </div>
            <div className="grid grid-cols-2 gap-4">
              <div>
                <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">
                  Currency
                </div>
                <div className="font-mono text-sm font-semibold text-ink-950">
                  {profile.currency}
                </div>
              </div>
              <div>
                <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">
                  Payment terms
                </div>
                <div className="text-sm font-semibold text-ink-950">
                  {profile.paymentTermsDays} days
                </div>
              </div>
              <div>
                <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">
                  Credit limit
                </div>
                {profile.hasCreditLimit ? (
                  <div className="text-sm font-semibold text-ink-950">
                    Set
                  </div>
                ) : (
                  <div className="text-sm font-semibold text-ink-400">
                    Not set
                  </div>
                )}
              </div>
              <div>
                <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">
                  Effective from
                </div>
                <div className="text-sm font-semibold text-ink-950">
                  {formatDateTime(profile.effectiveFrom)}
                </div>
              </div>
            </div>
          </div>

          <div className="rounded-2xl border border-line bg-surface p-5">
            <div className="mb-3 flex items-center gap-2.5">
              {isHoldActive ? (
                <Lock className="h-5 w-5 text-red" />
              ) : (
                <Unlock className="h-5 w-5 text-green" />
              )}
              <h3 className="text-[15px] font-semibold text-ink-950">
                Hold Status
              </h3>
            </div>
            <div className="flex flex-wrap items-center gap-4">
              <span
                className={`flex items-center gap-1.5 rounded-full px-3 py-1 text-sm font-semibold ${holdTone.bg} ${holdTone.text}`}
              >
                <span className={`h-1.5 w-1.5 rounded-full ${holdTone.dot}`} />
                {holdStatusLabel(profile.holdStatus)}
              </span>
              {profile.blocking && (
                <span className="text-sm text-ink-500">
                  New commitments are currently blocked.
                </span>
              )}
            </div>
          </div>
        </div>

        <div className="space-y-5">
          <div className="rounded-2xl border border-line bg-surface p-5">
            <h3 className="mb-3 text-[15px] font-semibold text-ink-950">
              Actions
            </h3>
            <div className="space-y-2">
              <button
                onClick={() => {
                  setNewLimit("");
                  setShowLimitModal(true);
                }}
                className="flex w-full items-center justify-between rounded-xl border border-line bg-surface px-4 py-2.5 text-[12.5px] font-medium text-ink-700 transition hover:bg-surface-2"
              >
                <span className="flex items-center gap-2">
                  <DollarSign className="h-4 w-4" />
                  Update credit limit
                </span>
                <ArrowRight className="h-4 w-4 text-ink-400" />
              </button>
              <button
                onClick={() => {
                  setHoldTarget("ON_HOLD");
                  setHoldReason("");
                  setShowHoldModal(true);
                }}
                className="flex w-full items-center justify-between rounded-xl border border-line bg-surface px-4 py-2.5 text-[12.5px] font-medium text-ink-700 transition hover:bg-surface-2"
              >
                <span className="flex items-center gap-2">
                  <Lock className="h-4 w-4" />
                  Apply hold
                </span>
                <ArrowRight className="h-4 w-4 text-ink-400" />
              </button>
              {profile.blocking && (
                <button
                  onClick={handleReleaseHold}
                  disabled={savingHold}
                  className="flex w-full items-center justify-between rounded-xl border border-line bg-surface px-4 py-2.5 text-[12.5px] font-medium text-ink-700 transition hover:bg-surface-2 disabled:opacity-50"
                >
                  <span className="flex items-center gap-2">
                    {savingHold ? (
                      <RefreshCw className="h-4 w-4 animate-spin" />
                    ) : (
                      <Unlock className="h-4 w-4" />
                    )}
                    Release hold
                  </span>
                  <ArrowRight className="h-4 w-4 text-ink-400" />
                </button>
              )}
            </div>
          </div>

          <div className="rounded-2xl border border-line bg-surface p-5">
            <h3 className="mb-3 text-[15px] font-semibold text-ink-950">
              Recent History
            </h3>
            {history.length === 0 ? (
              <div className="py-6 text-center text-sm text-ink-500">
                No credit history yet.
              </div>
            ) : (
              <div className="space-y-3">
                {history.map((event) => (
                  <div
                    key={event.id}
                    className="border-b border-line pb-2 last:border-0 last:pb-0"
                  >
                    <div className="flex items-start justify-between gap-3">
                      <div className="min-w-0">
                        <div className="text-sm font-medium text-ink-950">
                          {event.previousHoldStatus
                            ? `${holdStatusLabel(event.previousHoldStatus)} → ${holdStatusLabel(event.newHoldStatus)}`
                            : holdStatusLabel(event.newHoldStatus)}
                        </div>
                        {(event.previousLimit || event.newLimit) && (
                          <div className="mt-0.5 text-xs text-ink-500">
                            {event.previousLimit
                              ? formatMoney(event.previousLimit)
                              : "—"}{" "}
                            →{" "}
                            {event.newLimit ? formatMoney(event.newLimit) : "—"}
                          </div>
                        )}
                        {event.reasonCode && (
                          <div className="mt-0.5 text-xs text-ink-400">
                            {event.reasonCode}
                          </div>
                        )}
                      </div>
                      <div className="flex-shrink-0 text-right">
                        <div className="text-xs text-ink-400">
                          {timeAgo(event.occurredAt)}
                        </div>
                      </div>
                    </div>
                  </div>
                ))}
              </div>
            )}
          </div>
        </div>
      </div>

      {showLimitModal && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 backdrop-blur-sm">
          <div className="w-full max-w-md rounded-2xl bg-surface p-6 shadow-2xl">
            <div className="flex items-center gap-2">
              <DollarSign className="h-5 w-5 text-primary" />
              <h3 className="text-lg font-semibold text-ink-950">
                Update Credit Limit
              </h3>
            </div>
            <p className="mt-2 text-sm text-ink-600">
              Enter the new limit in {profile.currency}. Values are in major units.
            </p>
            <div className="mt-4">
              <label className="mb-1 block text-xs font-medium uppercase tracking-[0.06em] text-ink-400">
                New credit limit
              </label>
              <input
                type="number"
                min={0}
                value={newLimit}
                onChange={(e) => setNewLimit(e.target.value)}
                placeholder="0.00"
                className="w-full rounded-xl border border-line bg-surface px-4 py-2.5 text-sm text-ink-950 outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20"
              />
            </div>
            <div className="mt-5 flex gap-2.5">
              <button
                onClick={() => {
                  setShowLimitModal(false);
                  setNewLimit("");
                }}
                className="flex-1 rounded-lg border border-line bg-surface px-4 py-2 text-sm font-medium text-ink-600 transition hover:bg-surface-2"
              >
                Cancel
              </button>
              <button
                onClick={handleUpdateLimit}
                disabled={savingLimit || !newLimit}
                className="flex-1 rounded-lg bg-primary px-4 py-2 text-sm font-medium text-white transition hover:bg-primary/90 disabled:cursor-not-allowed disabled:opacity-50"
              >
                {savingLimit ? (
                  <RefreshCw className="mr-1 inline h-4 w-4 animate-spin" />
                ) : null}
                Update
              </button>
            </div>
          </div>
        </div>
      )}

      {showHoldModal && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 backdrop-blur-sm">
          <div className="w-full max-w-md rounded-2xl bg-surface p-6 shadow-2xl">
            <div className="flex items-center gap-2">
              <Lock className="h-5 w-5 text-amber" />
              <h3 className="text-lg font-semibold text-ink-950">
                Apply Credit Hold
              </h3>
            </div>
            <p className="mt-2 text-sm text-ink-600">
              Blocking holds stop new quotations and contract activation.
            </p>
            <div className="mt-4 space-y-3">
              <div>
                <label className="mb-1 block text-xs font-medium uppercase tracking-[0.06em] text-ink-400">
                  Hold status
                </label>
                <select
                  value={holdTarget}
                  onChange={(e) =>
                    setHoldTarget(e.target.value as CreditHoldStatus)
                  }
                  className="w-full rounded-xl border border-line bg-surface px-4 py-2.5 text-sm text-ink-950 outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20"
                >
                  {HOLD_OPTIONS.map((h) => (
                    <option key={h.value} value={h.value}>
                      {h.label}
                    </option>
                  ))}
                </select>
              </div>
              <div>
                <label className="mb-1 block text-xs font-medium uppercase tracking-[0.06em] text-ink-400">
                  Reason
                </label>
                <textarea
                  value={holdReason}
                  onChange={(e) => setHoldReason(e.target.value)}
                  rows={3}
                  placeholder="Explain why this hold is being applied…"
                  className="w-full resize-none rounded-xl border border-line bg-surface px-4 py-2.5 text-sm text-ink-950 outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20"
                />
              </div>
            </div>
            <div className="mt-5 flex gap-2.5">
              <button
                onClick={() => {
                  setShowHoldModal(false);
                  setHoldReason("");
                }}
                className="flex-1 rounded-lg border border-line bg-surface px-4 py-2 text-sm font-medium text-ink-600 transition hover:bg-surface-2"
              >
                Cancel
              </button>
              <button
                onClick={handleApplyHold}
                disabled={savingHold || !holdReason.trim()}
                className="flex-1 rounded-lg bg-amber px-4 py-2 text-sm font-medium text-white transition hover:bg-amber/90 disabled:cursor-not-allowed disabled:opacity-50"
              >
                {savingHold ? (
                  <RefreshCw className="mr-1 inline h-4 w-4 animate-spin" />
                ) : null}
                Apply hold
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}