"use client";

import { useEffect, useState, useCallback } from "react";
import { useParams, useRouter } from "next/navigation";
import {
  ArrowLeft,
  RefreshCw,
  AlertCircle,
  CheckCircle2,
  Send,
  HandCoins,
  FileText,
} from "lucide-react";
import { financeService } from "@/lib/services/financeService";
import type { SubcontractorSettlementView } from "@/lib/types/finance";
import {
  formatDate,
  formatDateTime,
  formatMinor,
  settlementStatusLabel,
  settlementStatusTone,
  timeAgo,
} from "@/lib/utils/financeLabels";

export default function SettlementDetailPage() {
  const router = useRouter();
  const params = useParams();
  const settlementId = params.settlementId as string;

  const [settlement, setSettlement] =
    useState<SubcontractorSettlementView | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState<string | null>(null);

  const fetchSettlement = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const result = await financeService.settlements.get(settlementId);
      setSettlement(result);
    } catch (err: unknown) {
      const message =
        (err as { response?: { data?: { error?: { message?: string } } } })
          ?.response?.data?.error?.message ??
        (err as Error)?.message ??
        "Failed to load settlement.";
      setError(message);
    } finally {
      setLoading(false);
    }
  }, [settlementId]);

  useEffect(() => {
    if (settlementId) fetchSettlement();
  }, [settlementId, fetchSettlement]);

  const run = async (
    key: string,
    fn: () => Promise<SubcontractorSettlementView>
  ) => {
    setBusy(key);
    setError(null);
    try {
      const updated = await fn();
      setSettlement(updated);
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

  if (loading && !settlement) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent" />
        <p className="mt-3 text-sm text-ink-600">Loading settlement…</p>
      </div>
    );
  }

  if (error && !settlement) {
    return (
      <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-surface py-14">
        <div className="rounded-full bg-red-bg p-4">
          <AlertCircle className="h-8 w-8 text-red" />
        </div>
        <h3 className="mt-3 text-[15px] font-semibold text-ink-950">
          Could not load settlement
        </h3>
        <p className="mt-1 max-w-sm px-4 text-center text-sm text-ink-500">
          {error}
        </p>
        <div className="mt-3.5 flex gap-2">
          <button
            onClick={fetchSettlement}
            className="rounded-lg border border-line bg-surface px-4 py-1.5 text-[11.5px] font-medium text-ink-600 transition hover:bg-surface-2"
          >
            Retry
          </button>
          <button
            onClick={() => router.push("/finance/settlements")}
            className="rounded-lg bg-primary px-4 py-1.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90"
          >
            Back to settlements
          </button>
        </div>
      </div>
    );
  }

  if (!settlement) return null;

  const statusTone = settlementStatusTone(settlement.status);
  const canApprove =
    settlement.status === "DRAFT" ||
    settlement.status === "CALCULATED" ||
    settlement.status === "PENDING_APPROVAL";
  const canPost = settlement.status === "APPROVED";

  return (
    <div className="space-y-5">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="flex items-start gap-3">
          <button
            onClick={() => router.push("/finance/settlements")}
            className="mt-0.5 flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-xl border border-line bg-surface text-ink-500 transition hover:bg-surface-2 hover:text-ink-700"
            aria-label="Back to settlements"
          >
            <ArrowLeft className="h-4 w-4" />
          </button>
          <div>
            <div className="flex flex-wrap items-center gap-3">
              <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
                Settlement {settlement.id}
              </h1>
              <span
                className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-xs font-medium ${statusTone.bg} ${statusTone.text}`}
              >
                <span className={`h-1.5 w-1.5 rounded-full ${statusTone.dot}`} />
                {settlementStatusLabel(settlement.status)}
              </span>
              <span className="text-sm text-ink-400">v{settlement.version}</span>
            </div>
            <div className="mt-1 flex flex-wrap items-center gap-x-4 gap-y-1 text-sm text-ink-500">
              <span className="font-mono">{settlement.supplier_reference}</span>
              <span className="text-ink-300">·</span>
              <span className="font-mono">
                {settlement.provider_or_carrier_reference}
              </span>
            </div>
          </div>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <button
            onClick={fetchSettlement}
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
                  financeService.settlements.approve(
                    settlement.id,
                    settlement.version
                  )
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
          {canPost && (
            <button
              onClick={() =>
                run("post", () =>
                  financeService.settlements.post(
                    settlement.id,
                    settlement.version
                  )
                )
              }
              disabled={busy !== null}
              className="flex h-9 items-center gap-1.5 rounded-xl bg-primary px-3.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90 disabled:opacity-50"
            >
              {busy === "post" ? (
                <RefreshCw className="h-3.5 w-3.5 animate-spin" />
              ) : (
                <Send className="h-3.5 w-3.5" />
              )}
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

      <div className="grid grid-cols-1 gap-5 lg:grid-cols-[2fr_1fr]">
        <div className="space-y-5">
          <div className="rounded-2xl border border-line bg-surface p-6">
            <div className="flex items-center gap-2 text-sm text-ink-500">
              <span>
                {formatDate(settlement.period_start)} –{" "}
                {formatDate(settlement.period_end)}
              </span>
              <span className="text-ink-300">·</span>
              <span className="font-mono">{settlement.currency}</span>
            </div>
            <div className="mt-2">
              <span className="text-3xl font-bold text-ink-950">
                {formatMinor(settlement.amount_minor, settlement.currency)}
              </span>
            </div>
          </div>

          <div className="rounded-2xl border border-line bg-surface p-5">
            <div className="mb-3 flex items-center justify-between">
              <h3 className="text-[15px] font-semibold text-ink-950">
                Covered jobs
              </h3>
              <span className="text-xs text-ink-400">
                {settlement.job_references.length} job
                {settlement.job_references.length === 1 ? "" : "s"}
              </span>
            </div>
            {settlement.job_references.length === 0 ? (
              <div className="py-6 text-center text-sm text-ink-500">
                No jobs attached.
              </div>
            ) : (
              <div className="flex flex-wrap gap-1.5">
                {settlement.job_references.map((ref) => (
                  <span
                    key={ref}
                    className="rounded-full bg-surface-2 px-2.5 py-0.5 font-mono text-[11px] text-ink-700"
                  >
                    {ref}
                  </span>
                ))}
              </div>
            )}
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
                  {settlement.version}
                </span>
              </div>
              <div className="flex items-center justify-between border-b border-line pb-2">
                <span className="text-ink-500">Created by</span>
                <span className="font-mono text-ink-950">
                  {settlement.created_by}
                </span>
              </div>
              <div className="flex items-center justify-between border-b border-line pb-2">
                <span className="text-ink-500">Created</span>
                <span className="text-ink-950">
                  {timeAgo(settlement.created_at)}
                </span>
              </div>
              {settlement.approved_by && (
                <div className="flex items-center justify-between border-b border-line pb-2">
                  <span className="text-ink-500">Approved by</span>
                  <span className="font-mono text-ink-950">
                    {settlement.approved_by}
                  </span>
                </div>
              )}
              <div className="flex items-center justify-between">
                <span className="text-ink-500">Updated</span>
                <span className="text-ink-950">
                  {timeAgo(settlement.updated_at)}
                </span>
              </div>
            </div>
          </div>

          {(settlement.posting_reference ||
            settlement.payment_reference) && (
            <div className="rounded-2xl border border-line bg-surface p-5">
              <h3 className="mb-3 text-[15px] font-semibold text-ink-950">
                Posting
              </h3>
              <div className="space-y-2 text-sm">
                {settlement.posting_reference && (
                  <div className="flex items-center justify-between border-b border-line pb-2">
                    <span className="text-ink-500">Posting reference</span>
                    <span className="font-mono text-ink-950">
                      {settlement.posting_reference}
                    </span>
                  </div>
                )}
                {settlement.payment_reference && (
                  <div className="flex items-center justify-between">
                    <span className="text-ink-500">Payment reference</span>
                    <span className="font-mono text-ink-950">
                      {settlement.payment_reference}
                    </span>
                  </div>
                )}
              </div>
            </div>
          )}

          <div className="rounded-2xl border border-line bg-surface p-5">
            <h3 className="mb-3 text-[15px] font-semibold text-ink-950">
              Quick links
            </h3>
            <button
              onClick={() => router.push("/finance/settlements")}
              className="flex w-full items-center justify-between rounded-xl border border-line bg-surface px-4 py-2.5 text-[12.5px] font-medium text-ink-700 transition hover:bg-surface-2"
            >
              <span className="flex items-center gap-2">
                <HandCoins className="h-4 w-4" />
                All settlements
              </span>
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}