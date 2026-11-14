// src/app/(dashboard)/settings/approvals/[requestId]/page.tsx

"use client";

import { useCallback, useEffect, useState } from "react";
import { useParams, useRouter } from "next/navigation";
import {
  FileCheck,
  RefreshCw,
  Clock,
  CheckCircle2,
  AlertTriangle,
  ArrowLeft,
  ListChecks,
  History,
  Check,
  X,
} from "lucide-react";
import { ApiErrorNotice } from "@/components/features/ApiErrorNotice";
import { formatMinor, humanize } from "@/components/features/OpsUi";
import { describeApiError, newIdempotencyKey, type ApiErrorInfo } from "@/lib/api/errors";
import { approvalRequestService } from "@/lib/services/governanceService";
import type { ApprovalDecision, ApprovalHistoryEntry, ApprovalRequestDetail } from "@/lib/types/governance";

const STATUS_STYLES: Record<string, { bg: string; text: string; icon: React.ReactNode }> = {
  PENDING: { bg: "bg-amber-bg", text: "text-amber", icon: <Clock className="h-3.5 w-3.5" /> },
  APPROVED: { bg: "bg-green-bg", text: "text-green", icon: <CheckCircle2 className="h-3.5 w-3.5" /> },
  REJECTED: { bg: "bg-red-bg", text: "text-red", icon: <X className="h-3.5 w-3.5" /> },
  CANCELLED: { bg: "bg-surface-2", text: "text-ink-500", icon: <X className="h-3.5 w-3.5" /> },
  EXPIRED: { bg: "bg-surface-2", text: "text-ink-500", icon: <Clock className="h-3.5 w-3.5" /> },
  INVALIDATED: { bg: "bg-surface-2", text: "text-ink-500", icon: <AlertTriangle className="h-3.5 w-3.5" /> },
  EXECUTED: { bg: "bg-blue-bg", text: "text-blue", icon: <Check className="h-3.5 w-3.5" /> },
};

const DECISION_STYLES: Record<string, { bg: string; text: string; icon: React.ReactNode }> = {
  APPROVED: { bg: "bg-green-bg", text: "text-green", icon: <CheckCircle2 className="h-4 w-4" /> },
  REJECTED: { bg: "bg-red-bg", text: "text-red", icon: <X className="h-4 w-4" /> },
};

type Command = "approve" | "reject" | "cancel";

const formatDateTime = (date: string | null | undefined) => {
  if (!date) return "N/A";
  return new Date(date).toLocaleString("en-GB", {
    day: "2-digit",
    month: "short",
    year: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
};

export default function ApprovalRequestDetailPage() {
  const params = useParams();
  const router = useRouter();
  const requestId = params.requestId as string;

  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<ApprovalRequestDetail | null>(null);
  const [decisions, setDecisions] = useState<ApprovalDecision[]>([]);
  const [history, setHistory] = useState<ApprovalHistoryEntry[]>([]);
  const [error, setError] = useState<ApiErrorInfo | null>(null);
  const [secondsAgo, setSecondsAgo] = useState(0);

  const [command, setCommand] = useState<Command | null>(null);
  const [reason, setReason] = useState("");
  // A decision id stays the same across retries of one attempt.
  const [clientDecisionId, setClientDecisionId] = useState(newIdempotencyKey);
  const [busy, setBusy] = useState(false);
  const [actionError, setActionError] = useState<ApiErrorInfo | null>(null);
  const [notice, setNotice] = useState<string | null>(null);

  const load = useCallback(async () => {
    try {
      setData(await approvalRequestService.get(requestId));
      setError(null);
      approvalRequestService.decisions(requestId).then(setDecisions).catch(() => setDecisions([]));
      approvalRequestService.history(requestId).then(setHistory).catch(() => setHistory([]));
    } catch (e) {
      setError(describeApiError(e, "The approval request could not be loaded."));
    } finally {
      setSecondsAgo(0);
      setLoading(false);
    }
  }, [requestId]);

  useEffect(() => {
    Promise.resolve().then(load);
  }, [load]);

  useEffect(() => {
    const interval = setInterval(() => setSecondsAgo((s) => s + 1), 1000);
    return () => clearInterval(interval);
  }, []);

  if (loading) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
        <p className="mt-3 text-sm text-ink-600">Loading approval details...</p>
      </div>
    );
  }

  if (!data) {
    return (
      <div className="space-y-4">
        <ApiErrorNotice error={error} />
        <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
          <div className="rounded-full bg-surface-2 p-4">
            <FileCheck className="h-8 w-8 text-ink-400" />
          </div>
          <h3 className="mt-3 text-[15px] font-semibold text-ink-950">Request not found</h3>
          <p className="mt-1 text-sm text-ink-500">No approval request found with this ID.</p>
          <button
            onClick={() => router.push("/settings/approvals")}
            className="mt-3.5 rounded-lg bg-primary px-4 py-1.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90"
          >
            Back to Approvals
          </button>
        </div>
      </div>
    );
  }

  const statusStyle = STATUS_STYLES[data.status] || STATUS_STYLES.PENDING;
  const approvedCount = decisions.filter((d) => d.decision === "APPROVED").length;

  const openCommand = (next: Command) => {
    setCommand(next);
    setReason("");
    setActionError(null);
    setClientDecisionId(newIdempotencyKey());
  };

  const submit = async () => {
    if (!command) return;
    setBusy(true);
    setActionError(null);
    try {
      const updated =
        command === "cancel"
          ? await approvalRequestService.cancel(data.id, reason.trim())
          : await approvalRequestService.decide(data.id, command, {
              reason: reason.trim(),
              clientDecisionId,
              expectedFingerprint: data.requestFingerprint,
            });
      setData({ ...data, ...updated });
      setNotice(command === "approve" ? "Your approval was recorded." : command === "reject" ? "The request was rejected." : "The request was cancelled.");
      setCommand(null);
      load();
    } catch (e) {
      setActionError(describeApiError(e, "The decision could not be recorded."));
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="space-y-5 max-w-4xl mx-auto">
      {/* ===== HEADER ===== */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="min-w-0">
          <div className="flex items-center gap-2 text-sm text-ink-500">
            <button
              onClick={() => router.push("/settings/approvals")}
              className="hover:text-primary transition-colors"
            >
              Approvals
            </button>
            <span>/</span>
            <span className="text-ink-700 font-medium font-mono">{data.id}</span>
          </div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl mt-1">
            Approval Request Detail
          </h1>
          <div className="mt-1 flex flex-wrap items-center gap-2">
            <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${statusStyle.bg} ${statusStyle.text}`}>
              {statusStyle.icon}
              {humanize(data.status)}
            </span>
            <span className="text-[10px] text-ink-500">{humanize(data.action)}</span>
            {data.branchId && <span className="text-[10px] text-ink-500">{data.branchId}</span>}
            {data.separationOfDuties && (
              <span className="rounded-full bg-surface-2 px-2 py-0.5 text-[9px] font-medium text-ink-500">Separation of duties</span>
            )}
            {!data.selfApprovalAllowed && (
              <span className="rounded-full bg-surface-2 px-2 py-0.5 text-[9px] font-medium text-ink-500">No self-approval</span>
            )}
          </div>
        </div>
        <div className="flex flex-wrap items-center gap-2 flex-shrink-0">
          <button
            onClick={load}
            className="flex h-9 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3 text-[11.5px] font-medium text-ink-500 shadow-sm transition hover:bg-white"
          >
            <RefreshCw className="h-3 w-3" />
            {secondsAgo}s
          </button>
          {data.status === "PENDING" && (
            <>
              <button
                onClick={() => openCommand("approve")}
                className="flex h-9 items-center gap-1.5 rounded-xl bg-green px-3.5 text-[11.5px] font-semibold text-white shadow-sm shadow-green/20 transition hover:bg-green/90"
              >
                <CheckCircle2 className="h-3.5 w-3.5" />
                Approve
              </button>
              <button
                onClick={() => openCommand("reject")}
                className="flex h-9 items-center gap-1.5 rounded-xl bg-red px-3.5 text-[11.5px] font-semibold text-white shadow-sm shadow-red/20 transition hover:bg-red/90"
              >
                <X className="h-3.5 w-3.5" />
                Reject
              </button>
              <button
                onClick={() => openCommand("cancel")}
                className="flex h-9 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-[11.5px] font-medium text-ink-600 shadow-sm transition hover:bg-white"
              >
                <X className="h-3.5 w-3.5" />
                Cancel
              </button>
            </>
          )}
          <button
            onClick={() => router.push("/settings/approvals")}
            className="flex h-9 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-[11.5px] font-medium text-ink-600 shadow-sm transition hover:bg-white"
          >
            <ArrowLeft className="h-3.5 w-3.5" />
            Back
          </button>
        </div>
      </div>

      <ApiErrorNotice error={error} />
      {notice && <div role="status" className="rounded-xl border border-green/30 bg-green-bg px-4 py-3 text-sm text-green">{notice}</div>}

      {command && (
        <div className="space-y-3 rounded-2xl border border-line bg-white p-5 shadow-sm">
          <div className="flex items-start justify-between gap-2">
            <div>
              <h3 className="text-[15px] font-semibold text-ink-950">
                {command === "approve" ? "Approve request" : command === "reject" ? "Reject request" : "Cancel request"}
              </h3>
              <p className="mt-0.5 text-xs text-ink-500">
                {command === "cancel"
                  ? "Cancelling withdraws the request."
                  : "You may be asked to confirm it is you before the decision is recorded."}
              </p>
            </div>
            <button type="button" onClick={() => setCommand(null)} aria-label="Close" className="text-ink-400 hover:text-ink-700">
              <X className="h-4 w-4" />
            </button>
          </div>
          <ApiErrorNotice error={actionError} />
          <label className="mb-1.5 block text-xs font-medium uppercase tracking-wide text-ink-500">Reason</label>
          <textarea
            rows={2}
            value={reason}
            onChange={(e) => setReason(e.target.value)}
            className="w-full rounded-lg border border-line bg-surface px-3 py-2.5 text-sm text-ink-950 outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
          />
          <button
            onClick={submit}
            disabled={busy}
            className={`flex h-9 items-center gap-1.5 rounded-xl px-3.5 text-[11.5px] font-semibold text-white shadow-sm transition disabled:opacity-60 ${
              command === "approve" ? "bg-green hover:bg-green/90" : command === "reject" ? "bg-red hover:bg-red/90" : "bg-ink-700 hover:bg-ink-700/90"
            }`}
          >
            {busy ? "Saving..." : command === "approve" ? "Confirm approval" : command === "reject" ? "Confirm rejection" : "Confirm cancellation"}
          </button>
        </div>
      )}

      {/* ===== OVERVIEW GRID ===== */}
      <div className="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-4">
        <div className="rounded-xl border border-line bg-white p-4 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Resource</div>
          <div className="mt-1 text-sm font-medium text-ink-950">{humanize(data.resourceType)}</div>
          <div className="mt-0.5 text-xs text-ink-500 font-mono break-all">{data.resourceId}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-4 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Requester</div>
          <div className="mt-1 text-sm font-medium text-ink-950 font-mono break-all">{data.requesterId}</div>
          <div className="mt-0.5 text-xs text-ink-500">Raised {formatDateTime(data.createdAt)}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-4 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Approvals</div>
          <div className="mt-1 text-sm font-medium text-ink-950">
            {approvedCount}/{data.minimumApprovals}
          </div>
          <div className="mt-0.5 text-xs text-ink-500">
            Step {data.currentStep} of {data.steps.length || 1}
          </div>
        </div>
        <div className="rounded-xl border border-line bg-white p-4 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Expiry</div>
          <div className="mt-1 text-sm font-medium text-ink-950">{formatDateTime(data.expiresAt)}</div>
          {data.amountMinor != null && (
            <div className="mt-0.5 text-xs text-ink-500">Amount {formatMinor(data.amountMinor, data.amountCurrency)}</div>
          )}
        </div>
      </div>

      {(data.invalidationReason || data.executedAt) && (
        <div className="rounded-xl border border-line bg-white p-4 text-sm text-ink-700 shadow-sm">
          {data.invalidationReason && <p>Invalidated: {humanize(data.invalidationReason)}</p>}
          {data.executedAt && (
            <p>
              Executed {formatDateTime(data.executedAt)}
              {data.executionReference ? ` · ${data.executionReference}` : ""}
            </p>
          )}
        </div>
      )}

      {/* ===== STEPS ===== */}
      {data.steps.length > 0 && (
        <div className="rounded-2xl border border-line bg-white shadow-sm overflow-hidden">
          <div className="p-4 border-b border-line bg-surface-2/50">
            <div className="flex items-center gap-2">
              <ListChecks className="h-4 w-4 text-ink-400" />
              <span className="text-sm font-semibold text-ink-950">Approval steps</span>
            </div>
          </div>
          <div className="divide-y divide-line">
            {data.steps.map((step) => (
              <div key={step.stepNumber} className="flex flex-wrap items-center justify-between gap-2 p-4 text-sm">
                <span className="font-medium text-ink-950">Step {step.stepNumber}</span>
                <span className="text-xs text-ink-500">Quorum {step.requiredQuorum}</span>
                <span className="text-xs text-ink-500">{step.eligibleRoleIds.length} eligible role(s)</span>
                <span className="text-xs text-ink-600">{humanize(step.status)}</span>
                <span className="text-xs text-ink-400">{step.completedAt ? formatDateTime(step.completedAt) : "—"}</span>
              </div>
            ))}
          </div>
        </div>
      )}

      {/* ===== DECISIONS ===== */}
      <div className="rounded-2xl border border-line bg-white shadow-sm overflow-hidden">
        <div className="p-4 border-b border-line bg-surface-2/50">
          <div className="flex items-center gap-2">
            <ListChecks className="h-4 w-4 text-ink-400" />
            <span className="text-sm font-semibold text-ink-950">Approvals</span>
            <span className="text-xs text-ink-400">{decisions.length} decided</span>
          </div>
        </div>
        <div className="divide-y divide-line">
          {decisions.length === 0 && <p className="p-4 text-sm text-ink-500">No decisions recorded yet.</p>}
          {decisions.map((d) => {
            const decisionStyle = DECISION_STYLES[d.decision] || DECISION_STYLES.APPROVED;
            return (
              <div key={d.id} className="flex items-center gap-4 p-4">
                <div className="flex-shrink-0">
                  <div className={`rounded-full p-2 ${decisionStyle.bg} ${decisionStyle.text}`}>{decisionStyle.icon}</div>
                </div>
                <div className="flex-1 min-w-0">
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="font-medium font-mono text-ink-950">{d.approverId}</span>
                    <span className={`flex items-center gap-1 rounded-full px-2 py-0.5 text-[9px] font-semibold ${decisionStyle.bg} ${decisionStyle.text}`}>
                      {humanize(d.decision)}
                    </span>
                    {d.onBehalfOf && <span className="text-[10px] text-ink-500">on behalf of {d.onBehalfOf}</span>}
                  </div>
                  {d.reason && <div className="mt-0.5 text-xs text-ink-500">{d.reason}</div>}
                  <div className="mt-0.5 text-[10px] text-ink-400">Decided {formatDateTime(d.decidedAt)}</div>
                </div>
              </div>
            );
          })}
        </div>
      </div>

      {/* ===== HISTORY ===== */}
      <div className="rounded-2xl border border-line bg-white shadow-sm overflow-hidden">
        <div className="p-4 border-b border-line bg-surface-2/50">
          <div className="flex items-center gap-2">
            <History className="h-4 w-4 text-ink-400" />
            <span className="text-sm font-semibold text-ink-950">History</span>
          </div>
        </div>
        <div className="divide-y divide-line">
          {history.length === 0 && <p className="p-4 text-sm text-ink-500">Nothing recorded yet.</p>}
          {history.map((h) => (
            <div key={h.id} className="flex flex-wrap items-center justify-between gap-2 p-4 text-xs text-ink-600">
              <span className="font-medium text-ink-950">
                {h.fromStatus ? `${humanize(h.fromStatus)} → ` : ""}
                {humanize(h.toStatus)}
              </span>
              <span>{[h.reasonCode ? humanize(h.reasonCode) : "", humanize(h.actorType)].filter(Boolean).join(" · ")}</span>
              <span className="text-ink-400">{formatDateTime(h.occurredAt)}</span>
            </div>
          ))}
        </div>
      </div>

      {/* ===== FOOTER ===== */}
      <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-line bg-white/50 px-4 py-3 text-sm text-ink-400">
        <span className="flex items-center gap-2">
          <FileCheck className="h-4 w-4" />
          IAM-10 · Approval Request Detail · {data.id}
        </span>
        <span className="flex items-center gap-1.5">
          <Clock className="h-4 w-4" />
          Updated {secondsAgo}s ago
        </span>
      </div>
    </div>
  );
}
