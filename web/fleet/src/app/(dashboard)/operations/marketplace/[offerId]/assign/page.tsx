// src/app/(dashboard)/operations/marketplace/[offerId]/assign/page.tsx

"use client";

import { useEffect, useMemo, useState } from "react";
import Link from "next/link";
import { useParams, useRouter, useSearchParams } from "next/navigation";
import {
  ArrowLeft,
  UserCheck,
  User,
  Clock,
  MapPin,
  Package,
  X,
  AlertCircle,
  CheckCircle2,
} from "lucide-react";
import EmptyState from "@/components/EmptyState";
import {
  marketplaceService,
  newIdempotencyKey,
} from "@/lib/services/marketplaceService";
import type {
  CorporateCandidate,
  CorporateOfferDetail,
} from "@/lib/types/marketplace";

// ---------------------------------------------------------------------------
// Presentation helpers
// ---------------------------------------------------------------------------

const DEADLINE_OPTIONS = [
  { label: "30 seconds", value: 30 },
  { label: "1 minute", value: 60 },
  { label: "90 seconds", value: 90 },
  { label: "5 minutes", value: 300 },
  { label: "15 minutes", value: 900 },
];

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

function summaryLabel(summary: Record<string, unknown> | undefined): string {
  if (!summary) return "—";
  const candidateKeys = ["label", "name", "reference", "address"];
  for (const key of candidateKeys) {
    const value = summary[key];
    if (typeof value === "string" && value.trim() !== "") {
      return value.trim();
    }
  }
  return "—";
}

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------

export default function AssignPage() {
  const params = useParams();
  const router = useRouter();
  const searchParams = useSearchParams();

  const offerId = typeof params.offerId === "string" ? params.offerId : "";
  const preselectedMembershipId = searchParams.get("membershipId") ?? "";

  const [detail, setDetail] = useState<CorporateOfferDetail | null>(null);
  const [candidates, setCandidates] = useState<CorporateCandidate[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | null>(null);

  const [selectedMembershipId, setSelectedMembershipId] =
    useState<string>(preselectedMembershipId);
  const [responseTimeoutSeconds, setResponseTimeoutSeconds] = useState<number>(90);
  const [instructions, setInstructions] = useState("");

  const [submitting, setSubmitting] = useState(false);
  const [submitError, setSubmitError] = useState<string | null>(null);

  // One idempotency key per page load. If the dispatcher retries, the same
  // key is reused so the backend replays the original outcome rather than
  // creating a second assignment.
  const idempotencyKey = useMemo(() => newIdempotencyKey(), []);

  useEffect(() => {
    if (!offerId) return;
    let cancelled = false;
    setLoading(true);
    setLoadError(null);

    marketplaceService
      .getOffer(offerId)
      .then((offerDetail) => {
        if (cancelled) return null;
        setDetail(offerDetail);
        return marketplaceService.listCandidates(
          offerDetail.offer.requestProjectionId,
        );
      })
      .then((list) => {
        if (cancelled || !list) return;
        setCandidates(list.filter((c) => c.eligible));
      })
      .catch((err) => {
        if (cancelled) return;
        const status = err?.response?.status;
        if (status === 404) {
          setLoadError("Offer not found.");
        } else {
          const message =
            err?.response?.data?.error?.message ??
            err?.message ??
            "Failed to load offer.";
          setLoadError(message);
        }
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });

    return () => {
      cancelled = true;
    };
  }, [offerId]);

  const selectedCandidate = useMemo(
    () => candidates.find((c) => c.membershipId === selectedMembershipId) ?? null,
    [candidates, selectedMembershipId],
  );

  const formValid = !!selectedMembershipId && responseTimeoutSeconds > 0;

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!formValid || !detail) return;
    setSubmitting(true);
    setSubmitError(null);

    try {
      const assignment = await marketplaceService.assignRequest(
        detail.offer.requestProjectionId,
        {
          membershipId: selectedMembershipId,
          responseTimeoutSeconds,
          expectedRequestVersion: detail.offer.version,
        },
        idempotencyKey,
      );
      router.push(`/operations/marketplace/${offerId}`);
      // The assignment object is available if we later want to navigate to
      // an assignment-detail page instead. For now we return to the offer.
      void assignment;
    } catch (err: any) {
      const message =
        err?.response?.data?.error?.message ??
        err?.message ??
        "Could not create the assignment.";
      setSubmitError(message);
      setSubmitting(false);
    }
  };

  // ---- Loading skeleton ----
  if (loading) {
    return (
      <div className="mx-auto max-w-4xl space-y-5">
        <div className="flex items-center gap-3">
          <div className="h-9 w-9 rounded-lg border border-line bg-surface" />
          <div>
            <div className="h-5 w-44 rounded bg-surface-2" />
            <div className="mt-1.5 h-3 w-64 rounded bg-surface-2" />
          </div>
        </div>
        <div className="rounded-lg border border-line bg-surface p-4">
          <div className="h-4 w-48 rounded bg-surface-2" />
        </div>
        <div className="grid grid-cols-1 gap-3.5 lg:grid-cols-2">
          <div className="h-64 rounded-lg border border-line bg-surface" />
          <div className="h-64 rounded-lg border border-line bg-surface" />
        </div>
      </div>
    );
  }

  // ---- Error / not-found ----
  if (loadError || !detail) {
    return (
      <div className="mx-auto max-w-4xl space-y-5">
        <div className="flex items-center gap-3">
          <Link
            href="/operations/marketplace"
            className="flex h-9 w-9 items-center justify-center rounded-lg border border-line bg-surface text-ink-600 transition-colors hover:bg-surface-2 hover:text-ink-900"
          >
            <ArrowLeft className="h-4 w-4" />
          </Link>
          <div>
            <h1>Create assignment</h1>
            <p className="font-mono text-sm text-ink-500">{offerId}</p>
          </div>
        </div>
        <div className="rounded-lg border border-line bg-surface">
          <EmptyState
            image="/images/trips-empty.png"
            title={loadError ?? "Offer not found"}
            description="This offer may have expired or been accepted by another organization."
            actionHref="/operations/marketplace"
            actionLabel="Back to marketplace"
          />
        </div>
      </div>
    );
  }

  const offer = detail.offer;
  const pickup = summaryLabel(offer.pickupSummary);
  const dropoff = summaryLabel(offer.dropoffSummary);

  return (
    <div className="mx-auto max-w-4xl space-y-5">
      {/* ── Header ── */}
      <div className="flex items-center gap-3">
        <Link
          href={`/operations/marketplace/${offerId}/candidates`}
          className="flex h-9 w-9 items-center justify-center rounded-lg border border-line bg-surface text-ink-600 transition-colors hover:bg-surface-2 hover:text-ink-900"
        >
          <ArrowLeft className="h-4 w-4" />
        </Link>
        <div>
          <h1>Create assignment</h1>
          <p className="mt-1.5 text-sm text-ink-500">
            Assign a provider to offer{" "}
            <span className="font-mono text-ink-800">{offerId}</span>
          </p>
        </div>
      </div>

      {/* ── Offer summary ── */}
      <div className="rounded-lg border border-line bg-surface p-4">
        <div className="flex flex-wrap items-center gap-4 text-sm">
          <div className="flex items-center gap-2">
            <Package className="h-4 w-4 text-ink-400" />
            <span className="font-semibold text-ink-950">{offer.offerId}</span>
          </div>
          <div className="flex items-center gap-2 text-ink-700">
            <MapPin className="h-4 w-4 text-ink-400" />
            <span>
              {pickup} → {dropoff}
            </span>
          </div>
          <div className="flex items-center gap-2 text-ink-700">
            <Clock className="h-4 w-4 text-ink-400" />
            <span>{serviceLabel(offer.sourceService)}</span>
          </div>
        </div>
      </div>

      {submitError && (
        <div className="flex items-start gap-2 rounded-lg border border-red-300/40 bg-red-bg px-4 py-3 text-sm text-red">
          <AlertCircle className="mt-0.5 h-4 w-4 flex-shrink-0" />
          <span>{submitError}</span>
        </div>
      )}

      {/* ── Form ── */}
      <form onSubmit={handleSubmit} className="space-y-3.5">
        <div className="grid grid-cols-1 gap-3.5 lg:grid-cols-2">
          {/* Provider selector */}
          <div className="rounded-lg border border-line bg-surface p-4">
            <label className="block text-xs font-semibold text-ink-700 mb-3">
              Select provider <span className="text-red">*</span>
            </label>

            {candidates.length === 0 ? (
              <div className="rounded-lg border border-dashed border-line bg-surface-2 py-6 text-center">
                <User className="mx-auto h-6 w-6 text-ink-400" />
                <p className="mt-2 text-xs text-ink-500">
                  No eligible providers for this offer.
                </p>
              </div>
            ) : (
              <div className="max-h-[280px] overflow-y-auto space-y-1.5 pr-1">
                {candidates.map((c) => {
                  const isSelected = selectedMembershipId === c.membershipId;
                  return (
                    <button
                      key={c.membershipId}
                      type="button"
                      onClick={() => setSelectedMembershipId(c.membershipId)}
                      className={`w-full rounded-lg border p-2.5 text-left transition ${
                        isSelected
                          ? "border-primary bg-primary-bg ring-2 ring-primary/30"
                          : "border-line bg-surface hover:bg-surface-2"
                      }`}
                    >
                      <div className="flex items-center justify-between gap-2">
                        <div className="flex items-center gap-2.5 min-w-0">
                          <div className="flex h-8 w-8 flex-shrink-0 items-center justify-center rounded-md bg-green-bg text-green">
                            <User className="h-4 w-4" />
                          </div>
                          <div className="min-w-0">
                            <div className="font-mono text-sm font-semibold text-ink-950 truncate">
                              {c.membershipId}
                            </div>
                            <div className="text-[11px] text-ink-500 truncate">
                              {c.providerId || "No provider id"} ·{" "}
                              {serviceLabel(c.serviceCategory)}
                            </div>
                          </div>
                        </div>
                        {isSelected && (
                          <CheckCircle2 className="h-4 w-4 flex-shrink-0 text-primary" />
                        )}
                      </div>
                    </button>
                  );
                })}
              </div>
            )}
          </div>

          {/* Response deadline */}
          <div className="rounded-lg border border-line bg-surface p-4">
            <label
              htmlFor="response-timeout"
              className="block text-xs font-semibold text-ink-700 mb-3"
            >
              Response deadline <span className="text-red">*</span>
            </label>
            <div className="relative">
              <select
                id="response-timeout"
                value={responseTimeoutSeconds}
                onChange={(e) =>
                  setResponseTimeoutSeconds(Number(e.target.value))
                }
                className="w-full appearance-none rounded-lg border border-line bg-surface px-3 py-2 text-sm text-ink-950 outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20"
              >
                {DEADLINE_OPTIONS.map((opt) => (
                  <option key={opt.value} value={opt.value}>
                    {opt.label}
                  </option>
                ))}
              </select>
              <Clock className="pointer-events-none absolute right-3 top-1/2 h-4 w-4 -translate-y-1/2 text-ink-400" />
            </div>
            <p className="mt-2 text-xs text-ink-500">
              The provider has this long to accept or decline the assignment.
            </p>
          </div>
        </div>

        {/* Instructions */}
        <div className="rounded-lg border border-line bg-surface p-4">
          <label
            htmlFor="instructions"
            className="block text-xs font-semibold text-ink-700 mb-3"
          >
            Instructions (optional)
          </label>
          <textarea
            id="instructions"
            value={instructions}
            onChange={(e) => setInstructions(e.target.value)}
            placeholder="Add any special instructions for this assignment..."
            rows={3}
            className="w-full resize-none rounded-lg border border-line bg-surface px-3 py-2 text-sm text-ink-950 outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20 placeholder:text-ink-400"
          />
        </div>

        {/* Selection summary */}
        {selectedCandidate && (
          <div className="rounded-lg border border-line bg-primary-bg/40 p-4">
            <div className="flex flex-wrap items-center gap-3">
              <div className="flex items-center gap-2 text-sm">
                <UserCheck className="h-4 w-4 text-primary" />
                <span className="font-semibold text-ink-950">
                  Assignment ready
                </span>
              </div>
              <div className="flex items-center gap-1.5 rounded-full border border-line bg-surface px-2.5 py-1 text-xs font-medium text-ink-700">
                <span className="h-1.5 w-1.5 rounded-full bg-green" />
                <span className="font-mono">
                  {selectedCandidate.membershipId}
                </span>
              </div>
              <div className="flex items-center gap-1.5 rounded-full border border-line bg-surface px-2.5 py-1 text-xs font-medium text-ink-700">
                <Clock className="h-3 w-3 text-ink-400" />
                {DEADLINE_OPTIONS.find(
                  (o) => o.value === responseTimeoutSeconds,
                )?.label ?? `${responseTimeoutSeconds}s`}
              </div>
            </div>
          </div>
        )}

        {/* Actions */}
        <div className="flex flex-col-reverse gap-2.5 sm:flex-row sm:justify-end">
          <Link
            href={`/operations/marketplace/${offerId}/candidates`}
            className="flex items-center justify-center gap-2 rounded-lg border border-line bg-surface px-5 py-2.5 text-sm font-medium text-ink-700 transition-colors hover:bg-surface-2"
          >
            <X className="h-4 w-4" />
            Cancel
          </Link>
          <button
            type="submit"
            disabled={!formValid || submitting}
            className="flex items-center justify-center gap-2 rounded-lg bg-primary px-5 py-2.5 text-sm font-semibold text-white transition-colors hover:bg-primary-hover disabled:cursor-not-allowed disabled:opacity-60"
          >
            {submitting ? (
              <>
                <div className="h-4 w-4 animate-spin rounded-full border-2 border-white/30 border-t-white" />
                Creating…
              </>
            ) : (
              <>
                <UserCheck className="h-4 w-4" />
                Create assignment
              </>
            )}
          </button>
        </div>
      </form>
    </div>
  );
}