"use client";

import { useEffect, useState, useCallback } from "react";
import { useParams, useRouter } from "next/navigation";
import {
  ArrowLeft,
  RefreshCw,
  AlertCircle,
  Send,
  Edit3,
  XCircle,
  CheckCircle2,
  FileText,
  Receipt,
} from "lucide-react";
import { financeService } from "@/lib/services/financeService";
import type { InvoiceView } from "@/lib/types/finance";
import {
  formatDateTime,
  formatMinor,
  invoiceStatusLabel,
  invoiceStatusTone,
  timeAgo,
} from "@/lib/utils/financeLabels";

export default function InvoiceDetailPage() {
  const router = useRouter();
  const params = useParams();
  const invoiceId = params.invoiceId as string;

  const [invoice, setInvoice] = useState<InvoiceView | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState<string | null>(null);

  const fetchInvoice = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const result = await financeService.invoices.get(invoiceId);
      setInvoice(result);
    } catch (err: unknown) {
      const message =
        (err as { response?: { data?: { error?: { message?: string } } } })
          ?.response?.data?.error?.message ??
        (err as Error)?.message ??
        "Failed to load invoice.";
      setError(message);
    } finally {
      setLoading(false);
    }
  }, [invoiceId]);

  useEffect(() => {
    if (invoiceId) fetchInvoice();
  }, [invoiceId, fetchInvoice]);

  const run = async (key: string, fn: () => Promise<InvoiceView>) => {
    setBusy(key);
    setError(null);
    try {
      const updated = await fn();
      setInvoice(updated);
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

  if (loading && !invoice) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent" />
        <p className="mt-3 text-sm text-ink-600">Loading invoice…</p>
      </div>
    );
  }

  if (error && !invoice) {
    return (
      <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-surface py-14">
        <div className="rounded-full bg-red-bg p-4">
          <AlertCircle className="h-8 w-8 text-red" />
        </div>
        <h3 className="mt-3 text-[15px] font-semibold text-ink-950">
          Could not load invoice
        </h3>
        <p className="mt-1 max-w-sm px-4 text-center text-sm text-ink-500">
          {error}
        </p>
        <div className="mt-3.5 flex gap-2">
          <button
            onClick={fetchInvoice}
            className="rounded-lg border border-line bg-surface px-4 py-1.5 text-[11.5px] font-medium text-ink-600 transition hover:bg-surface-2"
          >
            Retry
          </button>
          <button
            onClick={() => router.push("/finance/invoices")}
            className="rounded-lg bg-primary px-4 py-1.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90"
          >
            Back to invoices
          </button>
        </div>
      </div>
    );
  }

  if (!invoice) return null;

  const statusTone = invoiceStatusTone(invoice.status);
  const isDraft =
    invoice.status === "DRAFT" || invoice.status === "PENDING_APPROVAL";
  const isApprovable = invoice.status === "DRAFT" || invoice.status === "PENDING_APPROVAL";
  const isIssuable = invoice.status === "APPROVED";
  const isCancellable =
    invoice.status !== "PAID" &&
    invoice.status !== "CANCELLED" &&
    invoice.status !== "CREDITED" &&
    invoice.status !== "WRITTEN_OFF";

  return (
    <div className="space-y-5">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="flex items-start gap-3">
          <button
            onClick={() => router.push("/finance/invoices")}
            className="mt-0.5 flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-xl border border-line bg-surface text-ink-500 transition hover:bg-surface-2 hover:text-ink-700"
            aria-label="Back to invoices"
          >
            <ArrowLeft className="h-4 w-4" />
          </button>
          <div>
            <div className="flex flex-wrap items-center gap-3">
              <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
                Invoice {invoice.id}
              </h1>
              <span
                className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-xs font-medium ${statusTone.bg} ${statusTone.text}`}
              >
                <span className={`h-1.5 w-1.5 rounded-full ${statusTone.dot}`} />
                {invoiceStatusLabel(invoice.status)}
              </span>
              <span className="text-sm text-ink-400">v{invoice.version}</span>
            </div>
            <div className="mt-1 flex flex-wrap items-center gap-x-4 gap-y-1 text-sm text-ink-500">
              <span className="font-mono">{invoice.customer_id}</span>
              <span className="text-ink-300">·</span>
              <span>{invoice.currency}</span>
            </div>
          </div>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <button
            onClick={fetchInvoice}
            disabled={loading}
            className="flex h-9 w-9 items-center justify-center rounded-xl border border-line bg-surface text-ink-500 transition hover:bg-surface-2 disabled:opacity-50"
            aria-label="Refresh"
          >
            <RefreshCw className={`h-3.5 w-3.5 ${loading ? "animate-spin" : ""}`} />
          </button>
          {isDraft && (
            <button
              onClick={() =>
                router.push(`/finance/invoices/${invoice.id}/edit`)
              }
              className="flex h-9 items-center gap-1.5 rounded-xl border border-line bg-surface px-3.5 text-[11.5px] font-medium text-ink-600 transition hover:bg-surface-2"
            >
              <Edit3 className="h-3.5 w-3.5" />
              Edit
            </button>
          )}
          {isApprovable && (
            <button
              onClick={() =>
                run("approve", () =>
                  financeService.invoices.approve(invoice.id, invoice.version)
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
          {isIssuable && (
            <button
              onClick={() =>
                run("issue", () =>
                  financeService.invoices.issue(invoice.id, invoice.version)
                )
              }
              disabled={busy !== null}
              className="flex h-9 items-center gap-1.5 rounded-xl bg-primary px-3.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90 disabled:opacity-50"
            >
              {busy === "issue" ? (
                <RefreshCw className="h-3.5 w-3.5 animate-spin" />
              ) : (
                <Send className="h-3.5 w-3.5" />
              )}
              Issue
            </button>
          )}
          {isCancellable && (
            <button
              onClick={() =>
                run("cancel", () =>
                  financeService.invoices.cancel(
                    invoice.id,
                    invoice.version,
                    "cancelled_by_staff"
                  )
                )
              }
              disabled={busy !== null}
              className="flex h-9 items-center gap-1.5 rounded-xl border border-red/30 bg-red-bg/50 px-3.5 text-[11.5px] font-medium text-red transition hover:bg-red-bg disabled:opacity-50"
            >
              {busy === "cancel" ? (
                <RefreshCw className="h-3.5 w-3.5 animate-spin" />
              ) : (
                <XCircle className="h-3.5 w-3.5" />
              )}
              Cancel
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
            <div className="flex flex-wrap items-center gap-3 text-sm text-ink-500">
              <span>{invoice.currency}</span>
              <span className="text-ink-300">·</span>
              <span>Due {invoice.due_date}</span>
              {invoice.issued_at && (
                <>
                  <span className="text-ink-300">·</span>
                  <span className="text-green">
                    Issued {formatDateTime(invoice.issued_at)}
                  </span>
                </>
              )}
              {invoice.paid_at && (
                <>
                  <span className="text-ink-300">·</span>
                  <span className="text-green">
                    Paid {formatDateTime(invoice.paid_at)}
                  </span>
                </>
              )}
            </div>
            <div className="mt-2">
              <span className="text-3xl font-bold text-ink-950">
                {formatMinor(invoice.total_minor, invoice.currency)}
              </span>
            </div>
            <div className="mt-3 grid grid-cols-3 gap-3 text-sm">
              <div>
                <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">
                  Subtotal
                </div>
                <div className="font-medium text-ink-950">
                  {formatMinor(invoice.subtotal_minor, invoice.currency)}
                </div>
              </div>
              <div>
                <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">
                  Tax
                </div>
                <div className="font-medium text-ink-950">
                  {formatMinor(invoice.tax_total_minor, invoice.currency)}
                </div>
              </div>
              <div>
                <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">
                  Adjustment
                </div>
                <div className="font-medium text-ink-950">
                  {formatMinor(invoice.adjustment_total_minor, invoice.currency)}
                </div>
              </div>
            </div>
          </div>

          <div className="rounded-2xl border border-line bg-surface p-5">
            <div className="mb-3 flex items-center justify-between">
              <h3 className="text-[15px] font-semibold text-ink-950">
                Line items
              </h3>
              <span className="text-xs text-ink-400">
                {invoice.lines.length} items
              </span>
            </div>
            <div className="overflow-x-auto">
              <table className="w-full border-collapse text-sm">
                <thead>
                  <tr className="bg-surface-2/70">
                    <th className="border-b border-line px-3 py-2 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">
                      Description
                    </th>
                    <th className="border-b border-line px-3 py-2 text-right text-[10px] font-bold uppercase tracking-wide text-ink-500">
                      Qty
                    </th>
                    <th className="border-b border-line px-3 py-2 text-right text-[10px] font-bold uppercase tracking-wide text-ink-500">
                      Unit
                    </th>
                    <th className="border-b border-line px-3 py-2 text-right text-[10px] font-bold uppercase tracking-wide text-ink-500">
                      Total
                    </th>
                  </tr>
                </thead>
                <tbody>
                  {invoice.lines.map((line) => (
                    <tr
                      key={line.id}
                      className="border-b border-line transition-colors hover:bg-surface-2"
                    >
                      <td className="px-3 py-2 text-ink-950">
                        {line.description}
                      </td>
                      <td className="px-3 py-2 text-right text-ink-600">
                        {line.quantity}
                      </td>
                      <td className="px-3 py-2 text-right text-ink-600">
                        {formatMinor(line.unit_amount_minor, invoice.currency)}
                      </td>
                      <td className="px-3 py-2 text-right font-medium text-ink-950">
                        {formatMinor(line.line_total_minor, invoice.currency)}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
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
                  {invoice.version}
                </span>
              </div>
              <div className="flex items-center justify-between border-b border-line pb-2">
                <span className="text-ink-500">Created</span>
                <span className="text-ink-950">{timeAgo(invoice.created_at)}</span>
              </div>
              <div className="flex items-center justify-between">
                <span className="text-ink-500">Updated</span>
                <span className="text-ink-950">{timeAgo(invoice.updated_at)}</span>
              </div>
            </div>
          </div>

          <div className="rounded-2xl border border-line bg-surface p-5">
            <h3 className="mb-3 text-[15px] font-semibold text-ink-950">
              Quick links
            </h3>
            <div className="space-y-2">
              <button
                onClick={() => router.push("/finance/receivables")}
                className="flex w-full items-center justify-between rounded-xl border border-line bg-surface px-4 py-2.5 text-[12.5px] font-medium text-ink-700 transition hover:bg-surface-2"
              >
                <span className="flex items-center gap-2">
                  <Receipt className="h-4 w-4" />
                  View receivables
                </span>
              </button>
              <button
                onClick={() => router.push("/finance/invoices")}
                className="flex w-full items-center justify-between rounded-xl border border-line bg-surface px-4 py-2.5 text-[12.5px] font-medium text-ink-700 transition hover:bg-surface-2"
              >
                <span className="flex items-center gap-2">
                  <FileText className="h-4 w-4" />
                  All invoices
                </span>
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}