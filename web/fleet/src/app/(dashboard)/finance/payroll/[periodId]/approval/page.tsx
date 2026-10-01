"use client";

import { useEffect, useState, useCallback, useMemo } from "react";
import { useParams, useRouter } from "next/navigation";
import {
  ArrowLeft,
  RefreshCw,
  AlertCircle,
  CheckCircle2,
  Send,
  RotateCcw,
  FileCheck,
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
} from "@/lib/utils/financeLabels";

export default function PayrollApprovalPage() {
  const router = useRouter();
  const params = useParams();
  const periodId = params.periodId as string;

  const [period, setPeriod] = useState<PayrollPeriodView | null>(null);
  const [loading, setLoading] = useState(true);
  const [approving, setApproving] = useState(false);
  const [posting, setPosting] = useState(false);
  const [reversing, setReversing] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState<string | null>(null);

  const [showReverse, setShowReverse] = useState(false);
  const [reason, setReason] = useState("");

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

  const handleApprove = async () => {
    if (!period || approving) return;
    setApproving(true);
    setError(null);
    setSuccess(null);
    try {
      const updated = await financeService.payrollPeriods.approve(
        period.id,
        period.version
      );
      setPeriod(updated);
      setSuccess("Payroll approved.");
    } catch (err: unknown) {
      const message =
        (err as { response?: { data?: { error?: { message?: string } } } })
          ?.response?.data?.error?.message ??
        (err as Error)?.message ??
        "Failed to approve payroll.";
      setError(message);
    } finally {
      setApproving(false);
    }
  };

  const handlePost = async () => {
    if (!period || posting) return;
    setPosting(true);
    setError(null);
    setSuccess(null);
    try {
      const updated = await financeService.payrollPeriods.post(
        period.id,
        period.version
      );
      setPeriod(updated);
      setSuccess("Payroll posted to provider sub-wallets.");
    } catch (err: unknown) {
      const message =
        (err as { response?: { data?: { error?: { message?: string } } } })
          ?.response?.data?.error?.message ??
        (err as Error)?.message ??
        "Failed to post payroll.";
      setError(message);
    } finally {
      setPosting(false);
    }
  };

  const handleReverse = async () => {
    if (!period || reversing || !reason.trim()) return;
    setReversing(true);
    setError(null);
    setSuccess(null);
    try {
      const updated = await financeService.payrollPeriods.reverse(
        period.id,
        period.version,
        reason.trim()
      );
      setPeriod(updated);
      setSuccess("Payroll reversed.");
      setShowReverse(false);
      setReason("");
    } catch (err: unknown) {
      const message =
        (err as { response?: { data?: { error?: { message?: string } } } })
          ?.response?.data?.error?.message ??
        (err as Error)?.message ??
        "Failed to reverse payroll.";
      setError(message);
    } finally {
      setReversing(false);
    }
  };

  const summary = useMemo(() => {
    if (!period) {
      return {
        totalApproved: 0,
        totalPending: 0,
        totalPosted: 0,
        totalAmount: 0,
      };
    }
    let approved = 0;
    let pending = 0;
    let posted = 0;
    let amount = 0;
    for (const item of period.items) {
      amount += item.amount_minor;
      if (item.status === "POSTED") posted++;
      else if (item.status === "APPROVED") approved++;
      else pending++;
    }
    return {
      totalApproved: approved,
      totalPending: pending,
      totalPosted: posted,
      totalAmount: amount,
    };
  }, [period]);

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
        <button
          onClick={fetchPeriod}
          className="mt-3.5 rounded-lg bg-primary px-4 py-1.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90"
        >
          Retry
        </button>
      </div>
    );
  }

  if (!period) return null;

  const statusTone = payrollStatusTone(period.status);
  const canApprove =
    period.status === "CALCULATED" || period.status === "PENDING_APPROVAL";
  const canPost = period.status === "APPROVED";
  const canReverse = period.status === "POSTED";

  return (
    <div className="space-y-5">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="flex items-start gap-3">
          <button
            onClick={() => router.push(`/finance/payroll/${period.id}`)}
            className="mt-0.5 flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-xl border border-line bg-surface text-ink-500 transition hover:bg-surface-2 hover:text-ink-700"
            aria-label="Back to period"
          >
            <ArrowLeft className="h-4 w-4" />
          </button>
          <div>
            <div className="flex flex-wrap items-center gap-3">
              <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
                Payroll Approval
              </h1>
              <span
                className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-xs font-medium ${statusTone.bg} ${statusTone.text}`}
              >
                <span className={`h-1.5 w-1.5 rounded-full ${statusTone.dot}`} />
                {payrollStatusLabel(period.status)}
              </span>
              <span className="text-sm text-ink-400">v{period.version}</span>
            </div>
            <p className="mt-0.5 text-sm text-ink-500">
              {formatDate(period.period_start)} –{" "}
              {formatDate(period.period_end)} ·{" "}
              {period.branch_id ?? "All branches"}
            </p>
          </div>
        </div>

        <div className="flex flex-wrap items-center gap-2">
          <button
            onClick={fetchPeriod}
            disabled={loading}
            className="flex h-9 w-9 items-center justify-center rounded-xl border border-line bg-surface text-ink-500 transition hover:bg-surface-2 disabled:opacity-50"
            aria-label="Reload"
          >
            <RefreshCw className={`h-3.5 w-3.5 ${loading ? "animate-spin" : ""}`} />
          </button>
          {canApprove && (
            <button
              onClick={handleApprove}
              disabled={approving}
              className="flex h-9 items-center gap-1.5 rounded-xl bg-primary px-3.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90 disabled:opacity-50"
            >
              {approving ? (
                <RefreshCw className="h-3.5 w-3.5 animate-spin" />
              ) : (
                <CheckCircle2 className="h-3.5 w-3.5" />
              )}
              Approve
            </button>
          )}
          {canPost && (
            <button
              onClick={handlePost}
              disabled={posting}
              className="flex h-9 items-center gap-1.5 rounded-xl bg-primary px-3.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90 disabled:opacity-50"
            >
              {posting ? (
                <RefreshCw className="h-3.5 w-3.5 animate-spin" />
              ) : (
                <Send className="h-3.5 w-3.5" />
              )}
              Post to wallets
            </button>
          )}
          {canReverse && (
            <button
              onClick={() => {
                setReason("");
                setShowReverse(true);
              }}
              className="flex h-9 items-center gap-1.5 rounded-xl border border-red/30 bg-red-bg/50 px-3.5 text-[11.5px] font-medium text-red transition hover:bg-red-bg"
            >
              <RotateCcw className="h-3.5 w-3.5" />
              Reverse
            </button>
          )}
        </div>
      </div>

      {success && (
        <div className="rounded-lg border border-green/20 bg-green-bg px-4 py-2.5 text-sm text-green">
          {success}
        </div>
      )}

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
            Posted
          </div>
          <div className="text-lg font-bold text-green">
            {summary.totalPosted}
          </div>
        </div>
        <div className="rounded-xl border border-line bg-surface p-3.5">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">
            Approved
          </div>
          <div className="text-lg font-bold text-ink-950">
            {summary.totalApproved}
          </div>
        </div>
        <div className="rounded-xl border border-line bg-surface p-3.5">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">
            Pending
          </div>
          <div className="text-lg font-bold text-amber">
            {summary.totalPending}
          </div>
        </div>
        <div className="rounded-xl border border-line bg-surface p-3.5">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">
            Total amount
          </div>
          <div className="text-lg font-bold text-ink-950">
            {formatMinor(summary.totalAmount, period.currency)}
          </div>
        </div>
      </div>

      <div className="rounded-2xl border border-line bg-surface p-5">
        <div className="mb-3 flex items-center justify-between">
          <h3 className="text-[15px] font-semibold text-ink-950">
            Items by posting state
          </h3>
          <span className="text-xs text-ink-400">
            {period.items.length} item{period.items.length === 1 ? "" : "s"}
          </span>
        </div>
        {period.items.length === 0 ? (
          <div className="py-8 text-center text-sm text-ink-500">
            <FileCheck className="mx-auto h-8 w-8 text-ink-400" />
            <p className="mt-2">No items to approve.</p>
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

      {period.posted_at && (
        <div className="rounded-2xl border border-line bg-surface p-5">
          <h3 className="mb-3 text-[15px] font-semibold text-ink-950">
            Posting details
          </h3>
          <div className="space-y-2 text-sm">
            <div className="flex items-center justify-between border-b border-line pb-2">
              <span className="text-ink-500">Posted at</span>
              <span className="text-green">
                {formatDateTime(period.posted_at)}
              </span>
            </div>
            {period.wallet_operation_reference && (
              <div className="flex items-center justify-between">
                <span className="text-ink-500">Wallet operation</span>
                <span className="font-mono text-ink-950">
                  {period.wallet_operation_reference}
                </span>
              </div>
            )}
          </div>
        </div>
      )}

      {showReverse && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 backdrop-blur-sm">
          <div className="w-full max-w-md rounded-2xl bg-surface p-6 shadow-2xl">
            <div className="flex items-center gap-2">
              <RotateCcw className="h-5 w-5 text-red" />
              <h3 className="text-lg font-semibold text-ink-950">
                Reverse payroll
              </h3>
            </div>
            <p className="mt-2 text-sm text-ink-600">
              This posts compensating entries to reverse the posted payroll. A
              reason is required and will be audited.
            </p>
            <div className="mt-4">
              <label className="mb-1 block text-xs font-medium uppercase tracking-[0.06em] text-ink-400">
                Reason
              </label>
              <textarea
                value={reason}
                onChange={(e) => setReason(e.target.value)}
                rows={3}
                className="w-full resize-none rounded-xl border border-line bg-surface px-4 py-2.5 text-sm text-ink-950 outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20"
              />
            </div>
            <div className="mt-5 flex gap-2.5">
              <button
                onClick={() => {
                  setShowReverse(false);
                  setReason("");
                }}
                className="flex-1 rounded-lg border border-line bg-surface px-4 py-2 text-sm font-medium text-ink-600 transition hover:bg-surface-2"
              >
                Cancel
              </button>
              <button
                onClick={handleReverse}
                disabled={reversing || !reason.trim()}
                className="flex-1 rounded-lg bg-red px-4 py-2 text-sm font-medium text-white transition hover:bg-red/90 disabled:cursor-not-allowed disabled:opacity-50"
              >
                {reversing ? (
                  <RefreshCw className="mr-1 inline h-4 w-4 animate-spin" />
                ) : null}
                Reverse
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}