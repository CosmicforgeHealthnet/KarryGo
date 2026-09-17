"use client";

import { useEffect, useState, useCallback } from "react";
import { useParams, useRouter } from "next/navigation";
import {
  ArrowLeft,
  RefreshCw,
  AlertCircle,
  Send,
  CheckCircle2,
  XCircle,
  RotateCcw,
  FileText,
  Wallet,
} from "lucide-react";
import { financeService } from "@/lib/services/financeService";
import type { ExpenseRow } from "@/lib/types/finance";
import {
  expenseStatusLabel,
  expenseStatusTone,
  formatDateTime,
  formatMinor,
  timeAgo,
} from "@/lib/utils/financeLabels";

export default function ExpenseDetailPage() {
  const router = useRouter();
  const params = useParams();
  const expenseId = params.expenseId as string;

  const [expense, setExpense] = useState<ExpenseRow | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState<string | null>(null);

  const [showReject, setShowReject] = useState(false);
  const [showReverse, setShowReverse] = useState(false);
  const [reason, setReason] = useState("");

  const fetchExpense = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const result = await financeService.expenses.get(expenseId);
      setExpense(result);
    } catch (err: unknown) {
      const message =
        (err as { response?: { data?: { error?: { message?: string } } } })
          ?.response?.data?.error?.message ??
        (err as Error)?.message ??
        "Failed to load expense.";
      setError(message);
    } finally {
      setLoading(false);
    }
  }, [expenseId]);

  useEffect(() => {
    if (expenseId) fetchExpense();
  }, [expenseId, fetchExpense]);

  const run = async (key: string, fn: () => Promise<ExpenseRow>) => {
    setBusy(key);
    setError(null);
    try {
      const updated = await fn();
      setExpense(updated);
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

  const handleReject = () => {
    if (!expense || !reason.trim()) return;
    run("reject", () =>
      financeService.expenses.reject(expense.id, expense.version, reason.trim())
    ).then(() => {
      setShowReject(false);
      setReason("");
    });
  };

  const handleReverse = () => {
    if (!expense || !reason.trim()) return;
    run("reverse", () =>
      financeService.expenses.reverse(expense.id, expense.version, reason.trim())
    ).then(() => {
      setShowReverse(false);
      setReason("");
    });
  };

  if (loading && !expense) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent" />
        <p className="mt-3 text-sm text-ink-600">Loading expense…</p>
      </div>
    );
  }

  if (error && !expense) {
    return (
      <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-surface py-14">
        <div className="rounded-full bg-red-bg p-4">
          <AlertCircle className="h-8 w-8 text-red" />
        </div>
        <h3 className="mt-3 text-[15px] font-semibold text-ink-950">
          Could not load expense
        </h3>
        <p className="mt-1 max-w-sm px-4 text-center text-sm text-ink-500">
          {error}
        </p>
        <div className="mt-3.5 flex gap-2">
          <button
            onClick={fetchExpense}
            className="rounded-lg border border-line bg-surface px-4 py-1.5 text-[11.5px] font-medium text-ink-600 transition hover:bg-surface-2"
          >
            Retry
          </button>
          <button
            onClick={() => router.push("/finance/expenses")}
            className="rounded-lg bg-primary px-4 py-1.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90"
          >
            Back to expenses
          </button>
        </div>
      </div>
    );
  }

  if (!expense) return null;

  const statusTone = expenseStatusTone(expense.status);
  const canSubmit = expense.status === "DRAFT";
  const canApprove =
    expense.status === "SUBMITTED" || expense.status === "PENDING_APPROVAL";
  const canReject =
    expense.status === "SUBMITTED" || expense.status === "PENDING_APPROVAL";
  const canReverse =
    expense.status === "APPROVED" || expense.status === "POSTED";

  return (
    <div className="space-y-5">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="flex items-start gap-3">
          <button
            onClick={() => router.push("/finance/expenses")}
            className="mt-0.5 flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-xl border border-line bg-surface text-ink-500 transition hover:bg-surface-2 hover:text-ink-700"
            aria-label="Back to expenses"
          >
            <ArrowLeft className="h-4 w-4" />
          </button>
          <div>
            <div className="flex flex-wrap items-center gap-3">
              <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
                {expense.description}
              </h1>
              <span
                className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-xs font-medium ${statusTone.bg} ${statusTone.text}`}
              >
                <span className={`h-1.5 w-1.5 rounded-full ${statusTone.dot}`} />
                {expenseStatusLabel(expense.status)}
              </span>
              <span className="text-sm text-ink-400">v{expense.version}</span>
            </div>
            <div className="mt-1 flex flex-wrap items-center gap-x-4 gap-y-1 text-sm text-ink-500">
              <span className="font-mono">{expense.id}</span>
              <span className="text-ink-300">·</span>
              <span>{expense.category}</span>
              <span className="text-ink-300">·</span>
              <span>{expense.cost_centre}</span>
            </div>
          </div>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <button
            onClick={fetchExpense}
            disabled={loading}
            className="flex h-9 w-9 items-center justify-center rounded-xl border border-line bg-surface text-ink-500 transition hover:bg-surface-2 disabled:opacity-50"
            aria-label="Refresh"
          >
            <RefreshCw className={`h-3.5 w-3.5 ${loading ? "animate-spin" : ""}`} />
          </button>
          {canSubmit && (
            <button
              onClick={() =>
                run("submit", () =>
                  financeService.expenses.submit(expense.id, expense.version)
                )
              }
              disabled={busy !== null}
              className="flex h-9 items-center gap-1.5 rounded-xl bg-primary px-3.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90 disabled:opacity-50"
            >
              {busy === "submit" ? (
                <RefreshCw className="h-3.5 w-3.5 animate-spin" />
              ) : (
                <Send className="h-3.5 w-3.5" />
              )}
              Submit
            </button>
          )}
          {canApprove && (
            <button
              onClick={() =>
                run("approve", () =>
                  financeService.expenses.approve(expense.id, expense.version)
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
          {canReject && (
            <button
              onClick={() => {
                setReason("");
                setShowReject(true);
              }}
              className="flex h-9 items-center gap-1.5 rounded-xl border border-red/30 bg-red-bg/50 px-3.5 text-[11.5px] font-medium text-red transition hover:bg-red-bg"
            >
              <XCircle className="h-3.5 w-3.5" />
              Reject
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
            <div className="flex items-center gap-2 text-sm text-ink-500">
              <span className="rounded-full bg-surface-2 px-2.5 py-0.5 text-[10px] font-semibold text-ink-700">
                {expense.category}
              </span>
              <span className="text-ink-300">·</span>
              <span>{expense.cost_centre}</span>
              {expense.branch_id && (
                <>
                  <span className="text-ink-300">·</span>
                  <span className="font-mono">{expense.branch_id}</span>
                </>
              )}
            </div>
            <div className="mt-2">
              <span className="text-3xl font-bold text-ink-950">
                {formatMinor(expense.amount_minor, expense.currency)}
              </span>
              <span className="ml-2 text-sm text-ink-400">
                {expense.currency}
              </span>
            </div>
            <div className="mt-3 flex flex-wrap items-center gap-4 text-sm text-ink-500">
              <span className="flex items-center gap-1.5">
                <Wallet className="h-4 w-4" />
                <span className="font-mono">{expense.submitted_by}</span>
              </span>
              {expense.approved_at && (
                <>
                  <span className="text-ink-300">·</span>
                  <span className="flex items-center gap-1.5 text-green">
                    <CheckCircle2 className="h-4 w-4" />
                    Approved {formatDateTime(expense.approved_at)}
                  </span>
                </>
              )}
              {expense.rejected_at && (
                <>
                  <span className="text-ink-300">·</span>
                  <span className="flex items-center gap-1.5 text-red">
                    <XCircle className="h-4 w-4" />
                    Rejected {formatDateTime(expense.rejected_at)}
                  </span>
                </>
              )}
            </div>
          </div>

          <div className="rounded-2xl border border-line bg-surface p-5">
            <h3 className="mb-3 text-[15px] font-semibold text-ink-950">
              Description
            </h3>
            <p className="whitespace-pre-wrap text-sm text-ink-600">
              {expense.description}
            </p>
          </div>
        </div>

        <div className="space-y-5">
          <div className="rounded-2xl border border-line bg-surface p-5">
            <h3 className="mb-3 text-[15px] font-semibold text-ink-950">
              References
            </h3>
            <div className="space-y-2 text-sm">
              {expense.asset_id && (
                <div className="flex items-center justify-between border-b border-line pb-2">
                  <span className="text-ink-500">Asset</span>
                  <span className="font-mono text-ink-950">
                    {expense.asset_id}
                  </span>
                </div>
              )}
              {expense.order_id && (
                <div className="flex items-center justify-between border-b border-line pb-2">
                  <span className="text-ink-500">Order</span>
                  <span className="font-mono text-ink-950">
                    {expense.order_id}
                  </span>
                </div>
              )}
              {expense.shipment_id && (
                <div className="flex items-center justify-between border-b border-line pb-2">
                  <span className="text-ink-500">Shipment</span>
                  <span className="font-mono text-ink-950">
                    {expense.shipment_id}
                  </span>
                </div>
              )}
              {expense.work_order_id && (
                <div className="flex items-center justify-between border-b border-line pb-2">
                  <span className="text-ink-500">Work order</span>
                  <span className="font-mono text-ink-950">
                    {expense.work_order_id}
                  </span>
                </div>
              )}
              {expense.provider_membership_id && (
                <div className="flex items-center justify-between border-b border-line pb-2">
                  <span className="text-ink-500">Provider</span>
                  <span className="font-mono text-ink-950">
                    {expense.provider_membership_id}
                  </span>
                </div>
              )}
              {expense.incident_id && (
                <div className="flex items-center justify-between border-b border-line pb-2">
                  <span className="text-ink-500">Incident</span>
                  <span className="font-mono text-ink-950">
                    {expense.incident_id}
                  </span>
                </div>
              )}
              {expense.receipt_media_id && (
                <div className="flex items-center justify-between">
                  <span className="text-ink-500">Receipt</span>
                  <span className="font-mono text-ink-950">
                    {expense.receipt_media_id}
                  </span>
                </div>
              )}
              {!expense.asset_id &&
                !expense.order_id &&
                !expense.shipment_id &&
                !expense.work_order_id &&
                !expense.provider_membership_id &&
                !expense.incident_id &&
                !expense.receipt_media_id && (
                  <div className="py-2 text-center text-xs text-ink-400">
                    No references
                  </div>
                )}
            </div>
          </div>

          {expense.posting_reference && (
            <div className="rounded-2xl border border-line bg-surface p-5">
              <h3 className="mb-3 text-[15px] font-semibold text-ink-950">
                Posting
              </h3>
              <div className="text-sm">
                <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">
                  Reference
                </div>
                <div className="font-mono text-ink-950">
                  {expense.posting_reference}
                </div>
              </div>
            </div>
          )}

          <div className="rounded-2xl border border-line bg-surface p-5">
            <h3 className="mb-3 text-[15px] font-semibold text-ink-950">
              Metadata
            </h3>
            <div className="space-y-2 text-sm">
              <div className="flex items-center justify-between border-b border-line pb-2">
                <span className="text-ink-500">Version</span>
                <span className="font-mono font-medium text-ink-950">
                  {expense.version}
                </span>
              </div>
              <div className="flex items-center justify-between border-b border-line pb-2">
                <span className="text-ink-500">Created</span>
                <span className="text-ink-950">
                  {timeAgo(expense.created_at)}
                </span>
              </div>
              <div className="flex items-center justify-between">
                <span className="text-ink-500">Updated</span>
                <span className="text-ink-950">
                  {timeAgo(expense.updated_at)}
                </span>
              </div>
            </div>
          </div>

          <div className="rounded-2xl border border-line bg-surface p-5">
            <h3 className="mb-3 text-[15px] font-semibold text-ink-950">
              Quick links
            </h3>
            <button
              onClick={() => router.push("/finance/expenses")}
              className="flex w-full items-center justify-between rounded-xl border border-line bg-surface px-4 py-2.5 text-[12.5px] font-medium text-ink-700 transition hover:bg-surface-2"
            >
              <span className="flex items-center gap-2">
                <FileText className="h-4 w-4" />
                All expenses
              </span>
            </button>
          </div>
        </div>
      </div>

      {showReject && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 backdrop-blur-sm">
          <div className="w-full max-w-md rounded-2xl bg-surface p-6 shadow-2xl">
            <div className="flex items-center gap-2">
              <XCircle className="h-5 w-5 text-red" />
              <h3 className="text-lg font-semibold text-ink-950">
                Reject expense
              </h3>
            </div>
            <p className="mt-2 text-sm text-ink-600">
              A reason is required and will be recorded on the expense history.
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
                  setShowReject(false);
                  setReason("");
                }}
                className="flex-1 rounded-lg border border-line bg-surface px-4 py-2 text-sm font-medium text-ink-600 transition hover:bg-surface-2"
              >
                Cancel
              </button>
              <button
                onClick={handleReject}
                disabled={busy !== null || !reason.trim()}
                className="flex-1 rounded-lg bg-red px-4 py-2 text-sm font-medium text-white transition hover:bg-red/90 disabled:cursor-not-allowed disabled:opacity-50"
              >
                {busy === "reject" ? (
                  <RefreshCw className="mr-1 inline h-4 w-4 animate-spin" />
                ) : null}
                Reject
              </button>
            </div>
          </div>
        </div>
      )}

      {showReverse && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 backdrop-blur-sm">
          <div className="w-full max-w-md rounded-2xl bg-surface p-6 shadow-2xl">
            <div className="flex items-center gap-2">
              <RotateCcw className="h-5 w-5 text-red" />
              <h3 className="text-lg font-semibold text-ink-950">
                Reverse expense
              </h3>
            </div>
            <p className="mt-2 text-sm text-ink-600">
              Reversing posts a compensating operation to the ledger. A reason
              is required.
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
                disabled={busy !== null || !reason.trim()}
                className="flex-1 rounded-lg bg-red px-4 py-2 text-sm font-medium text-white transition hover:bg-red/90 disabled:cursor-not-allowed disabled:opacity-50"
              >
                {busy === "reverse" ? (
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