"use client";

import { useEffect, useState, useCallback } from "react";
import { useParams, useRouter } from "next/navigation";
import {
  ArrowLeft,
  RefreshCw,
  AlertCircle,
  CheckCircle2,
  Lock,
  PieChart,
} from "lucide-react";
import { financeService } from "@/lib/services/financeService";
import type { BudgetRow } from "@/lib/types/finance";
import {
  budgetStatusLabel,
  budgetStatusTone,
  formatDate,
  formatDateTime,
  formatMinor,
  timeAgo,
} from "@/lib/utils/financeLabels";

export default function BudgetDetailPage() {
  const router = useRouter();
  const params = useParams();
  const budgetId = params.budgetId as string;

  const [budget, setBudget] = useState<BudgetRow | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState<string | null>(null);

  const fetchBudget = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const result = await financeService.budgets.get(budgetId);
      setBudget(result);
    } catch (err: unknown) {
      const message =
        (err as { response?: { data?: { error?: { message?: string } } } })
          ?.response?.data?.error?.message ??
        (err as Error)?.message ??
        "Failed to load budget.";
      setError(message);
    } finally {
      setLoading(false);
    }
  }, [budgetId]);

  useEffect(() => {
    if (budgetId) fetchBudget();
  }, [budgetId, fetchBudget]);

  const run = async (key: string, fn: () => Promise<BudgetRow>) => {
    setBusy(key);
    setError(null);
    try {
      const updated = await fn();
      setBudget(updated);
    } catch (err: unknown) {
      const message =
        (err as { response?: { data?: { error?: { message?: string } } } })
          ?.response?.data?.error?.message ??
        (err as Error)?.message ??
        "Action failed.";
      setError(message);
    } finally {
      setBusy(null);
    }
  };

  if (loading && !budget) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent" />
        <p className="mt-3 text-sm text-ink-600">Loading budget…</p>
      </div>
    );
  }

  if (error && !budget) {
    return (
      <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-surface py-14">
        <div className="rounded-full bg-red-bg p-4">
          <AlertCircle className="h-8 w-8 text-red" />
        </div>
        <h3 className="mt-3 text-[15px] font-semibold text-ink-950">
          Could not load budget
        </h3>
        <p className="mt-1 max-w-sm px-4 text-center text-sm text-ink-500">
          {error}
        </p>
        <div className="mt-3.5 flex gap-2">
          <button
            onClick={fetchBudget}
            className="rounded-lg border border-line bg-surface px-4 py-1.5 text-[11.5px] font-medium text-ink-600 transition hover:bg-surface-2"
          >
            Retry
          </button>
          <button
            onClick={() => router.push("/finance/budgets")}
            className="rounded-lg bg-primary px-4 py-1.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90"
          >
            Back to budgets
          </button>
        </div>
      </div>
    );
  }

  if (!budget) return null;

  const statusTone = budgetStatusTone(budget.status);
  const canApprove =
    budget.status === "DRAFT" || budget.status === "PENDING_APPROVAL";
  const canClose =
    budget.status === "APPROVED" || budget.status === "ACTIVE";

  const utilization =
    budget.approved_amount_minor > 0
      ? Math.round(
          (budget.actual_amount_projection / budget.approved_amount_minor) * 100
        )
      : 0;

  const remaining =
    budget.approved_amount_minor - budget.actual_amount_projection;

  return (
    <div className="space-y-5">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="flex items-start gap-3">
          <button
            onClick={() => router.push("/finance/budgets")}
            className="mt-0.5 flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-xl border border-line bg-surface text-ink-500 transition hover:bg-surface-2 hover:text-ink-700"
            aria-label="Back to budgets"
          >
            <ArrowLeft className="h-4 w-4" />
          </button>
          <div>
            <div className="flex flex-wrap items-center gap-3">
              <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
                {budget.category}
              </h1>
              <span
                className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-xs font-medium ${statusTone.bg} ${statusTone.text}`}
              >
                <span className={`h-1.5 w-1.5 rounded-full ${statusTone.dot}`} />
                {budgetStatusLabel(budget.status)}
              </span>
              <span className="text-sm text-ink-400">v{budget.version}</span>
            </div>
            <div className="mt-1 flex flex-wrap items-center gap-x-4 gap-y-1 text-sm text-ink-500">
              <span className="font-mono">{budget.id}</span>
              <span className="text-ink-300">·</span>
              <span>{budget.cost_centre}</span>
              <span className="text-ink-300">·</span>
              <span>{budget.currency}</span>
            </div>
          </div>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <button
            onClick={fetchBudget}
            disabled={loading}
            className="flex h-9 w-9 items-center justify-center rounded-xl border border-line bg-surface text-ink-500 transition hover:bg-surface-2 disabled:opacity-50"
            aria-label="Refresh"
          >
            <RefreshCw className={`h-3.5 w-3.5 ${loading ? "animate-spin" : ""}`} />
          </button>
          {canApprove && (
            <button
              onClick={() =>
                run("approve", () =>
                  financeService.budgets.approve(budget.id, budget.version)
                )
              }
              disabled={busy !== null}
              className="flex h-9 items-center gap-1.5 rounded-xl bg-primary px-3.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90 disabled:opacity-50"
            >
              {busy === "approve" ? (
                <RefreshCw className="h-3.5 w-3.5 animate-spin" />
              ) : (
                <CheckCircle2 className="h-3.5 w-3.5" />
              )}
              Approve
            </button>
          )}
          {canClose && (
            <button
              onClick={() =>
                run("close", () =>
                  financeService.budgets.close(budget.id, budget.version)
                )
              }
              disabled={busy !== null}
              className="flex h-9 items-center gap-1.5 rounded-xl border border-line bg-surface px-3.5 text-[11.5px] font-medium text-ink-600 transition hover:bg-surface-2 disabled:opacity-50"
            >
              {busy === "close" ? (
                <RefreshCw className="h-3.5 w-3.5 animate-spin" />
              ) : (
                <Lock className="h-3.5 w-3.5" />
              )}
              Close
            </button>
          )}
        </div>
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

      <div className="grid grid-cols-1 gap-5 lg:grid-cols-[2fr_1fr]">
        <div className="space-y-5">
          <div className="rounded-2xl border border-line bg-surface p-6">
            <div className="text-sm text-ink-500">
              {formatDate(budget.period_start)} → {formatDate(budget.period_end)}
            </div>
            <div className="mt-2">
              <span className="text-3xl font-bold text-ink-950">
                {formatMinor(budget.approved_amount_minor, budget.currency)}
              </span>
              <span className="ml-2 text-sm text-ink-400">approved</span>
            </div>
            <div className="mt-3">
              <div className="flex items-center justify-between text-sm">
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
              <div className="mt-1 h-2 w-full overflow-hidden rounded-full bg-surface-3">
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

          <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
            <div className="rounded-2xl border border-line bg-surface p-5">
              <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">
                Committed
              </div>
              <div className="mt-1 text-lg font-semibold text-amber">
                {formatMinor(
                  budget.committed_amount_projection,
                  budget.currency
                )}
              </div>
            </div>
            <div className="rounded-2xl border border-line bg-surface p-5">
              <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">
                Actual
              </div>
              <div className="mt-1 text-lg font-semibold text-ink-950">
                {formatMinor(budget.actual_amount_projection, budget.currency)}
              </div>
            </div>
            <div className="rounded-2xl border border-line bg-surface p-5">
              <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">
                Remaining
              </div>
              <div
                className={`mt-1 text-lg font-semibold ${
                  remaining < 0 ? "text-red" : "text-green"
                }`}
              >
                {formatMinor(remaining, budget.currency)}
              </div>
            </div>
          </div>
        </div>

        <div className="space-y-5">
          <div className="rounded-2xl border border-line bg-surface p-5">
            <h3 className="mb-3 text-[15px] font-semibold text-ink-950">
              Metadata
            </h3>
            <div className="space-y-2 text-sm">
              <div className="flex items-center justify-between border-b border-line pb-2">
                <span className="text-ink-500">Version</span>
                <span className="font-mono font-medium text-ink-950">
                  {budget.version}
                </span>
              </div>
              <div className="flex items-center justify-between border-b border-line pb-2">
                <span className="text-ink-500">Created</span>
                <span className="text-ink-950">
                  {timeAgo(budget.created_at)}
                </span>
              </div>
              {budget.approved_at && (
                <div className="flex items-center justify-between border-b border-line pb-2">
                  <span className="text-ink-500">Approved</span>
                  <span className="text-green">
                    {formatDateTime(budget.approved_at)}
                  </span>
                </div>
              )}
              <div className="flex items-center justify-between">
                <span className="text-ink-500">Updated</span>
                <span className="text-ink-950">
                  {timeAgo(budget.updated_at)}
                </span>
              </div>
            </div>
          </div>

          <div className="rounded-2xl border border-line bg-surface p-5">
            <h3 className="mb-3 text-[15px] font-semibold text-ink-950">
              Quick links
            </h3>
            <button
              onClick={() => router.push("/finance/budgets")}
              className="flex w-full items-center justify-between rounded-xl border border-line bg-surface px-4 py-2.5 text-[12.5px] font-medium text-ink-700 transition hover:bg-surface-2"
            >
              <span className="flex items-center gap-2">
                <PieChart className="h-4 w-4" />
                All budgets
              </span>
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}