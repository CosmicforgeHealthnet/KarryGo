// src/app/(dashboard)/operations/assignments/[assignmentId]/reassign/page.tsx

"use client";

import { useEffect, useMemo, useState } from "react";
import Link from "next/link";
import { useParams, useRouter } from "next/navigation";
import {
  RefreshCw,
  CheckCircle2,
  Clock,
  Shield,
  Users,
  ArrowRight,
  X,
  AlertCircle,
} from "lucide-react";
import EmptyState from "@/components/EmptyState";
import {
  marketplaceService,
  newIdempotencyKey,
} from "@/lib/services/marketplaceService";
import type {
  Assignment,
  CorporateCandidate,
} from "@/lib/types/marketplace";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function serviceLabel(sourceService: string): string {
  switch (sourceService) {
    case "DISPATCH":
      return "Dispatch";
    case "TAXI":
      return "Taxi";
    case "HAULING":
      return "Hauling";
    default:
      return sourceService || "—";
  }
}

function reasonLabel(reason: string | undefined): string {
  if (!reason) return "";
  switch (reason) {
    case "eligibility_unavailable":
      return "Eligibility check unavailable";
    case "PROVIDER_NOT_ELIGIBLE":
      return "Provider not eligible";
    case "PROVIDER_MEMBERSHIP_NOT_ACTIVE":
      return "Membership not active";
    case "PROVIDER_ASSOCIATION_PENDING":
      return "Association pending";
    case "PROVIDER_PROVISIONING_INCOMPLETE":
      return "Provisioning incomplete";
    case "PROVIDER_SHIFT_INVALID":
      return "No valid shift";
    case "SOURCE_SERVICE_UNAVAILABLE":
      return "Source service unavailable";
    case "VEHICLE_NOT_ELIGIBLE":
      return "Vehicle not eligible";
    case "PROVIDER_HAS_CONFLICTING_WORK":
      return "Already on other work";
    default:
      return reason;
  }
}

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------

export default function ReassignPage() {
  const params = useParams();
  const router = useRouter();
  const assignmentId =
    typeof params.assignmentId === "string" ? params.assignmentId : "";

  const [assignment, setAssignment] = useState<Assignment | null>(null);
  const [candidates, setCandidates] = useState<CorporateCandidate[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | null>(null);

  const [selected, setSelected] = useState<string | null>(null);
  const [reason, setReason] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [submitError, setSubmitError] = useState<string | null>(null);

  // One key per page load. Retries reuse it.
  const idempotencyKey = useMemo(() => newIdempotencyKey(), []);

  useEffect(() => {
    if (!assignmentId) return;
    let cancelled = false;
    setLoading(true);
    setLoadError(null);

    marketplaceService
      .getAssignment(assignmentId)
      .then((a) => {
        if (cancelled) return null;
        setAssignment(a);
        return marketplaceService.listCandidates(a.RequestProjectionID);
      })
      .then((list) => {
        if (cancelled || !list) return;
        // The provider already on this assignment is never a valid
        // replacement. The backend enforces this at reassign time via the
        // reassignment case's excluded set; we mirror the exclusion in the
        // UI so the list is honest about who can actually be selected.
        const currentMembershipId =
          assignment?.MembershipID ?? "";
        const eligibleReplacements = list.filter(
          (c) => c.eligible && c.membershipId !== currentMembershipId,
        );
        setCandidates(eligibleReplacements);
      })
      .catch((err) => {
        if (cancelled) return;
        const status = err?.response?.status;
        if (status === 404) {
          setLoadError("Assignment not found.");
        } else {
          const message =
            err?.response?.data?.error?.message ??
            err?.message ??
            "Failed to load available replacements.";
          setLoadError(message);
        }
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });

    return () => {
      cancelled = true;
    };
  }, [assignmentId, assignment?.MembershipID]);

  const handleReassign = async () => {
    if (!assignment || !selected || reason.trim() === "") {
      setSubmitError("A reason is required.");
      return;
    }
    setSubmitting(true);
    setSubmitError(null);

    try {
      await marketplaceService.reassignAssignment(
        assignment.ID,
        {
          membershipId: selected,
          reason: reason.trim(),
        },
        idempotencyKey,
      );
      router.push(`/operations/assignments/${assignmentId}`);
    } catch (err: any) {
      const message =
        err?.response?.data?.error?.message ??
        err?.message ??
        "Could not create the reassignment.";
      setSubmitError(message);
      setSubmitting(false);
    }
  };

  // ---- Loading ----
  if (loading) {
    return (
      <div className="space-y-5">
        <div className="h-28 rounded-lg border border-line bg-surface" />
        <div className="space-y-2">
          {Array.from({ length: 3 }).map((_, i) => (
            <div
              key={i}
              className="h-16 rounded-lg border border-line bg-surface"
            />
          ))}
        </div>
      </div>
    );
  }

  // ---- Error ----
  if (loadError || !assignment) {
    return (
      <div className="rounded-lg border border-line bg-surface">
        <EmptyState
          image="/images/alerts-empty.png"
          title={loadError ?? "Assignment not found"}
          description="This assignment may have been removed or the URL may be incorrect."
          actionHref="/operations/assignments"
          actionLabel="Back to assignments"
        />
      </div>
    );
  }

  // ---- Not reassignable ----
  const reassignable =
    assignment.Status === "DECLINED" ||
    assignment.Status === "EXPIRED" ||
    assignment.Status === "CANCELLED" ||
    assignment.Status === "PENDING_PROVIDER_RESPONSE";

  if (!reassignable) {
    return (
      <div className="rounded-lg border border-line bg-surface">
        <EmptyState
          image="/images/alerts-empty.png"
          title="This assignment cannot be reassigned"
          description="Only assignments that were declined, expired, cancelled, or are still pending a response can be reassigned."
          actionHref={`/operations/assignments/${assignmentId}`}
          actionLabel="Back to assignment"
        />
      </div>
    );
  }

  return (
    <div className="space-y-5">
      {/* ── Info note ── */}
      <div className="rounded-lg border border-primary/20 bg-primary-bg/30 p-4">
        <div className="flex items-start gap-3">
          <RefreshCw className="mt-0.5 h-5 w-5 flex-shrink-0 text-primary" />
          <div>
            <p className="text-sm text-ink-700">
              Reassigning creates a new pending assignment for the replacement
              provider. The original assignment's history is preserved.
            </p>
            <div className="mt-2 flex flex-wrap gap-2">
              <span className="inline-flex items-center gap-1.5 rounded-full border border-line bg-surface px-2.5 py-1 text-xs font-medium text-ink-600">
                <CheckCircle2 className="h-3 w-3 text-green" />
                Original preserved
              </span>
              <span className="inline-flex items-center gap-1.5 rounded-full border border-line bg-surface px-2.5 py-1 text-xs font-medium text-ink-600">
                <Clock className="h-3 w-3 text-amber" />
                New pending created
              </span>
              <span className="inline-flex items-center gap-1.5 rounded-full border border-line bg-surface px-2.5 py-1 text-xs font-medium text-ink-600">
                <Shield className="h-3 w-3 text-primary" />
                Provider chooses
              </span>
            </div>
          </div>
        </div>
      </div>

      {/* ── Candidates ── */}
      <div>
        <div className="mb-3 flex items-center gap-2">
          <Users className="h-4 w-4 text-ink-400" />
          <span className="text-sm font-semibold text-ink-950">
            Available replacements
          </span>
          <span className="rounded-full bg-primary-bg px-2 py-0.5 text-xs font-semibold text-primary">
            {candidates.length}
          </span>
        </div>

        {candidates.length === 0 ? (
          <div className="rounded-lg border border-line bg-surface">
            <EmptyState
              image="/images/vehicles-empty.png"
              title="No eligible replacements"
              description="No other provider memberships are available for this request right now."
            />
          </div>
        ) : (
          <div className="space-y-2">
            {candidates.map((c) => {
              const isSelected = selected === c.membershipId;
              return (
                <button
                  key={c.membershipId}
                  type="button"
                  onClick={() => setSelected(c.membershipId)}
                  className={`flex w-full items-center justify-between gap-4 rounded-lg border p-4 text-left transition-colors ${
                    isSelected
                      ? "border-primary bg-primary-bg"
                      : "border-line bg-surface hover:bg-surface-2"
                  }`}
                >
                  <div className="flex flex-1 items-center gap-3 min-w-0">
                    <div className="flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-md bg-green-bg text-green">
                      <Users className="h-[18px] w-[18px]" />
                    </div>
                    <div className="min-w-0">
                      <div className="font-mono text-sm font-semibold text-ink-950">
                        {c.membershipId}
                      </div>
                      <div className="text-xs text-ink-500">
                        {c.providerId ? (
                          <span className="font-mono">{c.providerId}</span>
                        ) : (
                          "No provider id"
                        )}
                        {" · "}
                        {serviceLabel(c.serviceCategory)}
                      </div>
                    </div>
                  </div>
                  <div
                    className={`flex h-6 w-6 flex-shrink-0 items-center justify-center rounded-full border-2 transition-colors ${
                      isSelected
                        ? "border-primary bg-primary text-white"
                        : "border-ink-400"
                    }`}
                  >
                    {isSelected && <CheckCircle2 className="h-4 w-4" />}
                  </div>
                </button>
              );
            })}
          </div>
        )}
      </div>

      {/* ── Reason ── */}
      <div className="rounded-lg border border-line bg-surface p-4">
        <label
          htmlFor="reassign-reason"
          className="mb-2 block text-xs font-semibold text-ink-700"
        >
          Reason for reassignment <span className="text-red">*</span>
        </label>
        <textarea
          id="reassign-reason"
          value={reason}
          onChange={(e) => setReason(e.target.value)}
          placeholder="Why is this assignment being reassigned?"
          rows={3}
          className="w-full resize-none rounded-lg border border-line bg-surface px-3 py-2 text-sm text-ink-950 outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20 placeholder:text-ink-400"
        />
      </div>

      {submitError && (
        <div className="flex items-start gap-2 rounded-lg border border-red/20 bg-red-bg px-4 py-3 text-sm text-red">
          <AlertCircle className="mt-0.5 h-4 w-4 flex-shrink-0" />
          <span>{submitError}</span>
        </div>
      )}

      {/* ── Actions ── */}
      <div className="flex flex-col-reverse gap-2.5 sm:flex-row sm:justify-end">
        <Link
          href={`/operations/assignments/${assignmentId}`}
          className="flex items-center justify-center gap-2 rounded-lg border border-line bg-surface px-5 py-2.5 text-sm font-medium text-ink-700 transition-colors hover:bg-surface-2"
        >
          <X className="h-4 w-4" />
          Cancel
        </Link>
        <button
          type="button"
          onClick={handleReassign}
          disabled={!selected || reason.trim() === "" || submitting}
          className="flex items-center justify-center gap-2 rounded-lg bg-primary px-5 py-2.5 text-sm font-semibold text-white transition-colors hover:bg-primary-hover disabled:cursor-not-allowed disabled:opacity-60"
        >
          {submitting ? (
            <>
              <div className="h-4 w-4 animate-spin rounded-full border-2 border-white/30 border-t-white" />
              Reassigning…
            </>
          ) : (
            <>
              Confirm reassignment
              <ArrowRight className="h-4 w-4" />
            </>
          )}
        </button>
      </div>
    </div>
  );
}