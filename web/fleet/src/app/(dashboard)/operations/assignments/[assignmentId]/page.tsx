// src/app/(dashboard)/operations/assignments/[assignmentId]/page.tsx

"use client";

import { useEffect, useMemo, useState } from "react";
import Link from "next/link";
import { useParams, useRouter } from "next/navigation";
import {
  User,
  Truck,
  Calendar,
  Clock,
  CheckCircle2,
  XCircle,
  FileText,
  Shield,
  Package,
  ArrowRight,
  AlertCircle,
} from "lucide-react";
import { marketplaceService } from "@/lib/services/marketplaceService";
import { newIdempotencyKey } from "@/lib/services/marketplaceService";
import type { Assignment } from "@/lib/types/marketplace";

// ---------------------------------------------------------------------------
// Presentation helpers
// ---------------------------------------------------------------------------

function statusLabel(status: string): string {
  switch (status) {
    case "CREATED":
      return "Created";
    case "PENDING_PROVIDER_RESPONSE":
      return "Pending response";
    case "ACCEPTED":
      return "Accepted";
    case "READY_FOR_EXECUTION":
      return "Ready for execution";
    case "EXECUTION_START_PENDING":
      return "Starting execution";
    case "IN_PROGRESS":
      return "In progress";
    case "COMPLETED":
      return "Completed";
    case "DECLINED":
      return "Declined";
    case "EXPIRED":
      return "Expired";
    case "CANCELLED":
      return "Cancelled";
    case "RECONCILIATION_REQUIRED":
      return "Needs reconciliation";
    default:
      return status || "—";
  }
}

function statusStyle(status: string): { bg: string; text: string; dot: string } {
  switch (status) {
    case "ACCEPTED":
    case "READY_FOR_EXECUTION":
    case "COMPLETED":
      return { bg: "bg-green-bg", text: "text-green", dot: "bg-green" };
    case "PENDING_PROVIDER_RESPONSE":
    case "CREATED":
    case "RECONCILIATION_REQUIRED":
      return { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber" };
    case "DECLINED":
    case "CANCELLED":
      return { bg: "bg-red-bg", text: "text-red", dot: "bg-red" };
    case "EXECUTION_START_PENDING":
    case "IN_PROGRESS":
      return { bg: "bg-primary-bg", text: "text-primary", dot: "bg-primary" };
    default:
      return { bg: "bg-surface-2", text: "text-ink-600", dot: "bg-ink-400" };
  }
}

function formatTimestamp(iso: string | undefined): string {
  if (!iso) return "—";
  const t = new Date(iso).getTime();
  if (!Number.isFinite(t)) return "—";
  return new Date(t).toLocaleString(undefined, {
    year: "numeric",
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
}

function deadlineCountdown(deadlineIso: string): {
  text: string;
  minutes: number;
} {
  const t = new Date(deadlineIso).getTime();
  if (!Number.isFinite(t)) {
    return { text: "—", minutes: Number.MAX_SAFE_INTEGER };
  }
  const minutes = Math.max(0, Math.floor((t - Date.now()) / 60_000));
  if (minutes <= 0) return { text: "Closed", minutes: 0 };
  if (minutes < 60) return { text: `${minutes} min left`, minutes };
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return { text: `${hours} h left`, minutes };
  const days = Math.floor(hours / 24);
  return { text: `${days} d left`, minutes };
}

// A cancel is legal while the assignment is not in a terminal state and has
// not yet started canonical execution. The backend refuses otherwise; this
// mirrors the same rule so we don't show a button the API will reject.
function canCancel(a: Assignment): boolean {
  if (a.SourceExecutionID) return false;
  switch (a.Status) {
    case "CREATED":
    case "PENDING_PROVIDER_RESPONSE":
    case "ACCEPTED":
    case "READY_FOR_EXECUTION":
    case "RECONCILIATION_REQUIRED":
      return true;
    default:
      return false;
  }
}

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------

export default function AssignmentOverviewPage() {
  const params = useParams();
  const router = useRouter();
  const assignmentId =
    typeof params.assignmentId === "string" ? params.assignmentId : "";

  const [assignment, setAssignment] = useState<Assignment | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const [cancelOpen, setCancelOpen] = useState(false);
  const [cancelReason, setCancelReason] = useState("");
  const [cancelling, setCancelling] = useState(false);
  const [cancelError, setCancelError] = useState<string | null>(null);

  // One idempotency key per page load. Retries reuse it, so a timeout never
  // produces two cancels.
  const cancelIdempotencyKey = useMemo(() => newIdempotencyKey(), []);

  useEffect(() => {
    if (!assignmentId) return;
    let cancelled = false;
    setLoading(true);
    setError(null);

    marketplaceService
      .getAssignment(assignmentId)
      .then((data) => {
        if (!cancelled) setAssignment(data);
      })
      .catch((err) => {
        if (cancelled) return;
        const status = err?.response?.status;
        if (status === 404) {
          setError("Assignment not found.");
        } else {
          const message =
            err?.response?.data?.error?.message ??
            err?.message ??
            "Failed to load assignment.";
          setError(message);
        }
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });

    return () => {
      cancelled = true;
    };
  }, [assignmentId]);

  const handleCancel = async () => {
    if (!assignment || cancelReason.trim() === "") {
      setCancelError("A reason is required.");
      return;
    }
    setCancelling(true);
    setCancelError(null);

    try {
      await marketplaceService.cancelAssignment(
        assignment.ID,
        { reason: cancelReason.trim(), expectedVersion: assignment.Version },
        cancelIdempotencyKey,
      );
      // Refetch so the page reflects the cancelled state the backend returned.
      const updated = await marketplaceService.getAssignment(assignment.ID);
      setAssignment(updated);
      setCancelOpen(false);
      setCancelReason("");
    } catch (err: any) {
      const message =
        err?.response?.data?.error?.message ??
        err?.message ??
        "Could not cancel the assignment.";
      setCancelError(message);
    } finally {
      setCancelling(false);
    }
  };

  // ---- Loading skeleton ----
  if (loading) {
    return (
      <div className="grid grid-cols-1 gap-5 lg:grid-cols-[1.6fr_1fr]">
        <div className="space-y-5">
          <div className="rounded-2xl border border-line bg-surface p-5 shadow-sm">
            <div className="h-5 w-40 rounded bg-surface-2" />
            <div className="mt-4 grid grid-cols-2 gap-4">
              {Array.from({ length: 5 }).map((_, i) => (
                <div key={i} className={i === 4 ? "col-span-2" : ""}>
                  <div className="h-3 w-20 rounded bg-surface-2" />
                  <div className="mt-1.5 h-4 w-32 rounded bg-surface-2" />
                </div>
              ))}
            </div>
          </div>
        </div>
        <div className="space-y-5">
          <div className="h-48 rounded-2xl border border-line bg-surface" />
          <div className="h-32 rounded-2xl border border-line bg-surface" />
        </div>
      </div>
    );
  }

  if (error || !assignment) {
    return (
      <div className="rounded-2xl border border-line bg-surface p-6 text-center">
        <AlertCircle className="mx-auto h-8 w-8 text-ink-400" />
        <p className="mt-3 text-sm font-semibold text-ink-950">
          {error ?? "Assignment not found"}
        </p>
        <p className="mt-1 text-sm text-ink-500">
          This assignment may have been removed or the URL may be incorrect.
        </p>
        <Link
          href="/operations/assignments"
          className="mt-4 inline-flex items-center justify-center rounded-lg bg-primary-bg px-4 py-2 text-xs font-semibold text-primary transition-colors hover:bg-primary/10"
        >
          Back to assignments
        </Link>
      </div>
    );
  }

  const a = assignment;
  const status = statusStyle(a.Status);
  const isPending = a.Status === "PENDING_PROVIDER_RESPONSE";
  const deadline = isPending ? deadlineCountdown(a.ResponseDeadline) : null;
  const showCancel = canCancel(a);

  return (
    <div className="grid grid-cols-1 gap-5 lg:grid-cols-[1.6fr_1fr]">
      {/* ===== LEFT COLUMN ===== */}
      <div className="space-y-5">
        {/* Assignment Details */}
        <div className="rounded-2xl border border-line bg-surface p-5 shadow-sm">
          <div className="mb-4 flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-xl bg-primary-bg text-primary">
              <FileText className="h-4 w-4" />
            </div>
            <div>
              <div className="text-[15px] font-semibold text-ink-950">
                Assignment Details
              </div>
              {a.OfferID ? (
                <div className="text-[12px] text-ink-500">
                  Offer: <span className="font-mono">{a.OfferID}</span>
                </div>
              ) : (
                <div className="text-[12px] text-ink-500">
                  <span className="font-mono">{a.ID}</span>
                </div>
              )}
            </div>
            <span
              className={`ml-auto flex items-center gap-1.5 rounded-full px-3 py-1 text-[11px] font-semibold ${status.bg} ${status.text}`}
            >
              <span className={`h-1.5 w-1.5 rounded-full ${status.dot}`} />
              {statusLabel(a.Status)}
            </span>
          </div>

          <div className="grid grid-cols-2 gap-4">
            <div>
              <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">
                Provider
              </div>
              <div className="mt-1 flex items-center gap-2">
                <User className="h-4 w-4 text-ink-400" />
                <span className="font-mono text-[14px] font-medium text-ink-950">
                  {a.MembershipID}
                </span>
              </div>
            </div>

            <div>
              <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">
                Assigned Asset
              </div>
              <div className="mt-1 flex items-center gap-2">
                <Truck className="h-4 w-4 text-ink-400" />
                <span className="font-mono text-[14px] font-medium text-ink-950">
                  {a.AssetReference || "—"}
                </span>
              </div>
            </div>

            <div>
              <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">
                Source service
              </div>
              <div className="mt-1 flex items-center gap-2">
                <Package className="h-4 w-4 text-ink-400" />
                <span className="text-[14px] font-medium text-ink-950">
                  {a.SourceService || "—"}
                </span>
              </div>
            </div>

            <div>
              <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">
                Created
              </div>
              <div className="mt-1 flex items-center gap-2">
                <Calendar className="h-4 w-4 text-ink-400" />
                <span className="text-[14px] font-medium text-ink-950">
                  {formatTimestamp(a.CreatedAt)}
                </span>
              </div>
            </div>

            <div className="col-span-2">
              <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">
                Response Deadline
              </div>
              <div className="mt-1 flex items-center gap-2">
                <Clock
                  className={`h-4 w-4 ${isPending ? "text-amber" : "text-ink-400"}`}
                />
                {isPending && deadline ? (
                  <>
                    <span className="rounded-full bg-amber-bg px-3 py-1 text-[13px] font-bold text-amber">
                      {deadline.text}
                    </span>
                    <span className="text-[12px] text-ink-500">
                      Provider must respond by {formatTimestamp(a.ResponseDeadline)}
                    </span>
                  </>
                ) : (
                  <span className="text-[14px] font-medium text-ink-950">
                    {formatTimestamp(a.ResponseDeadline)}
                  </span>
                )}
              </div>
            </div>
          </div>
        </div>
      </div>

      {/* ===== RIGHT COLUMN ===== */}
      <div className="space-y-5">
        {/* Provider Response */}
        <div className="rounded-2xl border border-line bg-surface p-5 shadow-sm">
          <div className="mb-3 flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-xl bg-primary-bg text-primary">
              <Shield className="h-4 w-4" />
            </div>
            <span className="text-[15px] font-semibold text-ink-950">
              Provider Response
            </span>
          </div>

          {isPending ? (
            <div className="flex items-start gap-3 rounded-xl bg-amber-bg/30 p-3.5">
              <Clock className="mt-0.5 h-5 w-5 flex-shrink-0 text-amber" />
              <div>
                <div className="text-[13px] font-medium text-ink-950">
                  Awaiting response
                </div>
                <p className="mt-0.5 text-[13px] text-ink-600">
                  Waiting for the provider to accept or decline. Execution
                  cannot begin until they respond.
                </p>
              </div>
            </div>
          ) : a.Status === "ACCEPTED" ||
            a.Status === "READY_FOR_EXECUTION" ||
            a.Status === "IN_PROGRESS" ||
            a.Status === "COMPLETED" ? (
            <div className="flex items-start gap-3 rounded-xl bg-green-bg/40 p-3.5">
              <CheckCircle2 className="mt-0.5 h-5 w-5 flex-shrink-0 text-green" />
              <div>
                <div className="text-[13px] font-medium text-ink-950">
                  Provider accepted
                </div>
                <p className="mt-0.5 text-[13px] text-ink-600">
                  {a.AcceptedAt
                    ? `Accepted ${formatTimestamp(a.AcceptedAt)}.`
                    : "The provider accepted this assignment."}
                </p>
              </div>
            </div>
          ) : a.Status === "DECLINED" ? (
            <div className="flex items-start gap-3 rounded-xl bg-red-bg/30 p-3.5">
              <XCircle className="mt-0.5 h-5 w-5 flex-shrink-0 text-red" />
              <div>
                <div className="text-[13px] font-medium text-ink-950">
                  Provider declined
                </div>
                <p className="mt-0.5 text-[13px] text-ink-600">
                  {a.DeclinedAt
                    ? `Declined ${formatTimestamp(a.DeclinedAt)}.`
                    : "The provider declined this assignment."}{" "}
                  {a.ReassignmentReason
                    ? `Reason: ${a.ReassignmentReason}.`
                    : ""}
                </p>
              </div>
            </div>
          ) : a.Status === "EXPIRED" ? (
            <div className="flex items-start gap-3 rounded-xl bg-surface-2 p-3.5">
              <Clock className="mt-0.5 h-5 w-5 flex-shrink-0 text-ink-400" />
              <div>
                <div className="text-[13px] font-medium text-ink-950">
                  Response window closed
                </div>
                <p className="mt-0.5 text-[13px] text-ink-600">
                  The provider did not respond in time.
                  {a.ExpiredAt ? ` Expired ${formatTimestamp(a.ExpiredAt)}.` : ""}
                </p>
              </div>
            </div>
          ) : a.Status === "CANCELLED" ? (
            <div className="flex items-start gap-3 rounded-xl bg-red-bg/30 p-3.5">
              <XCircle className="mt-0.5 h-5 w-5 flex-shrink-0 text-red" />
              <div>
                <div className="text-[13px] font-medium text-ink-950">
                  Assignment cancelled
                </div>
                <p className="mt-0.5 text-[13px] text-ink-600">
                  {a.CancelledAt
                    ? `Cancelled ${formatTimestamp(a.CancelledAt)}.`
                    : "This assignment was cancelled."}
                </p>
              </div>
            </div>
          ) : (
            <p className="text-[13px] text-ink-500">
              No response recorded yet.
            </p>
          )}
        </div>

        {/* Cancel assignment — inline confirm, only when the backend allows */}
        {showCancel && (
          <div className="rounded-2xl border border-line bg-surface p-5 shadow-sm">
            <div className="text-[13px] font-semibold text-ink-950 mb-3">
              Danger zone
            </div>

            {!cancelOpen ? (
              <button
                onClick={() => {
                  setCancelOpen(true);
                  setCancelError(null);
                }}
                className="flex w-full items-center justify-center gap-2 rounded-xl border border-red/20 bg-red-bg/50 px-4 py-3 text-[13px] font-semibold text-red shadow-sm transition hover:bg-red-bg hover:border-red/40"
              >
                <XCircle className="h-4 w-4" />
                Cancel assignment
              </button>
            ) : (
              <div className="space-y-3">
                <label
                  htmlFor="cancel-reason"
                  className="block text-[12px] font-medium text-ink-700"
                >
                  Reason for cancellation{" "}
                  <span className="text-red">*</span>
                </label>
                <textarea
                  id="cancel-reason"
                  value={cancelReason}
                  onChange={(e) => setCancelReason(e.target.value)}
                  placeholder="Why is this assignment being cancelled?"
                  rows={3}
                  className="w-full resize-none rounded-lg border border-line bg-surface px-3 py-2 text-sm text-ink-950 outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20 placeholder:text-ink-400"
                />
                {cancelError && (
                  <div className="flex items-start gap-2 rounded-lg border border-red/20 bg-red-bg px-3 py-2 text-xs text-red">
                    <AlertCircle className="mt-0.5 h-3.5 w-3.5 flex-shrink-0" />
                    <span>{cancelError}</span>
                  </div>
                )}
                <div className="flex flex-col-reverse gap-2 sm:flex-row sm:justify-end">
                  <button
                    type="button"
                    onClick={() => {
                      setCancelOpen(false);
                      setCancelReason("");
                      setCancelError(null);
                    }}
                    disabled={cancelling}
                    className="flex items-center justify-center rounded-lg border border-line bg-surface px-4 py-2 text-sm font-medium text-ink-700 transition-colors hover:bg-surface-2 disabled:opacity-60"
                  >
                    Keep assignment
                  </button>
                  <button
                    type="button"
                    onClick={handleCancel}
                    disabled={cancelling || cancelReason.trim() === ""}
                    className="flex items-center justify-center gap-2 rounded-lg bg-red px-4 py-2 text-sm font-semibold text-white transition-colors hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-60"
                  >
                    {cancelling ? (
                      <>
                        <div className="h-3.5 w-3.5 animate-spin rounded-full border-2 border-white/30 border-t-white" />
                        Cancelling…
                      </>
                    ) : (
                      <>
                        <XCircle className="h-3.5 w-3.5" />
                        Confirm cancel
                      </>
                    )}
                  </button>
                </div>
              </div>
            )}
          </div>
        )}

        {/* Quick Actions */}
        <div className="rounded-2xl border border-line bg-surface p-5 shadow-sm">
          <div className="text-[13px] font-semibold text-ink-950 mb-3">
            Quick Actions
          </div>
          <div className="flex flex-col gap-2">
            {a.OfferID && (
              <Link
                href={`/operations/marketplace/${a.OfferID}`}
                className="flex items-center justify-between rounded-xl border border-line bg-surface px-4 py-2.5 text-[13px] font-medium text-ink-700 transition hover:bg-surface-2"
              >
                <span className="flex items-center gap-2">
                  <Package className="h-4 w-4" />
                  View original offer
                </span>
                <ArrowRight className="h-4 w-4 text-ink-400" />
              </Link>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}