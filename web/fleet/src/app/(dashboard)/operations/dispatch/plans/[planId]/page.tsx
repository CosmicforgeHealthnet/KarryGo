// src/app/(dashboard)/operations/dispatch/plans/[planId]/page.tsx

"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { useParams, useRouter } from "next/navigation";
import { AlertCircle, ArrowLeft, CheckCircle2, Send, XCircle } from "lucide-react";
import EmptyState from "@/components/EmptyState";
import {
  dispatchService,
  newIdempotencyKey,
} from "@/lib/services/dispatchService";
import type { DispatchPlan } from "@/lib/types/dispatch";

function statusStyle(status: string): { bg: string; text: string; dot: string } {
  switch ((status || "").toUpperCase()) {
    case "DRAFT":       return { bg: "bg-surface-2", text: "text-ink-600", dot: "bg-ink-400" };
    case "READY":       return { bg: "bg-primary-bg", text: "text-primary", dot: "bg-primary" };
    case "APPROVED":    return { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber" };
    case "PUBLISHED":   return { bg: "bg-green-bg", text: "text-green", dot: "bg-green" };
    case "IN_PROGRESS": return { bg: "bg-primary-bg", text: "text-primary", dot: "bg-primary" };
    case "COMPLETED":   return { bg: "bg-green-bg", text: "text-green", dot: "bg-green" };
    case "CANCELLED":   return { bg: "bg-red-bg", text: "text-red", dot: "bg-red" };
    default:            return { bg: "bg-surface-2", text: "text-ink-500", dot: "bg-ink-400" };
  }
}

function statusLabel(status: string): string {
  switch ((status || "").toUpperCase()) {
    case "DRAFT":       return "Draft";
    case "READY":       return "Ready";
    case "APPROVED":    return "Approved";
    case "PUBLISHED":   return "Published";
    case "IN_PROGRESS": return "In progress";
    case "COMPLETED":   return "Completed";
    case "CANCELLED":   return "Cancelled";
    default:            return status || "—";
  }
}

function formatTimestamp(iso: string | undefined): string {
  if (!iso) return "—";
  const t = new Date(iso).getTime();
  if (!Number.isFinite(t)) return "—";
  return new Date(t).toLocaleString(undefined, {
    year: "numeric", month: "short", day: "numeric",
    hour: "2-digit", minute: "2-digit",
  });
}

export default function DispatchPlanDetailPage() {
  const params = useParams();
  const router = useRouter();
  const planId = typeof params.planId === "string" ? params.planId : "";

  const [plan, setPlan] = useState<DispatchPlan | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const [busy, setBusy] = useState<"approve" | "publish" | "cancel" | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);
  const [actionKey, setActionKey] = useState(() => newIdempotencyKey());

  useEffect(() => {
    if (!planId) return;
    let cancelled = false;
    setLoading(true);
    setError(null);
    dispatchService
      .getPlan(planId)
      .then((data) => {
        if (!cancelled) setPlan(data);
      })
      .catch((err) => {
        if (cancelled) return;
        const status = err?.response?.status;
        if (status === 404) {
          setError("Plan not found.");
        } else {
          const message =
            err?.response?.data?.error?.message ??
            err?.message ??
            "Failed to load plan.";
          setError(message);
        }
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [planId]);

  const handleAction = async (kind: "approve" | "publish" | "cancel") => {
    if (!plan) return;
    setBusy(kind);
    setActionError(null);
    try {
      const input = { expectedVersion: plan.version };
      let updated: DispatchPlan;
      if (kind === "approve") {
        updated = await dispatchService.approvePlan(plan.id, input, actionKey);
      } else if (kind === "publish") {
        updated = await dispatchService.publishPlan(plan.id, input, actionKey);
      } else {
        updated = await dispatchService.cancelPlan(plan.id, input, actionKey);
      }
      setPlan(updated);
      setActionKey(newIdempotencyKey());
    } catch (err: any) {
      const message =
        err?.response?.data?.error?.message ??
        err?.message ??
        `Could not ${kind} the plan.`;
      setActionError(message);
    } finally {
      setBusy(null);
    }
  };

  if (loading) {
    return (
      <div className="space-y-5">
        <div className="h-6 w-48 rounded bg-surface-2" />
        <div className="h-40 rounded-lg border border-line bg-surface" />
      </div>
    );
  }

  if (error || !plan) {
    return (
      <div className="space-y-5">
        <div className="flex items-center gap-3">
          <Link
            href="/operations/dispatch/plans"
            className="flex h-9 w-9 items-center justify-center rounded-lg border border-line bg-surface text-ink-600 hover:bg-surface-2"
          >
            <ArrowLeft className="h-4 w-4" />
          </Link>
          <h1>{planId}</h1>
        </div>
        <div className="rounded-lg border border-line bg-surface">
          <EmptyState
            image="/images/reports-empty.png"
            title={error ?? "Plan not found"}
            description="The plan may have been deleted or the URL may be incorrect."
            actionHref="/operations/dispatch/plans"
            actionLabel="Back to plans"
          />
        </div>
      </div>
    );
  }

  const pill = statusStyle(plan.status);
  const s = (plan.status || "").toUpperCase();
  const canApprove = s === "DRAFT" || s === "READY";
  const canPublish = s === "APPROVED";
  const canCancel = s !== "CANCELLED" && s !== "COMPLETED" && s !== "PUBLISHED";

  return (
    <div className="space-y-5">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="flex items-center gap-3">
          <Link
            href="/operations/dispatch/plans"
            className="flex h-9 w-9 items-center justify-center rounded-lg border border-line bg-surface text-ink-600 hover:bg-surface-2"
          >
            <ArrowLeft className="h-4 w-4" />
          </Link>
          <div>
            <h1>{plan.id}</h1>
            <p className="mt-0.5 text-sm text-ink-500">
              {plan.branchId} · {plan.timezone}
            </p>
          </div>
        </div>
      </div>

      {actionError && (
        <div className="flex items-start gap-2 rounded-lg border border-red/20 bg-red-bg px-4 py-3 text-sm text-red">
          <AlertCircle className="mt-0.5 h-4 w-4 flex-shrink-0" />
          <span>{actionError}</span>
        </div>
      )}

      {/* Status + metadata */}
      <div className="rounded-lg border border-line bg-surface p-4">
        <div className="mb-4 flex items-center gap-3">
          <span className={`flex items-center gap-1.5 rounded-full px-3 py-1 text-xs font-semibold ${pill.bg} ${pill.text}`}>
            <span className={`h-1.5 w-1.5 rounded-full ${pill.dot}`} />
            {statusLabel(plan.status)}
          </span>
        </div>
        <div className="grid grid-cols-2 gap-4">
          <div>
            <div className="text-[10px] font-medium uppercase tracking-wide text-ink-500">Plan date</div>
            <div className="mt-0.5 text-sm font-medium text-ink-950">
              {formatTimestamp(plan.planDate)}
            </div>
          </div>
          <div>
            <div className="text-[10px] font-medium uppercase tracking-wide text-ink-500">Timezone</div>
            <div className="mt-0.5 font-mono text-sm font-medium text-ink-950">
              {plan.timezone}
            </div>
          </div>
          <div className="col-span-2">
            <div className="text-[10px] font-medium uppercase tracking-wide text-ink-500">Work window</div>
            <div className="mt-0.5 text-sm font-medium text-ink-950">
              {formatTimestamp(plan.workWindowStart)} → {formatTimestamp(plan.workWindowEnd)}
            </div>
          </div>
          <div>
            <div className="text-[10px] font-medium uppercase tracking-wide text-ink-500">Planner</div>
            <div className="mt-0.5 font-mono text-xs font-medium text-ink-950">
              {plan.plannerUserId || "—"}
            </div>
          </div>
          {plan.approvedBy && (
            <div>
              <div className="text-[10px] font-medium uppercase tracking-wide text-ink-500">Approved by</div>
              <div className="mt-0.5 font-mono text-xs font-medium text-ink-950">
                {plan.approvedBy}
              </div>
            </div>
          )}
          <div>
            <div className="text-[10px] font-medium uppercase tracking-wide text-ink-500">Created</div>
            <div className="mt-0.5 text-xs font-medium text-ink-950">
              {formatTimestamp(plan.createdAt)}
            </div>
          </div>
          <div>
            <div className="text-[10px] font-medium uppercase tracking-wide text-ink-500">Updated</div>
            <div className="mt-0.5 text-xs font-medium text-ink-950">
              {formatTimestamp(plan.updatedAt)}
            </div>
          </div>
          <div>
            <div className="text-[10px] font-medium uppercase tracking-wide text-ink-500">Version</div>
            <div className="mt-0.5 font-mono text-xs font-medium text-ink-950">
              {plan.version}
            </div>
          </div>
        </div>
      </div>

      {/* Actions */}
      <div className="flex flex-wrap gap-2.5">
        {canApprove && (
          <button
            onClick={() => handleAction("approve")}
            disabled={busy !== null}
            className="flex items-center gap-2 rounded-lg bg-primary px-4 py-2.5 text-sm font-semibold text-white hover:bg-primary-hover disabled:opacity-60 disabled:cursor-not-allowed"
          >
            {busy === "approve" ? (
              <>
                <div className="h-4 w-4 animate-spin rounded-full border-2 border-white/30 border-t-white" />
                Approving…
              </>
            ) : (
              <>
                <CheckCircle2 className="h-4 w-4" />
                Approve plan
              </>
            )}
          </button>
        )}
        {canPublish && (
          <button
            onClick={() => handleAction("publish")}
            disabled={busy !== null}
            className="flex items-center gap-2 rounded-lg bg-green px-4 py-2.5 text-sm font-semibold text-white hover:opacity-90 disabled:opacity-60 disabled:cursor-not-allowed"
          >
            {busy === "publish" ? (
              <>
                <div className="h-4 w-4 animate-spin rounded-full border-2 border-white/30 border-t-white" />
                Publishing…
              </>
            ) : (
              <>
                <Send className="h-4 w-4" />
                Publish plan
              </>
            )}
          </button>
        )}
        {canCancel && (
          <button
            onClick={() => handleAction("cancel")}
            disabled={busy !== null}
            className="flex items-center gap-2 rounded-lg border border-red/30 bg-red-bg px-4 py-2.5 text-sm font-semibold text-red hover:bg-red-bg/70 disabled:opacity-60 disabled:cursor-not-allowed"
          >
            {busy === "cancel" ? (
              <>
                <div className="h-4 w-4 animate-spin rounded-full border-2 border-red/30 border-t-red" />
                Cancelling…
              </>
            ) : (
              <>
                <XCircle className="h-4 w-4" />
                Cancel plan
              </>
            )}
          </button>
        )}
        <Link
          href="/operations/dispatch/plans"
          className="flex items-center gap-2 rounded-lg border border-line bg-surface px-4 py-2.5 text-sm font-medium text-ink-700 hover:bg-surface-2"
        >
          Back to plans
        </Link>
      </div>
    </div>
  );
}