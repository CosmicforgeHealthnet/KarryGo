"use client";

import { useEffect, useState, useCallback } from "react";
import { useParams, useRouter } from "next/navigation";
import {
  ArrowLeft,
  RefreshCw,
  AlertCircle,
  Users,
  FileText,
  Calculator,
  CheckCircle2,
  Send,
  RotateCcw,
} from "lucide-react";
import { financeService } from "@/lib/services/financeService";
import type { PayrollPeriodView } from "@/lib/types/finance";
import {
  compensationItemSourceLabel,
  compensationItemStatusLabel,
  compensationItemStatusTone,
  formatDate,
  formatDateTime,
  formatMinor,
  payrollStatusLabel,
  payrollStatusTone,
  timeAgo,
} from "@/lib/utils/financeLabels";

export default function PayrollPeriodDetailPage() {
  const router = useRouter();
  const params = useParams();
  const periodId = params.periodId as string;

  const [period, setPeriod] = useState<PayrollPeriodView | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const fetchPeriod = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const result = await financeService.payrollPeriods.get(periodId);
      setPeriod(result);
    } catch (err: unknown) {
      const message =
        (err as { response?: { data?: { error?: { message?: string } } } })
          ?.response?.data?.error?.message ??
        (err as Error)?.message ??
        "Failed to load payroll period.";
      setError(message);
    } finally {
      setLoading(false);
    }
  }, [periodId]);

  useEffect(() => {
    if (periodId) fetchPeriod();
  }, [periodId, fetchPeriod]);

  if (loading && !period) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent" />
        <p className="mt-3 text-sm text-ink-600">Loading payroll period…</p>
      </div>
    );
  }

  if (error && !period) {
    return (
      <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-surface py-14">
        <div className="rounded-full bg-red-bg p-4">
          <AlertCircle className="h-8 w-8 text-red" />
        </div>
        <h3 className="mt-3 text-[15px] font-semibold text-ink-950">
          Could not load period
        </h3>
        <p className="mt-1 max-w-sm px-4 text-center text-sm text-ink-500">
          {error}
        </p>
        <div className="mt-3.5 flex gap-2">
          <button
            onClick={fetchPeriod}
            className="rounded-lg border border-line bg-surface px-4 py-1.5 text-[11.5px] font-medium text-ink-600 transition hover:bg-surface-2"
          >
            Retry
          </button>
          <button
            onClick={() => router.push("/finance/payroll")}
            className="rounded-lg bg-primary px-4 py-1.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90"
          >
            Back to payroll
          </button>
        </div>
      </div>
    );
  }

  if (!period) return null;

  const statusTone = payrollStatusTone(period.status);
  const canCalculate =
    period.status === "DRAFT" || period.status === "CALCULATED";
  const canReview = period.status === "CALCULATED";
  const canApprove = period.status === "CALCULATED" || period.status === "PENDING_APPROVAL";
  const canPost = period.status === "APPROVED";

  const totalAmount = period.items.reduce(
    (sum, i) => sum + i.amount_minor,
    0
  );

  return (
    <div className="space-y-5">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="flex items-start gap-3">
          <button
            onClick={() => router.push("/finance/payroll")}
            className="mt-0.5 flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-xl border border-line bg-surface text-ink-500 transition hover:bg-surface-2 hover:text-ink-700"
            aria-label="Back to payroll"
          >
            <ArrowLeft className="h-4 w-4" />
          </button>
          <div>
            <div className="flex flex-wrap items-center gap-3">
              <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
                Payroll {formatDate(period.period_start)} –{" "}
                {formatDate(period.period_end)}
              </h1>
              <span
                className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-xs font-medium ${statusTone.bg} ${statusTone.text}`}
              >
                <span className={`h-1.5 w-1.5 rounded-full ${statusTone.dot}`} />
                {payrollStatusLabel(period.status)}
              </span>
              <span className="text-sm text-ink-400">v{period.version}</span>
            </div>
            <div className="mt-1 flex flex-wrap items-center gap-x-4 gap-y-1 text-sm text-ink-500">
              <span className="font-mono">{period.id}</span>
              <span className="text-ink-300">·</span>
              <span>{period.branch_id ?? "All branches"}</span>
              <span className="text-ink-300">·</span>
              <span className="font-mono">{period.currency}</span>
            </div>
          </div>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <button
            onClick={fetchPeriod}
            disabled={loading}
            className="flex h-9 w-9 items-center justify-center rounded-xl border border-line bg-surface text-ink-500 transition hover:bg-surface-2 disabled:opacity-50"
            aria-label="Refresh"
          >
            <RefreshCw className={`h-3.5 w-3.5 ${loading ? "animate-spin" : ""}`} />
          </button>
          {canCalculate && (
            <button
              onClick={() =>
                router.push(`/finance/payroll/${period.id}/review`)
              }
              className="flex h-9 items-center gap-1.5 rounded-xl bg-primary px-3.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90"
            >
              <Calculator className="h-3.5 w-3.5" />
              Calculate
            </button>
          )}
          {canReview && (
            <button
              onClick={() =>
                router.push(`/finance/payroll/${period.id}/review`)
              }
              className="flex h-9 items-center gap-1.5 rounded-xl border border-line bg-surface px-3.5 text-[11.5px] font-medium text-ink-600 transition hover:bg-surface-2"
            >
              <FileText className="h-3.5 w-3.5" />
              Review
            </button>
          )}
          {canApprove && (
            <button
              onClick={() =>
                router.push(`/finance/payroll/${period.id}/approval`)
              }
              className="flex h-9 items-center gap-1.5 rounded-xl bg-primary px-3.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90"
            >
              <CheckCircle2 className="h-3.5 w-3.5" />
              Approve
            </button>
          )}
          {canPost && (
            <button
              onClick={() =>
                router.push(`/finance/payroll/${period.id}/approval`)
              }
              className="flex h-9 items-center gap-1.5 rounded-xl bg-primary px-3.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90"
            >
              <Send className="h-3.5 w-3.5" />
              Post
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

      <div className="grid grid-cols-2 gap-2.5 sm:grid-cols-4">
        <div className="rounded-xl border border-line bg-surface p-3.5">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">
            Providers
          </div>
          <div className="text-lg font-bold text-ink-950">
            {period.items.length}
          </div>
        </div>
        <div className="rounded-xl border border-line bg-surface p-3.5">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">
            Total amount
          </div>
          <div className="text-lg font-bold text-ink-950">
            {formatMinor(totalAmount, period.currency)}
          </div>
        </div>
        <div className="rounded-xl border border-line bg-surface p-3.5">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">
            Calculation
          </div>
          <div className="font-mono text-lg font-bold text-ink-950">
            v{period.calculation_version}
          </div>
        </div>
        <div className="rounded-xl border border-line bg-surface p-3.5">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">
            Updated
          </div>
          <div className="text-lg font-bold text-ink-950">
            {timeAgo(period.updated_at)}
          </div>
        </div>
      </div>

      <div className="rounded-2xl border border-line bg-surface p-5">
        <div className="mb-3 flex items-center justify-between">
          <h3 className="text-[15px] font-semibold text-ink-950">
            Compensation items
          </h3>
          <span className="text-xs text-ink-400">
            {period.items.length} item{period.items.length === 1 ? "" : "s"}
          </span>
        </div>
        {period.items.length === 0 ? (
          <div className="py-8 text-center text-sm text-ink-500">
            No items calculated yet. Run calculation to generate items.
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full min-w-[900px] border-collapse text-sm">
              <thead>
                <tr className="bg-surface-2/70">
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">
                    Membership
                  </th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">
                    Source
                  </th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">
                    Rule
                  </th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">
                    Amount
                  </th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">
                    Status
                  </th>
                </tr>
              </thead>
              <tbody>
                {period.items.map((item) => {
                  const itemTone = compensationItemStatusTone(item.status);
                  const negative = item.amount_minor < 0;
                  return (
                    <tr
                      key={item.id}
                      className="border-b border-line transition-colors hover:bg-surface-2"
                    >
                      <td className="px-4 py-3 font-mono text-[12px] text-ink-600">
                        {item.membership_id}
                      </td>
                      <td className="px-4 py-3">
                        <div className="text-ink-950">
                          {compensationItemSourceLabel(item.source_type)}
                        </div>
                        <div className="font-mono text-[11px] text-ink-500">
                          {item.source_reference}
                        </div>
                      </td>
                      <td className="px-4 py-3">
                        <div className="font-mono text-[12px] text-ink-600">
                          {item.rule_id}
                        </div>
                        <div className="text-[11px] text-ink-500">
                          v{item.rule_version}
                        </div>
                      </td>
                      <td
                        className={`px-4 py-3 font-semibold ${
                          negative ? "text-red" : "text-ink-950"
                        }`}
                      >
                        {negative ? "− " : ""}
                        {formatMinor(Math.abs(item.amount_minor), item.currency)}
                      </td>
                      <td className="px-4 py-3">
                        <span
                          className={`inline-flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${itemTone.bg} ${itemTone.text}`}
                        >
                          <span
                            className={`h-1 w-1 rounded-full ${itemTone.dot}`}
                          />
                          {compensationItemStatusLabel(item.status)}
                        </span>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        )}
      </div>

      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
        <div className="rounded-2xl border border-line bg-surface p-5">
          <h3 className="mb-3 text-[15px] font-semibold text-ink-950">
            Timeline
          </h3>
          <div className="space-y-2 text-sm">
            <div className="flex items-center justify-between border-b border-line pb-2">
              <span className="text-ink-500">Created</span>
              <span className="text-ink-950">
                {formatDateTime(period.created_at)}
              </span>
            </div>
            {period.calculated_at && (
              <div className="flex items-center justify-between border-b border-line pb-2">
                <span className="text-ink-500">Calculated</span>
                <span className="text-amber">
                  {formatDateTime(period.calculated_at)}
                </span>
              </div>
            )}
            {period.approved_at && (
              <div className="flex items-center justify-between border-b border-line pb-2">
                <span className="text-ink-500">Approved</span>
                <span className="text-green">
                  {formatDateTime(period.approved_at)}
                </span>
              </div>
            )}
            {period.posted_at && (
              <div className="flex items-center justify-between">
                <span className="text-ink-500">Posted</span>
                <span className="text-green">
                  {formatDateTime(period.posted_at)}
                </span>
              </div>
            )}
          </div>
        </div>

        <div className="rounded-2xl border border-line bg-surface p-5">
          <h3 className="mb-3 text-[15px] font-semibold text-ink-950">
            Metadata
          </h3>
          <div className="space-y-2 text-sm">
            <div className="flex items-center justify-between border-b border-line pb-2">
              <span className="text-ink-500">Version</span>
              <span className="font-mono font-medium text-ink-950">
                {period.version}
              </span>
            </div>
            <div className="flex items-center justify-between border-b border-line pb-2">
              <span className="text-ink-500">Calculation</span>
              <span className="font-mono text-ink-950">
                v{period.calculation_version}
              </span>
            </div>
            <div className="flex items-center justify-between border-b border-line pb-2">
              <span className="text-ink-500">Created by</span>
              <span className="font-mono text-ink-950">
                {period.created_by}
              </span>
            </div>
            {period.approved_by && (
              <div className="flex items-center justify-between border-b border-line pb-2">
                <span className="text-ink-500">Approved by</span>
                <span className="font-mono text-ink-950">
                  {period.approved_by}
                </span>
              </div>
            )}
            {period.wallet_operation_reference && (
              <div className="flex items-center justify-between">
                <span className="text-ink-500">Wallet ref</span>
                <span className="font-mono text-ink-950">
                  {period.wallet_operation_reference}
                </span>
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}